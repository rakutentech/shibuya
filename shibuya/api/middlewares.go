package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

const (
	accountKey = "account"
)

func authWithSession(r *http.Request) (*model.Account, error) {
	account := model.GetAccountBySession(r)
	if account == nil {
		return nil, makeLoginError()
	}
	return account, nil
}

// TODO add JWT token auth in the future
func authWithToken(_ *http.Request) (*model.Account, error) {
	return nil, errors.New("No token presented")
}

func (s *ShibuyaAPI) authRequired(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		var account *model.Account
		var err error
		account, err = authWithSession(r)
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), accountKey, account)), params)
	})
}
