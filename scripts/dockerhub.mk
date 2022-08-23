define DOCKERFILE_DOCKERHUB
FROM --platform=linux/amd64 $(BASE_IMAGE) AS build
RUN apk add --no-cache git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
ARG VERSION
ARG OPTS
RUN export CGO_ENABLED=0 $${OPTS} \
	&& go build -ldflags "-X github.com/aler9/rtsp-simple-server/internal/core.version=$$VERSION" -o /rtsp-simple-server

FROM scratch
COPY --from=build /rtsp-simple-server /
COPY --from=build /s/rtsp-simple-server.yml /
ENTRYPOINT [ "/rtsp-simple-server" ]
endef
export DOCKERFILE_DOCKERHUB

dockerhub:
	$(eval export DOCKER_CLI_EXPERIMENTAL=enabled)
	$(eval VERSION := $(shell git describe --tags))

	docker login -u $(DOCKER_USER) -p $(DOCKER_PASSWORD)

	docker buildx rm builder 2>/dev/null || true
	rm -rf $$HOME/.docker/manifests/*
	docker buildx create --name=builder --use

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-amd64 --build-arg OPTS="GOOS=linux GOARCH=amd64" --platform=linux/amd64

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-armv6 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=6" --platform=linux/arm/v6

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-armv7 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=7" --platform=linux/arm/v7

	echo "$$DOCKERFILE_DOCKERHUB" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-arm64v8 --build-arg OPTS="GOOS=linux GOARCH=arm64" --platform=linux/arm64/v8

	docker manifest create aler9/rtsp-simple-server:$(VERSION) \
	$(foreach ARCH,amd64 armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:$(VERSION)

	docker manifest create aler9/rtsp-simple-server:latest-amd64 aler9/rtsp-simple-server:$(VERSION)-amd64
	docker manifest push aler9/rtsp-simple-server:latest-amd64

	docker manifest create aler9/rtsp-simple-server:latest-armv6 aler9/rtsp-simple-server:$(VERSION)-armv6
	docker manifest push aler9/rtsp-simple-server:latest-armv6

	docker manifest create aler9/rtsp-simple-server:latest-armv7 aler9/rtsp-simple-server:$(VERSION)-armv7
	docker manifest push aler9/rtsp-simple-server:latest-armv7

	docker manifest create aler9/rtsp-simple-server:latest-arm64v8 aler9/rtsp-simple-server:$(VERSION)-arm64v8
	docker manifest push aler9/rtsp-simple-server:latest-arm64v8

	docker manifest create aler9/rtsp-simple-server:latest \
	$(foreach ARCH,amd64 armv6 armv7 arm64v8,aler9/rtsp-simple-server:$(VERSION)-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:latest

	docker buildx rm builder
	rm -rf $$HOME/.docker/manifests/*
