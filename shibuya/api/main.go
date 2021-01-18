package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/controller"
	"github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/object_storage"
	"github.com/rakutentech/shibuya/shibuya/scheduler"
	utils "github.com/rakutentech/shibuya/shibuya/utils"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

type ShibuyaAPI struct {
	ctr *controller.Controller
}

func NewAPIServer() *ShibuyaAPI {
	return &ShibuyaAPI{
		ctr: controller.NewController(),
	}
}

type JSONMessage struct {
	Message string `json:"message"`
}

func (s *ShibuyaAPI) jsonise(w http.ResponseWriter, status int, content interface{}) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(content)
}

func (s *ShibuyaAPI) makeRespMessage(message string) *JSONMessage {
	return &JSONMessage{
		Message: message,
	}
}

func (s *ShibuyaAPI) makeFailMessage(w http.ResponseWriter, message string, statusCode int) {
	messageObj := s.makeRespMessage(message)
	s.jsonise(w, statusCode, messageObj)
}

func (s *ShibuyaAPI) handleErrors(w http.ResponseWriter, err error) {
	var dbe *model.DBError
	if errors.As(err, &dbe) {
		s.makeFailMessage(w, dbe.Error(), http.StatusNotFound)
		return
	}
	switch {
	case errors.Is(err, noPermissionErr):
		s.makeFailMessage(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, invalidRequestErr):
		s.makeFailMessage(w, err.Error(), http.StatusBadRequest)
	default:
		log.Printf("api error: %v", err)
		s.makeFailMessage(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *ShibuyaAPI) projectsGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.makeFailMessage(w, "Need to login", http.StatusForbidden)
		return
	}
	qs := r.URL.Query()
	var includeCollections, includePlans bool
	var err error

	includeCollectionsList := qs["include_collections"]
	includePlansList := qs["include_plans"]
	if len(includeCollectionsList) > 0 {
		if includeCollections, err = strconv.ParseBool(includeCollectionsList[0]); err != nil {
			includeCollections = false
		}
	} else {
		includeCollections = false
	}

	if len(includePlansList) > 0 {
		if includePlans, err = strconv.ParseBool(includePlansList[0]); err != nil {
			includePlans = false
		}
	} else {
		includePlans = false
	}
	projects, _ := model.GetProjectsByOwners(account.ML)
	if !includeCollections && !includePlans {
		s.jsonise(w, http.StatusOK, projects)
		return
	}
	for _, p := range projects {
		if includeCollections {
			p.Collections, _ = p.GetCollections()
		}
		if includePlans {
			p.Plans, _ = p.GetPlans()
		}
	}
	s.jsonise(w, http.StatusOK, projects)
}

func (s *ShibuyaAPI) projectGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	project, err := getProject(params.ByName("project_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, project)
}

func (s *ShibuyaAPI) projectUpdateHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) projectCreateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	r.ParseForm()
	name := r.Form.Get("name")
	if name == "" {
		s.handleErrors(w, makeInvalidRequestError("Project name cannot be empty"))
		return
	}
	owner := r.Form.Get("owner")
	if owner == "" {
		s.handleErrors(w, makeInvalidRequestError("Owner name cannot be empty"))
		return
	}
	if _, ok := account.MLMap[owner]; !ok {
		s.handleErrors(w, makeNoPermissionErr(fmt.Sprintf("You are not part of %s", owner)))
		return
	}
	projectID, err := model.CreateProject(name, owner)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	project, err := model.GetProject(projectID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, project)
}

func (s *ShibuyaAPI) projectDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	project, err := getProject(params.ByName("project_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, noPermissionErr)
		return
	}
	collectionIDs, err := project.GetCollections()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if len(collectionIDs) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot delete a project that has collections"))
		return
	}
	planIDs, err := project.GetPlans()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if len(planIDs) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot delete a project that has plans"))
		return
	}
	project.Delete()
}

