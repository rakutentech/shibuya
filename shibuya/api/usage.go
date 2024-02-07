package api

import (
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

func (s *ShibuyaAPI) usageSummaryHandler(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	qs := req.URL.Query()
	st := qs.Get("started_time")
	et := qs.Get("end_time")
	summary, err := model.GetUsageSummary(st, et)
	if err != nil {
		log.Println(err)
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, summary)
}

func (s *ShibuyaAPI) usageSummaryHandlerBySid(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	qs := req.URL.Query()
	st := qs.Get("started_time")
	et := qs.Get("end_time")
	sid := qs.Get("sid")
	history, err := model.GetUsageSummaryBySid(sid, st, et)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, history)
}
