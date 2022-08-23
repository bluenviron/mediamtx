define DOCKERFILE_TEST
ARG ARCH
FROM $$ARCH/$(BASE_IMAGE)
RUN apk add --no-cache make docker-cli gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
endef
export DOCKERFILE_TEST

test:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp --build-arg ARCH=amd64
	docker run --rm \
	--network=host \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	-v $(PWD):/s \
	temp \
	make test-nodocker COVERAGE=1

test32:
	echo "$$DOCKERFILE_TEST" | docker build -q . -f - -t temp --build-arg ARCH=i386
	docker run --rm \
	--network=host \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	-v $(PWD):/s \
	temp \
	make test-nodocker COVERAGE=0

ifeq ($(COVERAGE),1)
TEST_INTERNAL_OPTS=-race -coverprofile=coverage-internal.txt
TEST_CORE_OPTS=-race -coverprofile=coverage-core.txt
endif

test-internal:
	go test -v $(TEST_INTERNAL_OPTS) \
	./internal/conf \
	./internal/confwatcher \
	./internal/externalcmd \
	./internal/hls \
	./internal/logger \
	./internal/rlimit \
	./internal/rtmp/...

test-core:
	$(foreach IMG,$(shell echo testimages/*/ | xargs -n1 basename), \
	docker build -q testimages/$(IMG) -t rtsp-simple-server-test-$(IMG)$(NL))
	go test -v $(TEST_CORE_OPTS) ./internal/core

test-nodocker: test-internal test-core
