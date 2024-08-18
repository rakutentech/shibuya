package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

func hasProjectOwnership(project *model.Project, account *model.Account) bool {
	if _, ok := account.MLMap[project.Owner]; !ok {
		if !account.IsAdmin() {
			return false
		}
	}
	return true
}

func hasCollectionOwnership(r *http.Request, params httprouter.Params) (*model.Collection, error) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		return nil, err
	}
	account := r.Context().Value(accountKey).(*model.Account)
	project, err := model.GetProject(collection.ProjectID)
	if err != nil {
		return nil, err
	}
	if r := hasProjectOwnership(project, account); !r {
		return nil, makeCollectionOwnershipError()
	}
	return collection, nil
}
