BASE_IMAGE = golang:1.23-alpine3.20
LINT_IMAGE = golangci/golangci-lint:v1.61.0
NODE_IMAGE = node:20-alpine3.20
ALPINE_IMAGE = alpine:3.20
DEBIAN_IMAGE = debian:bookworm
RPI32_IMAGE = balenalib/raspberry-pi:bullseye-run-20240508
RPI64_IMAGE = balenalib/raspberrypi3-64:bullseye-run-20240429
JETSON_IMAGE = nvcr.io/nvidia/l4t-jetpack:r36.4.0

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy         run go mod tidy"
	@echo "  format           format source files"
	@echo "  test             run tests"
	@echo "  test32           run tests on a 32-bit system"
	@echo "  test-highlevel   run high-level tests"
	@echo "  lint             run linters"
	@echo "  run              run app"
	@echo "  apidocs          generate api docs HTML"
	@echo "  binaries         build binaries for all platforms"
	@echo "  dockerimg        build local Docker image"
	@echo "                   Current: PLATFORM=$(PLATFORM), USE_FFMPEG=$(USE_FFMPEG)"
	@echo "                   Valid PLATFORM options: amd64, rpi32, rpi64, jetson"
	@echo "                   Valid USE_FFMPEG options: true, false, 1, 0"
	@echo "  dockerhub        build and push images to Docker Hub"
	@echo "  dockerhub-legacy build and push images to Docker Hub (legacy)"
	@echo ""

blank :=
define NL

$(blank)
endef

include scripts/*.mk
