package ui

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/harpratap/shibuya/api"
	"github.com/harpratap/shibuya/auth"
	"github.com/harpratap/shibuya/config"
	"github.com/harpratap/shibuya/model"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

type UI struct {
	tmpl   *template.Template
	Routes []*api.Route
}

func NewUI() *UI {
	u := &UI{
		tmpl: template.Must(template.ParseGlob("/templates/*.html")),
	}
	return u
}

type HomeResp struct {
	Account               string
	BackgroundColour      string
	Context               string
	OnDemandCluster       bool
	IsAdmin               bool
	ResultDashboard       string
	EngineHealthDashboard string
	ProjectHome           string
	UploadFileHelp        string
}

func (u *UI) homeHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	IsAdmin := false
outer:
	for _, ml := range account.ML {
		for _, admin := range config.SC.AuthConfig.AdminUsers {
			if ml == admin {
				IsAdmin = true
				break outer
			}
		}
	}
	resultDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.RunDashboard
	engineHealthDashboardURL := config.SC.DashboardConfig.Url + config.SC.DashboardConfig.EnginesDashboard
	template := u.tmpl.Lookup("app.html")
	sc := config.SC
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		config.SC.ExecutorConfig.Cluster.OnDemand, IsAdmin, resultDashboardURL, engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp})
}

func (u *UI) loginHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	r.ParseForm()
	ss := auth.SessionStore
	session, err := ss.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
	}
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	authResult, err := auth.Auth(username, password)
	if err != nil {
		loginUrl := fmt.Sprintf("/login?error_msg=%v", err)
		http.Redirect(w, r, loginUrl, http.StatusSeeOther)
	}
	session.Values[auth.MLKey] = authResult.ML
	session.Values[auth.AccountKey] = username
	err = ss.Save(r, w, session)
	if err != nil {
		log.Panic(err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (u *UI) logoutHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	session, err := auth.SessionStore.Get(r, config.SC.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
		return
	}
	delete(session.Values, auth.MLKey)
	delete(session.Values, auth.AccountKey)
	session.Save(r, w)
}

type LoginResp struct {
	ErrorMsg string
}

func (u *UI) loginPageHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	template := u.tmpl.Lookup("login.html")
	qs := r.URL.Query()
	errMsgs := qs["error_msg"]
	e := new(LoginResp)
	e.ErrorMsg = ""
	if len(errMsgs) > 0 {
		e.ErrorMsg = errMsgs[0]
	}
	template.Execute(w, e)
}

func (u *UI) InitRoutes() api.Routes {
	return api.Routes{
		&api.Route{"home", "GET", "/", u.homeHandler},
		&api.Route{"login", "POST", "/login", u.loginHandler},
		&api.Route{"login", "GET", "/login", u.loginPageHandler},
		&api.Route{"logout", "POST", "/logout", u.logoutHandler},
	}
}
