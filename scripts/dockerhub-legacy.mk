DOCKER_REPOSITORY_LEGACY = aler9/rtsp-simple-server

dockerhub-legacy:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	docker login -u $(DOCKER_USER_LEGACY) -p $(DOCKER_PASSWORD_LEGACY)

	docker pull $(DOCKER_REPOSITORY):$(VERSION)
	docker tag $(DOCKER_REPOSITORY):$(VERSION) $(DOCKER_REPOSITORY_LEGACY):v$(VERSION)
	docker push $(DOCKER_REPOSITORY_LEGACY):v$(VERSION)

	docker pull $(DOCKER_REPOSITORY):$(VERSION)-rpi
	docker tag $(DOCKER_REPOSITORY):$(VERSION) $(DOCKER_REPOSITORY_LEGACY):v$(VERSION)-rpi
	docker push $(DOCKER_REPOSITORY_LEGACY):v$(VERSION)-rpi

	docker pull $(DOCKER_REPOSITORY):latest
	docker tag $(DOCKER_REPOSITORY):$(VERSION) $(DOCKER_REPOSITORY_LEGACY):latest
	docker push $(DOCKER_REPOSITORY_LEGACY):latest

	docker pull $(DOCKER_REPOSITORY):latest-rpi
	docker tag $(DOCKER_REPOSITORY):$(VERSION) $(DOCKER_REPOSITORY_LEGACY):latest-rpi
	docker push $(DOCKER_REPOSITORY_LEGACY):latest-rpi
