BASE_IMAGE = golang:1.20-alpine3.18
LINT_IMAGE = golangci/golangci-lint:v1.53.3
NODE_IMAGE = node:16-alpine3.18
ALPINE_IMAGE = alpine:3.18
RPI32_IMAGE = balenalib/raspberry-pi:bullseye-run-20230712
RPI64_IMAGE = balenalib/raspberrypi3-64:bullseye-run-20230530

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
	@echo "  bench NAME=n     run bench environment"
	@echo "  run              run app"
	@echo "  apidocs-lint     run api docs linters"
	@echo "  apidocs-gen      generate api docs HTML"
	@echo "  binaries         build binaries for all platforms"
	@echo "  dockerhub        build and push images to Docker Hub"
	@echo "  dockerhub-legacy build and push images to Docker Hub (legacy)"
	@echo ""

blank :=
define NL

$(blank)
endef

include scripts/*.mk
