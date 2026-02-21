# Introduction

Welcome to the MediaMTX documentation!

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other.

Main features:

- [Publish](../2-usage/02-publish.md) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
- [Read](../2-usage/03-read.md) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](../2-usage/07-always-available.md) even when the publisher is offline
- [Record](../2-usage/08-record.md) streams to disk in fMP4 or MPEG-TS format
- [Playback](../2-usage/09-playback.md) recorded streams
- [Authenticate](../2-usage/05-authentication.md) users with internal, HTTP or JWT authentication
- [Forward](../2-usage/10-forward.md) streams to other servers
- [Proxy](../2-usage/11-proxy.md) requests to other servers
- [Control](../2-usage/20-control-api.md) the server through the Control API
- [Extract metrics](../2-usage/21-metrics.md) from the server in a Prometheus-compatible format
- [Monitor performance](../2-usage/22-performance.md) to investigate CPU and RAM consumption
- [Run hooks](../2-usage/19-hooks.md) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
