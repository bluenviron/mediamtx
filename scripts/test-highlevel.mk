test-highlevel-nodocker:
	go test -v -race -tags enable_highlevel_tests ./internal/highleveltests

define DOCKERFILE_HIGHLEVEL_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_HIGHLEVEL_TEST

test-highlevel:
	echo "$$DOCKERFILE_HIGHLEVEL_TEST" | docker build -q . -f - -t temp
	docker run --rm -it \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	--network=host \
	temp \
	make test-highlevel-nodocker
