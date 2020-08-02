package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/context"
	"github.com/harpratap/shibuya/api"
	"github.com/harpratap/shibuya/ui"
	"github.com/julienschmidt/httprouter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
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
	r.ServeFiles("/static/*filepath", http.Dir("/static"))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", 8080), context.ClearHandler(r)))
}
