define DOCKERFILE_RUN
FROM $(BASE_IMAGE)
RUN apk add --no-cache ffmpeg
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /out .
WORKDIR /
ARG CONFIG_RUN
RUN echo "$$CONFIG_RUN" > mediamtx.yml
endef
export DOCKERFILE_RUN

define CONFIG_RUN
#rtspAddress: :8555
#rtpAddress: :8002
#rtcpAddress: :8003
#metrics: yes
#pprof: yes

paths:
  all:
#    runOnReady: ffmpeg -i rtsp://localhost:$$RTSP_PORT/$$MTX_PATH -c copy -f mpegts myfile_$$MTX_PATH.ts
#    readUser: test
#    readPass: tast
#    runOnDemand: ffmpeg -re -stream_loop -1 -i testimages/ffmpeg/emptyvideo.mkv -c copy -f rtsp rtsp://localhost:$$RTSP_PORT/$$MTX_PATH

#  proxied:
#    source: rtsp://192.168.2.198:554/stream
#    sourceProtocol: tcp
#    sourceOnDemand: yes
#    runOnDemand: ffmpeg -i rtsp://192.168.2.198:554/stream -c copy -f rtsp rtsp://localhost:$$RTSP_PORT/proxied2

#  original:
#    runOnReady: ffmpeg -i rtsp://localhost:554/original -b:a 64k -c:v libx264 -preset ultrafast -b:v 500k -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:8554/compressed

endef
export CONFIG_RUN

run:
	echo "$$DOCKERFILE_RUN" | docker build -q . -f - -t temp \
	--build-arg CONFIG_RUN="$$CONFIG_RUN"
	docker run --rm -it \
	--network=host \
	temp \
	sh -c "/out"
