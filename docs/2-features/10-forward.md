# Forward streams

Streams ingested into _MediaMTX_ can be forwarded to other servers, in order to make them available elsewhere (for instance to a CDN, a streaming platform or another _MediaMTX_ instance). Forwarding is performed natively by adding one or more entries to the `forward` parameter of a path, and the protocol used to forward the stream does not depend on the protocol used to publish it.

These are the protocols that can be used as forward destinations:

- [Media-over-QUIC](../6-forward/1-moq.md)
- [SRT](../6-forward/2-srt.md)
- [WebRTC](../6-forward/3-webrtc.md)
- [RTSP](../6-forward/4-rtsp.md)
- [RTMP](../6-forward/5-rtmp.md)

In case the destination requires transcoding, filtering or a protocol that is not supported natively, you can use [FFmpeg](../6-forward/6-ffmpeg.md) to perform the forwarding.

We also provide instructions for forwarding streams to specific services:

- [YouTube](../6-forward/7-youtube.md)
- [Twitch](../6-forward/8-twitch.md)
