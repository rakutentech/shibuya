package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/context"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rakutentech/shibuya/shibuya/api"
	"github.com/rakutentech/shibuya/shibuya/ui"
	log "github.com/sirupsen/logrus"
	_ "go.uber.org/automaxprocs"
)

func main() {
	api := api.NewAPIServer()
	routes := api.InitRoutes()
	ui := ui.NewUI()
	uiRoutes := ui.InitRoutes()
	routes = append(routes, uiRoutes...)
	r := httprouter.New()
	for _, route := range routes {
		r.Handle(route.Method, route.Path, route.HandlerFunc)
	}
	r.Handler("GET", "/metrics", promhttp.Handler())

	fileServer := http.FileServer(http.Dir("/static"))
	r.GET("/static/*filepath", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		req.URL.Path = ps.ByName("filepath")
		// Set the cache expiration time to 7 days
		w.Header().Set("Cache-Control", "public, max-age=604800")
		fileServer.ServeHTTP(w, req)
	})
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", 8080), context.ClearHandler(r)))
}
