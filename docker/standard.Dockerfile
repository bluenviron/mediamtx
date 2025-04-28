#################################################################
FROM --platform=linux/amd64 scratch AS binaries

ADD binaries/mediamtx_*_linux_amd64.tar.gz /linux/amd64
ADD binaries/mediamtx_*_linux_armv6.tar.gz /linux/arm/v6
ADD binaries/mediamtx_*_linux_armv7.tar.gz /linux/arm/v7
ADD binaries/mediamtx_*_linux_arm64.tar.gz /linux/arm64

#################################################################
FROM scratch

ARG TARGETPLATFORM
COPY --from=binaries /$TARGETPLATFORM /

ENTRYPOINT [ "/mediamtx" ]
