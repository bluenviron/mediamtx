# Publish a stream

Live streams can be published to the server with the following protocols and codecs:

| protocol                                                                | variants                                   | codecs                                                                                                                                                                                                                                                 |
| ----------------------------------------------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [SRT clients](../3-publish/01-srt-clients.md)                           |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [SRT cameras and servers](../3-publish/02-srt-cameras-and-servers.md)   |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [WebRTC clients](../3-publish/03-webrtc-clients.md)                     | WHIP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)                                                                                                                                                                      |
| [WebRTC servers](../3-publish/04-webrtc-servers.md)                     | WHEP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)                                                                                                                                                                      |
| [RTSP clients](../3-publish/05-rtsp-clients.md)                         | UDP, TCP, RTSPS                            | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec  |
| [RTSP cameras and servers](../3-publish/06-rtsp-cameras-and-servers.md) | UDP, UDP-Multicast, TCP, RTSPS             | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec  |
| [RTMP clients](../3-publish/07-rtmp-clients.md)                         | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [RTMP cameras and servers](../3-publish/08-rtmp-cameras-and-servers.md) | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [HLS cameras and servers](../3-publish/09-hls-cameras-and-servers.md)   | Low-Latency HLS, MP4-based HLS, legacy HLS | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC)                                                                                                                                                                                |
| [MPEG-TS](../3-publish/10-mpeg-ts.md)                                   | MPEG-TS over UDP, MPEG-TS over Unix socket | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [RTP](../3-publish/11-rtp.md)                                           | RTP over UDP                               | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec |

We provide instructions for publishing with the following devices:

- [Raspberry Pi Cameras](../3-publish/12-raspberry-pi-cameras.md)
- [Generic webcams](../3-publish/13-generic-webcams.md)

We provide instructions for publishing with the following software:

- [FFmpeg](../3-publish/14-ffmpeg.md)
- [GStreamer](../3-publish/15-gstreamer.md)
- [OBS Studio](../3-publish/16-obs-studio.md)
- [Python and OpenCV](../3-publish/17-python-opencv.md)
- [Golang](../3-publish/18-golang.md)
- [Unity](../3-publish/19-unity.md)
- [Web browsers](../3-publish/20-web-browsers.md)
