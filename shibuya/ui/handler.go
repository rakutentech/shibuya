package ui

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/api"
	"github.com/rakutentech/shibuya/shibuya/auth"
	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"
	log "github.com/sirupsen/logrus"
)

type UI struct {
	sc         config.ShibuyaConfig
	tmpl       *template.Template
	Routes     []*api.Route
	authMethod auth.LdapAuth
}

func NewUI(sc config.ShibuyaConfig) *UI {
	authMethod := auth.NewLdapAuth(*sc.AuthConfig.LdapConfig)
	u := &UI{
		sc:         sc,
		tmpl:       template.Must(template.ParseGlob("/templates/*.html")),
		authMethod: authMethod,
	}
	return u
}

type HomeResp struct {
	Account               string
	BackgroundColour      string
	Context               string
	IsAdmin               bool
	ResultDashboard       string
	EnableSid             bool
	EngineHealthDashboard string
	ProjectHome           string
	UploadFileHelp        string
	GCDuration            float64
}

func (u *UI) homeHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r, u.sc.AuthConfig)
	if account == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	IsAdmin := account.IsAdmin(u.sc.AuthConfig)
	sc := u.sc
	enableSid := sc.EnableSid
	resultDashboardURL := sc.DashboardConfig.Url + sc.DashboardConfig.RunDashboard
	engineHealthDashboardURL := sc.DashboardConfig.Url + sc.DashboardConfig.EnginesDashboard
	if sc.DashboardConfig.EnginesDashboard == "" {
		engineHealthDashboardURL = ""
	}
	template := u.tmpl.Lookup("app.html")
	gcDuration := sc.ExecutorConfig.Cluster.GCDuration
	template.Execute(w, &HomeResp{account.Name, sc.BackgroundColour, sc.Context,
		IsAdmin, resultDashboardURL, enableSid,
		engineHealthDashboardURL, sc.ProjectHome, sc.UploadFileHelp, gcDuration})
}

func (u *UI) loginHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	r.ParseForm()
	ss := auth.SessionStore
	session, err := ss.Get(r, u.sc.AuthConfig.SessionKey)
	if err != nil {
		log.Print(err)
	}
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	authResult, err := u.authMethod.Auth(username, password)
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
	session, err := auth.SessionStore.Get(r, u.sc.AuthConfig.SessionKey)
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
