package controller

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/scheduler"
	smodel "github.com/rakutentech/shibuya/shibuya/scheduler/model"
	"github.com/rakutentech/shibuya/shibuya/utils"
	log "github.com/sirupsen/logrus"
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
	schedulerKind      string
	Scheduler          scheduler.EngineScheduler
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
		readingEngines:     make(chan shibuyaEngine),
	}
	c.schedulerKind = config.SC.ExecutorConfig.Cluster.Kind
	c.Scheduler = scheduler.NewEngineScheduler(config.SC.ExecutorConfig.Cluster)
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

func (c *Controller) StartRunning() {
	// First we do is to resume the running plans
	// This method should not be moved as later goroutines rely on it.
	c.resumeRunningPlans()
	go c.streamToApi()
	go c.readConnectedEngines()
	go c.fetchEngineMetrics()
	go c.cleanLocalStore()
	// We can only move this func to an isolated controller process later
	// because when we are terminating, we also need to close the opening connections
	// Otherwise we might face connection leaks
	go c.CheckRunningThenTerminate()
	if !config.SC.DistributedMode {
		log.Info("Controller is running in non-distributed mode!")
		go c.IsolateBackgroundTasks()
	}
}

// In distributed mode, the func will be running as a standalone process
// In non-distributed mode, the func will be run as a goroutine.
func (c *Controller) IsolateBackgroundTasks() {
	go c.AutoPurgeDeployments()
	c.AutoPurgeProjectIngressController()
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
		pc := NewPlanController(ep, collection, c.Scheduler)
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
	nodesCount := int64(0)
	enginesCount := 0
	vu := 0
	for _, e := range eps {
		enginesCount += e.Engines
		vu += e.Engines * e.Concurrency
	}
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		nodesCount = c.calNodesRequired(enginesCount)
		operator := NewGCPOperator(collection.ID, nodesCount)
		err := operator.prepareNodes()
		if err != nil {
			return err
		}
	}
	sid := ""
	if project, err := model.GetProject(collection.ProjectID); err == nil {
		sid = project.SID
	}
	if err := collection.NewLaunchEntry(sid, config.SC.Context, int64(enginesCount), nodesCount, int64(vu)); err != nil {
		return err
	}
	err = utils.Retry(func() error {
		return c.Scheduler.ExposeProject(collection.ProjectID)
	}, nil)
	if err != nil {
		return err
	}
	// we will assume collection deployment will always be successful
	// For some large deployments, it might take more than 1 min to finish, which could result 504 at gateway side
	// So we do not wait for the deployment to be finished.
	go func() {
		var wg sync.WaitGroup
		now_ := time.Now()
		for _, e := range eps {
			wg.Add(1)
			go func(ep *model.ExecutionPlan) {
				defer wg.Done()
				pc := NewPlanController(ep, collection, c.Scheduler)
				utils.Retry(func() error {
					return pc.deploy()
				}, nil)
			}(e)
		}
		wg.Wait()
		duration := time.Now().Sub(now_)
		log.Infof("All engines deployment are finished for collection %d, total duration: %.2f seconds",
			collection.ID, duration.Seconds())
	}()
	return nil
}

func (c *Controller) CollectionStatus(collection *model.Collection) (*smodel.CollectionStatus, error) {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return nil, err
	}
	cs, err := c.Scheduler.CollectionStatus(collection.ProjectID, collection.ID, eps)
	if err != nil {
		return nil, err
	}
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
		// we don't bill for on-demand cluster as for now.
		//collection.MarkUsageFinished()
		return nil
	}
	return nil
}
