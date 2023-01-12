package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	log "github.com/sirupsen/logrus"

	_ "go.uber.org/automaxprocs"
)

type ShibuyaIngressController struct {
	client          *kubernetes.Clientset
	engineInventory sync.Map
	namespace       string
	projectID       string
}

type EngineEndPoint struct {
	collectionID string
	addr         string
	path         string
}

var tr = &http.Transport{
	// Currently we have 4 engines per host. Each engine will require at least 2 connections.
	// 1 for metric subscription and 1 for trigger/healthcheck requests.
	// So minimum per host is 8. Currently, the capacity should be big enough
	// because it's designed with 10 engines per host and 10 conns per engine.
	MaxIdleConnsPerHost: 100,

	// Usually one collection will not run longer than 1 hour. If it's longer than 1 Hour,
	// We should do some GC to prevent too many connections accumulated.
	IdleConnTimeout: 1 * time.Hour,

	// We wait max 5 minutes for engines to respond. A complex plan might take some time to start.
	// But it should no longer than 5 minutes.
	ResponseHeaderTimeout: 5 * time.Minute,
}

var httpClient = &http.Client{
	Transport: tr,
}

func makeK8sClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return client, err
}

func (sic *ShibuyaIngressController) makePath(projectID, collectionID, planID string, engineID int) string {
	return fmt.Sprintf("service-%s-%s-%s-%d", projectID, collectionID, planID, engineID)
}

func (sic *ShibuyaIngressController) findCollectionIDFromPath(path string) string {
	items := strings.Split(path, "-")
	return items[2]
}

func (sic *ShibuyaIngressController) getPlanEnginesCount(projectID, collectionID, planID string) (int, error) {
	planName := fmt.Sprintf("engine-%s-%s-%s", projectID, collectionID, planID)
	resp, err := sic.client.AppsV1().Deployments(sic.namespace).Get(context.TODO(), planName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}
	return int(*resp.Spec.Replicas), nil
}

func (sic *ShibuyaIngressController) updateInventory(inventoryByCollection map[string][]EngineEndPoint) {
	log.Debugf("Going to update inventory with following states %v", inventoryByCollection)
	for _, ep := range inventoryByCollection {
		for _, ee := range ep {
			sic.engineInventory.Store(ee.path, ee.addr)
			log.Infof("Added engine %s with addr %s into inventory", ee.path, ee.addr)
		}
	}
	sic.engineInventory.Range(func(path, addr interface{}) bool {
		p := path.(string)
		collectionID := sic.findCollectionIDFromPath(p)
		if _, ok := inventoryByCollection[collectionID]; !ok {
			sic.engineInventory.Delete(path)
			log.Infof("Cleaned the inventory for engine with path %s", path)
		}
		return true
	})
}

func (sic *ShibuyaIngressController) makeInventory() {
	labelSelector := fmt.Sprintf("project=%s", sic.projectID)
	for {
		time.Sleep(3 * time.Second)
		resp, err := sic.client.CoreV1().Endpoints(sic.namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			continue
		}
		// can we have the race condition that the inventory we make could make the shibuya controller mistakenly thinks the engines are ready?
		// controller is already checking whether all the engines within one collection are in running state
		// How can ensure the atomicity?
		inventoryByCollection := make(map[string][]EngineEndPoint)
		skipedCollections := make(map[string]struct{})
		for _, planEndpoints := range resp.Items {
			// need to sort the endpoints and update the inventory
			collectionID := planEndpoints.Labels["collection"]

			// If any of the plans inside the collection is not ready, we skip the further check
			if _, ok := skipedCollections[collectionID]; ok {
				log.Debugf("Collection %s is not ready, skip.", collectionID)
				continue
			}
			projectID := planEndpoints.Labels["project"]
			planID := planEndpoints.Labels["plan"]
			kind := planEndpoints.Labels["kind"]

			if kind != "executor" {
				continue
			}
			collectionReady := true
			subsets := planEndpoints.Subsets
			if len(subsets) == 0 {
				collectionReady = false
			}
			engineEndpoints := subsets[0].Addresses
			planEngineCount, err := sic.getPlanEnginesCount(projectID, collectionID, planID)
			if err != nil {
				log.Debugf("Getting count error %v", err)
				collectionReady = false
			}
			// If the engpoints are less than the pod count, it means the pods are not ready yet, we should skip
			log.Debugf("Engine endpoints count %d", len(engineEndpoints))
			log.Debugf("Number of engines in the plan %d", planEngineCount)
			if len(engineEndpoints) < planEngineCount {
				collectionReady = false
			}
			if !collectionReady {
				skipedCollections[collectionID] = struct{}{}
				continue
			}
			ports := subsets[0].Ports
			if len(ports) == 0 {
				//TODO is this an error? Shall we handle it?
				continue
			}
			port := ports[0].Port
			addresses := []string{}
			for _, e := range engineEndpoints {
				addresses = append(addresses, fmt.Sprintf("%s:%d", e.IP, port))
			}
			// Every engine is the same. but we need to ensure the engine url always matches to the same engine
			sort.Slice(addresses, func(i, j int) bool {
				return addresses[i] < addresses[j]
			})
			for i, addr := range addresses {
				path := sic.makePath(projectID, collectionID, planID, i)
				inventoryByCollection[collectionID] = append(inventoryByCollection[collectionID], EngineEndPoint{
					path: path,
					addr: addr,
				})
			}
		}
		sic.updateInventory(inventoryByCollection)
	}
}

func (sic *ShibuyaIngressController) findPodIPFromInventory(url string) (string, error) {
	item, ok := sic.engineInventory.Load(url)
	if !ok {
		return "", fmt.Errorf("Could not find the mapping with url %s", url)
	}
	return item.(string), nil
}

func makeAccessLogEntry(statusCode int, path string) string {
	return fmt.Sprintf("%d, %s", statusCode, path)
}

func (sic *ShibuyaIngressController) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	items := strings.Split(req.RequestURI, "/")
	if len(items) < 3 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	engine := items[1]
	podIP, err := sic.findPodIPFromInventory(engine)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	action := items[2]
	engineUrl := fmt.Sprintf("http://%s/%s", podIP, action)
	req.URL, err = url.Parse(engineUrl)
	if err != nil {
		log.Debug(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	t := req.RequestURI
	req.RequestURI = ""
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Debug(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	log.Debug(makeAccessLogEntry(resp.StatusCode, t))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func initFromEnv() (namespace, projectID, logLevel string) {
	namespace = os.Getenv("POD_NAMESPACE")
	projectID = os.Getenv("project_id")
	logLevel = os.Getenv("log_level")
	return
}

func main() {
	listenAddr := ":8080"
	namespace, projectID, logLevel := initFromEnv()
	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	log.Infof("Engine namespace %s", namespace)
	log.Infof("Project ID: %s", projectID)
	client, err := makeK8sClient()
	if err != nil {
		log.Fatal(err)
	}
	sic := &ShibuyaIngressController{client: client, namespace: namespace, projectID: projectID}
	go sic.makeInventory()
	if err := http.ListenAndServe(listenAddr, sic); err != nil {
		log.Fatal(err)
	}
}
