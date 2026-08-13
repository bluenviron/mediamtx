# Custom build for csiserv/mediamtx (branch: feat/patched-mediacommon-hevc-sublayer).
#
# This fork replaces github.com/bluenviron/mediacommon/v2 with
# csiserv/mediacommon (fix/hevc-sps-sublayer-profile-tier-level) via a go.mod
# replace directive, until the SPS sub-layer profile_tier_level fix lands in
# upstream mediacommon. Mirrors upstream's own docker/ffmpeg.Dockerfile final
# stage (alpine + ffmpeg), but builds directly from source for amd64 only
# instead of upstream's multi-arch release-tarball pipeline (scripts/binaries.mk),
# since this is a single-arch dev/prod deployment, not a public release.

FROM golang:1.26-alpine3.23 AS build
RUN apk add --no-cache git
WORKDIR /s
COPY . ./
ENV CGO_ENABLED=0
RUN go generate ./...
RUN GOOS=linux GOARCH=amd64 go build -o /mediamtx .

FROM alpine:3.23
RUN apk add --no-cache ffmpeg
COPY --from=build /mediamtx /mediamtx
COPY --from=build /s/mediamtx.yml /mediamtx.yml
ENTRYPOINT [ "/mediamtx" ]
