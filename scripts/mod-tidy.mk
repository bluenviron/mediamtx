mod-tidy:
	docker run --rm -it -v $(PWD):/s -w /s $(BASE_IMAGE) \
	sh -c "apk add git && GOPROXY=direct go mod tidy"
