
# rtsp-simple-server

[![Go Report Card](https://goreportcard.com/badge/github.com/aler9/rtsp-simple-server)](https://goreportcard.com/report/github.com/aler9/rtsp-simple-server)
[![Build Status](https://travis-ci.org/aler9/rtsp-simple-server.svg?branch=master)](https://travis-ci.org/aler9/rtsp-simple-server)

_rtsp-simple-server_ is a simple, ready-to-use and zero-dependency RTSP server, a program that allows multiple users to read or publish live video and audio streams. RTSP a standardized protocol that defines how to perform these operations with the help of a server, that is contacted by both readers and publishers in order to negotiate a streaming protocol and read or write data. The server is then responsible of linking the publisher stream with the readers.

This software was developed with the aim of simulating a live camera feed for debugging purposes, and therefore to use files instead of real streams. Another reason for the development was the deprecation of _FFserver_, the component of the _FFmpeg_ project that allowed to create a RTSP server (but this server is not bounded to _FFmpeg_ and can be used with any software that supports publishing to RTSP).

Features:
* Read and publish streams via UDP and TCP
* Publish multiple streams at once, each in a separate path, that can be read by multiple users
* Each stream can have multiple video and audio tracks
* Supports the RTP/RTCP streaming protocol
* Compatible with Linux and Windows, does not require any dependency or interpreter, it's a single executable


<br />

## Installation

Precompiled binaries are available in the [release](https://github.com/aler9/rtsp-simple-server/releases) page. Just download and extract the executable.


<br />

## Usage

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

<br />

## Full command-line usage

```
usage: rtsp-simple-server [<flags>]

rtsp-simple-server

RTSP server.

Flags:
  --help            Show context-sensitive help (also try --help-long and --help-man).
  --version         print rtsp-simple-server version
  --rtsp-port=8554  port of the RTSP TCP listener
  --rtp-port=8000   port of the RTP UDP listener
  --rtcp-port=8001  port of the RTCP UDP listener
```

<br />

## Links

IETF Standard
* https://tools.ietf.org/html/rfc7826
