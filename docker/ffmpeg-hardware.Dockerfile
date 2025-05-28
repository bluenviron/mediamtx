#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_amd64.tar.gz /linux/amd64
ADD binaries/mediamtx_*_linux_armv6.tar.gz /linux/arm/v6
ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################
FROM debian:trixie-slim

RUN  echo 'deb http://deb.debian.org/debian trixie non-free' > /etc/apt/sources.list.d/debian-non-free.list && \
     apt-get -y update && apt-get -y install ffmpeg alsa-utils  libasound2-plugins intel-media-va-driver-non-free mesa-va-drivers && \
     apt-get clean && rm -rf /var/lib/apt/lists/*

ARG TARGETPLATFORM
COPY --from=binaries /$TARGETPLATFORM /

ENTRYPOINT [ "/mediamtx" ]
