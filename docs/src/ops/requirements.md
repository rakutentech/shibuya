# Setup the dependencies

Shibuya relies on below components:

* Kubernetes: Shibuya deploys all the load generators into a kubernetes cluster
* MySQL: all the session data and business logic
* Grafana: Metrics collected by Prometheus will be rendered at Grafana
* Prometheus: Metrics collected from the load generators will be scraped by Prom.

We will discuss each of this dependencies separately.

## Kubernetes

Current supported versions should work well with Shibuya.

Please follow below steps to setup the k8s cluster:

1. `kubect create ns shibuya-executors`


## MySQL

Internally, we are using fork of MySQL, MariaDB. But current versions of MySQL should work well with Shibuya.

All the schema files are stored under db folder. You just need to load the schemas into the database.

## Grafana

## Prometheus

