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
	make test-nodocker COVERAGE=1

test32:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp --build-arg ARCH=i386
	docker run --rm \
	-v $(PWD):/s \
	temp \
	make test-nodocker COVERAGE=0

ifeq ($(COVERAGE),1)
TEST_INTERNAL_OPTS=-race -coverprofile=coverage-internal.txt
TEST_CORE_OPTS=-race -coverprofile=coverage-core.txt
endif

test-internal:
	go test -v $(TEST_INTERNAL_OPTS) \
	$$(go list ./internal/... | grep -v /core)

test-core:
	go test -v $(TEST_CORE_OPTS) ./internal/core

test-nodocker: test-internal test-core
