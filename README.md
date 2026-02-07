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

_MediaMTX_ is a ready-to-use and zero-dependency real-time media server and media proxy that allows to publish, read, proxy, record and playback video and audio streams. It has been conceived as a "media router" that routes media streams from one end to the other.

<div align="center">

  |[Install](https://mediamtx.org/docs/kickoff/install)|[Documentation](https://mediamtx.org/docs/kickoff/introduction)|
  |-|-|

</div>

<h3>Features</h3>

- [Publish](https://mediamtx.org/docs/usage/publish) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
- [Read](https://mediamtx.org/docs/usage/read) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](https://mediamtx.org/docs/usage/always-available) even when the publisher is offline
- [Record](https://mediamtx.org/docs/usage/record) streams to disk in fMP4 or MPEG-TS format
- [Playback](https://mediamtx.org/docs/usage/playback) recorded streams
- [Authenticate](https://mediamtx.org/docs/usage/authentication) users with internal, HTTP or JWT authentication
- [Forward](https://mediamtx.org/docs/usage/forward) streams to other servers
- [Proxy](https://mediamtx.org/docs/usage/proxy) requests to other servers
- [Control](https://mediamtx.org/docs/usage/control-api) the server through the Control API
- [Extract metrics](https://mediamtx.org/docs/usage/metrics) from the server in a Prometheus-compatible format
- [Monitor performance](https://mediamtx.org/docs/usage/performance) to investigate CPU and RAM consumption
- [Run hooks](https://mediamtx.org/docs/usage/hooks) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable
- ...and many [others](https://mediamtx.org/docs/kickoff/introduction).
