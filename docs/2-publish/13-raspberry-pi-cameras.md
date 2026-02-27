# Raspberry Pi Cameras

_MediaMTX_ natively supports most Raspberry Pi Camera models, enabling high-quality and low-latency video streaming from the camera to any user, for any purpose. There are some additional requirements:

1. The server must run on a Raspberry Pi, with one of the following operating systems:
   - Raspberry Pi OS Trixie
   - Raspberry Pi OS Bookworm
   - Raspberry Pi OS Bullseye

   Both 32-bit and 64-bit architectures are supported.

2. If you are using Raspberry Pi OS Bullseye, make sure that the legacy camera stack is disabled. Type `sudo raspi-config`, then go to `Interfacing options`, `enable/disable legacy camera support`, choose `no`. Reboot the system.

The setup procedure depends on whether you want to run the server outside or inside Docker:

- If you want to run the standard (non-Dockerized) version of the server:
  1. Download the server executable. If you're using 64-bit version of the operative system, make sure to pick the `arm64` variant.

  2. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

  ```yml
  paths:
    cam:
      source: rpiCamera
  ```

  The resulting stream will be available on path `/cam`.

- If you want to run the server inside Docker, you need to use the `1-rpi` image and launch the container with some additional flags:

  ```sh
  docker run --rm -it \
  --network=host \
  --privileged \
  --tmpfs /dev/shm:exec \
  -v /run/udev:/run/udev:ro \
  -e MTX_PATHS_CAM_SOURCE=rpiCamera \
  bluenviron/mediamtx:1-rpi
  ```

The Raspberry Pi Camera can be controlled through a wide range of parameters, that are listed in the [configuration file](../5-references/1-configuration-file.md).

Be aware that cameras that require a custom `libcamera` (like some ArduCam products) are not compatible with precompiled binaries and Docker images of _MediaMTX_, since these come with a bundled `libcamera`. If you want to use a custom one, you need to [compile from source](../6-misc/1-compile.md#custom-libcamera).

## Adding audio

In order to add audio from a USB microfone, install GStreamer and alsa-utils:

```sh
sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp gstreamer1.0-alsa alsa-utils
```

list available audio cards with:

```sh
arecord -L
```

Sample output:

```
surround51:CARD=ICH5,DEV=0
    Intel ICH5, Intel ICH5
    5.1 Surround output to Front, Center, Rear and Subwoofer speakers
default:CARD=U0x46d0x809
    USB Device 0x46d:0x809, USB Audio
    Default Audio Device
```

Find the audio card of the microfone and take note of its name, for instance `default:CARD=U0x46d0x809`. Then create a new path that takes the video stream from the camera and audio from the microphone:

```yml
paths:
  cam:
    source: rpiCamera

  cam_with_audio:
    runOnInit: >
      gst-launch-1.0
      rtspclientsink name=s location=rtsp://localhost:$RTSP_PORT/cam_with_audio
      rtspsrc location=rtsp://127.0.0.1:$RTSP_PORT/cam latency=0 ! rtph264depay ! s.
      alsasrc device=default:CARD=U0x46d0x809 ! opusenc bitrate=16000 ! s.
    runOnInitRestart: yes
```

The resulting stream will be available on path `/cam_with_audio`.

## Secondary stream

It is possible to enable a secondary stream from the same camera, with a different resolution, FPS and codec. Configuration is the same of a primary stream, with `rpiCameraSecondary` set to `true` and parameters adjusted accordingly:

```yml
paths:
  # primary stream
  rpi:
    source: rpiCamera
    # Width of frames.
    rpiCameraWidth: 1920
    # Height of frames.
    rpiCameraHeight: 1080
    # FPS.
    rpiCameraFPS: 30

  # secondary stream
  secondary:
    source: rpiCamera
    # This is a secondary stream.
    rpiCameraSecondary: true
    # Width of frames.
    rpiCameraWidth: 640
    # Height of frames.
    rpiCameraHeight: 480
    # FPS.
    rpiCameraFPS: 10
    # Codec. in case of secondary streams, it defaults to M-JPEG.
    rpiCameraCodec: auto
    # JPEG quality.
    rpiCameraMJPEGQuality: 60
```

The secondary stream will be available on path `/secondary`.
