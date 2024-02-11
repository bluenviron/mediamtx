ifeq ($(shell getconf LONG_BIT),64)
  RACE=-race
endif

test-internal:
	go generate ./...
	go test -v $(RACE) -coverprofile=coverage-internal.txt \
	$$(go list ./internal/... | grep -v /core)

test-core:
	go test -v $(RACE) -coverprofile=coverage-core.txt ./internal/core

test-nodocker: test-internal test-core

define DOCKERFILE_TEST
ARG ARCH
FROM $$ARCH/$(BASE_IMAGE)
RUN apk add --no-cache make gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp --build-arg ARCH=amd64
	docker run --rm \
	-v $(PWD):/s \
	temp \
	make test-nodocker

test32:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp --build-arg ARCH=i386
	docker run --rm \
	-v $(PWD):/s \
	temp \
	make test-nodocker
