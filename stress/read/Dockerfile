FROM golang:1.14-alpine3.12

RUN apk add --no-cache \
    ffmpeg

RUN wget -O /video.mkv http://jell.yfish.us/media/jellyfish-10-mbps-hd-h264.mkv

WORKDIR /s

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN go build -o /rtsp-simple-server .

COPY stress/read/start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
