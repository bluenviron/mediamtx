#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_armv6.tar.gz /linux/arm/v6
ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################

FROM --platform=linux/arm/v6 balenalib/raspberry-pi:bullseye-run-20240508 AS base-arm-v6
FROM --platform=linux/arm/v7 balenalib/raspberry-pi:bullseye-run-20240508 AS base-arm-v7
FROM --platform=linux/arm64 balenalib/raspberrypi3-64:bullseye-run-20240429 AS base-arm64

#################################################################
FROM --platform=linux/amd64 scratch AS base

COPY --from=base-arm-v6 / /linux/arm/v6
COPY --from=base-arm-v7 / /linux/arm/v7
COPY --from=base-arm64 / /linux/arm64

#################################################################
FROM scratch

ARG TARGETPLATFORM
COPY --from=base /$TARGETPLATFORM /

COPY --from=binaries /$TARGETPLATFORM /

ENTRYPOINT [ "/mediamtx" ]
