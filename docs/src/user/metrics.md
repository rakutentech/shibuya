# Metrics reported by Shibuya

When you trigger a collection, a unique run_id is generated and this is the identifier used for finding the related reporting metrics.
In the following sections, the docs will explain two metrics reported by Shibuya with samples.

## shibuya_status_counter

type: counter

This is how the QPS is being calculated. Sample query for showing the QPS in this run:

sample query: `sum(rate(shibuya_status_counter{run_id=\"$runID\"}[30s])) by (run_id)`

There are labels such as HTTP status code, plan_id in the metrics.

## shibuya_latency_collection_sum

type: summary

This shows the summary of latency of the running collection. Sample query for showing the average latency:

`(sum(rate(shibuya_latency_collection_sum{run_id=\"$runID\"}[2s])) by (run_id)) / (sum(rate(shibuya_status_counter{run_id=\"$runID\"}[2s])) by (run_id))`

For a complete list of the metrics reported by Shibuya, please check here: [https://github.com/rakutentech/shibuya/blob/master/shibuya/config/prometheus.go](https://github.com/rakutentech/shibuya/blob/master/shibuya/config/prometheus.go)
