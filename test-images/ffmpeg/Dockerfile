FROM amd64/alpine:3.11

RUN apk add --no-cache \
    ffmpeg

COPY emptyvideo.ts /

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
