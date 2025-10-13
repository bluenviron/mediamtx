# Install

There are several installation methods available: standalone binary, Docker image, Arch Linux package, FreeBSD Ports Collection or package and OpenWrt binary.

## Standalone binary

1. Visit the [Releases page](https://github.com/bluenviron/mediamtx/releases) on GitHub, download and extract a standalone binary that corresponds to your operating system and architecture (example: `mediamtx_{version_tag}_linux_amd64.tar.gz`).

2. Start the server:

   ```sh
   ./mediamtx
   ```

## Docker image

Download and launch the image:

```sh
docker run --rm -it --network=host bluenviron/mediamtx:1
```

The `1` tag corresponds to the latest `1.x.x` release, that should guarantee backward compatibility when upgrading. It is also possible to bind the image to a specific release, by using the release name as tag (`bluenviron/mediamtx:{docker_version_tag}`).

There are four image variants:

| name                             | FFmpeg included    | RPI Camera support |
| -------------------------------- | ------------------ | ------------------ |
| bluenviron/mediamtx:1            | :x:                | :x:                |
| bluenviron/mediamtx:1-ffmpeg     | :heavy_check_mark: | :x:                |
| bluenviron/mediamtx:1-rpi        | :x:                | :heavy_check_mark: |
| bluenviron/mediamtx:1-ffmpeg-rpi | :heavy_check_mark: | :heavy_check_mark: |

The `--network=host` flag is mandatory for RTSP to work, since Docker can change the source port of UDP packets for routing reasons, and this doesn't allow the server to identify the senders of the packets.

If the `--network=host` cannot be used (for instance, it is not compatible with Windows or Kubernetes), you can disable the RTSP UDP transport protocol, add the server IP to `MTX_WEBRTCADDITIONALHOSTS` and expose ports manually:

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
bluenviron/mediamtx
```

## Arch Linux package

If you are running the Arch Linux distribution, run:

```sh
git clone https://aur.archlinux.org/mediamtx.git
cd mediamtx
makepkg -si
```

## FreeBSD

Available via ports tree or using packages (2025Q2 and later) as listed below:

```sh
cd /usr/ports/multimedia/mediamtx && make install clean
pkg install mediamtx
```

## OpenWrt binary

If the architecture of the OpenWrt device is amd64, armv6, armv7 or arm64, use the [standalone binary method](#standalone-binary) and download a Linux binary that corresponds to your architecture.

Otherwise, [compile the server from source](/docs/other/compile).
