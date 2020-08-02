package auth

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/harpratap/shibuya/config"
	ldap "gopkg.in/ldap.v2"
)

var (
	CNPattern  = regexp.MustCompile(`CN=([^,]+)\,OU=DLM\sDistribution\sGroups`)
	AccountKey = "account"
	MLKey      = "ml"
)

type AuthResult struct {
	ML []string
}

func Auth(username, password string) (*AuthResult, error) {
	r := new(AuthResult)
	ac := config.SC.AuthConfig
	ldapServer := ac.LdapServer
	ldapPort := ac.LdapPort
	baseDN := ac.BaseDN
	r.ML = []string{}

	filter := "(&(objectClass=user)(sAMAccountName=%s))"
	l, err := ldap.Dial("tcp", fmt.Sprintf("%s:%s", ldapServer, ldapPort))
	if err != nil {
		return r, err
	}
	defer l.Close()
	systemUser := ac.SystemUser
	systemPassword := ac.SystemPassword
	err = l.Bind(systemUser, systemPassword)
	if err != nil {
		return r, err
	}
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(filter, username),
		[]string{"userprincipalname"},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		return r, err
	}

	entries := sr.Entries
	if len(entries) != 1 {
		return r, errors.New("Users does not exist")
	}

	attributes := entries[0].Attributes
	if len(attributes) == 0 {
		return r, errors.New("Cannot find the user")
	}
	values := attributes[0].Values
	if len(values) == 0 {
		return r, errors.New("Cannot find the principle name")
	}
	UserPrincipalName := values[0]
	err = l.Bind(UserPrincipalName, password)
	if err != nil {
		return r, errors.New("Incorrect password")
	}
	searchRequest = ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(filter, username),
		[]string{"memberOf"},
		nil,
	)
	sr, err = l.Search(searchRequest)
	if err != nil {
		return r, errors.New("Error in contacting LDAP server")
	}
	entries = sr.Entries
	if len(entries) == 0 {
		return r, errors.New("Cannot find user ml/group information")
	}
	attributes = entries[0].Attributes
	if len(attributes) == 0 {
		return r, errors.New("Cannot find user group/ml information")
	}
	values = attributes[0].Values
	for _, m := range values {
		match := CNPattern.FindStringSubmatch(m)
		if match == nil {
			continue
		}
		r.ML = append(r.ML, match[1])
	}
	return r, nil
}
