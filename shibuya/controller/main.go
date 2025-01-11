package controller

import (
	"context"
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
	sc                 config.ShibuyaConfig
}

func NewController(sc config.ShibuyaConfig) *Controller {
	c := &Controller{
		filePath: "/test-data",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ApiClosingClients: make(chan *ApiMetricStream),
		ApiNewClients:     make(chan *ApiMetricStream),
	}
	c.schedulerKind = sc.ExecutorConfig.Cluster.Kind
	c.Scheduler = scheduler.NewEngineScheduler(sc.ExecutorConfig)
	return c
}

type subscribeState struct {
	cancelfunc     context.CancelFunc
	ctx            context.Context
	readingEngines []shibuyaEngine
	readyToClose   chan struct{}
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
	go c.streamToApi()
	if !config.SC.DistributedMode {
		log.Info("Controller is running in non-distributed mode!")
		go c.IsolateBackgroundTasks()
	}
}

// In distributed mode, the func will be running as a standalone process
// In non-distributed mode, the func will be run as a goroutine.
func (c *Controller) IsolateBackgroundTasks() {
	go c.AutoPurgeDeployments()
	go c.CheckRunningThenTerminate()
	c.AutoPurgeProjectIngressController()
}

func (c *Controller) handleStreamForClient(item *ApiMetricStream) error {
	log.Printf("New Incoming connection :%s", item.ClientID)
	cid, err := strconv.ParseInt(item.CollectionID, 10, 64)
	if err != nil {
		return err
	}
	collection, err := model.GetCollection(cid)
	if err != nil {
		return err
	}
	readingEngines, err := c.SubscribeCollection(collection)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	ss := subscribeState{
		cancelfunc:     cancel,
		ctx:            ctx,
		readingEngines: readingEngines,
		readyToClose:   make(chan struct{}),
	}
	c.readingEngineRecords.Store(item.ClientID, ss)
	go func(readingEngines []shibuyaEngine) {
		var wg sync.WaitGroup
		for _, engine := range readingEngines {
			wg.Add(1)
			go func(e shibuyaEngine) {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case metric := <-e.readMetrics():
						if metric != nil {
							item.StreamClient <- &ApiMetricStreamEvent{
								CollectionID: metric.collectionID,
								PlanID:       metric.planID,
								Raw:          metric.raw,
							}
						}
					}
				}
			}(engine)
		}
		wg.Wait()
		ss.readyToClose <- struct{}{}
	}(readingEngines)
	return nil
}

func (c *Controller) streamToApi() {
	workerQueue := make(chan *ApiMetricStream)
	for i := 0; i < c.clientStreamingWorkers; i++ {
		go func() {
			for item := range workerQueue {
				if err := c.handleStreamForClient(item); err != nil {
					log.Error(err)
				}
			}
		}()
	}
	for {
		select {
		case item := <-c.ApiNewClients:
			workerQueue <- item
		case item := <-c.ApiClosingClients:
			clientID := item.ClientID
			collectionID := item.CollectionID
			if t, ok := c.readingEngineRecords.Load(clientID); ok {
				ss := t.(subscribeState)
				for _, e := range ss.readingEngines {
					go func(e shibuyaEngine) {
						e.closeStream()
					}(e)
				}
				ss.cancelfunc()
				<-ss.readyToClose
				close(item.StreamClient)
				c.readingEngineRecords.Delete(clientID)
				log.Printf("Client %s disconnect from the API for collection %s.", clientID, collectionID)
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
	sid := ""
	if project, err := model.GetProject(collection.ProjectID); err == nil {
		sid = project.SID
	}
	if err := collection.NewLaunchEntry(sid, c.sc.Context, int64(enginesCount), nodesCount, int64(vu)); err != nil {
		return err
	}
	err = utils.Retry(func() error {
		return c.Scheduler.ExposeProject(collection.ProjectID)
	}, nil)
	if err != nil {
		return err
	}
	if err = c.Scheduler.CreateCollectionScraper(collection.ID); err != nil {
		log.Error(err)
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
	if config.SC.DevMode {
		cs.PoolSize = 100
		cs.PoolStatus = "running"
	}
	return cs, nil
}
