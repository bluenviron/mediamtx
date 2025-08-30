# Introduction

Welcome to the MediaMTX documentation!

MediaMTX is a ready-to-use and zero-dependency live media server and media proxy. It has been conceived as a "media router" that routes media streams from one end to the other.

Main features:

- Publish live streams to the server with SRT, WebRTC, RTSP, RTMP, HLS, MPEG-TS, RTP
- Read live streams from the server with SRT, WebRTC, RTSP, RTMP, HLS
- Streams are automatically converted from a protocol to another
- Serve several streams at once in separate paths
- Record streams to disk in fMP4 or MPEG-TS format
- Playback recorded streams
- Authenticate users
- Redirect readers to other servers (load balancing)
- Control the server through the Control API
- Reload the configuration without disconnecting existing clients (hot reloading)
- Extract Prometheus-compatible metrics
- Run hooks (external commands) when clients connect, disconnect, read or publish streams
- Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

Use the menu to navigate through the documentation.
