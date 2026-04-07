define DOCKERFILE_FORMAT
FROM $(BASE_IMAGE)
RUN go install mvdan.cc/gofumpt@v0.5.0
endef
export DOCKERFILE_FORMAT

define DOCKERFILE_PRETTIER
FROM $(NODE_IMAGE)
RUN yarn global add prettier@3.6.2
endef
export DOCKERFILE_PRETTIER

format-go:
	echo "$$DOCKERFILE_FORMAT" | docker build -q . -f - -t temp
	docker run --rm -it -v "$(shell pwd):/s" -w /s temp \
	sh -c "gofumpt -l -w ."

format-docs:
	echo "$$DOCKERFILE_PRETTIER" | docker build . -f - -t temp
	docker run --rm -v "$(shell pwd)/docs:/s" -w /s temp \
	sh -c "prettier --write ."

format: format-go format-docs
