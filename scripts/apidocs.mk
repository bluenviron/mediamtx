define DOCKERFILE_APIDOCS_GEN
FROM $(NODE_IMAGE)
RUN yarn global add redoc-cli@0.13.7
endef
export DOCKERFILE_APIDOCS_GEN

apidocs:
	echo "$$DOCKERFILE_APIDOCS_GEN" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/apidocs:/s" -w /s temp \
	sh -c "redoc-cli bundle openapi.yaml"
