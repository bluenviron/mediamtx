# Install

There are several installation methods available:

- [Standalone binary](#standalone-binary): use this if you are running Windows, macOS or you just want to try out _MediaMTX_.
- [Docker container](#docker-container): use this to run _MediaMTX_ in an isolated and deterministic way. This is recommended for Linux and most production environments.
- [Arch Linux package](#arch-linux-package): use this if you are running Arch Linux and want to run the server outside a container.
- [FreeBSD package](#freebsd-package): use this if you are running FreeBSD and want to run the server outside a container.

## Standalone binary

Visit the [Releases page](https://github.com/bluenviron/mediamtx/releases) on GitHub, download and extract a standalone binary that corresponds to your operating system and architecture (example: `mediamtx_{version_tag}_linux_amd64.tar.gz`).

Then start the server by double clicking on `mediamtx` (`mediamtx.exe` on Windows) or writing in the terminal:

```sh
./mediamtx
```

Standalone binaries are provided for Linux (amd64, arm64, armv7, armv6), Windows (amd64) and macOS / Darwin (amd64, arm64). If you need a binary for a different operating system or architecture, you can [compile the server from source](../7-misc/1-compile.md).

## Docker container

Download and launch the `bluenviron/mediamtx:1` image with the following environment variables and ports:

```sh
docker run --rm -it \
-e MTX_RTSPTRANSPORTS=tcp \
-e MTX_WEBRTCADDITIONALHOSTS=192.168.x.x \
-p 8554:8554 \
-p 1935:1935 \
-p 8888:8888 \
-p 8889:8889 \
-p 8892:8892 \
-p 8890:8890/udp \
-p 8189:8189/udp \
-p 8892:8892/udp \
-p 8893:8893/udp \
bluenviron/mediamtx:1
```

Fill the `MTX_WEBRTCADDITIONALHOSTS` environment variable with the IP that will be used to connect to the server.

The `MTX_RTSPTRANSPORTS=tcp` environment variable is meant to disable the UDP transport protocol of the RTSP server (which requires the real IP address and port of incoming UDP packets, that are sometimes replaced by the Docker network stack). If you want to use it, you need to bypass the Docker network stack through the `--network=host` flag (which is not compatible with Windows, macOS and Kubernetes):

```sh
docker run --rm -it --network=host bluenviron/mediamtx:1
```

Docker images are all based on Linux and provided for the amd64, arm64 and armv7 architectures. If you need an image for a different architecture, you can [compile it from source](../7-misc/1-compile.md).

There are four image variants:

| name                             | FFmpeg included    | RPI Camera support |
| -------------------------------- | ------------------ | ------------------ |
| bluenviron/mediamtx:1            | :x:                | :x:                |
| bluenviron/mediamtx:1-ffmpeg     | :heavy_check_mark: | :x:                |
| bluenviron/mediamtx:1-rpi        | :x:                | :heavy_check_mark: |
| bluenviron/mediamtx:1-ffmpeg-rpi | :heavy_check_mark: | :heavy_check_mark: |

The `1` tag corresponds to the latest `1.x.x` release, that should guarantee backward compatibility when upgrading. It is also possible to bind the image to a specific release, by using the release name as tag (`bluenviron/mediamtx:{docker_version_tag}`).

The base image does not contain any utility, in order to minimize size and frequency of updates. If you need additional software (like curl, wget, GStreamer), you can build a custom image by creating a file named `Dockerfile` with this content:

```dockerfile
FROM bluenviron/mediamtx:1 AS mediamtx

# pick any Linux-based operating system you want.
FROM ubuntu:24.04

COPY --from=mediamtx /mediamtx /
COPY --from=mediamtx /mediamtx.yml /

# add anything you need, GStreamer is just an example.
RUN apt update && apt install -y \
   gstreamer1.0-tools

ENTRYPOINT [ "/mediamtx" ]
```

And then build it:

```sh
docker build . -t my-mediamtx
```

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
