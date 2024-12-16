registry=$(GAR_LOCATION)-docker.pkg.dev/$(GCP_PROJECT)
repository = shibuya
tag=$(tag_name)
img=$(registry)/$(repository)/metrics-dashboard:$(tag)
upstream = rakutentech

ifeq ($(GITHUB_REPOSITORY_OWNER), $(upstream))
	tag=$(tag_name)
else
	tag=$(GITHUB_REPOSITORY_OWNER)-$(tag_name)
endif

.PHONY: metrics_dashboard_image
metrics_dashboard_image:
	docker build -t $(img) .
	docker push $(img)
