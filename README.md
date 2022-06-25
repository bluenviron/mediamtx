
<p align="center">
    <img src="logo.png" alt="rtsp-simple-server">
</p>

_rtsp-simple-server_ is a ready-to-use and zero-dependency server and proxy that allows users to publish, read and proxy live video and audio streams through various protocols:

|protocol|description|publish|read|proxy|
|--------|-----------|-------|----|-----|
|RTSP|fastest way to publish and read streams|:heavy_check_mark:|:heavy_check_mark:|:heavy_check_mark:|
|RTMP|allows to interact with legacy software|:heavy_check_mark:|:heavy_check_mark:|:heavy_check_mark:|
|Low-Latency HLS|allows to embed streams into a web page|:x:|:heavy_check_mark:|:heavy_check_mark:|

Features:

* Publish live streams to the server
* Read live streams from the server
* Act as a proxy and serve streams from other servers or cameras, always or on-demand
* Each stream can have multiple video and audio tracks, encoded with any codec, including H264, H265, VP8, VP9, MPEG2, MP3, AAC, Opus, PCM, JPEG
* Streams are automatically converted from a protocol to another. For instance, it's possible to publish a stream with RTSP and read it with HLS
* Serve multiple streams at once in separate paths
* Authenticate users; use internal or external authentication
* Query and control the server through an HTTP API
* Read Prometheus-compatible metrics
* Redirect readers to other RTSP servers (load balancing)
* Run external commands when clients connect, disconnect, read or publish streams
* Reload the configuration without disconnecting existing clients (hot reloading)
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

