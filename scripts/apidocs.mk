define DOCKERFILE_APIDOCS_LINT
FROM $(NODE_IMAGE)
RUN yarn global add @redocly/openapi-cli@1.0.0-beta.82
endef
export DOCKERFILE_APIDOCS_LINT

apidocs-lint:
	echo "$$DOCKERFILE_APIDOCS_LINT" | docker build . -f - -t temp
	docker run --rm -v $(PWD)/apidocs:/s -w /s temp \
	sh -c "openapi lint openapi.yaml"

define DOCKERFILE_APIDOCS_GEN
FROM $(NODE_IMAGE)
RUN yarn global add redoc-cli@0.13.7
endef
export DOCKERFILE_APIDOCS_GEN

apidocs-gen:
	echo "$$DOCKERFILE_APIDOCS_GEN" | docker build . -f - -t temp
	docker run --rm -v $(PWD)/apidocs:/s -w /s temp \
	sh -c "redoc-cli bundle openapi.yaml"
