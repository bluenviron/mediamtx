define DOCKERFILE_FORMAT
FROM $(BASE_IMAGE)
RUN go install mvdan.cc/gofumpt@v0.5.0
endef
export DOCKERFILE_FORMAT

format:
	echo "$$DOCKERFILE_FORMAT" | docker build -q . -f - -t temp
	docker run --rm -it -v $(PWD):/s -w /s temp \
	sh -c "gofumpt -l -w ."
