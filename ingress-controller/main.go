package main

import (
	"context"
	"fmt"
	"io"
	"log"
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
)

type ShibuyaIngressController struct {
	client          *kubernetes.Clientset
	engineInventory sync.Map
	namespace       string
	projectID       string
}

var httpClient = &http.Client{}

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
		for _, planEndpoints := range resp.Items {
			// need to sort the endpoints and update the inventory
			projectID := planEndpoints.Labels["project"]
			collectionID := planEndpoints.Labels["collection"]
			planID := planEndpoints.Labels["plan"]
			kind := planEndpoints.Labels["kind"]
			if kind != "executor" {
				continue
			}
			subsets := planEndpoints.Subsets
			if len(subsets) == 0 {
				continue
			}
			engineEndpoints := subsets[0].Addresses
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
				sic.engineInventory.Store(path, addr)
			}
		}
		sic.engineInventory.Range(func(k, v interface{}) bool {
			fmt.Println(k, v)
			return true
		})
	}
}

func (sic *ShibuyaIngressController) findPodIPFromInventory(url string) (string, error) {
	item, ok := sic.engineInventory.Load(url)
	if !ok {
		return "", fmt.Errorf("Could not find the mapping with url %s", url)
	}
	return item.(string), nil
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
	log.Println(engineUrl)
	req.URL, err = url.Parse(engineUrl)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	t := req.RequestURI
	req.RequestURI = ""
	//client := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	log.Println(resp.StatusCode, "-l", t)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func initFromEnv() (namespace, projectID string) {
	namespace = os.Getenv("POD_NAMESPACE")
	projectID = os.Getenv("project_id")
	return
}

func main() {
	listenAddr := ":8080"
	namespace, projectID := initFromEnv()
	log.Println(fmt.Sprintf("Engine namespace %s", namespace))
	log.Println(fmt.Sprintf("Project ID: %s", projectID))
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
