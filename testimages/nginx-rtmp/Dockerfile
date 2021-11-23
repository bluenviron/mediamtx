FROM amd64/alpine:3.14

RUN apk add --no-cache \
    nginx-mod-rtmp

COPY nginx.conf /etc/nginx/

ENTRYPOINT [ "nginx", "-g", "daemon off;" ]
