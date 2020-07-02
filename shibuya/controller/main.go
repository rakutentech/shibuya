package controller

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"shibuya/config"
	"shibuya/model"
	"shibuya/scheduler"
	"shibuya/utils"

	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
)

type Controller struct {
	LabelStore         sync.Map
	StatusStore        sync.Map
	ApiNewClients      chan *ApiMetricStream
	ApiStreamClients   map[string]map[string]chan *ApiMetricStreamEvent
	ApiMetricStreamBus chan *ApiMetricStreamEvent
	ApiClosingClients  chan *ApiMetricStream
	readingEngines     chan shibuyaEngine
	connectedEngines   sync.Map
	filePath           string
	httpClient         *http.Client
	Kcm                *scheduler.K8sClientManager
}

func NewController() *Controller {
	c := &Controller{
		filePath: "/test-data",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ApiMetricStreamBus: make(chan *ApiMetricStreamEvent),
		ApiClosingClients:  make(chan *ApiMetricStream),
		ApiNewClients:      make(chan *ApiMetricStream),
		ApiStreamClients:   make(map[string]map[string]chan *ApiMetricStreamEvent),
		Kcm:                scheduler.NewK8sClientManager(),
		readingEngines:     make(chan shibuyaEngine),
	}
	// First we do is to resume the running plans
	// This method should not be moved as later goroutines rely on it.
	c.resumeRunningPlans()
	go c.streamToApi()
	go c.readConnectedEngines()
	go c.checkRunningThenTerminate()
	go c.fetchEngineMetrics()
	go c.cleanLocalStore()
	go c.autoPurgeDeployments()
	return c
}

type ApiMetricStream struct {
	CollectionID string
	StreamClient chan *ApiMetricStreamEvent
	ClientID     string
}

type ApiMetricStreamEvent struct {
	CollectionID string `json:"collection_id"`
	Raw          string `json:"metrics"`
	PlanID       string `json:"plan_id"`
}

func (c *Controller) streamToApi() {
	for {
		select {
		case item := <-c.ApiNewClients:
			collectionID := item.CollectionID
			clientID := item.ClientID
			if m, ok := c.ApiStreamClients[collectionID]; !ok {
				m = make(map[string]chan *ApiMetricStreamEvent)
				m[clientID] = item.StreamClient
				c.ApiStreamClients[collectionID] = m
			} else {
				m[clientID] = item.StreamClient
			}
			log.Printf("A client %s connects to collection %s, start streaming", clientID, collectionID)
		case item := <-c.ApiClosingClients:
			collectionID := item.CollectionID
			clientID := item.ClientID
			m := c.ApiStreamClients[collectionID]
			close(item.StreamClient)
			delete(m, clientID)
			log.Printf("Client %s disconnect from the API for collection %s.", clientID, collectionID)
		case event := <-c.ApiMetricStreamBus:
			streamClients, ok := c.ApiStreamClients[event.CollectionID]
			if !ok {
				continue
			}
			for _, streamClient := range streamClients {
				streamClient <- event
			}
		}
	}
}

// This is used for tracking all the running plans
// So even when Shibuya controller restarts, the tests can resume
type RunningPlan struct {
	ep         *model.ExecutionPlan
	collection *model.Collection
}

func (c *Controller) resumeRunningPlans() {
	runningPlans, err := model.GetRunningPlans()
	if err != nil {
		log.Print(err)
		return
	}
	localCache := make(map[int64]*model.Collection)
	for _, rp := range runningPlans {
		var collection *model.Collection
		var ok bool
		collection, ok = localCache[rp.CollectionID]
		if !ok {
			collection, err = model.GetCollection(rp.CollectionID)
			if err != nil {
				continue
			}
			localCache[rp.CollectionID] = collection
		}
		ep, err := model.GetExecutionPlan(collection.ID, rp.PlanID)
		if err != nil {
			continue
		}
		pc := NewPlanController(ep, collection, c.Kcm)
		pc.subscribe(&c.connectedEngines, c.readingEngines)
	}
}

