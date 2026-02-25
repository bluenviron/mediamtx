# Introduction

Welcome to the MediaMTX documentation!

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other.

Main features:

- [Publish](../2-publish/01-overview.md) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
- [Read](../3-read/01-overview.md) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](../4-other/05-always-available.md) even when the publisher is offline
- [Record](../4-other/06-record.md) streams to disk in fMP4 or MPEG-TS format
- [Playback](../4-other/07-playback.md) recorded streams
- [Authenticate](../4-other/03-authentication.md) users with internal, HTTP or JWT authentication
- [Forward](../4-other/08-forward.md) streams to other servers
- [Proxy](../4-other/09-proxy.md) requests to other servers
- [Control](../4-other/18-control-api.md) the server through the Control API
- [Extract metrics](../4-other/19-metrics.md) from the server in a Prometheus-compatible format
- [Monitor performance](../4-other/20-performance.md) to investigate CPU and RAM consumption
- [Run hooks](../4-other/17-hooks.md) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
