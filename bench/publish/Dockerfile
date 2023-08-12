FROM golang:1.20-alpine3.18

RUN apk add --no-cache \
    ffmpeg

RUN wget -O /video.mkv http://jell.yfish.us/media/jellyfish-10-mbps-hd-h264.mkv

WORKDIR /s

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go build -o /mediamtx .

COPY bench/publish/start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
