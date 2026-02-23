# Publish a stream

Live streams can be published to the server with the following protocols and codecs:

| protocol                                                   | variants                                   | codecs                                                                                                                                                                                                                                                 |
| ---------------------------------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [SRT clients](02-srt-clients.md)                           |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [SRT cameras and servers](03-srt-cameras-and-servers.md)   |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [WebRTC clients](04-webrtc-clients.md)                     | WHIP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)                                                                                                                                                                      |
| [WebRTC servers](05-webrtc-servers.md)                     | WHEP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)                                                                                                                                                                      |
| [RTSP clients](06-rtsp-clients.md)                         | UDP, TCP, RTSPS                            | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec  |
| [RTSP cameras and servers](07-rtsp-cameras-and-servers)    | UDP, UDP-Multicast, TCP, RTSPS             | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec  |
| [RTMP clients](08-rtmp-clients.md)                         | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [RTMP cameras and servers](09-rtmp-cameras-and-servers.md) | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [HLS cameras and servers](10-hls-cameras-and-servers.md)   | Low-Latency HLS, MP4-based HLS, legacy HLS | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC)                                                                                                                                                                                |
| [MPEG-TS](11-mpeg-ts.md)                                   | MPEG-TS over UDP, MPEG-TS over Unix socket | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [RTP](12-rtp.md)                                           | RTP over UDP                               | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec |

We provide instructions for publishing with the following devices:

- [Raspberry Pi Cameras](13-raspberry-pi-cameras.md)
- [Generic webcams](14-generic-webcams.md)

We provide instructions for publishing with the following software:

- [FFmpeg](15-ffmpeg.md)
- [GStreamer](16-gstreamer.md)
- [OBS Studio](17-obs-studio.md)
- [Python and OpenCV](18-python-opencv.md)
- [Golang](19-golang.md)
- [Unity](20-unity.md)
- [Web browsers](21-web-browsers.md)
