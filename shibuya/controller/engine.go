package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/scheduler"
	"github.com/rakutentech/shibuya/shibuya/utils"

	es "github.com/iandyh/eventsource"
	log "github.com/sirupsen/logrus"
)

type shibuyaEngine interface {
	trigger(edc *EngineDataConfig) error
	deploy(*scheduler.K8sClientManager) error
	subscribe(runID int64) error
	progress() bool
	readMetrics() chan *shibuyaMetric
	reachable(*scheduler.K8sClientManager) bool
	closeStream()
	terminate(force bool) error
	EngineID() int
	updateEngineUrl(url string)
}

type engineType struct{}

var JmeterEngineType engineType

// HttPClient shared by the engines to contact with the container
// deployed in the k8s cluster
var engineHttpClient = &http.Client{
	Timeout: 30 * time.Second,
}

type EngineDataConfig struct {
	EngineData  map[string]*model.ShibuyaFile `json:"engine_data"`
	Duration    string                        `json:"duration"`
	Concurrency string                        `json:"concurrency"`
	Rampup      string                        `json:"rampup"`
}

func (edc *EngineDataConfig) deepCopy() *EngineDataConfig {
	edcCopy := EngineDataConfig{
		EngineData:  map[string]*model.ShibuyaFile{},
		Duration:    edc.Duration,
		Concurrency: edc.Concurrency,
		Rampup:      edc.Rampup,
	}
	for filename, ed := range edc.EngineData {
		sf := model.ShibuyaFile{
			Filename:     ed.Filename,
			Filepath:     ed.Filepath,
			Filelink:     ed.Filelink,
			TotalSplits:  ed.TotalSplits,
			CurrentSplit: ed.CurrentSplit,
		}
		edcCopy.EngineData[filename] = &sf
	}
	return &edcCopy
}

func (edc *EngineDataConfig) deepCopies(size int) []*EngineDataConfig {
	edcCopies := []*EngineDataConfig{}
	for i := 0; i < size; i++ {
		edcCopies = append(edcCopies, edc.deepCopy())
	}
	return edcCopies
}

type shibuyaMetric struct {
	threads      float64
	latency      float64
	label        string
	status       string
	raw          string
	collectionID string
	planID       string
	engineID     string
	runID        string
}

const enginePlanRoot = "/test-data"

type baseEngine struct {
	name         string
	serviceName  string
	ingressName  string
	engineUrl    string
	ingressClass string
	collectionID int64
	planID       int64
	projectID    int64
	ID           int
	stream       *es.Stream
	cancel       context.CancelFunc
	runID        int64
	*config.ExecutorContainer
}

func sendTriggerRequest(url string, edc *EngineDataConfig) (*http.Response, error) {
	body := new(bytes.Buffer)
	json.NewEncoder(body).Encode(&edc)
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", "application/json")
	return engineHttpClient.Do(req)
}

func (be *baseEngine) EngineID() int {
	return be.ID
}

func (be *baseEngine) subscribe(runID int64) error {
	streamUrl := fmt.Sprintf("http://%s/%s", be.engineUrl, "stream")
	req, err := http.NewRequest("GET", streamUrl, nil)
	if err != nil {
		return err
	}
	log.Printf("Subscribing to engine url %s", streamUrl)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	httpClient := &http.Client{}
	stream, err := es.SubscribeWith("", httpClient, req)
	if err != nil {
		cancel()
		return err
	}
	be.stream = stream
	be.cancel = cancel
	be.runID = runID
	return nil
}

