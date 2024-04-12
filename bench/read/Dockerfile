ARG BASE_IMAGE
FROM $BASE_IMAGE

RUN apk add --no-cache \
    ffmpeg

RUN wget -O /video.mkv http://www.larmoire.info/jellyfish/media/jellyfish-10-mbps-hd-h264.mkv

WORKDIR /s

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN go generate ./...
RUN go build -o /mediamtx .

COPY bench/read/start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
