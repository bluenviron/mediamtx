FROM golang:alpine as builder
WORKDIR /build
COPY . .
RUN go build

FROM alpine:latest
COPY --from=builder /build/rtsp-simple-server /
CMD ["/rtsp-simple-server"]
