<h1 align="center">
  <a href="https://mediamtx.org">
    <img src="logo.png" alt="MediaMTX">
  </a>

  <br>
  <br>

  [![Website](https://img.shields.io/badge/website-website-blue)](https://mediamtx.org)
  [![Test](https://github.com/bluenviron/mediamtx/actions/workflows/code_test.yml/badge.svg)](https://github.com/bluenviron/mediamtx/actions/workflows/code_test.yml)
  [![Lint](https://github.com/bluenviron/mediamtx/actions/workflows/code_lint.yml/badge.svg)](https://github.com/bluenviron/mediamtx/actions/workflows/code_lint.yml)
  [![CodeCov](https://codecov.io/gh/bluenviron/mediamtx/branch/main/graph/badge.svg)](https://app.codecov.io/gh/bluenviron/mediamtx/tree/main)
  [![Release](https://img.shields.io/github/v/release/bluenviron/mediamtx)](https://github.com/bluenviron/mediamtx/releases)
  [![Docker Hub](https://img.shields.io/badge/docker-bluenviron/mediamtx-blue)](https://hub.docker.com/r/bluenviron/mediamtx)
</h1>

<br>

_MediaMTX_ is a ready-to-use and zero-dependency real-time media server and media proxy that allows to publish, read, proxy, record and playback video and audio streams. It has been conceived as a "media router" that routes media streams from one end to the other.

**Features**

* Publish live streams to the server
* Read live streams from the server
* Streams are automatically converted from a protocol to another
* Serve several streams at once in separate paths
* Record streams to disk
* Playback recorded streams
* Authenticate users
* Redirect readers to other RTSP servers (load balancing)
* Control the server through the Control API
* Reload the configuration without disconnecting existing clients (hot reloading)
* Read Prometheus-compatible metrics
* Run hooks (external commands) when clients connect, disconnect, read or publish streams
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable
