lint:
	touch internal/servers/hls/hls.min.js
	docker run --rm -v $(PWD):/app -w /app \
	$(LINT_IMAGE) \
	golangci-lint run -v