func (be *baseEngine) progress() bool {
	progressEndpoint := fmt.Sprintf("http://%s/%s", be.engineUrl, "progress")
	var resp *http.Response
	var httpError error
	err := utils.Retry(func() error {
		resp, httpError = engineHttpClient.Get(progressEndpoint)
		return httpError
	})
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (be *baseEngine) reachable(manager *scheduler.K8sClientManager) bool {
	return manager.ServiceReachable(be.ingressClass, be.serviceName)
}

func (be *baseEngine) closeStream() {
	be.cancel()
	be.stream.Close()
}

func (be *baseEngine) terminate(force bool) error {
	// If it's force, it means we are purging the collection
	// In this case, we don't send the stop request to test containers
	if force {
		return nil
	}
	stopUrl := fmt.Sprintf("http://%s/stop", be.engineUrl)
	resp, err := engineHttpClient.Post(stopUrl, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	be.closeStream()
	return nil
}

func (be *baseEngine) deploy(manager *scheduler.K8sClientManager) error {
	return manager.DeployEngine(be.name, be.serviceName, be.ingressClass, be.ingressName, be.planID,
		be.collectionID, be.projectID, be.ExecutorContainer)
}

func (be *baseEngine) trigger(edc *EngineDataConfig) error {
	engineUrl := be.engineUrl
	url := fmt.Sprintf("http://%s/%s", engineUrl, "start")
	return utils.Retry(func() error {
		resp, err := sendTriggerRequest(url, edc)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			log.Printf("%s is already triggered", engineUrl)
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Engine failed to trigger: %d %s", resp.StatusCode, resp.Status)
		}
		log.Printf("%s is triggered", engineUrl)
		return nil
	})
}

func (be *baseEngine) readMetrics() chan *shibuyaMetric {
	log.Println("BaseEngine does not readMetrics(). Use an engine type.")
	return nil
}

func (be *baseEngine) updateEngineUrl(collectionUrl string) {
	be.engineUrl = fmt.Sprintf("%s/%s", collectionUrl, be.serviceName)
}

func generateEngines(enginesRequired int, planID, collectionID, projectID int64, et engineType) (engines []shibuyaEngine, err error) {
	ingressClass := createIgName(collectionID)
	for i := 0; i < enginesRequired; i++ {
		serviceName := scheduler.GenerateName("service", planID, collectionID, projectID, i)
		engineC := &baseEngine{
			name:         scheduler.GenerateName("engine", planID, collectionID, projectID, i),
			serviceName:  serviceName,
			ingressName:  scheduler.GenerateName("ingress", planID, collectionID, projectID, i),
			ingressClass: ingressClass,
			ID:           i,
			projectID:    projectID,
			collectionID: collectionID,
			planID:       planID,
		}
		var e shibuyaEngine
		switch et {
		case JmeterEngineType:
			e = NewJmeterEngine(engineC)
		default:
			return nil, makeWrongEngineTypeError()
		}
		engines = append(engines, e)
	}
	return engines, nil
}

func generateEnginesWithUrl(enginesRequired int, planID, collectionID, projectID int64, et engineType, kcm *scheduler.K8sClientManager) (engines []shibuyaEngine, err error) {
	engines, err = generateEngines(enginesRequired, planID, collectionID, projectID, et)
	if err != nil {
		return nil, err
	}
	collectionUrl, err := kcm.GetIngressUrl(createIgName(collectionID))
	if err != nil {
		return engines, err
	}
	for _, e := range engines {
		e.updateEngineUrl(collectionUrl)
	}
	return engines, nil
}

func (ctr *Controller) fetchEngineMetrics() {
	for {
		time.Sleep(5 * time.Second)
		deployedCollections, err := ctr.Kcm.GetDeployedCollections()
		if err != nil {
			continue
		}
		for collectionID, _ := range deployedCollections {
			c, err := model.GetCollection(collectionID)
			if err != nil {
				continue
			}
			eps, err := c.GetExecutionPlans()
			if err != nil {
				continue
			}
			collectionID_str := strconv.FormatInt(collectionID, 10)
			for _, ep := range eps {
				podsMetrics, err := ctr.Kcm.GetPodsMetrics(collectionID, ep.PlanID)
				if err != nil {
					continue
				}
				planID_str := strconv.FormatInt(ep.PlanID, 10)
				for engineNumber, metrics := range podsMetrics {
					for resourceName, m := range metrics {
						if resourceName == "cpu" {
							config.CpuGauge.WithLabelValues(collectionID_str, planID_str, engineNumber).Set(float64(m.MilliValue()))
						} else {
							config.MemGauge.WithLabelValues(collectionID_str, planID_str, engineNumber).Set(float64(m.Value()))
						}
					}
				}
			}
		}
	}
}
