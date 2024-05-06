FROM golang:1.22 AS builder

LABEL maintainer="Magnus Gule <magnus.gule@piscada.com>" \
    description="MediaMTX with gst-launch and gst-rstp-server included"

WORKDIR /usr/src/app

COPY . .

RUN go generate ./...

# Build for linux/amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .

FROM debian:sid-20240423-slim AS production 

WORKDIR /root

# Copy the built binary
COPY --from=builder /usr/src/app/mediamtx .
COPY --from=builder /usr/src/app/mediamtx.yml ./mediamtx.yml

# Install GStreamer pipeline dependencies
RUN apt update && apt install -y --no-install-recommends \
    gstreamer1.0-tools \
    gstreamer1.0-rtsp \
    && rm -rf /var/lib/apt/lists/*

# Set the entry point to the startup script
ENTRYPOINT ["./mediamtx"]