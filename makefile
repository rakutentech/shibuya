all: | cluster permissions db prometheus grafana shibuya jmeter local_storage

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
	docker build grafana/ -t shibuya:grafana
	kind load docker-image shibuya:grafana --name shibuya
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/grafana.yaml --force

.PHONY: shibuya
shibuya: shibuya/ kubernetes/
	cp config.json shibuya/config.json
	docker build --build-arg env=local -t shibuya:local shibuya
	rm shibuya/config.json
	kind load docker-image shibuya:local --name shibuya
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/shibuya.yaml --force

.PHONY: jmeter
jmeter: jmeter/
	docker build -t shibuya:jmeter jmeter
	kind load docker-image shibuya:jmeter --name shibuya

.PHONY: expose
expose:
	-killall kubectl
	-kubectl -n $(shibuya-controller-ns) port-forward service/grafana 3000:3000 > /dev/null 2>&1 &
	-kubectl -n $(shibuya-controller-ns) port-forward service/shibuya 8080:8080 > /dev/null 2>&1 &

.PHONY: kubeconfig
kubeconfig:
	./kubernetes/generate_kubeconfig.sh $(shibuya-controller-ns)

.PHONY: permissions
permissions:
	kubectl -n $(shibuya-executor-ns) apply -f kubernetes/roles.yaml
	kubectl -n $(shibuya-controller-ns) apply -f kubernetes/serviceaccount.yaml
	-kubectl -n $(shibuya-executor-ns) create rolebinding shibuya --role=shibuya --serviceaccount $(shibuya-controller-ns):shibuya

.PHONY: permissions-gcp
permissions-gcp: node-permissions permissions

.PHONY: node-permissions
node-permissions:
	kubectl apply -f kubernetes/clusterrole.yaml
	-kubectl create clusterrolebinding shibuya --clusterrole=shibuya --serviceaccount $(shibuya-controller-ns):shibuya

.PHONY: local_storage
local_storage:
	docker build -t shibuya:storage local_storage
	kind load docker-image shibuya:storage --name shibuya
	kubectl -n $(shibuya-controller-ns) replace -f kubernetes/storage.yaml --force