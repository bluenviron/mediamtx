
.PHONY: $(shell ls)

BASE_IMAGE = amd64/golang:1.13-alpine3.10

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy      run go mod tidy"
	@echo "  format        format source files"
	@echo "  test          run available tests"
	@echo "  run           run app"
	@echo ""

mod-tidy:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "apk add git && cd /s && go get && go mod tidy"

format:
	docker run --rm -it -v $(PWD):/s $(BASE_IMAGE) \
	sh -c "cd /s && find . -type f -name '*.go' | xargs gofmt -l -w -s"

define DOCKERFILE_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp
	docker run --rm -it \
	--name temp \
	temp \
	make test-nodocker

IMAGES = $(shell echo test-images/*/ | xargs -n1 basename)

test-nodocker:
	$(eval export CGO_ENABLED = 0)
	go test -v ./rtsp

define DOCKERFILE_RUN
FROM $(BASE_IMAGE)
RUN apk add --no-cache git
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /out .
endef
export DOCKERFILE_RUN

run:
	echo "$$DOCKERFILE_RUN" | docker build -q . -f - -t temp
	docker run --rm -it \
	--network=host \
	--name temp \
	temp \
	/out

define DOCKERFILE_RELEASE
FROM $(BASE_IMAGE)
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
	$(eval VERSION := $(shell git describe --tags))
	$(eval GOBUILD := go build -ldflags '-X "main.Version=$(VERSION)"')
	rm -rf release && mkdir release

	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-server.exe
	cd /tmp && zip -q $(PWD)/release/rtsp-simple-server_$(VERSION)_windows_amd64.zip rtsp-simple-server.exe

	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_amd64.tar.gz --owner=0 --group=0 rtsp-simple-server

	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm6.tar.gz --owner=0 --group=0 rtsp-simple-server

	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm7.tar.gz --owner=0 --group=0 rtsp-simple-server

	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -o /tmp/rtsp-simple-server
	tar -C /tmp -czf $(PWD)/release/rtsp-simple-server_$(VERSION)_linux_arm64.tar.gz --owner=0 --group=0 rtsp-simple-server
