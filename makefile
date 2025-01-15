all: | cluster permissions db prometheus grafana shibuya jmeter local_storage ingress-controller

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
	docker build grafana/ -t grafana:local
	kind load docker-image grafana:local --name shibuya
	helm upgrade --install metrics-dashboard grafana/metrics-dashboard

.PHONY: local_api
local_api:
	cd shibuya && sh build.sh api
	docker build -f shibuya/Dockerfile --build-arg env=local -t api:local shibuya
	kind load docker-image api:local --name shibuya

.PHONY: local_controller
local_controller:
	cd shibuya && sh build.sh controller
	docker build -f shibuya/Dockerfile --build-arg env=local -t controller:local shibuya
	kind load docker-image controller:local --name shibuya

.PHONY: shibuya
shibuya: local_api local_controller grafana
	helm uninstall shibuya || true
	cd shibuya && helm upgrade --install shibuya install/shibuya

.PHONY: jmeter
jmeter: shibuya/engines/jmeter
	cd shibuya && sh build.sh jmeter
	docker build -t shibuya:jmeter -f shibuya/Dockerfile.engines.jmeter shibuya
	kind load docker-image shibuya:jmeter --name shibuya

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
	docker build -t shibuya:storage local_storage
	kind load docker-image shibuya:storage --name shibuya
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/storage.yaml --force

.PHONY: local_coordinator
local_coordinator:
	# if you need to debug the controller, please use the makefile in the ingress controller folder
	# And update the image in the config.json
	cd shibuya && sh build.sh coordinator
	docker build -f shibuya/Dockerfile --build-arg binary_name=shibuya-coordinator -t coordinator:local shibuya
	kind load docker-image coordinator:local --name shibuya