func (s *ShibuyaAPI) planGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	plan, err := getPlan(params.ByName("plan_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, plan)
}

func (s *ShibuyaAPI) planUpdateHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

type AdminCollectionResponse struct {
	RunningCollections []*model.RunningPlan   `json:"running_collections"`
	NodePools          scheduler.AllNodesInfo `json:"node_pools"`
}

func (s *ShibuyaAPI) collectionAdminGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collections, err := model.GetRunningCollections()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	acr := new(AdminCollectionResponse)
	acr.RunningCollections = collections
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		// we ignore errors here for simplicity
		acr.NodePools, _ = s.ctr.Kcm.GetAllNodesInfo()
	}
	s.jsonise(w, http.StatusOK, acr)
}

func (s *ShibuyaAPI) planCreateHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	r.ParseForm()
	projectID := r.Form.Get("project_id")
	project, err := getProject(projectID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, makeNoPermissionErr("You don't own the project"))
		return
	}
	name := r.Form.Get("name")
	if name == "" {
		s.handleErrors(w, makeInvalidRequestError("plan name cannot be empty"))
		return
	}
	planID, err := model.CreatePlan(name, project.ID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	plan, err := model.GetPlan(planID)
	if err != nil {
		s.handleErrors(w, err)
	}
	s.jsonise(w, http.StatusOK, plan)
}

func (s *ShibuyaAPI) planDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	plan, err := getPlan(params.ByName("plan_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	project, err := model.GetProject(plan.ProjectID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, makeLoginError())
		return
	}
	using, err := plan.IsBeingUsed()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if using {
		s.handleErrors(w, makeInvalidRequestError("plan is being used"))
		return
	}
	plan.Delete()
}

func (s *ShibuyaAPI) planFilesUploadHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	plan, err := getPlan(params.ByName("plan_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	r.ParseMultipartForm(100 << 20) //parse 100 MB of data
	file, handler, err := r.FormFile("planFile")
	if err != nil {
		s.handleErrors(w, makeInvalidRequestError("Something wrong with file you uploaded"))
		return
	}
	err = plan.StoreFile(file, handler.Filename)
	if err != nil {
		// TODO need to handle the upload error here
		s.handleErrors(w, err)
		return
	}
	w.Write([]byte("success"))
}

func (s *ShibuyaAPI) planFilesGetHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) collectionFilesGetHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) collectionFilesUploadHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	r.ParseMultipartForm(100 << 20) //parse 100 MB of data
	file, handler, err := r.FormFile("collectionFile")
	if err != nil {
		s.handleErrors(w, makeInvalidRequestError("Something wrong with file you uploaded"))
		return
	}
	err = collection.StoreFile(file, handler.Filename)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	w.Write([]byte("success"))
}

func (s *ShibuyaAPI) collectionFilesDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	r.ParseForm()
	filename := r.Form.Get("filename")
	if filename == "" {
		s.handleErrors(w, makeInvalidRequestError("Collection file name cannot be empty"))
		return
	}
	err = collection.DeleteFile(filename)
	if err != nil {
		s.handleErrors(w, makeInternalServerError("Deletion was unsuccessful"))
		return
	}
	w.Write([]byte("Deleted successfully"))
}

func (s *ShibuyaAPI) planFilesDeleteHandler(w http.ResponseWriter, r *http.Request, param httprouter.Params) {
	plan, err := getPlan(param.ByName("plan_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	r.ParseForm()
	filename := r.Form.Get("filename")
	if filename == "" {
		s.handleErrors(w, makeInvalidRequestError("plan file name cannot be empty"))
		return
	}
	err = plan.DeleteFile(filename)
	if err != nil {
		s.handleErrors(w, makeInternalServerError("Deletetion was unsuccessful"))
		return
	}
	w.Write([]byte("Deleted successfully"))
}

func (s *ShibuyaAPI) collectionCreateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	r.ParseForm()
	collectionName := r.Form.Get("name")
	if collectionName == "" {
		s.handleErrors(w, makeInvalidRequestError("collection name cannot be empty"))
		return
	}
	projectID := r.Form.Get("project_id")
	project, err := getProject(projectID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, makeNoPermissionErr("You don't have the permission"))
		return
	}
	collectionID, err := model.CreateCollection(collectionName, project.ID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	collection, err := model.GetCollection(int64(collectionID))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.jsonise(w, http.StatusOK, collection)
}

func (s *ShibuyaAPI) collectionDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	account := model.GetAccountBySession(r)
	if account == nil {
		s.handleErrors(w, makeLoginError())
		return
	}
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	project, err := model.GetProject(collection.ProjectID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, makeNoPermissionErr("You don't have permission"))
		return
	}
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		operator := controller.NewGCPOperator(collection.ID, 0)
		pool := operator.GetNodePool()
		if pool != nil {
			s.handleErrors(w, makeInvalidRequestError("You cannot delete collection when you have nodes launched"))
			return
		}
	}
	collectionLabel := fmt.Sprintf("collection=%d", collection.ID)
	if s.ctr.Kcm.PodReady(collectionLabel) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot launch engines when there are engines already deployed"))
		return
	}
	runningPlans, err := model.GetRunningPlansByCollection(collection.ID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if len(runningPlans) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot delete the collection during testing period"))
		return
	}
	collection.Delete()
}

