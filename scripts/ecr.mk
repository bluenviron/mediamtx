# This file is based on binaries.mk and dockerhub.mk. It's simplified to
# build just the platform we need, and push to ECR.
#
# This file assumes you already have a valid ECR login, see Amazon's
# instructions: https://docs.aws.amazon.com/AmazonECR/latest/userguide/getting-started-cli.html#cli-authenticate-registry
#
BINARY_NAME = mediamtx

define DOCKERFILE_BINARIES
FROM $(BASE_IMAGE) AS build-base
RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
ARG VERSION
ENV CGO_ENABLED 0
RUN rm -rf tmp binaries
RUN mkdir tmp binaries
RUN cp mediamtx.yml LICENSE tmp/
RUN go generate ./...

FROM build-base AS build-linux-amd64
RUN GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME)
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_linux_amd64.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE

FROM $(BASE_IMAGE)
COPY --from=build-linux-amd64 /s/binaries /s/binaries
endef
export DOCKERFILE_BINARIES

ecr-binaries:
	echo "$$DOCKERFILE_BINARIES" | DOCKER_BUILDKIT=1 docker build . -f - \
	--build-arg VERSION=$$(git describe --tags) \
	-t temp
	docker run --rm -v $(PWD):/out \
	temp sh -c "rm -rf /out/binaries && cp -r /s/binaries /out/"

define DOCKERFILE_ECR
FROM scratch
ARG TARGETPLATFORM
ADD tmp/binaries/$$TARGETPLATFORM.tar.gz /
ENTRYPOINT [ "/mediamtx" ]
endef
export DOCKERFILE_ECR

ecr-push:
	$(eval VERSION := $(shell git describe --tags | tr -d v))

	if [ -z "$(ECR_TAG)" ]; then \
		echo "ECR_TAG is required"; \
		exit 1; \
	fi

	rm -rf tmp
	mkdir -p tmp tmp/binaries/linux/arm tmp/rpi_base/linux/arm

	cp binaries/*linux_amd64.tar.gz tmp/binaries/linux/amd64.tar.gz

	echo "$$DOCKERFILE_ECR" | docker buildx build . -f - \
	--provenance=false \
	--platform=linux/amd64 \
	-t $(ECR_TAG) \
	--push

	docker buildx rm builder || true
	rm -rf $$HOME/.docker/manifests/*
