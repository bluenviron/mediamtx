# Upgrade

If you have an existing MediaMTX installation, you can upgrade it to the latest version. The procedure depends on how MediaMTX was installed.

## Standalone binary

The standalone binary comes with a upgrade utility that can be launched with:

```sh
./mediamtx --upgrade
```

This will replace the MediaMTX executable with its latest version. Privileges to write to the executable location are required.

## Docker image

If you used the `1` tag or the `latest` tag, remove the image from cache and re-download it:

```sh
docker rm bluenviron/mediamtx:1
docker restart id-of-mediamtx-container
```

## Arch Linux package

Repeat the installation procedure.

## FreeBSD

Repeat the installation procedure.

## OpenWrt binary

If the architecture of the OpenWrt device is amd64, armv6, armv7 or arm64, you can use the standalone binary method.

Otherwise, recompile the server from source.
