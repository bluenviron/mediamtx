FROM golang:1.22 AS builder

LABEL maintainer="Magnus Gule <magnus.gule@piscada.com>" \
    description="MediaMTX with gst-launch and gst-rstp-server included"

WORKDIR /usr/src/app

COPY . .

RUN go generate ./...

# Build for linux/amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build .

FROM debian:sid-slim AS production 

WORKDIR /root

# Copy the built binary
COPY --from=builder /usr/src/app/mediamtx .
COPY --from=builder /usr/src/app/config/mediamtx.yml ./config/mediamtx.yml
# VOLUME [ "/config" ]

COPY pipeme .

RUN chmod +x ./mediamtx
RUN chmod +x ./pipeme

# Install GStreamer pipeline dependencies
RUN apt update && apt install -y --no-install-recommends \
    gstreamer1.0-tools \
    gstreamer1.0-rtsp \
    jq \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Verify installation of curl
RUN curl --version

# Set the entry point to the startup script
CMD ["./mediamtx", "./config/mediamtx.yml"]

