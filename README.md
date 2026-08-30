<h1 align="center">
  <a href="https://mediamtx.org">
    <img src="logo.png" alt="MediaMTX">
  </a>

  <br>
  <br>

[![Website](https://img.shields.io/badge/website-mediamtx.org-1c94b5)](https://mediamtx.org)
[![Test](https://github.com/bluenviron/mediamtx/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/bluenviron/mediamtx/actions/workflows/test.yml?query=branch%3Amain)
[![Lint](https://github.com/bluenviron/mediamtx/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/bluenviron/mediamtx/actions/workflows/lint.yml?query=branch%3Amain)
[![CodeCov](https://codecov.io/gh/bluenviron/mediamtx/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/mediamtx/tree/main)
[![Release](https://img.shields.io/github/v/release/bluenviron/mediamtx)](https://github.com/bluenviron/mediamtx/releases)
[![Docker Hub](https://img.shields.io/badge/docker-bluenviron/mediamtx-blue)](https://hub.docker.com/r/bluenviron/mediamtx)

</h1>

<br>

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy that allows to publish, read, proxy, record and playback real-time video and audio streams. It has been conceived as a "media router" that routes media streams from one end to the other, with a focus on efficiency and portability.

<div align="center">

| [Install](https://mediamtx.org/docs/kickoff/install) | [Documentation](https://mediamtx.org/docs/kickoff/introduction) |
| ---------------------------------------------------- | --------------------------------------------------------------- |

</div>

<h3>Features</h3>

- [Publish streams](https://mediamtx.org/docs/features/publish) to the server with Media-over-QUIC, SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP, using FFmpeg, GStreamer, OBS Studio, Python , Golang, Unity, Web browsers, Raspberry Pi Cameras and more.
- [Read streams](https://mediamtx.org/docs/features/read) from the server with Media-over-QUIC, SRT, WebRTC, RTSP, RTMP, HLS, using FFmpeg, GStreamer, VLC, OBS Studio, Python , Golang, Unity, Web browsers and more.
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](https://mediamtx.org/docs/features/always-available) even when the publisher is offline
- [Record streams](https://mediamtx.org/docs/features/record) to disk in fMP4 or MPEG-TS format
- [Playback recorded streams](https://mediamtx.org/docs/features/playback) from disk
- [Authenticate](https://mediamtx.org/docs/features/authentication) users with internal, HTTP or JWT authentication
- [Forward streams](https://mediamtx.org/docs/features/forward) to other servers
- [Proxy requests](https://mediamtx.org/docs/features/proxy) to other servers
- [Control](https://mediamtx.org/docs/features/control-api) the server through the Control API
- [Extract metrics](https://mediamtx.org/docs/features/metrics) from the server in a Prometheus-compatible format
- [Monitor performance](https://mediamtx.org/docs/features/performance) to investigate CPU and RAM consumption
- [Run hooks](https://mediamtx.org/docs/features/hooks) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable
- ...and many [others](https://mediamtx.org/docs/kickoff/introduction).
