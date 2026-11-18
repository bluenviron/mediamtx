#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################
FROM --platform=linux/arm/v7 debian:trixie-slim AS base-arm-v7

RUN apt update \
	&& apt install -y wget gpg \
	&& wget -O /usr/share/keyrings/raspberrypi-archive-keyring.pgp \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspberrypi-archive-keyring.pgp \
	&& wget -O /etc/apt/sources.list.d/raspi.sources \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspi.sources \
	&& sed -i "s/RELEASE/trixie/g" /etc/apt/sources.list.d/raspi.sources \
	&& rm -rf /var/lib/apt/lists/*

#################################################################
FROM --platform=linux/arm64 debian:trixie-slim AS base-arm64

RUN apt update \
	&& apt install -y wget gpg \
	&& wget -O /usr/share/keyrings/raspberrypi-archive-keyring.pgp \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspberrypi-archive-keyring.pgp \
	&& wget -O /etc/apt/sources.list.d/raspi.sources \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspi.sources \
	&& sed -i "s/RELEASE/trixie/g" /etc/apt/sources.list.d/raspi.sources \
	&& rm -rf /var/lib/apt/lists/*

#################################################################
FROM --platform=linux/amd64 scratch AS base

COPY --from=base-arm-v7 / /linux/arm/v7
COPY --from=base-arm64 / /linux/arm64

#################################################################
FROM scratch

ARG TARGETPLATFORM
COPY --from=base /$TARGETPLATFORM /

COPY --from=binaries /$TARGETPLATFORM /

ENTRYPOINT [ "/mediamtx" ]
