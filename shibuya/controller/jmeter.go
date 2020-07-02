package controller

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"shibuya/config"
	"shibuya/model"
	"shibuya/utils"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type jmeterEngine struct {
	*baseEngine
}

func NewJmeterEngine(be *baseEngine) *jmeterEngine {
	be.defaultPlanPath = filepath.Join(enginePlanRoot, "modified.jmx")
	be.ExecutorContainer = config.SC.ExecutorConfig.JmeterContainer.ExecutorContainer
	e := &jmeterEngine{be}
	return e
}

func modifyJMX(engineFolder, jmxFile string, ep *model.ExecutionPlan) error {
	planDoc, err := ParseTestPlan(jmxFile)
	if err != nil {
		return err
	}
	// it includes threadgroups and setupthreadgroups
	threadGroups, err := GetThreadGroups(planDoc)
	if err != nil {
		return err
	}
	for _, tg := range threadGroups {
		children := tg.ChildElements()
		for _, child := range children {
			attrName := child.SelectAttrValue("name", "")
			switch attrName {
			case "ThreadGroup.duration":
				child.SetText(strconv.Itoa(int(ep.Duration) * 60))
			case "ThreadGroup.scheduler":
				child.SetText("true")
			case "ThreadGroup.num_threads":
				child.SetText(strconv.Itoa(ep.Concurrency))
			case "ThreadGroup.ramp_time":
				child.SetText(strconv.Itoa(ep.Rampup))
			}
		}
	}
	modifiedJMXFile := filepath.Join(engineFolder, "modified.jmx")
	f, err := os.Create(modifiedJMXFile)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := planDoc.WriteTo(f); err != nil {
		return err
	}
	return nil
}

func (je *jmeterEngine) prepareTestData(fileName string, ep *model.ExecutionPlan,
	engineData map[string]*model.ShibuyaFile) (*bytes.Buffer, error) {
	folderPath := filepath.Join(strconv.FormatInt(je.collectionID, 10),
		strconv.FormatInt(ep.PlanID, 10), strconv.Itoa(je.ID))
	utils.MakeFolder(folderPath)
	for _, sf := range engineData {
		if err := ioutil.WriteFile(filepath.Join(folderPath, sf.Filename), sf.RawFile, 0644); err != nil {
			return nil, err
		}
	}
	jmxFile := filepath.Join(folderPath, fileName)
	if err := modifyJMX(folderPath, jmxFile, ep); err != nil {
		return nil, err
	}
	return je.zipFiles(folderPath)
}

func (je *jmeterEngine) trigger(fileName string, ep *model.ExecutionPlan,
	engineData map[string]*model.ShibuyaFile) error {
	// implement trigger in every engine type because it depends on it's own prepareTestData()
	fileBuffer, err := je.prepareTestData(fileName, ep, engineData)
	if err != nil {
		return err
	}
	engineUrl := je.engineUrl
	url := fmt.Sprintf("http://%s/%s", engineUrl, "start")
	return utils.Retry(func() error {
		resp, err := sendTriggerRequest(url, fileBuffer, je.defaultPlanPath)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			log.Printf("%s is already triggered", engineUrl)
			return nil
		}
		log.Printf("%s is triggered", engineUrl)
		return nil
	})
}

func (je *jmeterEngine) readMetrics() chan *shibuyaMetric {
	ch := make(chan *shibuyaMetric)
	go func() {
	outer:
		for {
			select {
			case ev, ok := <-je.stream.Events:
				if !ok {
					break outer
				}
				raw := ev.Data()
				line := strings.Split(raw, "|")

				label := line[2]
				status := line[3]
				threads, _ := strconv.ParseFloat(line[9], 64)
				latency, err := strconv.ParseFloat(line[10], 64)
				if err != nil {
					continue outer // no csv headers
				}
				ch <- &shibuyaMetric{
					threads:      threads,
					label:        label,
					status:       status,
					latency:      latency,
					collectionID: strconv.FormatInt(je.collectionID, 10),
					planID:       strconv.FormatInt(je.planID, 10),
					engineID:     strconv.FormatInt(int64(je.ID), 10),
					runID:        strconv.FormatInt(je.runID, 10),
				}
			case _, ok := <-je.stream.Errors:
				if !ok {
					break outer
				}
			}
		}
		close(ch)
	}()
	return ch
}
