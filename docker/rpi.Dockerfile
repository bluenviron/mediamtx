#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_armv6.tar.gz /linux/arm/v6
ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################
FROM --platform=linux/arm/v7 debian:trixie-slim AS base-arm-v7

# even though the base image is armv7,
# Raspbian libraries and compilers provide armv6 compatibility.

RUN apt update \
	&& apt install -y wget gpg \
	&& wget -O /raspbian-archive-keyring.deb \
		https://archive.raspbian.org/raspbian/pool/main/r/raspbian-archive-keyring/raspbian-archive-keyring_20120528.4_all.deb \
	&& dpkg -i /raspbian-archive-keyring.deb \
	&& rm /raspbian-archive-keyring.deb \
	&& wget -O /etc/apt/sources.list.d/raspbian.sources \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspbian.sources \
	&& sed -i "s/RELEASE/trixie/g" /etc/apt/sources.list.d/raspbian.sources \
	&& wget -O /usr/share/keyrings/raspberrypi-archive-keyring.pgp \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspberrypi-archive-keyring.pgp \
	&& wget -O /etc/apt/sources.list.d/raspi.sources \
		https://github.com/RPi-Distro/pi-gen/raw/358f9785089fa9fce397b7c36de2d90a9ae9a50e/stage0/00-configure-apt/files/raspi.sources \
	&& sed -i "s/RELEASE/trixie/g" /etc/apt/sources.list.d/raspi.sources \
	&& rm -rf /var/lib/apt/lists/*

RUN apt update && apt install --reinstall -y \
    libc6 \
    libc-bin \
    libstdc++6 \
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

COPY --from=base-arm-v7 / /linux/arm/v6
COPY --from=base-arm-v7 / /linux/arm/v7
COPY --from=base-arm64 / /linux/arm64

#################################################################
FROM scratch

ARG TARGETPLATFORM
COPY --from=base /$TARGETPLATFORM /

COPY --from=binaries /$TARGETPLATFORM /

ENTRYPOINT [ "/mediamtx" ]
