package api

import (
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

func retrieveClientIP(r *http.Request) string {
	t := r.Header.Get("x-forwarded-for")
	if t == "" {
		return r.RemoteAddr
	}
	return strings.Split(t, ",")[0]
}

func checkCollectionOwnership(r *http.Request, params httprouter.Params) (*model.Collection, error) {
	account := model.GetAccountBySession(r)
	if account == nil {
		return nil, makeLoginError()
	}
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		return nil, err
	}
	project, err := model.GetProject(collection.ProjectID)
	if err != nil {
		return nil, err
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		if !account.IsAdmin() {
			return nil, makeNoPermissionErr("You are not the owner of the collection")
		}
	}
	return collection, nil
}
