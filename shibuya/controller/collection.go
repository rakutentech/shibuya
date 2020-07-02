package controller

import (
	"shibuya/model"
	"shibuya/utils"
)

func prepareCollection(collection *model.Collection) (*ExecutionData, error) {
	if err := collection.FetchCollectionFiles(); err != nil {
		return &ExecutionData{}, err
	}
	planCount := len(collection.ExecutionPlans)
	splittedData := newExecutionData(planCount, collection.CSVSplit)
	for _, d := range collection.Data {
		if err := splittedData.PrepareExecutionData(d); err != nil {
			return &ExecutionData{}, err
		}
	}
	//dereference the data files so gc can remove them if not needed anymore
	collection.Data = []*model.ShibuyaFile{}
	return splittedData, nil
}

func (c *Controller) TermAndPurgeCollection(collection *model.Collection) error {
	// This is a force remove so we ignore the errors happened at test termination
	c.TermCollection(collection, true)
	err := utils.Retry(func() error {
		return c.Kcm.PurgeCollection(collection.ID)
	})
	return err
}
