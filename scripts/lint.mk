define DOCKERFILE_APIDOCS_LINT
FROM $(NODE_IMAGE)
RUN yarn global add @redocly/cli@1.0.0-beta.123
endef
export DOCKERFILE_APIDOCS_LINT

lint-go:
	docker run --rm -v "$(shell pwd):/app" -w /app \
	$(GOLANGCI_LINT_IMAGE) \
	golangci-lint run -v

lint-go-mod:
	go mod tidy -diff

lint-conf:
	go test -v -tags enable_linters ./internal/linters/conf

lint-docslinks:
	go test -v -tags enable_linters ./internal/linters/docslinks

lint-docsorder:
	go test -v -tags enable_linters ./internal/linters/docsorder

lint-apidocs:
	go run ./internal/apidocsgen --check
	echo "$$DOCKERFILE_APIDOCS_LINT" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/api:/s" -w /s temp \
	sh -c "openapi lint openapi.yaml"

lint-other:
	echo "$$DOCKERFILE_PRETTIER" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/:/s" -w /s temp \
	sh -c "prettier --check ."

lint: lint-go lint-go-mod lint-conf lint-docslinks lint-docsorder lint-apidocs lint-other
