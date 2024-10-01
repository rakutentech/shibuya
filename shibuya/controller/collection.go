package controller

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/rakutentech/shibuya/shibuya/config"
	enginesModel "github.com/rakutentech/shibuya/shibuya/engines/model"
	"github.com/rakutentech/shibuya/shibuya/model"
	log "github.com/sirupsen/logrus"
)

func prepareCollection(collection *model.Collection) []*enginesModel.EngineDataConfig {
	planCount := len(collection.ExecutionPlans)
	edc := enginesModel.EngineDataConfig{
		EngineData: map[string]*model.ShibuyaFile{},
	}
	engineDataConfigs := edc.DeepCopies(planCount)
	for i := 0; i < planCount; i++ {
		for _, d := range collection.Data {
			sf := model.ShibuyaFile{
				Filename:     d.Filename,
				Filepath:     d.Filepath,
				TotalSplits:  1,
				CurrentSplit: 0,
			}
			if collection.CSVSplit {
				sf.TotalSplits = planCount
				sf.CurrentSplit = i
			}
			engineDataConfigs[i].EngineData[sf.Filename] = &sf
		}
	}
	return engineDataConfigs
}

func (c *Controller) calculateUsage(collection *model.Collection) error {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	vu := 0
	for _, ep := range eps {
		vu += ep.Engines * ep.Concurrency
	}
	return collection.MarkUsageFinished(config.SC.Context, int64(vu))
}

func (c *Controller) TermAndPurgeCollection(collection *model.Collection) (err error) {
	// This is a force remove so we ignore the errors happened at test termination
	defer func() {
		// This is a bit tricky. We only set the error to the outer scope to not nil when e is not nil
		// Otherwise the nil will override the err value in the main func.
		if e := c.calculateUsage(collection); e != nil {
			err = e
		}
	}()
	c.TermCollection(collection, true)
	if err = c.Scheduler.PurgeCollection(collection.ID); err != nil {
		return err
	}
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	for _, p := range eps {
		c.deleteEngineHealthMetrics(strconv.Itoa(int(collection.ID)), strconv.Itoa(int(p.PlanID)), p.Engines)
	}
	return err
}

func (c *Controller) TriggerCollection(collection *model.Collection) error {
	var err error
	// Get all the execution plans within the collection
	// Execution plans are the buiding block of a collection.
	// They define the concurrent/duration etc
	// All the pre-fetched resources will go alone with the collection object
	collection.ExecutionPlans, err = collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	engineDataConfigs := prepareCollection(collection)
	for _, ep := range collection.ExecutionPlans {
		plan, err := model.GetPlan(ep.PlanID)
		if err != nil {
			return err
		}
		if plan.TestFile == nil {
			return fmt.Errorf("Triggering plan aborted. There is no Test file (.jmx) in this plan %d", plan.ID)
		}
	}
	runID, err := collection.StartRun()
	if err != nil {
		return err
	}
	errs := make(chan error, len(collection.ExecutionPlans))
	defer close(errs)
	for i, ep := range collection.ExecutionPlans {
		go func(i int, ep *model.ExecutionPlan) {
			// We wait for all the engines. Because we can only all the plan into running status
			// When all the engines are triggered

			pc := NewPlanController(ep, collection, c.Scheduler)
			if err := pc.trigger(engineDataConfigs[i], runID); err != nil {
				errs <- err
				return
			}
			// We don't wait all the engines. Because stream establishment can take some time
			// We don't want the UI to be freeze for long time
			if err := pc.subscribe(&c.connectedEngines, c.readingEngines); err != nil {
				errs <- err
				return
			}
			if err := model.AddRunningPlan(collection.ID, ep.PlanID); err != nil {
				errs <- err
				return
			}
			errs <- nil
		}(i, ep)
	}
	triggerErrors := []error{}
	for i := 0; i < len(collection.ExecutionPlans); i++ {
		if err := <-errs; err != nil {
			triggerErrors = append(triggerErrors, err)
		}
	}
	collection.NewRun(runID)
	if len(triggerErrors) == len(collection.ExecutionPlans) {
		// every plan in collection has error
		c.TermCollection(collection, true)
	}
	if len(triggerErrors) > 0 {
		return fmt.Errorf("Triggering errors %v", triggerErrors)
	}
	return nil
}

func (c *Controller) TermCollection(collection *model.Collection, force bool) (e error) {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	currRunID, err := collection.GetCurrentRun()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, ep := range eps {
		wg.Add(1)
		go func(ep *model.ExecutionPlan) {
			defer wg.Done()
			pc := NewPlanController(ep, collection, nil) // we don't need scheduler here
			if err := pc.term(force, &c.connectedEngines); err != nil {
				log.Error(err)
				e = err
			}
			log.Printf("Plan %d is terminated.", ep.PlanID)
		}(ep)
	}
	wg.Wait()
	collection.StopRun()
	collection.RunFinish(currRunID)
	return e
}
