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

func GetAccountBySession(r *http.Request, authConfig *config.AuthConfig) *Account {
	a := new(Account)
	a.MLMap = make(map[string]interface{})
	if authConfig.NoAuth {
		a.Name = "shibuya"
		a.ML = []string{a.Name}
		a.MLMap[a.Name] = es
		return a
	}
	session, err := auth.SessionStore.Get(r, authConfig.SessionKey)
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

func (a *Account) IsAdmin(authConfig *config.AuthConfig) bool {
	for _, ml := range a.ML {
		for _, admin := range authConfig.AdminUsers {
			if ml == admin {
				return true
			}
		}
	}
	// systemuser is the user used for LDAP auth. If a user login with that account
	// we can also treat it as a admin
	if a.Name == authConfig.SystemUser {
		return true
	}
	return false
}
