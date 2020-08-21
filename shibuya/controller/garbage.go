package controller

import (
	"shibuya/config"
	"shibuya/model"
	"strconv"

	"time"

	log "github.com/sirupsen/logrus"
)

func (c *Controller) checkRunningThenTerminate() {
	jobs := make(chan *RunningPlan)
	for w := 1; w <= 3; w++ {
		go func(jobs <-chan *RunningPlan) {
		jobLoop:
			for j := range jobs {
				pc := NewPlanController(j.ep, j.collection, c.Kcm)
				if running := pc.progress(); !running {
					collection := j.collection
					currRunID, err := collection.GetCurrentRun()
					if currRunID != int64(0) {
						pc.term(false, &c.connectedEngines)
						log.Printf("Plan %d is terminated.", j.ep.PlanID)
					}
					if err != nil {
						continue jobLoop
					}
					if t, err := collection.HasRunningPlan(); t || err != nil {
						continue jobLoop
					}
					collection.StopRun()
					collection.RunFinish(currRunID)
				}
			}
		}(jobs)
	}
	log.Printf("Getting all the running plans for %s", config.SC.Context)
	for {
		runningPlans, err := model.GetRunningPlans()
		if err != nil {
			continue
		}
		localCache := make(map[int64]*model.Collection)
		for _, rp := range runningPlans {
			var collection *model.Collection
			var ok bool
			collection, ok = localCache[rp.CollectionID]
			if !ok {
				collection, err = model.GetCollection(rp.CollectionID)
				if err != nil {
					continue
				}
				localCache[rp.CollectionID] = collection
			}
			ep, err := model.GetExecutionPlan(collection.ID, rp.PlanID)
			if err != nil {
				continue
			}
			item := &RunningPlan{
				ep:         ep,
				collection: collection,
			}
			jobs <- item
		}
		time.Sleep(2 * time.Second)
	}
}

func (c *Controller) cleanLocalStore() {
	for {
		// we can iterate any one of Labelstore or StatusStore because writes/deletes always happen at the same time on both
		c.LabelStore.Range(func(runID interface{}, _ interface{}) bool {
			runIDInt := runID.(int64)
			runProperty, err := model.GetRun(runIDInt)
			if err != nil {
				log.Error(err)
				return false
			}
			// if EndTime is Zero the plan is still running
			if runProperty.EndTime.IsZero() {
				return true
			}
			c.deleteMetricByRunID(runIDInt, runProperty.CollectionID)
			return true
		})
		time.Sleep(120 * time.Second)
	}
	// this won't delete in edge case where the collection configuration has changed immediately
}

func isCollectionStale(rh *model.RunHistory, launchTime time.Time) (bool, error) {
	// wait for X minutes before purging any collection
	if time.Since(launchTime).Minutes() < config.SC.ExecutorConfig.Cluster.GCDuration {
		return false, nil
	}
	// if the collection has never been run before
	if rh == nil {
		return true, nil
	}
	// if collection is running or
	// if X minutes haven't passed since last run, collection is still being used
	if rh.EndTime.IsZero() || (time.Since(rh.EndTime).Minutes() < config.SC.ExecutorConfig.Cluster.GCDuration) {
		return false, nil
	}
	return true, nil
}

func (c *Controller) autoPurgeDeployments() {
	for {
		deployedCollections, err := c.Kcm.GetDeployedCollections()
		if err != nil {
			log.Error(err)
			continue
		}
		if config.SC.ExecutorConfig.Cluster.OnDemand {
			c.autoPurgeNodes(deployedCollections)
		}
		for collectionID, launchTime := range deployedCollections {
			collection, err := model.GetCollection(collectionID)
			if err != nil {
				log.Error(err)
				continue
			}
			lr, err := collection.GetLastRun()
			if err != nil {
				log.Error(err)
				continue
			}
			status, err := isCollectionStale(lr, launchTime)
			if err != nil {
				log.Error(err)
				continue
			}
			if !status {
				continue
			}
			err = c.TermAndPurgeCollection(collection)
			if err != nil {
				log.Error(err)
				continue
			}
		}
		time.Sleep(60 * time.Second)
	}
}

func (c *Controller) autoPurgeNodes(deployedCollections map[int64]time.Time) {
	launchedNodes, err := c.Kcm.GetAllNodesInfo()
	if err != nil {
		log.Error(err)
		return
	}
	for id, nodeInfo := range launchedNodes {
		collectionID, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			log.Error(err)
			return
		}
		// if has pods deployed, don't touch
		if _, ok := deployedCollections[collectionID]; ok {
			return
		}
		collection, err := model.GetCollection(collectionID)
		if err != nil {
			log.Error(err)
			return
		}
		lr, err := collection.GetLastRun()
		if err != nil {
			log.Error(err)
			return
		}
		status, err := isCollectionStale(lr, nodeInfo.LaunchTime)
		if err != nil {
			log.Error(err)
			return
		}
		if !status {
			return
		}
		err = c.PurgeNodes(collection)
		if err != nil {
			log.Error(err)
			return
		}
	}
}
