package api

import (
	"net/http"
	"strings"
)

func retrieveClientIP(r *http.Request) string {
	t := r.Header.Get("x-forwarded-for")
	if t == "" {
		return r.RemoteAddr
	}
	return strings.Split(t, ",")[0]
}
