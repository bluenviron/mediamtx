# FPT Camera WebRTC Adapter Makefile

.PHONY: all build-adapter run-adapter clean test

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build output
ADAPTER_BINARY=bin/fpt-adapter
ADAPTER_BINARY_WIN=bin/fpt-adapter.exe

# Main packages
ADAPTER_MAIN=./cmd/adapter

# Version info
VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

all: build-adapter

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build the adapter
build-adapter: deps
	$(GOBUILD) $(LDFLAGS) -o $(ADAPTER_BINARY) $(ADAPTER_MAIN)

# Build for Windows
build-adapter-win: deps
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(ADAPTER_BINARY_WIN) $(ADAPTER_MAIN)

# Run the adapter
run-adapter:
	$(GORUN) $(ADAPTER_MAIN)

# Run adapter with .env file
run-adapter-env:
	@if [ -f .env ]; then \
		export $$(cat .env | grep -v '^#' | xargs) && $(GORUN) $(ADAPTER_MAIN); \
	else \
		echo "No .env file found"; \
		$(GORUN) $(ADAPTER_MAIN); \
	fi

# Test adapter package
test-adapter:
	$(GOTEST) -v ./internal/adapter/...

# Test all packages
test:
	$(GOTEST) -v ./...

# Format code
fmt:
	$(GOFMT) ./internal/adapter/...
	$(GOFMT) ./cmd/adapter/...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f $(ADAPTER_BINARY)
	rm -f $(ADAPTER_BINARY_WIN)

# Show environment example
env-example:
	$(GORUN) $(ADAPTER_MAIN) -env-example

# Show version
version:
	$(GORUN) $(ADAPTER_MAIN) -version

# Help
help:
	@echo "FPT Camera WebRTC Adapter Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all              - Build the adapter (default)"
	@echo "  deps             - Download and tidy dependencies"
	@echo "  build-adapter    - Build the adapter binary"
	@echo "  build-adapter-win- Build the adapter for Windows"
	@echo "  run-adapter      - Run the adapter"
	@echo "  run-adapter-env  - Run the adapter with .env file"
	@echo "  test-adapter     - Run adapter tests"
	@echo "  test             - Run all tests"
	@echo "  fmt              - Format code"
	@echo "  clean            - Clean build artifacts"
	@echo "  env-example      - Show example environment variables"
	@echo "  version          - Show version"
	@echo "  help             - Show this help"
