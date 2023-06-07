DOCKER_REPOSITORY_LEGACY = aler9/rtsp-simple-server

dockerhub-legacy:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	docker login -u $(DOCKER_USER_LEGACY) -p $(DOCKER_PASSWORD_LEGACY)

	docker run --rm \
	-v $(HOME)/.docker:/.docker:ro \
	quay.io/skopeo/stable:latest copy --all \
	--authfile /.docker/config.json \
	docker://docker.io/$(DOCKER_REPOSITORY):$(VERSION)-rpi \
	docker://docker.io/$(DOCKER_REPOSITORY_LEGACY):v$(VERSION)-rpi

	docker run --rm \
	-v $(HOME)/.docker:/.docker:ro \
	quay.io/skopeo/stable:latest copy --all \
	--authfile /.docker/config.json \
	docker://docker.io/$(DOCKER_REPOSITORY):latest-rpi \
	docker://docker.io/$(DOCKER_REPOSITORY_LEGACY):latest-rpi

	docker run --rm \
	-v $(HOME)/.docker:/.docker:ro \
	quay.io/skopeo/stable:latest copy --all \
	--authfile /.docker/config.json \
	docker://docker.io/$(DOCKER_REPOSITORY):$(VERSION) \
	docker://docker.io/$(DOCKER_REPOSITORY_LEGACY):v$(VERSION)

	docker run --rm \
	-v $(HOME)/.docker:/.docker:ro \
	quay.io/skopeo/stable:latest copy --all \
	--authfile /.docker/config.json \
	docker://docker.io/$(DOCKER_REPOSITORY):latest \
	docker://docker.io/$(DOCKER_REPOSITORY_LEGACY):latest
