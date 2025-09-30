all: | cluster permissions db prometheus grafana shibuya jmeter local_storage ingress-controller

# Container runtime - can be docker or podman
CONTAINER_RUNTIME ?= $(shell which podman >/dev/null 2>&1 && echo podman || echo docker)

shibuya-controller-ns = shibuya-executors
shibuya-executor-ns = shibuya-executors

.PHONY: cluster
cluster:
	-kind create cluster --name shibuya --wait 180s
	-kubectl create namespace $(shibuya-controller-ns)
	-kubectl create namespace $(shibuya-executor-ns)
	kubectl apply -f kubernetes/metricServer.yaml
	kubectl config set-context --current --namespace=$(shibuya-controller-ns)
	touch shibuya/shibuya-gcp.json

.PHONY: clean
clean:
	kind delete cluster --name shibuya
	-killall kubectl

.PHONY: prometheus
prometheus:
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/prometheus.yaml --force

.PHONY: db
db: shibuya/db kubernetes/db.yaml
	-kubectl -n $(shibuya-controller-ns) delete configmap database
	kubectl -n $(shibuya-controller-ns) create configmap database --from-file=shibuya/db/
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/db.yaml --force

.PHONY: grafana
grafana: grafana/
	helm uninstall metrics-dashboard || true
	$(CONTAINER_RUNTIME) build grafana/ -t grafana:local
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/grafana:local -o /tmp/grafana-local.tar
	kind load image-archive /tmp/grafana-local.tar --name shibuya
	rm -f /tmp/grafana-local.tar
else
	kind load docker-image grafana:local --name shibuya
endif
	helm upgrade --install metrics-dashboard grafana/metrics-dashboard

.PHONY: local_api
local_api:
	cd shibuya && sh build.sh api
	$(CONTAINER_RUNTIME) build -f shibuya/Dockerfile --build-arg env=local -t api:local shibuya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/api:local -o /tmp/api-local.tar
	kind load image-archive /tmp/api-local.tar --name shibuya
	rm -f /tmp/api-local.tar
else
	kind load docker-image api:local --name shibuya
endif

.PHONY: local_controller
local_controller:
	cd shibuya && sh build.sh controller
	$(CONTAINER_RUNTIME) build -f shibuya/Dockerfile --build-arg env=local -t controller:local shibuya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/controller:local -o /tmp/controller-local.tar
	kind load image-archive /tmp/controller-local.tar --name shibuya
	rm -f /tmp/controller-local.tar
else
	kind load docker-image controller:local --name shibuya
endif

.PHONY: shibuya
shibuya: local_api local_controller grafana
	helm uninstall shibuya || true
	cd shibuya && helm upgrade --install shibuya install/shibuya

.PHONY: jmeter
jmeter: shibuya/engines/jmeter
	cd shibuya && sh build.sh jmeter
	$(CONTAINER_RUNTIME) build -t shibuya:jmeter -f shibuya/Dockerfile.engines.jmeter shibuya
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/shibuya:jmeter -o /tmp/shibuya-jmeter.tar
	kind load image-archive /tmp/shibuya-jmeter.tar --name shibuya
	rm -f /tmp/shibuya-jmeter.tar
else
	kind load docker-image shibuya:jmeter --name shibuya
endif

.PHONY: expose
expose:
	-killall kubectl
	-kubectl -n $(shibuya-controller-ns) port-forward service/shibuya-metrics-dashboard 3000:3000 > /dev/null 2>&1 &
	-kubectl -n $(shibuya-controller-ns) port-forward service/shibuya-api-local 8080:8080 > /dev/null 2>&1 &

# TODO!
# After k8s 1.22, service account token is no longer auto generated. We need to manually create the secret
# for the service account. ref: "https://kubernetes.io/docs/reference/access-authn-authz/service-accounts-admin/#manual-secret-management-for-serviceaccounts"
# So we should fetch the token details from the manually created secret instead of the automatically created ones
.PHONY: kubeconfig
kubeconfig:
	./kubernetes/generate_kubeconfig.sh $(shibuya-controller-ns)

.PHONY: permissions
permissions:
	kubectl -n $(shibuya-executor-ns) apply -f kubernetes/roles.yaml
	kubectl -n $(shibuya-controller-ns) apply -f kubernetes/serviceaccount.yaml
	kubectl -n $(shibuya-controller-ns) apply -f kubernetes/service-account-secret.yaml
	-kubectl -n $(shibuya-executor-ns) create rolebinding shibuya --role=shibuya --serviceaccount $(shibuya-controller-ns):shibuya
	kubectl -n $(shibuya-executor-ns) replace -f kubernetes/ingress.yaml --force

.PHONY: permissions-gcp
permissions-gcp: node-permissions permissions

.PHONY: node-permissions
node-permissions:
	kubectl apply -f kubernetes/clusterrole.yaml
	-kubectl create clusterrolebinding shibuya --clusterrole=shibuya --serviceaccount $(shibuya-controller-ns):shibuya
	kubectl apply -f kubernetes/pdb.yaml

.PHONY: local_storage
local_storage:
	$(CONTAINER_RUNTIME) build -t shibuya:storage local_storage
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/shibuya:storage -o /tmp/shibuya-storage.tar
	kind load image-archive /tmp/shibuya-storage.tar --name shibuya
	rm -f /tmp/shibuya-storage.tar
else
	kind load docker-image shibuya:storage --name shibuya
endif
	kubectl -n $(shibuya-controller-ns) apply -f kubernetes/storage.yaml

.PHONY: ingress-controller
ingress-controller:
	# if you need to debug the controller, please use the makefile in the ingress controller folder
	# And update the image in the config.json
	$(CONTAINER_RUNTIME) build -t shibuya:ingress-controller -f ingress-controller/Dockerfile ingress-controller
ifeq ($(CONTAINER_RUNTIME),podman)
	podman save localhost/shibuya:ingress-controller -o /tmp/shibuya-ingress-controller.tar
	kind load image-archive /tmp/shibuya-ingress-controller.tar --name shibuya
	rm -f /tmp/shibuya-ingress-controller.tar
else
	kind load docker-image shibuya:ingress-controller --name shibuya
endif
