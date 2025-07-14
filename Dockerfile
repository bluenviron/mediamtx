FROM golang:1.24-alpine3.20 AS build-base

RUN apk add --no-cache zip make git tar
WORKDIR /s
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

ENV CGO_ENABLED=0
RUN rm -rf tmp binaries &&\
	mkdir tmp binaries &&\
	cp mediamtx.yml LICENSE tmp/ &&\
	go generate ./...

ENV GOOS=linux GOARCH=amd64
RUN go build -o /mediamtx

FROM alpine:3.20
COPY --from=build-base /mediamtx /mediamtx
ENTRYPOINT [ "/mediamtx" ]