func (s *ShibuyaAPI) collectionGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	// we ignore errors here as the front end will do the retry
	collection.ExecutionPlans, _ = collection.GetExecutionPlans()
	collection.RunHistories, _ = collection.GetRuns()
	s.jsonise(w, http.StatusOK, collection)
}

func hasInvalidDiff(curr, updated []*model.ExecutionPlan) (bool, string) {
	if len(updated) != len(curr) {
		return true, "You cannot add/remove plans while have engines deployed"
	}
	currCache := make(map[int64]*model.ExecutionPlan)
	for _, item := range curr {
		currCache[item.PlanID] = item
	}
	for _, item := range updated {
		currPlan, ok := currCache[item.PlanID]
		if !ok {
			return true, "You cannot add a new plan while have engines deployed"
		}
		if currPlan.Engines != item.Engines {
			return true, "You cannot change engine numbers while have engineds deployed"
		}
	}
	return false, ""
}

func (s *ShibuyaAPI) collectionUpdateHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) collectionUploadHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	e := new(model.ExecutionWrapper)
	r.ParseMultipartForm(1 << 20) //parse 1 MB of data
	file, _, err := r.FormFile("collectionYAML")
	if err != nil {
		s.handleErrors(w, makeInvalidResourceError("file"))
		return
	}
	raw, err := ioutil.ReadAll(file)
	if err != nil {
		s.handleErrors(w, makeInvalidRequestError("invalid file"))
		return
	}
	err = yaml.Unmarshal(raw, e)
	if err != nil {
		s.handleErrors(w, makeInvalidRequestError(err.Error()))
		return
	}
	if e == nil {
		s.handleErrors(w, makeInvalidResourceError("YAML file"))
		return
	}
	collectionID, _ := strconv.ParseInt(params.ByName("collection_id"), 10, 64)
	if (e.Content.CollectionID != 0) && (e.Content.CollectionID != collectionID) {
		s.handleErrors(w, makeInvalidRequestError("collection ID mismatch"))
		return
	}
	collection, err := model.GetCollection(collectionID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	project, _ := model.GetProject(collection.ProjectID)
	account := model.GetAccountBySession(r)
	if _, ok := account.MLMap[project.Owner]; !ok {
		s.handleErrors(w, makeNoPermissionErr("You don't own the project"))
		return
	}
	for _, ep := range e.Content.Tests {
		plan, err := model.GetPlan(ep.PlanID)
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		planProject, err := model.GetProject(plan.ProjectID)
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		if project.ID != planProject.ID {
			s.handleErrors(w, makeInvalidRequestError("You can only add plan within the same project"))
			return
		}
	}
	runningPlans, err := model.GetRunningPlansByCollection(collection.ID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if len(runningPlans) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot change the collection during testing period"))
		return
	}
	for _, ep := range e.Content.Tests {
		if ep.Engines <= 0 {
			s.handleErrors(w, makeInvalidRequestError("You cannot configure a plan with zero engine"))
			return
		}
	}
	collectionLabel := fmt.Sprintf("collection=%d", collection.ID)
	if s.ctr.Kcm.PodReady(collectionLabel) > 0 {
		currentPlans, err := collection.GetExecutionPlans()
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		if ok, message := hasInvalidDiff(currentPlans, e.Content.Tests); ok {
			s.handleErrors(w, makeInvalidRequestError(message))
			return
		}

	}
	err = collection.Store(e.Content)
	if err != nil {
		s.handleErrors(w, err)
	}
}

