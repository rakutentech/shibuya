package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type ShibuyaIngressController struct {
	engineInventory sync.Map
}

var httpClient = &http.Client{}

func (sic *ShibuyaIngressController) makeInventory() {
	podIP := os.Getenv("pod_ip")
	sic.engineInventory.Store("service-1-1-1-0", podIP)
}

func (sic *ShibuyaIngressController) findPodIPFromInventory(url string) (string, error) {
	item, ok := sic.engineInventory.Load(url)
	if !ok {
		return "", errors.New("Could not find the mapping")
	}
	return item.(string), nil
}

func (sic *ShibuyaIngressController) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	items := strings.Split(req.RequestURI, "/")
	if len(items) < 3 {
		log.Println("items", "bad")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	engine := items[1]
	podIP, err := sic.findPodIPFromInventory(engine)
	if err != nil {
		log.Println("not found")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	action := items[2]
	engineUrl := fmt.Sprintf("http://%s:8080/%s", podIP, action)
	req.URL, err = url.Parse(engineUrl)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println("---")
	log.Println(req.URL.Path)
	log.Println(req.URL.Host)
	log.Println(req.RequestURI)
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

	io.Copy(w, resp.Body)
}

func main() {
	listenAddr := ":8080"
	sic := &ShibuyaIngressController{}
	sic.makeInventory()
	if err := http.ListenAndServe(listenAddr, sic); err != nil {
		log.Fatal(err)
	}
}
