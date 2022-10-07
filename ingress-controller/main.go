package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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

func (sic *ShibuyaIngressController) makeInventoryFromPods(pods []apiv1.Pod) {
	for i, p := range pods {
		podIP := p.Status.PodIP
		container := p.Spec.Containers[0]
		port := container.Ports[0].ContainerPort
		addr := fmt.Sprintf("%s:%d", podIP, port)
		projectID := p.Labels["project"]
		collectionID := p.Labels["collection"]
		planID := p.Labels["plan"]
		path := sic.makePath(projectID, collectionID, planID, i)
		log.Println(fmt.Sprintf("%s:%s", path, addr))
		sic.engineInventory.Store(path, addr)
	}
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
			for _, pe := range planEndpoints.Subsets {

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
