define DOCKERFILE_APIDOCS_LINT
FROM $(NODE_IMAGE)
RUN yarn global add @redocly/cli@1.0.0-beta.123
endef
export DOCKERFILE_APIDOCS_LINT

lint-golangci:
	docker run --rm -v "$(shell pwd):/app" -w /app \
	$(LINT_IMAGE) \
	golangci-lint run -v

lint-mod-tidy:
	go mod tidy
	git diff --exit-code

lint-apidocs:
	echo "$$DOCKERFILE_APIDOCS_LINT" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/apidocs:/s" -w /s temp \
	sh -c "openapi lint openapi.yaml"

lint: lint-golangci lint-mod-tidy lint-apidocs