func (c *Controller) readConnectedEngines() {
	for engine := range c.readingEngines {
		go func(engine shibuyaEngine) {
			ch := engine.readMetrics()
			for metric := range ch {
				collectionID := metric.collectionID
				planID := metric.planID
				runID := metric.runID
				engineID := metric.engineID
				label := metric.label
				status := metric.status
				latency := metric.latency
				threads := metric.threads
				c.ApiMetricStreamBus <- &ApiMetricStreamEvent{
					CollectionID: metric.collectionID,
					PlanID:       metric.planID,
					Raw:          metric.raw,
				}
				config.StatusCounter.WithLabelValues(metric.collectionID, metric.planID, runID, engineID, label, status).Inc()
				config.CollectionLatencySummary.WithLabelValues(collectionID, runID).Observe(latency)
				config.PlanLatencySummary.WithLabelValues(collectionID, planID, runID).Observe(latency)
				config.LabelLatencySummary.WithLabelValues(collectionID, label, runID).Observe(latency)
				config.ThreadsGauge.WithLabelValues(collectionID, planID, runID, engineID).Set(threads)

				rid, _ := strconv.ParseInt(runID, 10, 64)
				go c.storeLocally(rid, label, status)
			}
		}(engine)
	}
}

func (c *Controller) TriggerCollection(collection *model.Collection) error {
	var err error
	// Get all the execution plans within the collection
	// Execution plans are the buiding block of a collection.
	// They define the concurrent/duration etc
	// All the pre-fetched resources will go alone with the collection object
	collection.ExecutionPlans, err = collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	ed, err := prepareCollection(collection)
	if err != nil {
		return err
	}
	defer utils.DeleteFolder(strconv.FormatInt(collection.ID, 10))
	for _, ep := range collection.ExecutionPlans {
		plan, err := model.GetPlan(ep.PlanID)
		if err != nil {
			return err
		}
		if plan.TestFile == nil {
			return fmt.Errorf("Triggering plan aborted. There is no Test file (.jmx) in this plan %d", plan.ID)
		}
	}
	runID, err := collection.StartRun()
	if err != nil {
		return err
	}
	errs := make(chan error, len(collection.ExecutionPlans))
	defer close(errs)
	for i, ep := range collection.ExecutionPlans {
		go func(i int, ep *model.ExecutionPlan) {
			// We wait for all the engines. Because we can only all the plan into running status
			// When all the engines are triggered
			pc := NewPlanController(ep, collection, c.Kcm)
			if err := pc.trigger(ed.files[i]); err != nil {
				errs <- err
				return
			}
			// We don't wait all the engines. Because stream establishment can take some time
			// We don't want the UI to be freeze for long time
			if err := pc.subscribe(&c.connectedEngines, c.readingEngines); err != nil {
				errs <- err
				return
			}
			if err := model.AddRunningPlan(collection.ID, ep.PlanID); err != nil {
				errs <- err
				return
			}
			errs <- nil
		}(i, ep)
	}
	triggerErrors := []error{}
	for i := 0; i < len(collection.ExecutionPlans); i++ {
		if err := <-errs; err != nil {
			triggerErrors = append(triggerErrors, err)
		}
	}
	collection.NewRun(runID)
	if len(triggerErrors) > 0 {
		return fmt.Errorf("Triggering errors %v", triggerErrors)
	}
	return nil
}

func (c *Controller) TermCollection(collection *model.Collection, force bool) (e error) {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	currRunID, err := collection.GetCurrentRun()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, ep := range eps {
		wg.Add(1)
		go func(ep *model.ExecutionPlan) {
			defer wg.Done()
			pc := NewPlanController(ep, collection, nil) // we don't need kcm here
			if err := pc.term(force, &c.connectedEngines); err != nil {
				log.Error(err)
				e = err
			}
			log.Printf("Plan %d is terminated.", ep.PlanID)
		}(ep)
	}
	wg.Wait()
	collection.StopRun()
	collection.RunFinish(currRunID)
	return e
}

func (c *Controller) calNodesRequired(enginesNum int) int64 {
	masterCPU, _ := strconv.ParseFloat(config.SC.ExecutorConfig.JmeterContainer.CPU, 64)
	enginePerNode := math.Floor(float64(config.SC.ExecutorConfig.Cluster.NodeCPUSpec) / masterCPU)
	nodesRequired := math.Ceil(float64(enginesNum) / enginePerNode)
	return int64(nodesRequired)
}

func (c *Controller) DeployCollection(collection *model.Collection) error {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		enginesNum := 0
		for _, e := range eps {
			enginesNum += e.Engines
		}
		operator := NewGCPOperator(collection.ID, c.calNodesRequired(enginesNum))
		err := operator.prepareNodes()
		if err != nil {
			return err
		}
	}
	err = utils.Retry(func() error {
		return c.DeployIngressController(collection.ID, collection.ProjectID, collection.Name)
	})
	if err != nil {
		return err
	}

	// we will assume collection deployment will always be successful
	var wg sync.WaitGroup
	for _, e := range eps {
		wg.Add(1)
		go func(ep *model.ExecutionPlan) {
			defer wg.Done()
			pc := NewPlanController(ep, collection, c.Kcm)
			utils.Retry(func() error {
				return pc.deploy()
			})
		}(e)
	}
	wg.Wait()
	return nil
}

