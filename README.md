
# rtsp-simple-server

[![Go Report Card](https://goreportcard.com/badge/github.com/aler9/rtsp-simple-server)](https://goreportcard.com/report/github.com/aler9/rtsp-simple-server)
[![Build Status](https://travis-ci.org/aler9/rtsp-simple-server.svg?branch=master)](https://travis-ci.org/aler9/rtsp-simple-server)

_rtsp-simple-server_ is a simple, ready-to-use and zero-dependency RTSP server, a software that allows multiple users to read or publish live video and audio streams. RTSP is a standardized protocol that defines how to perform these operations with the help of a server, that is contacted by both readers and publishers in order to negotiate a streaming protocol and read or write data. The server is then responsible of linking the publisher stream with the readers.

This software was developed with the aim of simulating a live camera feed for debugging purposes, and therefore to use files instead of real streams. Another reason for the development was the deprecation of _FFserver_, the component of the _FFmpeg_ project that allowed to create a RTSP server (but this server is not bounded to _FFmpeg_ and can be used with any software that supports publishing to RTSP).

Features:
* Read and publish streams via UDP and TCP
* Publish multiple streams at once, each in a separate path, that can be read by multiple users
* Each stream can have multiple video and audio tracks
* Supports the RTP/RTCP streaming protocol
* Supports authentication for publishers
* Run a script when a client connects or disconnects
* Compatible with Linux, Windows and Mac, does not require any dependency or interpreter, it's a single executable

## Installation

Precompiled binaries are available in the [release](https://github.com/aler9/rtsp-simple-server/releases) page. Just download and extract the executable.

## Usage

#### Basic usage

1. Start the server:
   ```
   ./rtsp-simple-server
   ```

2. In another terminal, publish something with FFmpeg (in this example it's a video file, but it can be anything you want):
   ```
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

3. Open the stream with VLC:
   ```
   vlc rtsp://localhost:8554/mystream
   ```

   you can alternatively use GStreamer:
   ```
   gst-launch-1.0 -v rtspsrc location=rtsp://localhost:8554/mystream ! rtph264depay ! decodebin ! autovideosink
   ```

#### Publisher authentication

1. Start the server and set a username and a password:
   ```
   ./rtsp-simple-server --publish-user=admin --publish-pass=mypassword
   ```

 2. Only publishers that know both username and password will be able to publish:
    ```
    ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://admin:mypassword@localhost:8554/mystream
    ```

#### Remuxing, re-encoding, compression

`rtsp-simple-server` is an RTSP server: it publishes existing streams and does not touch them. It is not a media server, that is a far more complex software that can receive existing streams, re-encode them and publish them. Therefore, `rtsp-simple-server` alone cannot change the format, codec or compression of a stream.

Nothing forbids from using `ffmpeg` or `gstreamer` together with `rtsp-simple-server`, obtaining the same features of a media server. For instance, if we want to re-encode an existing stream, that is available in the `/original` path, and make the modified stream available in the `/compressed` path, it is enough to launch `ffmpeg` in parallel with `rtsp-simple-server`, with the following syntax:
```
ffmpeg -i rtsp://localhost:8554/original -c:v libx264 -preset ultrafast -tune zerolatency -b 600k -f rtsp rtsp://localhost:8554/compressed
```

#### Full command-line usage

```
usage: rtsp-simple-server [<flags>]

rtsp-simple-server v0.0.0

RTSP server.

Flags:
  --help                 Show context-sensitive help (also try --help-long and --help-man).
  --version              print version
  --protocols="udp,tcp"  supported protocols
  --rtsp-port=8554       port of the RTSP TCP listener
  --rtp-port=8000        port of the RTP UDP listener
  --rtcp-port=8001       port of the RTCP UDP listener
  --read-timeout=5s      timeout for read operations
  --write-timeout=5s     timeout for write operations
  --publish-user=""      optional username required to publish
  --publish-pass=""      optional password required to publish
  --read-user=""         optional username required to read
  --read-pass=""         optional password required to read
  --pre-script=""        optional script to run on client connect
  --post-script=""       optional script to run on client disconnect
```

## Links

Related projects
* https://github.com/aler9/rtsp-simple-proxy
* https://github.com/aler9/gortsplib
* https://github.com/flaviostutz/rtsp-relay

IETF Standard
* (1.0) https://tools.ietf.org/html/rfc2326
* (2.0) https://tools.ietf.org/html/rfc7826
