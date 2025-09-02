# Introduction

Welcome to the MediaMTX documentation!

MediaMTX is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other.

Main features:

* [Publish](/docs/usage/publish) live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
* [Read](/docs/usage/read) live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
* Streams are automatically converted from a protocol to another
* Serve several streams at once in separate paths
* [Record](/docs/usage/record) streams to disk in fMP4 or MPEG-TS format
* [Playback](/docs/usage/playback) recorded streams
* [Authenticate](/docs/usage/authentication) users with internal, HTTP or JWT authentication
* [Forward](/docs/usage/forward) streams to other servers
* [Proxy](/docs/usage/proxy) requests to other servers
* [Control](/docs/usage/control-api) the server through the Control API
* Reload the configuration without disconnecting existing clients (hot reloading)
* [Monitor](/docs/usage/metrics) the server through Prometheus-compatible metrics
* [Run hooks](/docs/usage/hooks) (external commands) when clients connect, disconnect, read or publish streams
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
