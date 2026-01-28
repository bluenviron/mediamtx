# Upgrade

If you have an existing _MediaMTX_ installation, you can upgrade it to the latest version. The procedure depends on how _MediaMTX_ was installed.

## Standalone binary

The standalone binary comes with a upgrade utility that can be launched with:

```sh
./mediamtx --upgrade
```

This will replace the _MediaMTX_ executable with its latest version. Privileges to write to the executable location are required.

## Docker image

Stop and remove the container:

```sh
docker stop id-of-mediamtx-container
docker rm id-of-mediamtx-container
```

Remove the _MediaMTX_ image from cache:

```sh
docker rm bluenviron/mediamtx:1
```

Then recreate the container as described in [Install](install#docker-image).

## Arch Linux package

Repeat the installation procedure.

## FreeBSD

Repeat the installation procedure.

## OpenWrt binary

If the architecture of the OpenWrt device is amd64, armv6, armv7 or arm64, you can use the standalone binary method.

Otherwise, recompile the server from source.
