package controller

import (
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
	c.collectionStatusCache.Delete(collection.ID)
	return c.Scheduler.PurgeCollection(collection.ID)
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
