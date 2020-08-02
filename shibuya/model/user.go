package model

import (
	"net/http"

	"github.com/harpratap/shibuya/auth"
	"github.com/harpratap/shibuya/config"
)

type Account struct {
	ML    []string
	MLMap map[string]interface{}
	Name  string
}

var es interface{}

func GetAccountBySession(r *http.Request) *Account {
	a := new(Account)
	a.MLMap = make(map[string]interface{})
	if config.SC.AuthConfig.NoAuth {
		a.Name = "shibuya"
		a.ML = []string{a.Name}
		a.MLMap[a.Name] = es
		return a
	}
	session, err := auth.SessionStore.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		return nil
	}
	accountName := session.Values[auth.AccountKey]
	if accountName == nil {
		return nil
	}
	a.Name = accountName.(string)
	a.ML = session.Values[auth.MLKey].([]string)
	for _, m := range a.ML {
		a.MLMap[m] = es
	}
	return a
}
