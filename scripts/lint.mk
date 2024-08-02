lint:
	go generate ./...
	docker run --rm -v $(PWD):/app -w /app \
	$(LINT_IMAGE) \
	golangci-lint run -v
