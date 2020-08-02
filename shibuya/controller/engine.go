package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/harpratap/shibuya/config"
	"github.com/harpratap/shibuya/model"
	"github.com/harpratap/shibuya/scheduler"
	"github.com/harpratap/shibuya/utils"
	es "github.com/iandyh/eventsource"
	log "github.com/sirupsen/logrus"
)

type shibuyaEngine interface {
	prepareTestData(fileName string, ep *model.ExecutionPlan, engineData map[string]*model.ShibuyaFile) (*bytes.Buffer, error)
	zipFiles(engineFolder string) (*bytes.Buffer, error)
	trigger(fileName string, ep *model.ExecutionPlan, engineData map[string]*model.ShibuyaFile) error
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
	name            string
	serviceName     string
	ingressName     string
	engineUrl       string
	ingressClass    string
	collectionID    int64
	planID          int64
	projectID       int64
	ID              int
	folder          string
	defaultPlanPath string
	stream          *es.Stream
	cancel          context.CancelFunc
	runID           int64
	*config.ExecutorContainer
}

func sendTriggerRequest(url string, zipFileBuf *bytes.Buffer, testPlan string) (*http.Response, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("test-data", "test-data.zip")
	if err != nil {
		return nil, err
	}
	io.Copy(part, zipFileBuf)
	writer.WriteField("plan", testPlan)
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
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

func (be *baseEngine) zipFiles(engineFolder string) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	defer w.Close()
	files, err := ioutil.ReadDir(engineFolder)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		filePath := filepath.Join(engineFolder, f.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		fileContents, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}
		fs, err := file.Stat()
		if err != nil {
			return nil, err
		}
		f, err := w.Create(fs.Name())
		if err != nil {
			return nil, err
		}
		f.Write(fileContents)
	}
	return buf, nil
}

func (be *baseEngine) prepareTestData(fileName string, ep *model.ExecutionPlan,
	engineData map[string]*model.ShibuyaFile) (*bytes.Buffer, error) {
	log.Println("BaseEngine does not implement prepareTestData(). Use an engine type.")
	return nil, nil
}

func (be *baseEngine) trigger(fileName string, ep *model.ExecutionPlan,
	engineData map[string]*model.ShibuyaFile) error {
	log.Println("BaseEngine does not implement trigger(). Use an engine type.")
	return nil
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