type planStatus struct {
	PlanID           int64     `json:"plan_id"`
	EnginesReachable bool      `json:"engines_reachable"`
	Engines          int       `json:"engines"`
	EnginesDeployed  int       `json:"engines_deployed"`
	InProgress       bool      `json:"in_progress"`
	StartedTime      time.Time `json:"started_time"`
}

type collectionStatus struct {
	Plans      []*planStatus `json:"status"`
	PoolSize   int           `json:"pool_size"`
	PoolStatus string        `json:"pool_status"`
}

func (c *Controller) PlanStatus(collectionID int64, jobs <-chan *planStatus, result chan<- *planStatus) {
	for ps := range jobs {
		if ps.Engines != ps.EnginesDeployed {
			result <- ps
			continue
		}
		rp, err := model.GetRunningPlan(collectionID, ps.PlanID)
		if err == nil {
			ps.StartedTime = rp.StartedTime
			ps.InProgress = true
		}
		result <- ps
	}
}

func (c *Controller) CheckRunningPodsByCollection(collection *model.Collection, eps []*model.ExecutionPlan) *collectionStatus {
	planStatuses := make(map[int64]*planStatus)
	var engineReachable bool
	cs := &collectionStatus{}
	pods := c.Kcm.GetPodsByCollection(collection.ID, "")
	ingressControllerDeployed := false
	for _, ep := range eps {
		ps := &planStatus{
			PlanID:  ep.PlanID,
			Engines: ep.Engines,
		}
		planStatuses[ep.PlanID] = ps
	}
	enginesReady := true
	for _, pod := range pods {
		if pod.Labels["kind"] == "ingress-controller" {
			ingressControllerDeployed = true
			continue
		}
		planID, err := strconv.Atoi(pod.Labels["plan"])
		if err != nil {
			log.Error(err)
		}
		ps, ok := planStatuses[int64(planID)]
		if !ok {
			log.Error("Could not find running pod in ExecutionPlan")
			continue
		}
		ps.EnginesDeployed += 1
		if pod.Status.Phase != apiv1.PodRunning {
			enginesReady = false
		}
	}
	// if it's unrechable, we can assume it's not in progress as well
	fieldSelector := fmt.Sprintf("status.phase=Running")
	ingressPods := c.Kcm.GetPodsByCollection(collection.ID, fieldSelector)
	ingressControllerDeployed = len(ingressPods) >= 1
	if !ingressControllerDeployed || !enginesReady {
		for _, ps := range planStatuses {
			cs.Plans = append(cs.Plans, ps)
		}
		return cs
	}
	randomPlan := eps[0]
	engines, err := generateEnginesWithUrl(randomPlan.Engines, randomPlan.PlanID, collection.ID, collection.ProjectID, JmeterEngineType, c.Kcm)
	if err != nil {
		log.Error(err)
		return cs
	}
	engineReachable = engines[0].reachable(c.Kcm)
	jobs := make(chan *planStatus)
	result := make(chan *planStatus)
	for w := 0; w < len(eps); w++ {
		go c.PlanStatus(collection.ID, jobs, result)
	}
	for _, ps := range planStatuses {
		jobs <- ps
	}
	defer close(jobs)
	defer close(result)
	for range eps {
		ps := <-result
		if ps.Engines == ps.EnginesDeployed && engineReachable {
			ps.EnginesReachable = true
		}
		cs.Plans = append(cs.Plans, ps)
	}
	return cs
}

func (c *Controller) CollectionStatus(collection *model.Collection) (*collectionStatus, error) {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return nil, err
	}
	cs := c.CheckRunningPodsByCollection(collection, eps)
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		operator := NewGCPOperator(collection.ID, 0)
		info := operator.GCPNodesInfo()
		cs.PoolStatus = "LAUNCHED"
		if info != nil {
			cs.PoolSize = info.Size
			cs.PoolStatus = info.Status
		}
	}
	if config.SC.DevMode {
		cs.PoolSize = 100
		cs.PoolStatus = "running"
	}
	return cs, nil
}

func (c *Controller) PurgeNodes(collection *model.Collection) error {
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		operator := NewGCPOperator(collection.ID, int64(0))
		if err := operator.destroyNodes(); err != nil {
			return err
		}
		return nil
	}
	return nil
}
