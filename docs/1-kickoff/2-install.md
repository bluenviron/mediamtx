# Install

There are several installation methods available:

- [Standalone binary](#standalone-binary): use this if you are running Windows, macOS or you just want to try out _MediaMTX_.
- [Docker image](#docker-image): use this if you want to run _MediaMTX_ in an isolated and deterministic way. This is recommended for production environments.
- [Arch Linux package](#arch-linux-package): use this if you are running Arch Linux.
- [FreeBSD package](#freebsd-package): use this if you are running FreeBSD.
- [OpenWrt binary](#openwrt-binary): use this if you are running OpenWrt.

## Standalone binary

1. Visit the [Releases page](https://github.com/bluenviron/mediamtx/releases) on GitHub, download and extract a standalone binary that corresponds to your operating system and architecture (example: `mediamtx_{version_tag}_linux_amd64.tar.gz`).

2. Start the server by double clicking on `mediamtx` (`mediamtx.exe` on Windows) or writing in the terminal:

   ```sh
   ./mediamtx
   ```

## Docker image

Download and launch the `bluenviron/mediamtx:1` image with the following environment variables and ports:

```sh
docker run --rm -it \
-e MTX_RTSPTRANSPORTS=tcp \
-e MTX_WEBRTCADDITIONALHOSTS=192.168.x.x \
-p 8554:8554 \
-p 1935:1935 \
-p 8888:8888 \
-p 8889:8889 \
-p 8890:8890/udp \
-p 8189:8189/udp \
bluenviron/mediamtx:1
```

Fill the `MTX_WEBRTCADDITIONALHOSTS` environment variable with the IP that will be used to connect to the server.

The `MTX_RTSPTRANSPORTS=tcp` environment variable is meant to disable the UDP transport protocol of the RTSP server (which require the real IP address and port of incoming UDP packets, that are sometimes replaced by the Docker network stack). If you want to use it, you need to bypass the Docker network stack through the `--network=host` flag (which is not compatible with Windows, macOS and Kubernetes):

```sh
docker run --rm -it --network=host bluenviron/mediamtx:1
```

There are four image variants:

| name                             | FFmpeg included    | RPI Camera support |
| -------------------------------- | ------------------ | ------------------ |
| bluenviron/mediamtx:1            | :x:                | :x:                |
| bluenviron/mediamtx:1-ffmpeg     | :heavy_check_mark: | :x:                |
| bluenviron/mediamtx:1-rpi        | :x:                | :heavy_check_mark: |
| bluenviron/mediamtx:1-ffmpeg-rpi | :heavy_check_mark: | :heavy_check_mark: |

The `1` tag corresponds to the latest `1.x.x` release, that should guarantee backward compatibility when upgrading. It is also possible to bind the image to a specific release, by using the release name as tag (`bluenviron/mediamtx:{docker_version_tag}`).

The base image does not contain any utility, in order to minimize size and frequency of updates. If you need additional software (like curl, wget, GStreamer), you can build a custom image by creating a file named `Dockerfile` with this content:

```Dockerfile
FROM bluenviron/mediamtx:1 AS mediamtx
FROM ubuntu:24.04

COPY --from=mediamtx /mediamtx /
COPY --from=mediamtx /mediamtx.yml /

# add anything you need.
RUN apt update && apt install -y \
   gstreamer1.0-tools

ENTRYPOINT [ "/mediamtx" ]
```

And then build it:

```sh
docker build . -t my-mediamtx
```

In particular, the custom image is using the official _MediaMTX_ image as a base stage, and then adds a Linux-based operating system on top of it. Since _MediaMTX_ binaries are not tied to a specific Linux distribution or version, you can use anything you like.

## Arch Linux package

If you are running the Arch Linux distribution, launch:

```sh
git clone https://aur.archlinux.org/mediamtx.git
cd mediamtx
makepkg -si
```

## FreeBSD package

Available via ports tree or using packages (2025Q2 and later) as listed below:

```sh
cd /usr/ports/multimedia/mediamtx && make install clean
pkg install mediamtx
```

## OpenWrt binary

If the architecture of the OpenWrt device is amd64, armv6, armv7 or arm64, use the [standalone binary method](#standalone-binary) and download a Linux binary that corresponds to your architecture.

Otherwise, [compile the server from source](../6-misc/1-compile.md).
