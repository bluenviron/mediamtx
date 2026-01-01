#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_armv6.tar.gz /linux/arm/v6
ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################
FROM --platform=linux/arm/v7 debian:bullseye-slim AS base-arm-v7

# even though the base image is arm v7,
# Raspbian libraries and compilers provide arm v6 compatibility.

RUN apt update \
	&& apt install -y wget gpg \
	&& echo "deb http://archive.raspbian.org/raspbian bullseye main rpi firmware" > /etc/apt/sources.list \
	&& echo "deb http://archive.raspberrypi.org/debian bullseye main" > /etc/apt/sources.list.d/raspi.list \
	&& wget -O- https://archive.raspbian.org/raspbian.public.key | gpg --dearmor -o /etc/apt/trusted.gpg.d/raspbian.gpg \
	&& wget -O- https://archive.raspberrypi.org/debian/raspberrypi.gpg.key | gpg --dearmor -o /etc/apt/trusted.gpg.d/raspberrypi.gpg \
	&& rm -rf /var/lib/apt/lists/*

RUN apt update && apt install --reinstall -y \
    libc6 \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

#################################################################
FROM --platform=linux/arm64 debian:bullseye-slim AS base-arm64

RUN apt update \
	&& apt install -y wget gpg \
	&& echo "deb http://archive.raspberrypi.org/debian bullseye main" > /etc/apt/sources.list.d/raspi.list \
	&& wget -O- https://archive.raspberrypi.org/debian/raspberrypi.gpg.key | gpg --dearmor -o /etc/apt/trusted.gpg.d/raspberrypi.gpg \
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
