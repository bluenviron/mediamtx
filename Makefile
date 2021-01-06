
BASE_IMAGE = golang:1.15-alpine3.12
GO_LINT_IMAGE = golangci/golangci-lint:v1.33.0

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy       run go mod tidy"
	@echo "  format         format source files"
	@echo "  test           run tests"
	@echo "  lint           run linters"
	@echo "  bench NAME=n  run bench environment"
	@echo "  run            run app"
	@echo "  release        build release assets"
	@echo "  dockerhub      build and push docker hub images"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/s amd64/$(BASE_IMAGE) \
	sh -c "apk add git && cd /s && GOPROXY=direct go get && go mod tidy"

format:
	docker run --rm -it -v $(PWD):/s amd64/$(BASE_IMAGE) \
	sh -c "cd /s && find . -type f -name '*.go' | xargs gofmt -l -w -s"

define DOCKERFILE_TEST
FROM amd64/$(BASE_IMAGE)
RUN apk add --no-cache make docker-cli git ffmpeg gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp
	docker run --rm \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	temp \
	make test-nodocker

test-nodocker:
	$(foreach IMG,$(shell echo testimages/*/ | xargs -n1 basename), \
	docker build -q testimages/$(IMG) -t rtsp-simple-server-test-$(IMG)$(NL))
	go test -race -v .

lint:
	docker run --rm -v $(PWD):/app -w /app \
	$(GO_LINT_IMAGE) \
	golangci-lint run -v

bench:
	docker build -q . -f bench/$(NAME)/Dockerfile -t temp
	docker run --rm -it -p 9999:9999 temp

define DOCKERFILE_RUN
FROM amd64/$(BASE_IMAGE)
RUN apk add --no-cache git ffmpeg
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /out .
WORKDIR /
ARG CONFIG_RUN
RUN echo "$$CONFIG_RUN" > rtsp-simple-server.yml
endef
export DOCKERFILE_RUN

define CONFIG_RUN
#rtspPort: 8555
#rtpPort: 8002
#rtcpPort: 8003
#metrics: yes
#pprof: yes

paths:
  all:
#    runOnPublish: ffmpeg -i rtsp://localhost:$$RTSP_PORT/$$RTSP_PATH -c copy -f mpegts myfile_$$RTSP_PATH.ts
#    readUser: test
#    readPass: tast
#    runOnDemand: ffmpeg -re -stream_loop -1 -i testimages/ffmpeg/emptyvideo.ts -c copy -f rtsp rtsp://localhost:$$RTSP_PORT/$$RTSP_PATH

#  proxied:
#    source: rtsp://192.168.2.198:554/stream
#    sourceProtocol: tcp
#    sourceOnDemand: yes
#    runOnDemand: ffmpeg -i rtsp://192.168.2.198:554/stream -c copy -f rtsp rtsp://localhost:$$RTSP_PORT/proxied2

#  original:
#    runOnPublish: ffmpeg -i rtsp://localhost:554/original -b:a 64k -c:v libx264 -preset ultrafast -b:v 500k -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:8554/compressed

endef
export CONFIG_RUN

run:
	echo "$$DOCKERFILE_RUN" | docker build -q . -f - -t temp \
	--build-arg CONFIG_RUN="$$CONFIG_RUN"
	docker run --rm -it \
	--network=host \
	temp \
	sh -c "/out"

define DOCKERFILE_RELEASE
FROM amd64/$(BASE_IMAGE)
RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN make release-nodocker
endef
export DOCKERFILE_RELEASE

release:
	echo "$$DOCKERFILE_RELEASE" | docker build . -f - -t temp \
	&& docker run --rm -v $(PWD):/out \
	temp sh -c "rm -rf /out/release && cp -r /s/release /out/"

release-nodocker:
	$(eval export CGO_ENABLED=0)
	$(eval VERSION := $(shell git describe --tags))
	$(eval GOBUILD := go build -ldflags '-X main.version=$(VERSION)')
	rm -rf tmp && mkdir tmp
	rm -rf release && mkdir release
	cp rtsp-simple-server.yml tmp/

	GOOS=windows GOARCH=amd64 $(GOBUILD) -o tmp/rtsp-simple-server.exe
	cd tmp && zip -q $(PWD)/release/rtsp-simple-server_$(VERSION)_windows_amd64.zip rtsp-simple-server.exe rtsp-simple-server.yml

	GOOS=linux GOARCH=amd64 $(GOBUILD) -o tmp/rtsp-simple-server
	tar -C tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml

	GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o tmp/rtsp-simple-server
	tar -C tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm6.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml

	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -o tmp/rtsp-simple-server
	tar -C tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm7.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml

	GOOS=linux GOARCH=arm64 $(GOBUILD) -o tmp/rtsp-simple-server
	tar -C tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm64v8.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml

	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o tmp/rtsp-simple-server
	tar -C tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_darwin_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server rtsp-simple-server.yml

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
	&& go build -ldflags "-X main.version=$$VERSION" -o /rtsp-simple-server

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
