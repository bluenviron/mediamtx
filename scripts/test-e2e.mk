test-e2e-nodocker:
	go generate ./...
	go test -v -race -tags enable_e2e_tests ./internal/teste2e

define DOCKERFILE_E2E_TEST
FROM $(BASE_IMAGE)
RUN apk add --no-cache make docker-cli gcc musl-dev
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
endef
export DOCKERFILE_E2E_TEST

test-e2e:
	echo "$$DOCKERFILE_E2E_TEST" | docker build -q . -f - -t temp
	docker run --rm -it \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	--network=host \
	temp \
	make test-e2e-nodocker
