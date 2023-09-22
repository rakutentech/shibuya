package api

import (
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"
)

func retrieveClientIP(r *http.Request) string {
	t := r.Header.Get("x-forwarded-for")
	if t == "" {
		return r.RemoteAddr
	}
	return strings.Split(t, ",")[0]
}

func isAdmin(account *model.Account) bool {
	for _, ml := range account.ML {
		for _, admin := range config.SC.AuthConfig.AdminUsers {
			if ml == admin {
				return true
			}
		}
	}
	return false
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
		if !isAdmin(account) {
			return nil, makeNoPermissionErr("")
		}

	}
	return collection, nil
}
