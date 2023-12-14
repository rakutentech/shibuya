package model

import (
	"net/http"

	"github.com/rakutentech/shibuya/shibuya/auth"
	"github.com/rakutentech/shibuya/shibuya/config"
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

func (a *Account) IsAdmin() bool {
	for _, ml := range a.ML {
		for _, admin := range config.SC.AuthConfig.AdminUsers {
			if ml == admin {
				return true
			}
		}
	}
	// systemuser is the user used for LDAP auth. If a user login with that account
	// we can also treat it as a admin
	if a.Name == config.SC.AuthConfig.SystemUser {
		return true
	}
	return false
}
