bench:
	docker build -q . -f bench/$(NAME)/Dockerfile -t temp \
	--build-arg BASE_IMAGE=$(BASE_IMAGE)
	docker run --rm -it -p 9999:9999 temp
