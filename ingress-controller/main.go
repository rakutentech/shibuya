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

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ShibuyaIngressController struct {
	client          *kubernetes.Clientset
	engineInventory sync.Map
	namespace       string
	collectionID    string
	planIDs         []string
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

func (sic *ShibuyaIngressController) makeInventoryFromPlan(planID string, pods []apiv1.Pod) {
	for i, p := range pods {
		podIP := p.Status.PodIP
		container := p.Spec.Containers[0]
		port := container.Ports[0].ContainerPort
		addr := fmt.Sprintf("%s:%d", podIP, port)
		projectID := p.Labels["project"]
		path := sic.makePath(projectID, sic.collectionID, planID, i)
		log.Println(fmt.Sprintf("%s:%s", path, addr))
		sic.engineInventory.Store(path, addr)
	}
}

func (sic *ShibuyaIngressController) makeInventory() {
	labelSelector := fmt.Sprintf("collection=%s", sic.collectionID)
	for {
		time.Sleep(3 * time.Second)

		resp, err := sic.client.CoreV1().Pods(sic.namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			continue
		}
		// we need to ensure the engines order is deterministic
		// Because we use engine id to find the pod ip.
		sort.Slice(resp.Items, func(i, j int) bool {
			p1 := resp.Items[i]
			p2 := resp.Items[j]
			return p1.Name > p2.Name
		})
		enginesByPlan := make(map[string][]apiv1.Pod)
		allEnginesReady := true
		for _, p := range resp.Items {
			if p.Status.Phase != apiv1.PodRunning {
				allEnginesReady = false
				break
			}
			labels := p.Labels
			planID := labels["plan"]
			if labels["kind"] != "executor" {
				continue
			}
			engines, ok := enginesByPlan[planID]
			if !ok {
				engines = []apiv1.Pod{}
			}
			engines = append(engines, p)
			enginesByPlan[planID] = engines
		}
		log.Println("All engines are ready, going to make the endpoints")
		if allEnginesReady {
			for planID, pods := range enginesByPlan {
				sic.makeInventoryFromPlan(planID, pods)
			}
		}
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

func initFromEnv() (namespace, collectionID string, planIDs []string) {
	namespace = os.Getenv("POD_NAMESPACE")
	collectionID = os.Getenv("collection_id")
	planIDs = strings.Split(os.Getenv("plan_ids"), ",")
	return
}

func main() {
	listenAddr := ":8080"
	namespace, collectionID, planIDs := initFromEnv()
	log.Println(fmt.Sprintf("Engine namespace %s", namespace))
	log.Println(fmt.Sprintf("Collection ID: %s", collectionID))
	client, err := makeK8sClient()
	if err != nil {
		log.Fatal(err)
	}
	sic := &ShibuyaIngressController{client: client, namespace: namespace, collectionID: collectionID, planIDs: planIDs}
	go sic.makeInventory()
	if err := http.ListenAndServe(listenAddr, sic); err != nil {
		log.Fatal(err)
	}
}
