# Docker Build Documentation

This directory contains Dockerfiles for building MediaMTX with various configurations.

## Available Dockerfiles

### `ffmpeg.Dockerfile`
Multi-platform Dockerfile for building MediaMTX with ffmpeg support. This is the recommended Dockerfile for production use.

**Features:**
- Includes ffmpeg for video processing and transcoding
- Includes curl for health checks
- Includes font packages (DejaVu Sans Mono, Liberation) for ffmpeg's `drawtext` filter
- Supports multiple architectures: amd64, armv6, armv7, arm64

**Build:**
```bash
# Build binaries first
make binaries

# Build Docker image
docker build -f docker/ffmpeg.Dockerfile -t mediamtx:latest .
```

**Usage:**
```bash
docker run -p 8554:8554 -p 9997:9997 \
  -v /path/to/mediamtx.yml:/mediamtx.yml \
  mediamtx:latest
```

### `ffmpeg.ci.Dockerfile`
CI-specific version of the ffmpeg Dockerfile. Used by GitHub Actions workflows. Same as `ffmpeg.Dockerfile` but with paths adjusted for CI build context.

### `standard.Dockerfile`
Standard Dockerfile without ffmpeg. Use this if you don't need video processing capabilities.

### `rpi.Dockerfile` / `ffmpeg-rpi.Dockerfile`
Raspberry Pi specific Dockerfiles for ARM architectures.

## Dependencies

The ffmpeg Dockerfile includes:
- **ffmpeg**: Video/audio processing and transcoding
- **curl**: For health checks and API calls
- **fontconfig**: Font configuration system
- **ttf-dejavu**: DejaVu font family (includes DejaVu Sans Mono)
- **ttf-liberation**: Liberation font family

## Multi-Platform Builds

The Dockerfiles support building for multiple platforms:
- `linux/amd64` (x86_64)
- `linux/arm/v6` (ARMv6)
- `linux/arm/v7` (ARMv7)
- `linux/arm64` (ARM64)

To build for multiple platforms:
```bash
docker buildx create --use
docker buildx build --platform linux/amd64,linux/arm64 \
  -f docker/ffmpeg.Dockerfile \
  -t mediamtx:latest .
```

## Configuration

Mount your MediaMTX configuration file:
```bash
docker run -v /path/to/mediamtx.yml:/mediamtx.yml mediamtx:latest
```

Or use environment variables for configuration (see MediaMTX documentation).

## Ports

Default ports exposed by MediaMTX:
- `8554`: RTSP (TCP/UDP)
- `1935`: RTMP
- `8888`: HLS
- `8889`: WebRTC (HTTP)
- `8189`: WebRTC (UDP/ICE)
- `9997`: API

Example with port mappings:
```bash
docker run -p 8554:8554 -p 1935:1935 -p 8888:8888 \
  -p 8889:8889 -p 8189:8189/udp -p 9997:9997 \
  mediamtx:latest
```

## Health Checks

The image includes `curl` for health checks. Example health check:
```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:9997/v3/info"]
  interval: 30s
  timeout: 10s
  retries: 3
```

## GitHub Actions CI/CD

The `.github/workflows/docker-build.yml` workflow automatically:
1. Builds binaries for all supported platforms
2. Builds multi-platform Docker images
3. Pushes to AWS ECR registry

The workflow uses `ffmpeg.ci.Dockerfile` for CI builds.

