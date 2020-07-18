FROM amd64/alpine:3.12

RUN apk add --no-cache \
    gstreamer-tools \
    gst-plugins-good \
    gst-plugins-bad \
    && apk add --no-cache \
    -X http://dl-cdn.alpinelinux.org/alpine/edge/testing \
    gst-rtsp-server

COPY emptyvideo.ts /

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
