FROM amd64/alpine:3.14

RUN apk add --no-cache \
    vlc

RUN adduser -D -H -s /bin/sh -u 9337 user

COPY start.sh /
RUN chmod +x /start.sh

RUN mkdir /out \
    && chown user:user /out

USER user
ENTRYPOINT [ "/start.sh" ]
