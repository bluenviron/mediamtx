define DOCKERFILE_DOCS_LINT
FROM $(NODE_IMAGE)
RUN yarn global add prettier@3.6.2
endef
export DOCKERFILE_DOCS_LINT

define DOCKERFILE_API_DOCS_LINT
FROM $(NODE_IMAGE)
RUN yarn global add @redocly/cli@1.0.0-beta.123
endef
export DOCKERFILE_API_DOCS_LINT

lint-go:
	docker run --rm -v "$(shell pwd):/app" -w /app \
	$(GOLANGCI_LINT_IMAGE) \
	golangci-lint run -v

lint-go-mod:
	go mod tidy
	git diff --exit-code

lint-go2api:
	go test -v -tags enable_linters ./internal/linters/go2api

lint-docs:
	echo "$$DOCKERFILE_DOCS_LINT" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/docs:/s" -w /s temp \
	sh -c "prettier --write ."
	git diff --exit-code

lint-api-docs:
	echo "$$DOCKERFILE_API_DOCS_LINT" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/api:/s" -w /s temp \
	sh -c "openapi lint openapi.yaml"

lint: lint-go lint-go-mod lint-go2api lint-docs lint-api-docs
