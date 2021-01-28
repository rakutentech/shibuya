package controller

import (
	"fmt"
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
	LabelStore                     sync.Map
	StatusStore                    sync.Map
	ApiNewClients                  chan *ApiMetricStream
	ApiStreamClients               map[string]map[string]chan *ApiMetricStreamEvent
	ApiMetricStreamBus             chan *ApiMetricStreamEvent
	ApiClosingClients              chan *ApiMetricStream
	readingEngines                 chan shibuyaEngine
	connectedEngines               sync.Map
	filePath                       string
	httpClient                     *http.Client
	schedulerKind                  string
	Scheduler                      scheduler.EngineScheduler
	collectionStatusCache          sync.Map
	collectionStatusProcessingList sync.Map
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

	// First we do is to resume the running plans
	// This method should not be moved as later goroutines rely on it.
	c.resumeRunningPlans()
	go c.streamToApi()
	go c.readConnectedEngines()
	go c.checkRunningThenTerminate()
	go c.fetchEngineMetrics()
	go c.cleanLocalStore()
	go c.autoPurgeDeployments()
	go c.fetchCollectionStatus()
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

func (c *Controller) fetchCollectionStatus() {
	for {
		c.collectionStatusProcessingList.Range(func(key, value interface{}) bool {
			collectionID := key.(int64)
			keyCreatedTime := value.(time.Time)
			expiredTime := keyCreatedTime.Add(20 * time.Minute)
			if expiredTime.Before(time.Now()) {
				c.collectionStatusProcessingList.Delete(collectionID)
				c.collectionStatusCache.Delete(collectionID)
			} else {
				collection, err := model.GetCollection(collectionID)
				if err == nil {
					cs, err := c.CollectionStatus(collection)
					if err == nil {
						c.collectionStatusCache.Store(collectionID, cs)
					}
				}
			}
			return true
		})
		time.Sleep(3 * time.Second)
	}
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
	engineDataConfigs := prepareCollection(collection)
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

			pc := NewPlanController(ep, collection, c.Scheduler)
			if err := pc.trigger(engineDataConfigs[i]); err != nil {
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
	if len(triggerErrors) == len(collection.ExecutionPlans) {
		// every plan in collection has error
		c.TermCollection(collection, true)
	}
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
			pc := NewPlanController(ep, collection, nil) // we don't need scheduler here
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
	collection.MarkUsageFinished()
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
	nodesCount := int64(0)
	enginesCount := 0
	for _, e := range eps {
		enginesCount += e.Engines
	}
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		nodesCount = c.calNodesRequired(enginesCount)
		operator := NewGCPOperator(collection.ID, nodesCount)
		err := operator.prepareNodes()
		if err != nil {
			return err
		}
	}
	owner := ""
	if project, err := model.GetProject(collection.ProjectID); err == nil {
		owner = project.Owner
	}
	collection.NewLaunchEntry(owner, config.SC.Context, int64(enginesCount), nodesCount)

	err = utils.Retry(func() error {
		return c.Scheduler.ExposeCollection(collection.ProjectID, collection.ID)
	}, nil)
	if err != nil {
		return err
	}
	// we will assume collection deployment will always be successful
	var wg sync.WaitGroup
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

func (c *Controller) CollectionStatusWrapper(collection *model.Collection) *smodel.CollectionStatus {
	c.collectionStatusProcessingList.Store(collection.ID, time.Now())
	raw, ok := c.collectionStatusCache.Load(collection.ID)
	if !ok {
		return nil
	}
	return raw.(*smodel.CollectionStatus)
}

func (c *Controller) PurgeNodes(collection *model.Collection) error {
	if config.SC.ExecutorConfig.Cluster.OnDemand {
		operator := NewGCPOperator(collection.ID, int64(0))
		if err := operator.destroyNodes(); err != nil {
			return err
		}
		collection.MarkUsageFinished()
		return nil
	}
	return nil
}
