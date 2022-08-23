BASE_IMAGE = golang:1.18-alpine3.15
LINT_IMAGE = golangci/golangci-lint:v1.45.2
NODE_IMAGE = node:16-alpine3.15
RPI32_IMAGE = balenalib/raspberrypi3:buster-run
RPI64_IMAGE = balenalib/raspberrypi3-64:buster-run

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy       run go mod tidy"
	@echo "  format         format source files"
	@echo "  test           run tests"
	@echo "  test32         run tests on a 32-bit system"
	@echo "  lint           run linters"
	@echo "  bench NAME=n   run bench environment"
	@echo "  run            run app"
	@echo "  apidocs-lint   run api docs linters"
	@echo "  apidocs-gen    generate api docs HTML"
	@echo "  release        build release assets"
	@echo "  dockerhub      build and push images to Docker Hub"
	@echo ""

blank :=
define NL

$(blank)
endef

include scripts/*.mk
