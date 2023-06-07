DOCKER_REPOSITORY = bluenviron/mediamtx

define DOCKERFILE_DOCKERHUB
FROM scratch
ARG TARGETPLATFORM
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
ENTRYPOINT [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERHUB

define DOCKERFILE_DOCKERHUB_FFMPEG
FROM $(ALPINE_IMAGE)
RUN apk add --no-cache ffmpeg
ARG TARGETPLATFORM
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
ENTRYPOINT [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERHUB_FFMPEG

define DOCKERFILE_DOCKERHUB_RPI_BASE_32
FROM $(RPI32_IMAGE)
endef
export DOCKERFILE_DOCKERHUB_RPI_BASE_32

define DOCKERFILE_DOCKERHUB_RPI_BASE_64
FROM $(RPI64_IMAGE)
endef
export DOCKERFILE_DOCKERHUB_RPI_BASE_64

define DOCKERFILE_DOCKERHUB_RPI
FROM scratch
ARG TARGETPLATFORM
ADD tmp/rpi_base/$$TARGETPLATFORM.tar /
RUN apt update && apt install -y --no-install-recommends libcamera0 libfreetype6
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
ENTRYPOINT [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERHUB_RPI

define DOCKERFILE_DOCKERHUB_FFMPEG_RPI
FROM scratch
ARG TARGETPLATFORM
ADD tmp/rpi_base/$$TARGETPLATFORM.tar /
RUN apt update && apt install -y --no-install-recommends libcamera0 libfreetype6 ffmpeg
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
ENTRYPOINT [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERHUB_FFMPEG_RPI

dockerhub:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	docker login -u $(DOCKER_USER) -p $(DOCKER_PASSWORD)

	rm -rf tmp
	mkdir -p tmp tmp/binaries/linux/arm tmp/rpi_base/linux/arm

	cp binaries/*linux_amd64.tar.gz tmp/binaries/linux/amd64.tar.gz
	cp binaries/*linux_armv6.tar.gz tmp/binaries/linux/arm/v6.tar.gz
	cp binaries/*linux_armv7.tar.gz tmp/binaries/linux/arm/v7.tar.gz
	cp binaries/*linux_arm64v8.tar.gz tmp/binaries/linux/arm64.tar.gz

	docker buildx rm builder 2>/dev/null || true
	rm -rf $$HOME/.docker/manifests/*
	docker buildx create --name=builder --use

	echo "$$DOCKERFILE_DOCKERHUB_RPI_BASE_32" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v6 \
	--output type=tar,dest=tmp/rpi_base/linux/arm/v6.tar

	echo "$$DOCKERFILE_DOCKERHUB_RPI_BASE_32" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v7 \
	--output type=tar,dest=tmp/rpi_base/linux/arm/v7.tar

	echo "$$DOCKERFILE_DOCKERHUB_RPI_BASE_64" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm64/v8 \
	--output type=tar,dest=tmp/rpi_base/linux/arm64.tar

	echo "$$DOCKERFILE_DOCKERHUB_FFMPEG_RPI" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v6,linux/arm/v7,linux/arm64/v8 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-ffmpeg-rpi \
	-t $(DOCKER_REPOSITORY):latest-ffmpeg-rpi \
	--push

	echo "$$DOCKERFILE_DOCKERHUB_RPI" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/arm/v6,linux/arm/v7,linux/arm64/v8 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-rpi \
	-t $(DOCKER_REPOSITORY):latest-rpi \
	--push

	echo "$$DOCKERFILE_DOCKERHUB_FFMPEG" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64/v8 \
	-t $(DOCKER_REPOSITORY):$(VERSION)-ffmpeg \
	-t $(DOCKER_REPOSITORY):latest-ffmpeg \
	--push

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64/v8 \
	-t $(DOCKER_REPOSITORY):$(VERSION) \
	-t $(DOCKER_REPOSITORY):latest \
	--push

	docker buildx rm builder
	rm -rf $$HOME/.docker/manifests/*
