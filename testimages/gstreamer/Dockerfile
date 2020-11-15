FROM amd64/ubuntu:20.04

RUN apt update && apt install -y --no-install-recommends \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-rtsp \
    && rm -rf /var/lib/apt/lists/*

COPY emptyvideo.ts /

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
