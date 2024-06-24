package controller

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) CheckRunningThenTerminate() {
	jobs := make(chan *RunningPlan)
	for w := 1; w <= 3; w++ {
		go func(jobs <-chan *RunningPlan) {
		jobLoop:
			for j := range jobs {
				pc := NewPlanController(j.ep, j.collection, c.Scheduler)
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
			log.Error(err)
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

func (c *Controller) AutoPurgeDeployments() {
	log.Info("Start the loop for purging idle engines")
	for {
		deployedCollections, err := c.Scheduler.GetDeployedCollections()
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

// We'll keep the IP for defined period of time since the project was last time used.
// Last time used is defined as:
// 1. If none of the collections has a run, it will be the last launch time of the engines of a collection
// 2. If any of the collection has a run, it will be the end time of that run
func (c *Controller) AutoPurgeProjectIngressController() {
	log.Info("Start the loop for purging idle ingress controllers")
	projectLastUsedTime := make(map[int64]time.Time)
	ingressLifespan, err := time.ParseDuration(config.SC.IngressConfig.Lifespan)
	if err != nil {
		log.Fatal(err)
	}
	gcInterval, err := time.ParseDuration(config.SC.IngressConfig.GCInterval)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(fmt.Sprintf("Project ingress lifespan is %v. And the GC Interval is %v", ingressLifespan, gcInterval))
	for {
		deployedServices, err := c.Scheduler.GetDeployedServices()
		if err != nil {
			continue
		}
	svcLoop:
		for projectID := range deployedServices {
			// Because of this if, the GC could not know any operation happening in the span.
			// When the time is up and there is no engines deployment, the ingress ip will be deleted right away.
			// if time.Since(createdTime) < ingressLifespan {
			// 	continue
			// }
			pods, err := c.Scheduler.GetEnginesByProject(projectID)
			if err != nil {
				continue
			}
			t, err := time.Parse("2006-01-03", "2000-01-01")
			if err != nil {
				log.Fatal(err)
			}
			latestRun := &model.RunHistory{EndTime: t}
			for _, p := range pods {
				collectionID, err := strconv.ParseInt(p.Labels["collection"], 10, 64)
				if err != nil {
					log.Error(err)
					continue svcLoop
				}
				collection, err := model.GetCollection(collectionID)
				if err != nil {
					continue svcLoop
				}
				lr, err := collection.GetLastRun()
				if err != nil {
					continue svcLoop
				}
				if lr != nil {
					// We need to track the ongoing run because if run stops before the loop and engines are purged,
					// the lastUsedTime will be the engine launch time.
					if lr.EndTime.IsZero() {
						lr.EndTime = time.Now()
					}
					if lr.EndTime.After(latestRun.EndTime) {
						latestRun = lr
					}
				}
			}
			plu := projectLastUsedTime[projectID]
			if len(pods) > 0 {
				// the pods are ordered by created time in asc order. So the first pod in the list
				// is the most reccently being created
				podLastCreatedTime := pods[0].CreationTimestamp.Time
				if podLastCreatedTime.After(plu) {
					plu = podLastCreatedTime
				}
			}
			// we also need this line because if there is no engines deployed, we could not find the latest run
			if latestRun.EndTime.After(plu) {
				plu = latestRun.EndTime
			}
			projectLastUsedTime[projectID] = plu
			if time.Since(plu) > ingressLifespan {
				log.Println(fmt.Sprintf("Going to delete ingress for project %d. Last used time was %v", projectID, plu))
				c.Scheduler.PurgeProjectIngress(projectID)
			}
		}
		// The interval should not be very long. For example, a collection has been launched for 30 minutes,
		// If there is a run being executed for a minute, there is a chance the GC misses that run and the project
		// ip will be deleted.
		time.Sleep(gcInterval)
	}
}

func (c *Controller) autoPurgeNodes(deployedCollections map[int64]time.Time) {
	launchedNodes, err := c.Scheduler.GetAllNodesInfo()
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
