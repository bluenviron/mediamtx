# Introduction

Welcome to the MediaMTX documentation!

_MediaMTX_ is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other.

Main features:

- [Publish](/docs/usage/publish) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
- [Read](/docs/usage/read) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Reload the configuration without disconnecting existing clients (hot reloading)
- [Serve always-available streams](/docs/usage/always-available) even when the publisher is offline
- [Record](/docs/usage/record) streams to disk in fMP4 or MPEG-TS format
- [Playback](/docs/usage/playback) recorded streams
- [Authenticate](/docs/usage/authentication) users with internal, HTTP or JWT authentication
- [Forward](/docs/usage/forward) streams to other servers
- [Proxy](/docs/usage/proxy) requests to other servers
- [Control](/docs/usage/control-api) the server through the Control API
- [Extract metrics](/docs/usage/metrics) from the server in a Prometheus-compatible format
- [Monitor performance](/docs/usage/performance) to investigate CPU and RAM consumption
- [Run hooks](/docs/usage/hooks) (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
