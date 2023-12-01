package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
	"gopkg.in/yaml.v2"
)

func getCollection(collectionID string) (*model.Collection, error) {
	cid, err := strconv.Atoi(collectionID)
	if err != nil {
		return nil, makeInvalidResourceError("collection_id")
	}
	collection, err := model.GetCollection(int64(cid))
	if err != nil {
		return nil, err
	}
	return collection, nil
}

func (s *ShibuyaAPI) collectionConfigGetHandler(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	collection, err := checkCollectionOwnership(req, params)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	for _, ep := range eps {
		plan, err := model.GetPlan(ep.PlanID)
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		ep.Name = plan.Name
	}
	e := &model.ExecutionWrapper{
		Content: &model.ExecutionCollection{
			Name:         collection.Name,
			ProjectID:    collection.ProjectID,
			CollectionID: collection.ID,
			Tests:        eps,
			CSVSplit:     collection.CSVSplit,
		},
	}
	content, err := yaml.Marshal(e)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	r := bytes.NewReader(content)
	filename := fmt.Sprintf("%d.yaml", collection.ID)
	header := fmt.Sprintf("Attachment; filename=%s", filename)
	w.Header().Add("Content-Disposition", header)

	http.ServeContent(w, req, filename, time.Now(), r)
}
