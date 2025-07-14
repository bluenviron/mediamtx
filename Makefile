.PHONY: login build push

login:
	aws ecr get-login-password --region ap-northeast-1 | docker login --username AWS --password-stdin 870718863047.dkr.ecr.ap-northeast-1.amazonaws.com/gb-media-server

build:
	docker build --platform linux/amd64 -t 870718863047.dkr.ecr.ap-northeast-1.amazonaws.com/gb-media-server:latest .
	
push: login build
	docker push 870718863047.dkr.ecr.ap-northeast-1.amazonaws.com/gb-media-server:latest