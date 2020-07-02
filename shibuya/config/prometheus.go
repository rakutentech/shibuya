package config

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Average latency is not a good metric, percentile latency is the way to go
	// But percentiles cannot be aggregated, so we need seperate latency vector for individual observations
	CollectionLatencySummary = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "shibuya",
		Name:       "latency_collection",
		Help:       "Percentile latency of a collection",
		Objectives: map[float64]float64{0.9: 0.01, 0.99: 0.001},
	}, []string{"collection_id", "run_id"})
	PlanLatencySummary = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "shibuya",
		Name:       "latency_plan",
		Help:       "Percentile latency of a collection",
		Objectives: map[float64]float64{0.9: 0.01, 0.99: 0.001},
	}, []string{"collection_id", "plan_id", "run_id"})
	LabelLatencySummary = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Namespace:  "shibuya",
		Name:       "latency_label",
		Help:       "Percentile latency of a collection",
		Objectives: map[float64]float64{0.9: 0.01, 0.99: 0.001},
	}, []string{"collection_id", "label", "run_id"})

	// This is similar to Latency but cannot use histogram here because we need a very accurate count of every status error that occured.
	// So 200s are different bucket than 201s responses.
	StatusCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "shibuya",
		Name:      "status_counter",
		Help:      "stores count of responses and groups in buckets of response codes",
	}, []string{"collection_id", "plan_id", "run_id", "engine_no", "label", "status"})

	// Gauge is the most intuitive way to count threads here.
	// We don't care about accuracy and there's no use of rate of threads
	ThreadsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "shibuya",
		Name:      "threads_gauge",
		Help:      "Current number of threads running in JMeter",
	}, []string{"collection_id", "plan_id", "run_id", "engine_no"})

	CpuGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "shibuya",
		Name:      "cpu_gauge",
		Help:      "CPU used by engine",
	}, []string{"collection_id", "plan_id", "engine_no"})

	MemGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "shibuya",
		Name:      "mem_gauge",
		Help:      "Memory used by engine",
	}, []string{"collection_id", "plan_id", "engine_no"})
)
