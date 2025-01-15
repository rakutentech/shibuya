package controller

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	enginesModel "github.com/rakutentech/shibuya/shibuya/engines/model"
	"github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/scheduler"
	_ "github.com/rakutentech/shibuya/shibuya/utils"
	log "github.com/sirupsen/logrus"
)

type PlanController struct {
	ep         *model.ExecutionPlan
	collection *model.Collection
	scheduler  scheduler.EngineScheduler
}

func NewPlanController(ep *model.ExecutionPlan, collection *model.Collection, scheduler scheduler.EngineScheduler) *PlanController {
	return &PlanController{
		ep:         ep,
		collection: collection,
		scheduler:  scheduler,
	}
}

func (pc *PlanController) deploy() error {
	engineConfig := findEngineConfig(JmeterEngineType)
	if err := pc.scheduler.DeployPlan(pc.collection.ProjectID, pc.collection.ID, pc.ep.PlanID,
		pc.ep.Engines, engineConfig); err != nil {
		return err
	}
	return nil
}

func (pc *PlanController) prepare(plan *model.Plan, edc *enginesModel.EngineDataConfig, runID int64) []*enginesModel.EngineDataConfig {
	edc.Duration = strconv.Itoa(pc.ep.Duration)
	edc.Concurrency = strconv.Itoa(pc.ep.Concurrency)
	edc.Rampup = strconv.Itoa(pc.ep.Rampup)
	engineDataConfigs := edc.DeepCopies(pc.ep.Engines)
	for i := 0; i < pc.ep.Engines; i++ {
		// we split the data inherited from collection if the plan specifies split too
		if pc.ep.CSVSplit {
			for _, ed := range engineDataConfigs[i].EngineData {
				ed.TotalSplits *= pc.ep.Engines
				ed.CurrentSplit = (ed.CurrentSplit * pc.ep.Engines) + i
			}
		}
		// Add test file to all engines
		engineDataConfigs[i].EngineData[plan.TestFile.Filename] = plan.TestFile
		engineDataConfigs[i].RunID = runID
		engineDataConfigs[i].EngineID = i
		// add all data uploaded in plans. This will override common data if same filename already exists
		for _, d := range plan.Data {
			sf := model.ShibuyaFile{
				Filename:     d.Filename,
				Filepath:     d.Filepath,
				TotalSplits:  1,
				CurrentSplit: 0,
			}
			if pc.ep.CSVSplit {
				sf.TotalSplits = pc.ep.Engines
				sf.CurrentSplit = i
			}
			engineDataConfigs[i].EngineData[d.Filename] = &sf
		}
	}
	return engineDataConfigs
}

func (pc *PlanController) trigger(engineDataConfig *enginesModel.EngineDataConfig, runID int64) error {
	plan, err := model.GetPlan(pc.ep.PlanID)
	if err != nil {
		return err
	}
	engineDataConfigs := pc.prepare(plan, engineDataConfig, runID)
	engines, err := generateEnginesWithUrl(pc.ep.Engines, pc.ep.PlanID, pc.collection.ID, pc.collection.ProjectID,
		JmeterEngineType, pc.scheduler)
	if err != nil {
		return err
	}
	errs := make(chan error, len(engines))
	defer close(errs)
	planErrors := []error{}
	for i, engine := range engines {
		go func(engine shibuyaEngine, i int) {
			if err := engine.trigger(engineDataConfigs[i]); err != nil {
				errs <- err
				return
			}
			errs <- nil
		}(engine, i)
	}
	for i := 0; i < len(engines); i++ {
		if err := <-errs; err != nil {
			planErrors = append(planErrors, err)
		}
	}
	if len(planErrors) > 0 {
		return fmt.Errorf("Trigger plan errors:%v", planErrors)
	}
	log.Printf("Triggering for plan %d is finished", pc.ep.PlanID)
	return nil
}

func (pc *PlanController) subscribe() ([]shibuyaEngine, error) {
	ep := pc.ep
	collection := pc.collection
	engines, err := generateEnginesWithUrl(ep.Engines, ep.PlanID, collection.ID, collection.ProjectID,
		JmeterEngineType, pc.scheduler)
	if err != nil {
		return nil, err
	}
	runID, err := collection.GetCurrentRun()
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	readingEngines := []shibuyaEngine{}
	for _, engine := range engines {
		wg.Add(1)
		go func(engine shibuyaEngine, runID int64) {
			defer wg.Done()
			//After this step, the engine instance has states including stream client
			err := engine.subscribe(runID)
			if err != nil {
				return
			}
			readingEngines = append(readingEngines, engine)
		}(engine, runID)
	}
	wg.Wait()
	log.Printf("Subscribe to Plan %d", ep.PlanID)
	return readingEngines, err
}

func (pc *PlanController) UnSubscribe() {

}

// TODO. we can use the cached clients here.
func (pc *PlanController) progress() bool {
	r := true
	ep := pc.ep
	collection := pc.collection
	engines, err := generateEnginesWithUrl(ep.Engines, ep.PlanID, collection.ID, collection.ProjectID, JmeterEngineType, pc.scheduler)
	if errors.Is(err, scheduler.IngressError) {
		log.Error(err)
		return true
	} else if err != nil {
		return false
	}
	for _, engine := range engines {
		engineRunning := engine.progress()
		r = r && !engineRunning
	}
	return !r
}

func (pc *PlanController) term(force bool) error {
	var wg sync.WaitGroup
	ep := pc.ep
	collection := pc.collection
	engines, err := generateEnginesWithUrl(ep.Engines, ep.PlanID, collection.ID, collection.ProjectID, JmeterEngineType, pc.scheduler)
	if err != nil {
		return err
	}
	for _, engine := range engines {
		wg.Add(1)
		go func(e shibuyaEngine) {
			defer wg.Done()
			if err := e.terminate(force); err != nil {
				log.Error(err)
			}
		}(engine)
	}
	wg.Wait()
	model.DeleteRunningPlan(pc.collection.ID, ep.PlanID)
	return nil
}
