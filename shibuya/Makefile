registry=$(GAR_LOCATION)-docker.pkg.dev/$(GCP_PROJECT)
repository = shibuya
tag=$(GITHUB_SHA)
img=$(registry)/$(repository)/$(component):$(tag)

.PHONY: api_build
api_build:
	sh build.sh

.PHONY: api_image
api_image: api_build
	docker build -t $(img) -f Dockerfile .
	docker push $(img)

.PHONY: controller_build
controller_build:
	sh build.sh controller

.PHONY: controller_image
controller_image: controller_build
	docker build -t $(img) -f Dockerfile --build-arg="binary_name=shibuya-controller" .
	docker push $(img)

.PHONY: helm_charts
helm_charts:
	helm create shibuya-install
	helm package shibuya-install/
