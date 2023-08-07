BINARY_NAME = mediamtx

define DOCKERFILE_BINARIES
FROM $(RPI32_IMAGE) AS rpicamera32
RUN ["cross-build-start"]
RUN apt update && apt install -y --no-install-recommends g++ pkg-config make libcamera-dev libfreetype-dev xxd
WORKDIR /s/internal/rpicamera/exe
COPY internal/rpicamera/exe .
RUN make -j$$(nproc)

FROM $(RPI64_IMAGE) AS rpicamera64
RUN ["cross-build-start"]
RUN apt update && apt install -y --no-install-recommends g++ pkg-config make libcamera-dev libfreetype-dev xxd
WORKDIR /s/internal/rpicamera/exe
COPY internal/rpicamera/exe .
RUN make -j$$(nproc)

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

FROM build-base AS build-windows-amd64
RUN GOOS=windows GOARCH=amd64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME).exe
RUN cd tmp && zip -q ../binaries/$(BINARY_NAME)_$${VERSION}_windows_amd64.zip $(BINARY_NAME).exe mediamtx.yml LICENSE

FROM build-base AS build-linux-amd64
RUN GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME)
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_linux_amd64.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE

FROM build-base AS build-darwin-amd64
RUN GOOS=darwin GOARCH=amd64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME)
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_darwin_amd64.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE

FROM build-base AS build-darwin-arm64
RUN GOOS=darwin GOARCH=arm64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME)
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_darwin_arm64.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE

FROM build-base AS build-linux-armv6
COPY --from=rpicamera32 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME) -tags rpicamera
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_linux_armv6.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE
RUN rm internal/rpicamera/exe/exe

FROM build-base AS build-linux-armv7
COPY --from=rpicamera32 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME) -tags rpicamera
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_linux_armv7.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE
RUN rm internal/rpicamera/exe/exe

FROM build-base AS build-linux-arm64
COPY --from=rpicamera64 /s/internal/rpicamera/exe/exe internal/rpicamera/exe/
RUN GOOS=linux GOARCH=arm64 go build -ldflags "-X github.com/bluenviron/mediamtx/internal/core.version=$$VERSION" -o tmp/$(BINARY_NAME) -tags rpicamera
RUN tar -C tmp -czf binaries/$(BINARY_NAME)_$${VERSION}_linux_arm64v8.tar.gz --owner=0 --group=0 $(BINARY_NAME) mediamtx.yml LICENSE
RUN rm internal/rpicamera/exe/exe

FROM $(BASE_IMAGE)
COPY --from=build-windows-amd64 /s/binaries /s/binaries
COPY --from=build-linux-amd64 /s/binaries /s/binaries
COPY --from=build-darwin-amd64 /s/binaries /s/binaries
COPY --from=build-darwin-arm64 /s/binaries /s/binaries
COPY --from=build-linux-armv6 /s/binaries /s/binaries
COPY --from=build-linux-armv7 /s/binaries /s/binaries
COPY --from=build-linux-arm64 /s/binaries /s/binaries
endef
export DOCKERFILE_BINARIES

binaries:
	echo "$$DOCKERFILE_BINARIES" | DOCKER_BUILDKIT=1 docker build . -f - \
	--build-arg VERSION=$$(git describe --tags) \
	-t temp
	docker run --rm -v $(PWD):/out \
	temp sh -c "rm -rf /out/binaries && cp -r /s/binaries /out/"
