# Introduction

Welcome to the MediaMTX documentation!

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy that allows to publish, read, proxy, record and playback real-time video and audio streams. It has been conceived as a "media router" that routes media streams from one end to the other, with a focus on efficiency and portability.

Main features:

- [Publish streams](../2-features/03-publish-and-read.md) to the server with Media-over-QUIC, SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP, using FFmpeg, GStreamer, OBS Studio, Python , Golang, Unity, Web browsers, Raspberry Pi Cameras and more.
- [Read streams](../2-features/03-publish-and-read.md) from the server with Media-over-QUIC, SRT, WebRTC, RTSP, RTMP, HLS, using FFmpeg, GStreamer, VLC, OBS Studio, Python , Golang, Unity, Web browsers and more.
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](../2-features/07-always-available.md) even when the publisher is offline
- [Record streams](../2-features/08-record.md) to disk in fMP4 or MPEG-TS format
- [Playback recorded streams](../2-features/09-playback.md) from disk
- [Authenticate](../2-features/05-authentication.md) users with internal, HTTP or JWT authentication
- [Forward streams](../2-features/10-forward.md) to other servers
- [Proxy requests](../2-features/11-proxy.md) to other servers
- [Control](../2-features/20-control-api.md) the server through the Control API
- [Extract metrics](../2-features/21-metrics.md) from the server in a Prometheus-compatible format
- [Monitor performance](../2-features/22-performance.md) to investigate CPU and RAM consumption
- [Run hooks](../2-features/19-hooks.md) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

You can learn more about how everything is assembled in [Architecture](../2-features/02-architecture.md).

Use the menu to navigate through the documentation.
