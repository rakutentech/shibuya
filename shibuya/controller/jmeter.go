package controller

import (
	"strconv"
	"strings"

	"github.com/rakutentech/shibuya/shibuya/config"
	log "github.com/sirupsen/logrus"
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
				// We use char "|" as the separator in jmeter jtl file. If some users somehow put another | in their label name
				// we could end up a broken split. For those requests, we simply ignore otherwise the process will crash.
				// With current jmeter setup, we are expecting 12 items to be presented in the JTL file after split.
				// The column in the JTL files are:
				// timeStamp|elapsed|label|responseCode|responseMessage|threadName|success|bytes|grpThreads|allThreads|Latency|Connect
				if len(line) < 12 {
					log.Infof("line length was less than required. Raw line is %s", raw)
					continue
				}
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
