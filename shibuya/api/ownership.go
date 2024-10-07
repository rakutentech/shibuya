package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

func (s *ShibuyaAPI) hasProjectOwnership(project *model.Project, account *model.Account) bool {
	if _, ok := account.MLMap[project.Owner]; !ok {
		if !account.IsAdmin(s.sc.AuthConfig) {
			return false
		}
	}
	return true
}

func (s *ShibuyaAPI) hasCollectionOwnership(r *http.Request, params httprouter.Params) (*model.Collection, error) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		return nil, err
	}
	account := r.Context().Value(accountKey).(*model.Account)
	project, err := model.GetProject(collection.ProjectID)
	if err != nil {
		return nil, err
	}
	if r := s.hasProjectOwnership(project, account); !r {
		return nil, makeCollectionOwnershipError()
	}
	return collection, nil
}