[![Test](https://github.com/aler9/rtsp-simple-server/workflows/test/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions?query=workflow:test)
[![Lint](https://github.com/aler9/rtsp-simple-server/workflows/lint/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions?query=workflow:lint)
[![CodeCov](https://codecov.io/gh/aler9/rtsp-simple-server/branch/main/graph/badge.svg)](https://codecov.io/gh/aler9/rtsp-simple-server/branch/main)
[![Release](https://img.shields.io/github/v/release/aler9/rtsp-simple-server)](https://github.com/aler9/rtsp-simple-server/releases)
[![Docker Hub](https://img.shields.io/badge/docker-aler9/rtsp--simple--server-blue)](https://hub.docker.com/r/aler9/rtsp-simple-server)
[![API Documentation](https://img.shields.io/badge/api-documentation-blue)](https://aler9.github.io/rtsp-simple-server)

## Table of contents

* [Installation](#installation)
  * [Standard](#standard)
  * [Docker](#docker)
* [Basic usage](#basic-usage)
* [General](#general)
  * [Configuration](#configuration)
  * [Authentication](#authentication)
  * [Encrypt the configuration](#encrypt-the-configuration)
  * [Proxy mode](#proxy-mode)
  * [Remuxing, re-encoding, compression](#remuxing-re-encoding-compression)
  * [Save streams to disk](#save-streams-to-disk)
  * [On-demand publishing](#on-demand-publishing)
  * [Start on boot](#start-on-boot)
    * [Linux](#linux)
    * [Windows](#windows)
  * [HTTP API](#http-api)
  * [Metrics](#metrics)
  * [pprof](#pprof)
  * [Compile and run from source](#compile-and-run-from-source)
* [Publish to the server](#publish-to-the-server)
  * [From a webcam](#from-a-webcam)
  * [From a Raspberry Pi Camera](#from-a-raspberry-pi-camera)
  * [From OBS Studio](#from-obs-studio)
  * [From OpenCV](#from-opencv)
* [Read from the server](#read-from-the-server)
  * [From VLC and Ubuntu](#from-vlc-and-ubuntu)
* [RTSP protocol](#rtsp-protocol)
  * [RTSP general usage](#rtsp-general-usage)
  * [TCP transport](#tcp-transport)
  * [UDP-multicast transport](#udp-multicast-transport)
  * [Encryption](#encryption)
  * [Redirect to another server](#redirect-to-another-server)
  * [Fallback stream](#fallback-stream)
  * [Corrupted frames](#corrupted-frames)
* [RTMP protocol](#rtmp-protocol)
  * [RTMP general usage](#rtmp-general-usage)
* [HLS protocol](#hls-protocol)
  * [HLS general usage](#hls-general-usage)
  * [Embedding](#embedding)
  * [Low-Latency variant](#low-latency-variant)
  * [Decreasing latency](#decreasing-latency)
* [Links](#links)

## Installation

### Standard

1. Download and extract a precompiled binary from the [release page](https://github.com/aler9/rtsp-simple-server/releases).

2. Start the server:

   ```
   ./rtsp-simple-server
   ```

### Docker

Download and launch the image:

```
docker run --rm -it --network=host aler9/rtsp-simple-server
```

The `--network=host` flag is mandatory since Docker can change the source port of UDP packets for routing reasons, and this doesn't allow the server to find out the author of the packets. This issue can be avoided by disabling the UDP transport protocol:

```
docker run --rm -it -e RTSP_PROTOCOLS=tcp -p 8554:8554 -p 1935:1935 -p 8888:8888 aler9/rtsp-simple-server
```

Please keep in mind that the Docker image doesn't include _FFmpeg_. if you need to use _FFmpeg_ for an external command or anything else, you need to build a Docker image that contains both _rtsp-simple-server_ and _FFmpeg_, by following instructions [here](https://github.com/aler9/rtsp-simple-server/discussions/278#discussioncomment-549104).

## Basic usage

1. Publish a stream. For instance, you can publish a video/audio file with _FFmpeg_:

   ```
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream filesrc location=file.mp4 ! qtdemux name=d d.video_0 ! queue ! s.sink_0 d.audio_0 ! queue ! s.sink_1
   ```

   To publish from other hardware / software, take a look at the [Publish to the server](#publish-to-the-server) section.

2. Open the stream. For instance, you can open the stream with _VLC_:

   ```
   vlc rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-play-1.0 rtsp://localhost:8554/mystream
   ```

   or _FFmpeg_:

   ```
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```

## General

### Configuration

All the configuration parameters are listed and commented in the [configuration file](rtsp-simple-server.yml).

There are 3 ways to change the configuration:

1. By editing the `rtsp-simple-server.yml` file, that is

   * included into the release bundle
   * available in the root folder of the Docker image (`/rtsp-simple-server.yml`); it can be overridden in this way:

     ```
     docker run --rm -it --network=host -v $PWD/rtsp-simple-server.yml:/rtsp-simple-server.yml aler9/rtsp-simple-server
     ```

   The configuration can be changed dynamically when the server is running (hot reloading) by writing to the configuration file. Changes are detected and applied without disconnecting existing clients, whenever it's possible.

2. By overriding configuration parameters with environment variables, in the format `RTSP_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspAddress` parameter can be overridden in the following way:

   ```
   RTSP_RTSPADDRESS="127.0.0.1:8554" ./rtsp-simple-server
   ```

   Parameters in maps can be overridden by using underscores, in the following way:

   ```
   RTSP_PATHS_TEST_SOURCE=rtsp://myurl ./rtsp-simple-server
   ```

   This method is particularly useful when using Docker; any configuration parameter can be changed by passing environment variables with the `-e` flag:

   ```
   docker run --rm -it --network=host -e RTSP_PATHS_TEST_SOURCE=rtsp://myurl aler9/rtsp-simple-server
   ```

3. By using the [HTTP API](#http-api).

### Authentication

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
    publishUser: myuser
    publishPass: mypass
```

Only publishers that provide both username and password will be able to proceed:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://myuser:mypass@localhost:8554/mystream
```

It's possible to setup authentication for readers too:

```yml
paths:
  all:
    publishUser: myuser
    publishPass: mypass

    readUser: user
    readPass: userpass
```

If storing plain credentials in the configuration file is a security problem, username and passwords can be stored as sha256-hashed strings; a string must be hashed with sha256 and encoded with base64:

```
echo -n "userpass" | openssl dgst -binary -sha256 | openssl base64
```

Then stored with the `sha256:` prefix:

```yml
paths:
  all:
    readUser: sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo=
    readPass: sha256:BdSWkrdV+ZxFBLUQQY7+7uv9RmiSVA8nrPmjGjJtZQQ=
```

**WARNING**: enable encryption or use a VPN to ensure that no one is intercepting the credentials.

Authentication can be delegated to an external HTTP server:

```yml
externalAuthenticationURL: http://myauthserver/auth
```

Each time a user needs to be authenticated, the specified URL will be requested with the POST method and this payload:

```json
{
  "ip": "ip",
  "user": "user",
  "password": "password",
  "path": "path",
  "action": "read|publish"
}
```

If the URL returns a status code that begins with `20` (i.e. `200`), authentication is successful, otherwise it fails.

Please be aware that it's perfectly normal for the authentication server to receive requests with empty users and passwords, i.e.:

```json
{
  "user": "",
  "password": "",
}
```

This happens because a RTSP client doesn't provide credentials until it is asked to. In order to receive the credentials, the authentication server must reply with status code `401` - the client will then send credentials.

### Encrypt the configuration

The configuration file can be entirely encrypted for security purposes.

An online encryption tool is [available here](https://play.golang.org/p/rX29jwObNe4).

The encryption procedure is the following:

1. NaCL's `crypto_secretbox` function is applied to the content of the configuration. NaCL is a cryptographic library available for [C/C++](https://nacl.cr.yp.to/secretbox.html), [Go](https://pkg.go.dev/golang.org/x/crypto/nacl/secretbox), [C#](https://github.com/somdoron/NaCl.net) and many other languages;

2. The string is prefixed with the nonce;

3. The string is encoded with base64.

After performing the encryption, put the base64-encoded result into the configuration file, and launch the server with the `RTSP_CONFKEY` variable:

```
RTSP_CONFKEY=mykey ./rtsp-simple-server
```

### Proxy mode

_rtsp-simple-server_ is also a proxy, that is usually deployed in one of these scenarios:

* when there are multiple users that are reading a stream and the bandwidth is limited; the proxy is used to receive the stream once. Users can then connect to the proxy instead of the original source.
* when there's a NAT / firewall between a stream and the users; the proxy is installed on the NAT and makes the stream available to the outside world.

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

After starting the server, users can connect to `rtsp://localhost:8554/proxied`, instead of connecting to the original url. The server supports any number of source streams, it's enough to add additional entries to the `paths` section:

```yml
paths:
  proxied1:
    source: rtsp://url1

  proxied2:
    source: rtsp://url1
```

It's possible to save bandwidth by enabling the on-demand mode: the stream will be pulled only when at least a client is connected:

```yml
paths:
  proxied:
    source: rtsp://original-url
    sourceOnDemand: yes
```

### Remuxing, re-encoding, compression

To change the format, codec or compression of a stream, use _FFmpeg_ or _GStreamer_ together with _rtsp-simple-server_. For instance, to re-encode an existing stream, that is available in the `/original` path, and publish the resulting stream in the `/compressed` path, edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
  original:
    runOnReady: ffmpeg -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -pix_fmt yuv420p -c:v libx264 -preset ultrafast -b:v 600k -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    runOnReadyRestart: yes
```

### Save streams to disk

To save available streams to disk, you can use the `runOnReady` parameter and _FFmpeg_:

```yml
paths:
  all:
  original:
    runOnReady: ffmpeg -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -c copy -f segment -strftime 1 -segment_time 60 -segment_format mpegts saved_%Y-%m-%d_%H-%M-%S.ts
    runOnReadyRestart: yes
```

In the example configuration, streams are saved into TS files, that can be read even if the system crashes, while MP4 files can't.

### On-demand publishing

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  ondemand:
    runOnDemand: ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnDemandRestart: yes
```

The command inserted into `runOnDemand` will start only when a client requests the path `ondemand`, therefore the file will start streaming only when requested.

### Start on boot

#### Linux

Systemd is the service manager used by Ubuntu, Debian and many other Linux distributions, and allows to launch rtsp-simple-server on boot.

Download a release bundle from the [release page](https://github.com/aler9/rtsp-simple-server/releases), unzip it, and move the executable and configuration in the system:

```
sudo mv rtsp-simple-server /usr/local/bin/
sudo mv rtsp-simple-server.yml /usr/local/etc/
```

Create the service:

```
sudo tee /etc/systemd/system/rtsp-simple-server.service >/dev/null << EOF
[Unit]
Wants=network.target
[Service]
ExecStart=/usr/local/bin/rtsp-simple-server /usr/local/etc/rtsp-simple-server.yml
[Install]
WantedBy=multi-user.target
EOF
```

Enable and start the service:

```
sudo systemctl enable rtsp-simple-server
sudo systemctl start rtsp-simple-server
```

#### Windows

Download the [WinSW v2 executable](https://github.com/winsw/winsw/releases/download/v2.11.0/WinSW-x64.exe) and place it into the same folder of `rtsp-simple-server.exe`.

In the same folder, create a file named `WinSW-x64.xml` with this content:

```xml
<service>
  <id>rtsp-simple-server</id>
  <name>rtsp-simple-server</name>
  <description></description>
  <executable>%BASE%/rtsp-simple-server.exe</executable>
</service>
```

Open a terminal, navigate to the folder and run:

```
WinSW-x64 install
```

The server is now installed as a system service and will start at boot time.

### HTTP API

The server can be queried and controlled with an HTTP API, that must be enabled by setting the `api` parameter in the configuration:

```yml
api: yes
```

The API listens on `apiAddress`, that by default is `127.0.0.1:9997`; for instance, to obtain a list of active paths, run:

```
curl http://127.0.0.1:9997/v1/paths/list
```

Full documentation of the API is available on the [dedicated site](https://aler9.github.io/rtsp-simple-server/).

### Metrics

A metrics exporter, compatible with Prometheus, can be enabled with the parameter `metrics: yes`; then the server can be queried for metrics with Prometheus or with a simple HTTP request:

```
wget -qO- localhost:9998/metrics
```

Obtaining:

```
paths{name="<path_name>",state="ready"} 1
rtsp_sessions{state="idle"} 0
rtsp_sessions{state="read"} 0
rtsp_sessions{state="publish"} 1
rtsps_sessions{state="idle"} 0
rtsps_sessions{state="read"} 0
rtsps_sessions{state="publish"} 0
rtmp_conns{state="idle"} 0
rtmp_conns{state="read"} 0
rtmp_conns{state="publish"} 1
hls_muxers{name="<name>"} 1
```

where:

* `paths{name="<path_name>",state="ready"} 1` is replicated for every path and shows the name and state of every path
* `rtsp_sessions{state="idle"}` is the count of RTSP sessions that are idle
* `rtsp_sessions{state="read"}` is the count of RTSP sessions that are reading
* `rtsp_sessions{state="publish"}` is the counf ot RTSP sessions that are publishing
* `rtsps_sessions{state="idle"}` is the count of RTSPS sessions that are idle
* `rtsps_sessions{state="read"}` is the count of RTSPS sessions that are reading
* `rtsps_sessions{state="publish"}` is the counf ot RTSPS sessions that are publishing
* `rtmp_conns{state="idle"}` is the count of RTMP connections that are idle
* `rtmp_conns{state="read"}` is the count of RTMP connections that are reading
* `rtmp_conns{state="publish"}` is the count of RTMP connections that are publishing
* `hls_muxers{name="<name>"}` is replicated for every HLS muxer and shows the name and state of every HLS muxer

### pprof

A performance monitor, compatible with pprof, can be enabled with the parameter `pprof: yes`; then the server can be queried for metrics with pprof-compatible tools, like:

```
go tool pprof -text http://localhost:9999/debug/pprof/goroutine
go tool pprof -text http://localhost:9999/debug/pprof/heap
go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=30
```

### Compile and run from source

Install Go 1.17, download the repository, open a terminal in it and run:

```
go run .
```

You can perform the entire operation inside Docker:

```
make run
```

## Publish to the server

### From a webcam

To publish the video stream of a generic webcam to the server, edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f v4l2 -i /dev/video0 -pix_fmt yuv420p -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

If the platform is Windows:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f dshow -i video="USB2.0 HD UVC WebCam" -pix_fmt yuv420p -c:v libx264 -preset ultrafast -b:v 600k -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

Where `USB2.0 HD UVC WebCam` is the name of your webcam, that can be obtained with:

```
ffmpeg -list_devices true -f dshow -i dummy
```

After starting the server, the webcam can be reached on `rtsp://localhost:8554/cam`.

### From a Raspberry Pi Camera

To publish the video stream of a Raspberry Pi Camera to the server, install a couple of dependencies:

1. _GStreamer_ and _h264parse_:

   ```
   sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp gstreamer1.0-plugins-bad
   ```

2. _gst-rpicamsrc_, by following [instruction here](https://github.com/thaytan/gst-rpicamsrc)

Then edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: gst-launch-1.0 rpicamsrc preview=false bitrate=2000000 keyframe-interval=50 ! video/x-h264,width=1920,height=1080,framerate=25/1 ! h264parse ! rtspclientsink location=rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

After starting the server, the camera is available on `rtsp://localhost:8554/cam`.

### From OBS Studio

OBS Studio can publish to the server by using the RTMP protocol. In `Settings -> Stream` (or in the Auto-configuration Wizard), use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream`

If credentials are in use, use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream?user=myuser&pass=mypass`

### From OpenCV

To publish a video stream from OpenCV to the server, OpenCV must be compiled with GStreamer support, by following this procedure:

```
sudo apt install -y libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-ugly gstreamer1.0-rtsp python3-dev python3-numpy
git clone --depth=1 -b 4.5.4 https://github.com/opencv/opencv
cd opencv
mkdir build && cd build
cmake -D CMAKE_INSTALL_PREFIX=/usr -D WITH_GSTREAMER=ON ..
make -j$(nproc)
sudo make install
```

Videos can then be published with `VideoWriter`:

```python
import cv2
import numpy as np
from time import sleep

fps = 20
width = 800
height = 600

out = cv2.VideoWriter('appsrc ! videoconvert' + \
    ' ! x264enc speed-preset=ultrafast bitrate=600 key-int-max=40' + \
    ' ! rtspclientsink location=rtsp://localhost:8554/mystream',
    cv2.CAP_GSTREAMER, 0, fps, (width, height), True)
if not out.isOpened():
    raise Exception("can't open video writer")

while True:
    frame = np.zeros((height, width, 3), np.uint8)

    # create a red rectangle
    for y in range(0, int(frame.shape[0] / 2)):
        for x in range(0, int(frame.shape[1] / 2)):
            frame[y][x] = (0, 0, 255)

    out.write(frame)
    print("frame written to the server")

    sleep(1 / fps)
```

## Read from the server

### From VLC and Ubuntu

The VLC shipped with Ubuntu 21.10 doesn't support playing RTSP due to a license issue (see [here](https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=982299) and [here](https://stackoverflow.com/questions/69766748/cvlc-cannot-play-rtsp-omxplayer-instead-can)).

To overcome the issue, remove the default VLC instance and install the snap version:

```
sudo apt purge -y vlc
snap install vlc
```

Then use it to read the stream:

```
vlc rtsp://localhost:8554/mystream
```

## RTSP protocol

### RTSP general usage

RTSP is a standardized protocol that allows to publish and read streams; in particular, it supports different underlying transport protocols, that are chosen by clients during the handshake with the server:

* UDP: the most performant, but doesn't work when there's a NAT/firewall between server and clients. It doesn't support encryption.
* UDP-multicast: allows to save bandwidth when clients are all in the same LAN, by sending packets once to a fixed multicast IP. It doesn't support encryption.
* TCP: the most versatile, does support encryption.

The default transport protocol is UDP. To change the transport protocol, you have to tune the configuration of your client of choice.

### TCP transport

The RTSP protocol supports the TCP transport protocol, that allows to receive packets even when there's a NAT/firewall between server and clients, and supports encryption (see [Encryption](#encryption)).

You can use _FFmpeg_ to publish a stream with the TCP transport protocol:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp -rtsp_transport tcp rtsp://localhost:8554/mystream
```

You can use _FFmpeg_ to read that stream with the TCP transport protocol:

```
ffmpeg -rtsp_transport tcp -i rtsp://localhost:8554/mystream -c copy output.mp4
```

You can use _GStreamer_ to read that stream with the TCP transport protocol:

```
gst-launch-1.0 rtspsrc protocols=tcp location=rtsp://localhost:8554/mystream ! fakesink
```

You can use _VLC_ to read that stream with the TCP transport protocol:

```
vlc --rtsp-tcp rtsp://localhost:8554/mystream
```

### UDP-multicast transport

The RTSP protocol supports the UDP-multicast transport protocol, that allows a server to send packets once, regardless of the number of connected readers, saving bandwidth.

This mode must be requested by readers when handshaking with the server; once a reader has completed a handshake, the server will start sending multicast packets. Other readers will be instructed to read existing multicast packets. When all multicast readers have disconnected from the server, the latter will stop sending multicast packets.

If you want to use the UDP-multicast protocol in a Wireless LAN, please be aware that the maximum bitrate supported by multicast is the one that corresponds to the lowest enabled WiFi data rate. For instance, if the 1 Mbps data rate is enabled on your router (and it is on most routers), the maximum bitrate will be 1 Mbps. To increase the maximum bitrate, use a cabled LAN or change your router settings.

To request and read a stream with UDP-multicast, you can use _FFmpeg_:

```
ffmpeg -rtsp_transport udp_multicast -i rtsp://localhost:8554/mystream -c copy output.mp4
```

or _GStreamer_:

```
gst-launch-1.0 rtspsrc protocols=udp-mcast location=rtsps://ip:8554/...
```

or _VLC_ (append `?vlcmulticast` to the URL):

```
vlc rtsp://localhost:8554/mystream?vlcmulticast
```

### Encryption

Incoming and outgoing RTSP streams can be encrypted with TLS (obtaining the RTSPS protocol). A TLS certificate is needed and can be generated with OpenSSL:

```
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit `rtsp-simple-server.yml`, and set the `protocols`, `encryption`, `serverKey` and `serverCert` parameters:

```yml
protocols: [tcp]
encryption: optional
serverKey: server.key
serverCert: server.crt
```

Streams can then be published and read with the `rtsps` scheme and the `8322` port:

```
ffmpeg -i rtsps://ip:8322/...
```

If the client is _GStreamer_, disable the certificate validation:

```
gst-launch-1.0 rtspsrc tls-validation-flags=0 location=rtsps://ip:8322/...
```

At the moment _VLC_ doesn't support reading encrypted RTSP streams. A workaround consists in launching an instance of _rtsp-simple-server_ on the same machine in which _VLC_ is running, using it for reading the encrypted stream with the proxy mode, and reading the proxied stream with _VLC_.

### Redirect to another server

To redirect to another server, use the `redirect` source:

```yml
paths:
  redirected:
    source: redirect
    sourceRedirect: rtsp://otherurl/otherpath
```

### Fallback stream

If no one is publishing to the server, readers can be redirected to a fallback path or URL that is serving a fallback stream:

```yml
paths:
  withfallback:
    fallback: /otherpath
```

### Corrupted frames

In some scenarios, when reading RTSP from the server, decoded frames can be corrupted or incomplete. This can be caused by multiple reasons:

* the packet buffer of the server is too small and can't keep up with the stream throughput. A solution consists in increasing its size:

  ```yml
  readBufferCount: 1024
  ```

* The stream throughput is too big and the stream can't be sent correctly with the UDP transport. UDP is more performant, faster and more efficient than TCP, but doesn't have a retransmission mechanism, that is needed in case of streams that need a large bandwidth. A solution consists in switching to TCP:

  ```yml
  protocols: [tcp]
  ```

  In case the source is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      sourceProtocol: tcp
  ```

## RTMP protocol

### RTMP general usage

RTMP is a protocol that allows to read and publish streams, but is less versatile and less efficient than RTSP (doesn't support UDP, encryption, doesn't support most RTSP codecs, doesn't support feedback mechanism). It is used when there's need of publishing or reading streams from a software that supports only RTMP (for instance, OBS Studio and DJI drones).

At the moment, only the H264 and AAC codecs can be used with the RTMP protocol.

Streams can be published or read with the RTMP protocol, for instance with _FFmpeg_:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f flv rtmp://localhost/mystream
```

or _GStreamer_:

```
gst-launch-1.0 -v flvmux name=s ! rtmpsink location=rtmp://localhost/mystream filesrc location=file.mp4 ! qtdemux name=d d.video_0 ! queue ! s.video d.audio_0 ! queue ! s.audio
```

Credentials can be provided by appending to the URL the `user` and `pass` parameters:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f flv rtmp://localhost:8554/mystream?user=myuser&pass=mypass
```

## HLS protocol

### HLS general usage

HLS is a protocol that allows to embed live streams into web pages. It works by splitting streams into segments, and by serving these segments with the HTTP protocol. Every stream published to the server can be accessed by visiting:

```
http://localhost:8888/mystream
```

where `mystream` is the name of a stream that is being published.

### Embedding

The simples way to embed a live stream into a web page consists in using an iframe tag:

```
<iframe src="http://rtsp-simple-server-ip:8888/mystream" scrolling="no"></iframe>
```

Alternatively you can create a video tag that points directly to the stream playlist:

```
<video src="http://localhost:8888/mystream/index.m3u8"></video>
```

Please note that most browsers don't support HLS directly (except Safari); a Javascript library, like [hls.js](https://github.com/video-dev/hls.js), must be used to load the stream. You can find a working example by looking at the [source code of the HLS muxer](internal/core/hls_muxer.go).

### Low-Latency variant

Low-Latency HLS is a [recently standardized](https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis) variant of the protocol that allows to greatly reduce playback latency. It works by splitting segments into parts, that are served before the segment is complete.

LL-HLS is disabled by default. To enable it, a TLS certificate is needed and can be generated with OpenSSL:

```
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Set the `hlsVariant`, `hlsEncryption`, `hlsServerKey` and `hlsServerCert` parameters in the configuration file:

```yml
hlsVariant: lowLatency
hlsEncryption: yes
hlsServerKey: server.key
hlsServerCert: server.crt
```

Every stream published to the server can then be read with LL-HLS by visiting:

```
https://localhost:8888/mystream
```

If the stream is not shown correctly, try tuning the `hlsPartDuration` parameter, for instance:

```yml
hlsPartDuration: 500ms
```

### Decreasing latency

in HLS, latency is introduced since a client must wait for the server to generate segments before downloading them. This latency amounts to 1-15secs depending on the duration of each segment, and to 500ms-3s if the Low-Latency variant is enabled.

To decrease the latency, you can:

* enable the Low-Latency variant of the HLS protocol, as explained in the previous section;

* if Low-latency is enabled, try decreasing the `hlsPartDuration` parameter;

* try decreasing the `hlsSegmentDuration` parameter;

* The segment duration is influenced by the interval between the IDR frames of the video track. An IDR frame is a frame that can be decoded independently from the others. The server changes the segment duration in order to include at least one IDR frame into each segment. Therefore, you need to decrease the interval between the IDR frames. This can be done in two ways:

  * if the stream is being hardware-generated (i.e. by a camera), there's usually a setting called _Key-Frame Interval_ in the camera configuration page

  * otherwise, the stream must be re-encoded. It's possible to tune the IDR frame interval by using ffmpeg's `-g` option:

    ```
    ffmpeg -i rtsp://original-stream -pix_fmt yuv420p -c:v libx264 -preset ultrafast -b:v 600k -max_muxing_queue_size 1024 -g 30 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    ```

## Links

Related projects

* https://github.com/aler9/gortsplib (RTSP library used internally)
* https://github.com/pion/sdp (SDP library used internally)
* https://github.com/pion/rtcp (RTCP library used internally)
* https://github.com/pion/rtp (RTP library used internally)
* https://github.com/notedit/rtmp (RTMP library used internally)
* https://github.com/asticode/go-astits (MPEG-TS library used internally)
* https://github.com/abema/go-mp4 (MP4 library used internally)
* https://github.com/flaviostutz/rtsp-relay

Standards

* RTSP 1.0 https://datatracker.ietf.org/doc/html/rfc2326
* RTSP 2.0 https://datatracker.ietf.org/doc/html/rfc7826
* HTTP 1.1 https://datatracker.ietf.org/doc/html/rfc2616
* HLS https://datatracker.ietf.org/doc/html/rfc8216
* HLS v2 https://datatracker.ietf.org/doc/html/draft-pantos-hls-rfc8216bis
* Golang project layout https://github.com/golang-standards/project-layout
