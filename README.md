## Introduction

Shibuya is a scheduler for load generator (Currently, Jmeter) that can deploy these executors in a kubernetes cluster and generate the results for you on the fly. Shibuya can scale quickly and to a higher capacity than distributed Jmeter mode. It also provides real time test results. Shibuya can be run on-prem or in public cloud.

Tests(Plans) are organised into collections. One collection belongs to one project. All these resources can be managed by an onwer based on LDAP information.
Collection is the unit where the actual tests are managed. Therefore, multiple test plans can be triggered simultaneounsly. The results of these plans will be converged and demostrated at a central place.


## Getting started

### Local setup

*Tested primarily on Ubuntu and OSX*

Pre-requisites:
1. Kind (https://kind.sigs.k8s.io)
2. kubectl (https://kubernetes.io/docs/tasks/tools/install-kubectl)
3. Helm (https://helm.sh/docs/intro/install/)
4. Docker (https://docs.docker.com/install) *On OSX please increase your docker machine's spec or you may face performance issues*


Run `make` to start local cluster

`make expose` to expose the Shibuya and Grafana pods on port 8080 and 3000 of your localhost respectively

Then you can go to http://localhost:8080 to check.

`make clean` to destroy the local cluster (nothing wil be saved)

`make shibuya` to build and deploy changes to shibuya controller

note: Local Shibuya does not have authentication. So you need to put `shibuya` as the ownership of the project. This is the same if you turn off authentication in the config file.

## Distributed mode(WIP)

In order to improve the scalibility of Shibuya, we are going to split the single Shibuya process into three components:

- apiserver
- controller.
- Engine metric streamer(Not existing yet)

By default, at locall, it will be run as non-distributed mode. You can enable to by set the `runtime.distributed_mode` to `true`.


### Production setup

Please read the makefile to understand what components are needed and how to set them up in detail.

1. Kubeconfig file:
   - Incluster config: You can deploy engines to same cluster the controller is running in. All you need is to specify a service account in your shibuya-controller deployment that has enough permissions (read `kuberenetes/roles.yaml`). Specify `"in_cluster": true` in your config_env.json
   - Out of cluster config: Shibuya can also deploy engines to a cluster other than the one it is running in. You need to generate the kubeconfig manually and place it in `shibuya/config/kube_configs/config` file. Specify `"in_cluster": false` in your config_env.json
2. GCP Token (optional):
   In case you want to automatically scale nodes in GCP you need to pass GCP token which has permissions to create, scale and delete node pools in your project. Place it at `shibuya/shibuya-gcp.json`
3. Prometheus:
   Create Prometheus with configs that can scrape `http://your-shibuya-controller/metrics` endpoint to fetch the results.
4. Grafana:
   Setup your Grafana with dashboards specified in `grafana/dashboards` folder. Point the data source to your prometheus. Specify this Grafana endpoint with correct dashboards in your config_env.json
5. MySQL:
   We use mariaDB v10.0.23 to store our metadata. So anything that is compatible with this spec of mariaDB can be used in place.
6. Nexus:
   We use Nexus to store the test plans and their data. If you want to use some other kind of storage you can implement the storage interface for your data source and use it in place of Nexus. As an alternative to Nexus, we also support GCP Bucket.
7. LDAP:
   We use this for Authentication purpose.

---
## Terminology
- Controller - The main Shibuya process which works as a scheduler of engines, shows the UI and collects the metrics from engines to create a comprehensive report
- Engine/executor - The actual load generating pod (Jmeter + Agent)
- Agent - a sidecar process that runs alongside Jmeter which communicates with the controller to start/stop Jmeter and read the report from it's Jmeter process and stream it back to controller.
- Context - The k8s cluster that Shibuya controller is managing

## Limitation

- Currently, one controller can only manage one k8s cluster. If you need to send the traffic from multiple k8s clusters, you need to have multiple controllers.
- Currently, Shibuya does not support executing test from multiple contexts at the same time.

## Future roadmap

- Adding more executor type support. For example, Gatling. Technically speaking, Shibuya can support any executor as long as the executor can provide real time metrics data in some way.
- Manage muliple contexts in one controller.
- Better Authentication
