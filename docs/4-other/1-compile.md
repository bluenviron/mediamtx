# Compile from source

## Standard method

Install git and Go &ge; 1.25. Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 go build .
```

The command will produce the `mediamtx` binary.

## OpenWrt

The compilation procedure is the same as the standard one. On the OpenWrt device, install git and Go:

```sh
opkg update
opkg install golang git git-http
```

Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 go build .
```

The command will produce the `mediamtx` binary.

If the OpenWrt device doesn't have enough resources to compile, you can [cross compile](#cross-compile) from another machine.

## Custom libcamera

If you need to use a custom or external libcamera when interacting with the Raspberry Pi Camera, some additional steps are needed:

1. Download [mediamtx-rpicamera source code](https://github.com/bluenviron/mediamtx-rpicamera) and compile it against the external libcamera. Instructions are in the repository.

2. Download [MediaMTX source code](https://github.com/bluenviron/mediamtx) and run:

   ```sh
   go generate ./...
   ```

3. Copy `build/mtxrpicam_32` and/or `build/mtxrpicam_64` (depending on the architecture) from `mediamtx-rpicamera` to `mediamtx`, inside folder `internal/staticsources/rpicamera/`, overriding existing folders.

4. Compile:

   ```sh
   go run .
   ```

## Cross compile

Cross compilation allows to build an executable for a target machine from another machine with different operating system or architecture. This is useful in case the target machine doesn't have enough resources for compilation or if you don't want to install the compilation dependencies on it.

On the machine you want to use to compile, install git and Go &ge; 1.25. Clone the repository, enter into the folder and start the building process:

```sh
git clone https://github.com/bluenviron/mediamtx
cd mediamtx
go generate ./...
CGO_ENABLED=0 GOOS=my_os GOARCH=my_arch go build .
```

Replace `my_os` and `my_arch` with the operating system and architecture of your target machine. A list of all supported combinations can be obtained with:

```sh
go tool dist list
```

For instance:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build .
```

In case of the `arm` architecture, there's an additional flag available, `GOARM`, that allows to set the ARM version:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOARM=7 go build .
```

In case of the `mips` architecture, there's an additional flag available, `GOMIPS`, that allows to set additional parameters:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build .
```

The command will produce the `mediamtx` binary.

## Compile for all supported platforms

Install Docker and launch:

```sh
make binaries
```

The command will produce tarballs in folder `binaries/`.

## Docker image

The official Docker image can be recompiled by following these steps:

1. Build binaries for all supported platforms:

   ```sh
   make binaries
   ```

2. Build the image by using one of the Dockerfiles inside the `docker/` folder:

   ```
   docker build . -f docker/standard.Dockerfile -t my-mediamtx
   ```

   A Dockerfile is available for each image variant (`standard.Dockerfile`, `ffmpeg.Dockerfile`, `rpi.Dockerfile`, `ffmpeg-rpi.Dockerfile`).
