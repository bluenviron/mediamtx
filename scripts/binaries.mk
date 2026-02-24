BINARY_NAME = mediamtx

define DOCKERFILE_BINARIES
FROM $(BASE_IMAGE) AS build-base
RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
ENV CGO_ENABLED=0
RUN rm -rf tmp binaries
RUN mkdir tmp binaries
RUN cp mediamtx.yml LICENSE tmp/
RUN go generate ./...

FROM build-base AS build-windows-amd64
RUN GOOS=windows GOARCH=amd64 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME).exe"
RUN go install github.com/tc-hib/go-winres@v0.3.3
RUN go-winres patch --in scripts/winres.json --product-version "$$(git describe --tags --abbrev=0 | sed 's/^v//')" --file-version "$$(git describe --tags --abbrev=0 | sed 's/^v//')" tmp/mediamtx.exe
RUN cd tmp && zip -q "../binaries/$(BINARY_NAME)_$$(cat ../internal/core/VERSION)_windows_amd64.zip" "$(BINARY_NAME).exe" mediamtx.yml LICENSE

FROM build-base AS build-linux-amd64
RUN GOOS=linux GOARCH=amd64 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_linux_amd64.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

FROM build-base AS build-darwin-amd64
RUN GOOS=darwin GOARCH=amd64 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_darwin_amd64.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

FROM build-base AS build-darwin-arm64
RUN GOOS=darwin GOARCH=arm64 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_darwin_arm64.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

FROM build-base AS build-linux-armv6
RUN GOOS=linux GOARCH=arm GOARM=6 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_linux_armv6.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

FROM build-base AS build-linux-armv7
RUN GOOS=linux GOARCH=arm GOARM=7 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_linux_armv7.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

FROM build-base AS build-linux-arm64
RUN GOOS=linux GOARCH=arm64 go build -tags enable_upgrade -o "tmp/$(BINARY_NAME)"
RUN tar -C tmp -czf "binaries/$(BINARY_NAME)_$$(cat internal/core/VERSION)_linux_arm64.tar.gz" --owner=0 --group=0 "$(BINARY_NAME)" mediamtx.yml LICENSE

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
	echo "$$DOCKERFILE_BINARIES" | docker build . -f - \
	-t temp
	docker run --rm -v "$(shell pwd):/out" \
	temp sh -c "rm -rf /out/binaries && cp -r /s/binaries /out/"
	sudo chown -R $(shell id -u):$(shell id -g) binaries
