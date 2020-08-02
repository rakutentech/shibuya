package api

import (
	"strconv"

	"github.com/harpratap/shibuya/model"
)

func getCollection(collectionID string) (*model.Collection, error) {
	cid, err := strconv.Atoi(collectionID)
	if err != nil {
		return nil, makeInvalidResourceError("collection_id")
	}
	collection, err := model.GetCollection(int64(cid))
	if err != nil {
		return nil, err
	}
	return collection, nil
}