func (s *ShibuyaAPI) collectionDeploymentHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if err := s.ctr.DeployCollection(collection); err != nil {
		s.handleErrors(w, makeInternalServerError(err.Error()))
		return
	}
}

func (s *ShibuyaAPI) collectionTriggerHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if err := s.ctr.TriggerCollection(collection); err != nil {
		s.handleErrors(w, err)
		return
	}
}

func (s *ShibuyaAPI) collectionTermHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	if err := s.ctr.TermCollection(collection, false); err != nil {
		s.handleErrors(w, makeInternalServerError(err.Error()))
		return
	}
}

func (s *ShibuyaAPI) collectionStatusHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	collectionStatus, err := s.ctr.CollectionStatus(collection)
	if err != nil {
		s.handleErrors(w, err)
	}
	s.jsonise(w, http.StatusOK, collectionStatus)
}

func (s *ShibuyaAPI) collectionPurgeHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	s.ctr.TermAndPurgeCollectionAsync(collection)
}

func (s *ShibuyaAPI) collectionNodesGetHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) collectionNodesShutdownHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	collectionLabel := fmt.Sprintf("collection=%d", collection.ID)
	if s.ctr.Kcm.PodReady(collectionLabel) > 0 {
		s.handleErrors(w, makeInvalidRequestError("You cannot shut down nodes while you have engines deployed. Please purge collection first."))
		return
	}
	if err := s.ctr.PurgeNodes(collection); err != nil {
		s.handleErrors(w, makeInternalServerError(err.Error()))
		return
	}
	s.jsonise(w, http.StatusOK, true)
}

func (s *ShibuyaAPI) planLogHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	collectionID, err := strconv.Atoi(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, makeInvalidResourceError("collection_id"))
		return
	}
	planID, err := strconv.Atoi(params.ByName("plan_id"))
	if err != nil {
		s.handleErrors(w, makeInvalidResourceError("plan_id"))
		return
	}
	content, err := s.ctr.Kcm.DownloadPodLog(int64(collectionID), int64(planID))
	if err != nil {
		s.handleErrors(w, makeInvalidRequestError(err.Error()))
		return
	}
	m := make(map[string]string)
	m["c"] = content
	s.jsonise(w, http.StatusOK, m)
}

func (s *ShibuyaAPI) streamCollectionMetrics(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	// Currently we don't do authentication for simplicity.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	collection, err := getCollection(params.ByName("collection_id"))
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	clientIP := retrieveClientIP(r)
	item := &controller.ApiMetricStream{
		StreamClient: make(chan *controller.ApiMetricStreamEvent),
		CollectionID: fmt.Sprintf("%d", collection.ID),
		ClientID:     fmt.Sprintf("%s-%s", clientIP, utils.RandStringRunes(6)),
	}
	s.ctr.ApiNewClients <- item
	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		s.ctr.ApiClosingClients <- item
	}()
	for event := range item.StreamClient {
		if event == nil {
			continue
		}
		s, err := json.Marshal(event)
		if err != nil {
			fmt.Fprintf(w, "data:%v\n\n", err)
		} else {
			fmt.Fprintf(w, "data:%s\n\n", s)
		}
		flusher.Flush()
	}
}

func (s *ShibuyaAPI) runGetHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) runDeleteHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	s.jsonise(w, http.StatusNotImplemented, nil)
}

func (s *ShibuyaAPI) fileDownloadHandler(w http.ResponseWriter, req *http.Request, params httprouter.Params) {
	kind := params.ByName("kind")
	id := params.ByName("id")
	name := params.ByName("name")
	filename := fmt.Sprintf("%s/%s/%s", kind, id, name)

	data, err := object_storage.Client.Storage.Download(filename)
	if err != nil {
		s.jsonise(w, http.StatusNotFound, "not found")
		return
	}
	r := bytes.NewReader(data)
	w.Header().Add("Content-Disposition", "Attachment")
	http.ServeContent(w, req, filename, time.Now(), r)
}

