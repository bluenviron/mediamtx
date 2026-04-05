DOCKER_REPOSITORY = effectiverange/mediamtx

dockerhub:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	docker login -u $(DOCKER_USER) -p $(DOCKER_PASSWORD)

	docker buildx rm builder 2>/dev/null || true
	docker buildx create --name=builder

	docker build --builder=builder \
	-f docker/trixie.Dockerfile . \
	--platform=linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-trixie \
	-t $(DOCKER_REPOSITORY):1-trixie \
	-t $(DOCKER_REPOSITORY):latest-trixie \
	--push

	docker buildx rm builder
