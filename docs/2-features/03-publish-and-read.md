# Publish and read streams

Live streams can be published or read by using a client (software) that connects to _MediaMTX_ and communicates through a protocol.

There is a significant number of protocols available for publishing and reading real-time streaming, due to the fact that this technology was investigated by several industries, in different ages and for different purposes (security, web streaming, TV broadcasting). In _MediaMTX_, streams are not tied to a specific protocol, they can be published with a protocol and read with another one, conversion is effortless and automatic.

Streams can also be ingested as _Static sources_, in which _MediaMTX_ acts as the client and pulls streams from other servers.

These are the protocols that are currently supported and their capabilities (publish / static source / read):

- Media-over-QUIC ([publish](../3-publish/01-moq.md), [static source](../4-static-sources/1-moq.md), [read](../5-read/01-moq.md)): a new generation streaming protocol based on QUIC, HTTP/3 and browser APIs. _MediaMTX_ listens for incoming clients on port `8892/TCP` and `8893/UDP` and is capable of pulling streams from other servers.

- WebRTC ([publish](../3-publish/03-webrtc.md), [static source](../4-static-sources/3-webrtc.md), [read](../5-read/03-webrtc.md)): a streaming protocol based on a set of ready-to-use browser APIs. _MediaMTX_ listens for incoming clients on port `8889/TCP`, `8189/UDP` and is capable of pulling streams from other servers.

- RTSP ([publish](../3-publish/04-rtsp.md), [static source](../4-static-sources/4-rtsp.md), [read](../5-read/04-rtsp.md)): the streaming protocol used by most IP cameras. _MediaMTX_ listens for incoming clients on ports `:8554/TCP` (and uses `:8000/UDP` and `:8001/UDP` under the hood) and is capable of pulling streams from other servers.

- RTMP ([publish](../3-publish/05-rtmp.md), [static source](../4-static-sources/5-rtmp.md), [read](../5-read/05-rtmp.md)): streaming protocol used mostly for publishing live streams to broadcast websites. _MediaMTX_ listens for incoming clients on port `:1935/TCP` and is capable of pulling streams from other servers.

- SRT ([publish](../3-publish/02-srt.md), [static source](../4-static-sources/2-srt.md), [read](../5-read/02-srt.md)): a streaming protocol developed for robustness and precise timing for the enterprise broadcasting industry. _MediaMTX_ listens for incoming clients on port `8890/UDP` and is capable of pulling streams from other servers.

- HLS ([read](../5-read/06-hls.md), [static source](../4-static-sources/6-hls.md)): a streaming protocol based on files (segments) server through an HTTP server, fit for reading streams from websites. _MediaMTX_ listens for incoming clients on port `8888/TCP`.

- MPEG-TS ([static source](../4-static-sources/7-mpeg-ts.md)): a packet format used for TV broadcasting. _MediaMTX_ is capable of pulling MPEG-TS packets from an IP address.

- RTP ([static source](../4-static-sources/8-rtp.md)): a packet format used for IP streaming. _MediaMTX_ is capable of pulling RTP streams from an IP address.

We also provide instructions for ingesting streams from some devices:

- [Raspberry Pi Cameras](../4-static-sources/9-raspberry-pi-cameras.md)
- [Generic webcams](../3-publish/06-generic-webcams.md)

We also provide instructions for publishing streams with specific software:

- [Web browsers](../3-publish/07-web-browsers.md)
- [FFmpeg](../3-publish/08-ffmpeg.md)
- [GStreamer](../3-publish/09-gstreamer.md)
- [OBS Studio](../3-publish/10-obs-studio.md)
- [Python and OpenCV](../3-publish/11-python-opencv.md)
- [Golang](../3-publish/12-golang.md)
- [Unity](../3-publish/13-unity.md)

We also provide instructions for reading streams with specific software:

- [Web browsers](../5-read/07-web-browsers.md)
- [FFmpeg](../5-read/08-ffmpeg.md)
- [GStreamer](../5-read/09-gstreamer.md)
- [VLC](../5-read/10-vlc.md)
- [OBS Studio](../5-read/11-obs-studio.md)
- [Python and OpenCV](../5-read/12-python-opencv.md)
- [Golang](../5-read/13-golang.md)
- [Unity](../5-read/14-unity.md)

**Which protocol(s) should i use?**

There isn't a unique answer to this question, it mostly depends on the specific use case. Think about the overall system you want to implement, starting from the origin of the source up to the final destination.

If the origin is an IP camera, you probably want to use the protocol offered by the camera (RTSP, RTMP), if the origin is a broadcasting software you might want to use its native protocol (RTMP, WebRTC, SRT), if the origin is some user connected to your website through a browser you might want to use a browser-native protocol (Media-over-QUIC, WebRTC).

Protocols used by readers are most of the times different from the one used to publish the stream, and depends on what's the intended audience. If readers are users connected through a website, then you might want to use a browser-native protocol (Media-over-QUIC, WebRTC), if they are huge in number and you need to scale you might want to consider HLS.

Anyway, protocol is not a constraint and can be changed effortlessly and at any time by switching client and target port.
