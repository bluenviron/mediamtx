DOCKER_TAG = bluenviron/mediamtx

# Base images for each platform
BASE_IMAGE.amd64 = ${DEBIAN_IMAGE}
BASE_IMAGE.rpi32 = ${RPI32_IMAGE}
BASE_IMAGE.rpi64 = ${RPI64_IMAGE}
BASE_IMAGE.jetson = ${JETSON_IMAGE}

# Map simple platform names to Docker BuildKit platform strings
PLATFORM_MAP.amd64 = linux/amd64
PLATFORM_MAP.rpi32 = linux/arm/v6
PLATFORM_MAP.rpi64 = linux/arm64/v8
PLATFORM_MAP.jetson = linux/arm64/v8

# Default platform
PLATFORM ?= amd64
TARGETPLATFORM = $(PLATFORM_MAP.$(PLATFORM))
BASE_IMAGE = $(BASE_IMAGE.$(PLATFORM))

# Default flag to include ffmpeg
USE_FFMPEG ?= false

# Dockerfile templates
define DOCKERFILE_DOCKERIMG
FROM $(BASE_IMAGE)
ARG TARGETPLATFORM
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
#ENTRYPOINT [ "/mediamtx" ]
CMD [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERIMG

define DOCKERFILE_DOCKERIMG_FFMPEG
FROM $(BASE_IMAGE)
RUN apt update && apt install -y --no-install-recommends ffmpeg python3-pip python3-venv && rm -rf /var/lib/apt/lists/*
RUN python3 -m venv /opt/yt-dlp-venv && \
    /opt/yt-dlp-venv/bin/pip install --no-cache-dir yt-dlp && \
    ln -s /opt/yt-dlp-venv/bin/yt-dlp /usr/local/bin/yt-dlp
ARG TARGETPLATFORM
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
#ENTRYPOINT [ "/mediamtx" ]
CMD [ "/mediamtx" ]
endef
export DOCKERFILE_DOCKERIMG_FFMPEG

dockerimg:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	# Validate platform
	@if [ -z "$(TARGETPLATFORM)" ] || [ -z "$(BASE_IMAGE)" ]; then \
	  echo "Error: Unsupported PLATFORM '$(PLATFORM)'"; \
	  echo "Supported platforms: amd64, rpi32, rpi64, jetson"; \
	  exit 1; \
	fi

	@echo "Building for platform: $(PLATFORM) ($(TARGETPLATFORM))"
	@echo "Using base image: $(BASE_IMAGE)"
	@echo "Including FFmpeg: $(USE_FFMPEG)"

	rm -rf tmp
	mkdir -p tmp tmp/binaries/linux/arm tmp/rpi_base/linux/arm

	cp binaries/*linux_amd64.tar.gz tmp/binaries/linux/amd64.tar.gz
	cp binaries/*linux_armv6.tar.gz tmp/binaries/linux/arm/v6.tar.gz
	cp binaries/*linux_armv7.tar.gz tmp/binaries/linux/arm/v7.tar.gz
	cp binaries/*linux_arm64v8.tar.gz tmp/binaries/linux/arm64.tar.gz

	docker buildx rm builder 2>/dev/null || true
	rm -rf "$$HOME/.docker/manifests"/*
	docker buildx create --name=builder --use

	# Build with or without FFmpeg
	@if [ "$(USE_FFMPEG)" = "true" ] || [ "$(USE_FFMPEG)" = "1" ]; then \
	  echo "$$DOCKERFILE_DOCKERIMG_FFMPEG" | docker buildx build . -f - \
	  --provenance=false \
	  --platform=$(TARGETPLATFORM) \
	  -t $(DOCKER_TAG):$(VERSION)-ffmpeg \
	  -t $(DOCKER_TAG):latest-ffmpeg \
	  --load; \
	else \
	  echo "$$DOCKERFILE_DOCKERIMG" | docker buildx build . -f - \
	  --provenance=false \
	  --platform=$(TARGETPLATFORM) \
	  -t $(DOCKER_TAG):$(VERSION) \
	  -t $(DOCKER_TAG):latest \
	  --load; \
	fi

	docker buildx rm builder
	rm -rf "$$HOME/.docker/manifests"/*
