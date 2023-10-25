package controller

import (
	"strconv"
	"sync"

	"github.com/rakutentech/shibuya/shibuya/config"
	"github.com/rakutentech/shibuya/shibuya/model"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// We use a KV map of [runID:set(values)] to store labels and statuses
// To make a set() we use map of strings and empty struct{} which is of 0 bytes
// When doing a Load() and Store() on outer map we don't need to care about atomicity
// because it will always reference the same pointer, so this value never changes.
// Also we don't need to care about consistency of data here since we would typically have
// hundreds of same type of inserts, so eventually everything will be consistent.
func syncMapInserter(sm *sync.Map, id int64, value string) {
	nestedSyncMapInterface, ok := sm.Load(id)
	if !ok {
		// run id does not exist
		var l sync.Map
		sm.Store(id, &l)
		syncMapInserter(sm, id, value)
		return
	}
	nestedSyncMap := nestedSyncMapInterface.(*sync.Map)
	nestedSyncMap.Store(value, struct{}{})
}

func (c *Controller) storeLocally(id int64, label string, status string) {
	syncMapInserter(&c.LabelStore, id, label)
	syncMapInserter(&c.StatusStore, id, status)
}

func (c *Controller) removeLocally(id int64) {
	c.LabelStore.Delete(id)
	c.StatusStore.Delete(id)
}

func (c *Controller) deleteEngineHealthMetrics(collectionID string, planID string, engines int) {
	for i := 0; i < engines; i++ {
		engineID := strconv.Itoa(i)
		config.CpuGauge.Delete(prometheus.Labels{
			"collection_id": collectionID,
			"plan_id":       planID,
			"engine_no":     engineID,
		})
		config.MemGauge.Delete(prometheus.Labels{
			"collection_id": collectionID,
			"plan_id":       planID,
			"engine_no":     engineID,
		})
		log.Infof("Delete engine health metrics %s-%s-%s", collectionID, planID, engineID)
	}
}

func (c *Controller) deleteMetrics(runID string, collectionID string, planID string, engines int) {
	for i := 0; i < engines; i++ {
		engineID := strconv.Itoa(i)
		config.ThreadsGauge.Delete(prometheus.Labels{
			"collection_id": collectionID,
			"plan_id":       planID,
			"run_id":        runID,
			"engine_no":     engineID,
		})
	}
	config.PlanLatencySummary.Delete(prometheus.Labels{
		"collection_id": collectionID,
		"plan_id":       planID,
		"run_id":        runID,
	})
	config.CollectionLatencySummary.Delete(prometheus.Labels{
		"collection_id": collectionID,
		"run_id":        runID,
	})
	c.deleteMetricsUsingLabelStore(runID, collectionID, planID, engines)
}

func (c *Controller) deleteMetricsUsingLabelStore(runID string, collectionID string, planID string, engines int) {
	runID_int, _ := strconv.ParseInt(runID, 10, 64)
	labelInterface, ok := c.LabelStore.Load(runID_int)
	if !ok {
		return
	}
	labelMap := labelInterface.(*sync.Map)
	labelMap.Range(func(label interface{}, _ interface{}) bool {
		config.LabelLatencySummary.Delete(prometheus.Labels{
			"collection_id": collectionID,
			"run_id":        runID,
			"label":         label.(string),
		})
		c.deleteMetricsUsingStatusStore(runID, collectionID, planID,
			engines, label.(string))
		return true
	})
}

func (c *Controller) deleteMetricsUsingStatusStore(runID string, collectionID string,
	planID string, engines int, label string) {
	runID_int, _ := strconv.ParseInt(runID, 10, 64)
	statusInterface, ok := c.StatusStore.Load(runID_int)
	if !ok {
		return
	}
	statusMap := statusInterface.(*sync.Map)
	statusMap.Range(func(status interface{}, _ interface{}) bool {
		for i := 0; i < engines; i++ {
			config.StatusCounter.Delete(prometheus.Labels{
				"collection_id": collectionID,
				"run_id":        runID,
				"plan_id":       planID,
				"engine_no":     strconv.Itoa(i),
				"label":         label,
				"status":        status.(string),
			})
		}
		return true
	})
}

func (c *Controller) deleteMetricByRunID(runID int64, collectionID int64) {
	collection, err := model.GetCollection(collectionID)
	if err != nil {
		log.Error(err)
		return
	}
	defer c.removeLocally(runID)
	collection.ExecutionPlans, err = collection.GetExecutionPlans()
	if err != nil {
		return
	}
	for _, ep := range collection.ExecutionPlans {
		c.deleteMetrics(strconv.FormatInt(runID, 10), strconv.FormatInt(collectionID, 10),
			strconv.FormatInt(ep.PlanID, 10), ep.Engines)
	}
}
