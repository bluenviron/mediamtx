#################################################################
# Multi-stage build for MediaMTX ARM64
# Stage 1: Build stage
#################################################################
FROM golang:1.24-alpine3.20 AS build

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    tar \
    zip \
    ca-certificates \
    tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Set build environment variables for ARM64
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=arm64 \
    GOARM=8

# Build the application
RUN go build -ldflags="-w -s" -o mediamtx

# Create distribution directory
RUN mkdir -p /dist
RUN cp mediamtx /dist/
RUN cp mediamtx.yml /dist/
RUN cp LICENSE /dist/

#################################################################
# Stage 2: Runtime stage
#################################################################
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    ffmpeg \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 mediamtx && \
    adduser -D -s /bin/sh -u 1000 -G mediamtx mediamtx

# Set working directory
WORKDIR /app

# Copy built binary and config files
COPY --from=build /dist/mediamtx /app/
COPY --from=build /dist/mediamtx.yml /app/
COPY --from=build /dist/LICENSE /app/

# Create necessary directories
RUN mkdir -p /app/recordings /app/logs && \
    chown -R mediamtx:mediamtx /app

# Switch to non-root user
USER mediamtx

# Expose default ports
EXPOSE 1935 8888 8889 9997 9998

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8888/metrics || exit 1

# Default command
ENTRYPOINT ["/app/mediamtx"]
CMD ["mediamtx.yml"]
