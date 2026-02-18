BASE_IMAGE = golang:1.25-alpine3.22
GOLANGCI_LINT_IMAGE = golangci/golangci-lint:v2.10.1
NODE_IMAGE = node:20-alpine3.22

.PHONY: $(shell ls)

help:
	@echo "usage: make [action]"
	@echo ""
	@echo "available actions:"
	@echo ""
	@echo "  format           format code"
	@echo "  test             run tests"
	@echo "  test-32          run tests on a 32-bit system"
	@echo "  test-e2e         run end-to-end tests"
	@echo "  lint             run linters"
	@echo "  binaries         build binaries for all supported platforms"
	@echo "  dockerhub        build and push images to Docker Hub"
	@echo ""

blank :=
define NL

$(blank)
endef

include scripts/*.mk
