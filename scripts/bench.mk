bench:
	docker build -q . -f bench/$(NAME)/Dockerfile -t temp
	docker run --rm -it -p 9999:9999 temp
