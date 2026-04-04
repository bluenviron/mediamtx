# Introduction

Welcome to the MediaMTX documentation!

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other, with a focus on efficiency and portability.

Main features:

- [Publish](../2-features/03-publish.md) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP, using FFmpeg, GStreamer, OBS Studio, Python , Golang, Unity, Web browsers, Raspberry Pi Cameras and more.
- [Read](../2-features/04-read.md) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS, using FFmpeg, GStreamer, VLC, OBS Studio, Python , Golang, Unity, Web browsers and more.
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](../2-features/08-always-available.md) even when the publisher is offline
- [Record](../2-features/09-record.md) streams to disk in fMP4 or MPEG-TS format
- [Playback](../2-features/10-playback.md) recorded streams
- [Authenticate](../2-features/06-authentication.md) users with internal, HTTP or JWT authentication
- [Forward](../2-features/11-forward.md) streams to other servers
- [Proxy](../2-features/12-proxy.md) requests to other servers
- [Control](../2-features/21-control-api.md) the server through the Control API
- [Extract metrics](../2-features/22-metrics.md) from the server in a Prometheus-compatible format
- [Monitor performance](../2-features/23-performance.md) to investigate CPU and RAM consumption
- [Run hooks](../2-features/20-hooks.md) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
