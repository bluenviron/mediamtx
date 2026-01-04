# Build stage
FROM golang:1.25-alpine3.22 AS build

RUN apk add --no-cache git

WORKDIR /src
COPY . .

RUN go mod download
RUN go generate ./...

ENV CGO_ENABLED=0
RUN go build -o /mediamtx

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache \
    ffmpeg \
    curl \
    ca-certificates \
    fontconfig \
    ttf-dejavu \
    ttf-liberation

COPY --from=build /mediamtx /mediamtx

# Copy custom configuration
COPY config/mediamtx.yml /config/mediamtx.yml

ENTRYPOINT [ "/mediamtx" ]
