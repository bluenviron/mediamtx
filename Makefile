
.PHONY: $(shell ls)

BASE_IMAGE = golang:1.14-alpine3.11

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy       run go mod tidy"
	@echo "  format         format source files"
	@echo "  test           run available tests"
	@echo "  run ARGS=args  run app"
	@echo "  release        build release assets"
	@echo "  dockerhub      build and push docker hub images"
	@echo ""

blank :=
define NL

$(blank)
endef

mod-tidy:
	docker run --rm -it -v $(PWD):/s amd64/$(BASE_IMAGE) \
	sh -c "apk add git && cd /s && GOPROXY=direct go get && GOPROXY=direct go mod tidy"

format:
	docker run --rm -it -v $(PWD):/s amd64/$(BASE_IMAGE) \
	sh -c "cd /s && find . -type f -name '*.go' | xargs gofmt -l -w -s"

define DOCKERFILE_TEST
FROM amd64/$(BASE_IMAGE)
RUN apk add --no-cache make docker-cli git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp
	docker run --rm -it \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	temp \
	make test-nodocker

test-nodocker:
	$(eval export CGO_ENABLED=0)
	$(foreach IMG,$(shell echo test-images/*/ | xargs -n1 basename), \
	docker build -q test-images/$(IMG) -t rtsp-simple-server-test-$(IMG)$(NL))
	go test -v .

define DOCKERFILE_RUN
FROM amd64/$(BASE_IMAGE)
RUN apk add --no-cache git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN GOPROXY=direct go build -o /out .
endef
export DOCKERFILE_RUN

run:
	echo "$$DOCKERFILE_RUN" | docker build -q . -f - -t temp
	docker run --rm -it \
	--network=host \
	temp \
	/out $(ARGS)

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
	&& docker run --rm -it -v $(PWD):/out \
	temp sh -c "rm -rf /out/release && cp -r /s/release /out/"

release-nodocker:
	$(eval export CGO_ENABLED=0)
	$(eval VERSION := $(shell git describe --tags))
	$(eval GOBUILD := go build -ldflags '-X main.Version=$(VERSION)')
	rm -rf release && mkdir release

	GOOS=windows GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-server.exe
	cd /tmp && zip -q $(PWD)/release/rtsp-simple-server_$(VERSION)_windows_amd64.zip rtsp-simple-server.exe

	GOOS=linux GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server

	GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm6.tar.gz --owner=0 --group=0 rtsp-simple-server

	GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm7.tar.gz --owner=0 --group=0 rtsp-simple-server

	GOOS=linux GOARCH=arm64 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm64.tar.gz --owner=0 --group=0 rtsp-simple-server

	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_darwin_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server

define DOCKERFILE_IMAGE
FROM --platform=linux/amd64 $(BASE_IMAGE) AS build
RUN apk add --no-cache git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
ARG VERSION
ARG OPTS
RUN export CGO_ENABLED=0 $${OPTS} \
	&& go build -ldflags "-X main.Version=$$VERSION" -o /rtsp-simple-server

FROM scratch
COPY --from=build /rtsp-simple-server /rtsp-simple-server
ENTRYPOINT [ "/rtsp-simple-server"]
endef
export DOCKERFILE_IMAGE

dockerhub:
	$(eval export DOCKER_CLI_EXPERIMENTAL=enabled)
	$(eval VERSION := $(shell git describe --tags))

	docker buildx rm test 2>/dev/null || true
	docker buildx create --name=builder --use

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-amd64 --build-arg OPTS="GOOS=linux GOARCH=amd64" --platform=linux/amd64

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-armv6 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=6" --platform=linux/arm/v6

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-armv7 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=7" --platform=linux/arm/v7

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:$(VERSION)-arm64 --build-arg OPTS="GOOS=linux GOARCH=arm64" --platform=linux/arm64/v8

	docker manifest create --amend aler9/rtsp-simple-server:$(VERSION) \
	$(foreach ARCH,amd64 armv6 armv7 arm64,aler9/rtsp-simple-server:$(VERSION)-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:$(VERSION)

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:latest-amd64 --build-arg OPTS="GOOS=linux GOARCH=amd64" --platform=linux/amd64

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:latest-armv6 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=6" --platform=linux/arm/v6

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:latest-armv7 --build-arg OPTS="GOOS=linux GOARCH=arm GOARM=7" --platform=linux/arm/v7

	echo "$$DOCKERFILE_IMAGE" | docker buildx build . -f - --build-arg VERSION=$(VERSION) \
	--push -t aler9/rtsp-simple-server:latest-arm64 --build-arg OPTS="GOOS=linux GOARCH=arm64" --platform=linux/arm64/v8

	docker manifest create --amend aler9/rtsp-simple-server:latest \
	$(foreach ARCH,amd64 armv6 armv7 arm64,aler9/rtsp-simple-server:latest-$(ARCH))
	docker manifest push aler9/rtsp-simple-server:latest

	docker buildx rm builder