type Route struct {
	Name        string
	Method      string
	Path        string
	HandlerFunc httprouter.Handle
}

type Routes []*Route

func (s *ShibuyaAPI) InitRoutes() Routes {
	return Routes{
		&Route{"get_projects", "GET", "/api/projects", s.projectsGetHandler},
		&Route{"create_project", "POST", "/api/projects", s.projectCreateHandler},
		&Route{"delete_project", "DELETE", "/api/projects/:project_id", s.projectDeleteHandler},
		&Route{"get_project", "GET", "/api/projects/:project_id", s.projectGetHandler},
		&Route{"update_project", "PUT", "/api/projects/:project_id", s.projectUpdateHandler},

		&Route{"create_plan", "POST", "/api/plans", s.planCreateHandler},
		&Route{"get_plan", "GET", "/api/plans/:plan_id", s.planGetHandler},
		&Route{"update_plan", "PUT", "/api/plans/:plan_id", s.planUpdateHandler},
		&Route{"delete_plan", "DELETE", "/api/plans/:plan_id", s.planDeleteHandler},
		&Route{"get_plan_files", "GET", "/api/plans/:plan_id/files", s.planFilesGetHandler},
		&Route{"upload_plan_files", "PUT", "/api/plans/:plan_id/files", s.planFilesUploadHandler},
		&Route{"delete_plan_files", "DELETE", "/api/plans/:plan_id/files", s.planFilesDeleteHandler},

		&Route{"create_collection", "POST", "/api/collections", s.collectionCreateHandler},
		&Route{"delete_collection", "DELETE", "/api/collections/:collection_id", s.collectionDeleteHandler},
		&Route{"get_collection", "GET", "/api/collections/:collection_id", s.collectionGetHandler},
		&Route{"edit_collection", "PUT", "/api/collections/:collection_id", s.collectionUpdateHandler},
		&Route{"get_collection_files", "GET", "/api/collections/:collection_id/files", s.collectionFilesGetHandler},
		&Route{"upload_collection_files", "PUT", "/api/collections/:collection_id/files", s.collectionFilesUploadHandler},
		&Route{"delete_collection_files", "DELETE", "/api/collections/:collection_id/files", s.collectionFilesDeleteHandler},
		&Route{"deploy", "POST", "/api/collections/:collection_id/deploy", s.collectionDeploymentHandler},
		&Route{"trigger", "POST", "/api/collections/:collection_id/trigger", s.collectionTriggerHandler},
		&Route{"stop", "POST", "/api/collections/:collection_id/stop", s.collectionTermHandler},
		&Route{"purge", "POST", "/api/collections/:collection_id/purge", s.collectionPurgeHandler},
		&Route{"get_runs", "GET", "/api/collections/:collection_id/runs", s.runGetHandler},
		&Route{"get_run", "GET", "/api/collections/:collection_id/runs/:run_id", s.runGetHandler},
		&Route{"delete_runs", "DELETE", "/api/collections/:collection_id/runs", s.runDeleteHandler},
		&Route{"delete_run", "DELETE", "/api/collections/:collection_id/runs/:run_id", s.runDeleteHandler},
		&Route{"get_nodes", "GET", "/api/collections/:collection_id/nodes", s.collectionNodesGetHandler},
		&Route{"shutdown_nodes", "DELETE", "/api/collections/:collection_id/nodes", s.collectionNodesShutdownHandler},
		&Route{"status", "GET", "/api/collections/:collection_id/status", s.collectionStatusHandler},
		&Route{"stream", "GET", "/api/collections/:collection_id/stream", s.streamCollectionMetrics},
		&Route{"get_plan_log", "GET", "/api/collections/:collection_id/logs/:plan_id", s.planLogHandler},
		&Route{"upload_collection_config", "PUT", "/api/collections/:collection_id/config", s.collectionUploadHandler},

		&Route{"files", "GET", "/api/files/:kind/:id/:name", s.fileDownloadHandler},

		&Route{"admin_collections", "GET", "/api/admin/collections", s.collectionAdminGetHandler},
	}
}
