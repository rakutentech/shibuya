package controller

import (
	"strconv"
	"strings"

	"github.com/rakutentech/shibuya/shibuya/config"
)

type jmeterEngine struct {
	*baseEngine
}

func NewJmeterEngine(be *baseEngine) *jmeterEngine {
	be.ExecutorContainer = config.SC.ExecutorConfig.JmeterContainer.ExecutorContainer
	e := &jmeterEngine{be}
	return e
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
					raw:          raw,
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
