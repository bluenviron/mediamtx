DOCKER_REPOSITORY = bluenviron/mediamtx

dockerhub:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	docker login -u $(DOCKER_USER) -p $(DOCKER_PASSWORD)

	docker buildx rm builder 2>/dev/null || true
	docker buildx create --name=builder

	docker build --builder=builder \
	-f docker/ffmpeg-rpi.Dockerfile . \
	--platform=linux/arm/v6,linux/arm/v7,linux/arm64 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-ffmpeg-rpi \
	-t $(DOCKER_REPOSITORY):latest-ffmpeg-rpi \
	--push

	docker build --builder=builder \
	-f docker/rpi.Dockerfile . \
	--platform=linux/arm/v6,linux/arm/v7,linux/arm64 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-rpi \
	-t $(DOCKER_REPOSITORY):latest-rpi \
	--push

	docker build --builder=builder \
	-f docker/ffmpeg.Dockerfile . \
	--platform=linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-ffmpeg \
	-t $(DOCKER_REPOSITORY):latest-ffmpeg \
	--push

	docker build --builder=builder \
	-f docker/standard.Dockerfile . \
	--platform=linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64 \
	-t $(DOCKER_REPOSITORY):$(VERSION) \
	-t $(DOCKER_REPOSITORY):latest \
	--push

	docker buildx rm builder
