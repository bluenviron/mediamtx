# Publish and read streams

Live streams can be published or read by using a client (software) that connects to _MediaMTX_ and communicates through a protocol.

Many protocols are available for publishing and reading real-time streams because different industries developed streaming technology at different times and for different purposes (security, web streaming, TV broadcasting). In _MediaMTX_, streams are not tied to a specific protocol: they can be published with one protocol and read with another. Conversion is effortless and automatic.

These are the protocols that are currently supported and their capabilities (publish from a client / publish from a server / read):

- Media-over-QUIC ([publish from a client](../3-publish/01-moq-clients.md), [publish from a server](../3-publish/02-moq-servers.md), [read](../4-read/01-moq.md)): a new generation streaming protocol based on QUIC, HTTP/3 and browser APIs. _MediaMTX_ listens for incoming clients on ports `8892/TCP` and `8893/UDP` and is capable of pulling streams from other servers.

- WebRTC ([publish from a client](../3-publish/05-webrtc-clients.md), [publish from a server](../3-publish/06-webrtc-servers.md), [read](../4-read/03-webrtc.md)): a streaming protocol based on a set of ready-to-use browser APIs. _MediaMTX_ listens for incoming clients on ports `8889/TCP`, `8189/UDP` and is capable of pulling streams from other servers.

- RTSP ([publish from a client](../3-publish/07-rtsp-clients.md), [publish from a server](../3-publish/08-rtsp-cameras-and-servers.md), [read](../4-read/04-rtsp.md)): the streaming protocol used by most IP cameras. _MediaMTX_ listens for incoming clients on port `:8554/TCP` (and uses `:8000/UDP` and `:8001/UDP` under the hood) and is capable of pulling streams from other servers.

- RTMP ([publish from a client](../3-publish/09-rtmp-clients.md), [publish from a server](../3-publish/10-rtmp-cameras-and-servers.md), [read](../4-read/05-rtmp.md)): streaming protocol used mostly for publishing live streams to broadcast websites. _MediaMTX_ listens for incoming clients on port `:1935/TCP` and is capable of pulling streams from other servers.

- SRT ([publish from a client](../3-publish/03-srt-clients.md), [publish from a server](../3-publish/04-srt-cameras-and-servers.md), [read](../4-read/02-srt.md)): a streaming protocol developed for robustness and precise timing for the enterprise broadcasting industry. _MediaMTX_ listens for incoming clients on port `8890/UDP` and is capable of pulling streams from other servers.

- HLS ([publish from a server](../3-publish/11-hls-cameras-and-servers.md), [read](../4-read/06-hls.md)): a streaming protocol based on files (segments) served through an HTTP server, suitable for reading streams on websites. _MediaMTX_ listens for incoming clients on port `8888/TCP`.

- MPEG-TS ([publish from a server](../3-publish/12-mpeg-ts.md)): a packet format used for TV broadcasting. _MediaMTX_ is capable of pulling MPEG-TS packets from an IP address.

- RTP ([publish from a server](../3-publish/13-rtp.md)): a packet format used for IP streaming. _MediaMTX_ is capable of pulling RTP streams from an IP address.

We also provide instructions for ingesting streams from some devices:

- [Raspberry Pi Cameras](../3-publish/14-raspberry-pi-cameras.md)
- [Generic webcams](../3-publish/15-generic-webcams.md)

We also provide instructions for publishing streams with specific software:

- [Web browsers](../3-publish/16-web-browsers.md)
- [FFmpeg](../3-publish/17-ffmpeg.md)
- [GStreamer](../3-publish/18-gstreamer.md)
- [OBS Studio](../3-publish/19-obs-studio.md)
- [Python and OpenCV](../3-publish/20-python-opencv.md)
- [Golang](../3-publish/21-golang.md)
- [Unity](../3-publish/22-unity.md)

We also provide instructions for reading streams with specific software:

- [Web browsers](../4-read/07-web-browsers.md)
- [FFmpeg](../4-read/08-ffmpeg.md)
- [GStreamer](../4-read/09-gstreamer.md)
- [VLC](../4-read/10-vlc.md)
- [OBS Studio](../4-read/11-obs-studio.md)
- [Python and OpenCV](../4-read/12-python-opencv.md)
- [Golang](../4-read/13-golang.md)
- [Unity](../4-read/14-unity.md)

**Which protocol(s) should i use?**

There isn't a single answer to this question; it depends on the specific use case. Think about the overall system you want to implement, from the source to the final destination. Usually some protocol is used for the publishing/ingestion operation, and a different one is used by clients to read streams.

Regarding publishing/ingestion, if the source is an IP camera, you probably want to use the protocol offered by the camera (RTSP, RTMP). If the source is broadcasting software, you might want to use its native protocol (RTMP, WebRTC, SRT). If the source is a user connected to your website through a browser, you might want to use a browser-native protocol (Media-over-QUIC, WebRTC).

Regarding reading, the protocol choice depends on the intended audience. If readers are users connected through a website, you might want to use a browser-native protocol (Media-over-.QUIC, WebRTC). If you need to serve a large number of readers, you might want to consider HLS. On a local network, multicast-capable protocols (RTSP, RTP) allow to save bandwidth since packets are sent once per stream and not per user.

In any case, the protocol is not a constraint and can be changed at any time by switching the client and target port.
