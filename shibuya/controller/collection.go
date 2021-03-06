package controller

import (
	"fmt"
	"sync"

	"github.com/rakutentech/shibuya/shibuya/config"
	controllerModel "github.com/rakutentech/shibuya/shibuya/controller/model"
	"github.com/rakutentech/shibuya/shibuya/model"
	log "github.com/sirupsen/logrus"
)

func prepareCollection(collection *model.Collection) []*controllerModel.EngineDataConfig {
	planCount := len(collection.ExecutionPlans)
	edc := controllerModel.EngineDataConfig{
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

func (c *Controller) TermAndPurgeCollection(collection *model.Collection) error {
	// This is a force remove so we ignore the errors happened at test termination
	c.TermCollection(collection, true)
	err := c.Scheduler.PurgeCollection(collection.ID)
	if err == nil {
		eps, err := collection.GetExecutionPlans()

		// if we cannot get the eps, we ignore as we don't want billing to have impact on UX.
		if err != nil {
			return nil
		}
		vu := 0
		for _, ep := range eps {
			vu += ep.Engines * ep.Concurrency
		}
		collection.MarkUsageFinished(config.SC.Context, int64(vu))
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
			if err := pc.trigger(engineDataConfigs[i]); err != nil {
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

func (c *Controller) TermAndPurgeCollectionAsync(collection *model.Collection) {
	// This method is supposed to be only used by API side because for large collections, k8s api might take long time to respond
	go func() {
		err := c.TermAndPurgeCollection(collection)
		if err != nil {
			log.Print(err)
		}
	}()
}
