define DOCKERFILE_PRETTIER
FROM $(NODE_IMAGE)
RUN yarn global add prettier@3.6.2
endef
export DOCKERFILE_PRETTIER

format-go:
	docker run --rm -v "$(shell pwd):/app" -w /app \
	$(GOLANGCI_LINT_IMAGE) \
	golangci-lint fmt

format-other:
	echo "$$DOCKERFILE_PRETTIER" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/:/s" -w /s temp \
	sh -c "prettier --write ."

format: format-go format-other
