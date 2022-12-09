build:
	mkdir -p bin
	go build -o bin/ingress-controller

docker:
	docker build -t ${IMG} .
	docker push ${IMG}

docker-local:
	docker build -t ${IMG} .
	kind load docker-image ${IMG} --name shibuya