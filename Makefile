BASE_IMAGE = golang:1.23-alpine3.20
LINT_IMAGE = golangci/golangci-lint:v1.64.5
NODE_IMAGE = node:20-alpine3.20

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  mod-tidy         run go mod tidy"
	@echo "  format           format source files"
	@echo "  test             run tests"
	@echo "  test-32          run tests on a 32-bit system"
	@echo "  test-e2e         run end-to-end tests"
	@echo "  lint             run linters"
	@echo "  run              run app"
	@echo "  apidocs          generate api docs HTML"
	@echo "  binaries         build binaries for all platforms"
	@echo "  dockerhub        build and push images to Docker Hub"
	@echo "  dockerhub-legacy build and push images to Docker Hub (legacy)"
	@echo ""

blank :=
define NL

$(blank)
endef

include scripts/*.mk
