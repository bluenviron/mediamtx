FROM golang:1.17-alpine3.14

RUN apk add --no-cache \
    ffmpeg

RUN wget -O /video.mkv http://jell.yfish.us/media/jellyfish-10-mbps-hd-h264.mkv

WORKDIR /s

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o /rtsp-simple-server .

COPY bench/publish/start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
