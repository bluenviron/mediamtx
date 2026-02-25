# Read a stream

Live streams can be read from the server with the following protocols and codecs:

| protocol               | variants                                   | codecs                                                                                                                                                                                                                                                 |
| ---------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [SRT](02-srt.md)       |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [WebRTC](03-webrtc.md) | WHEP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)<br/>**Other**: KLV                                                                                                                                                   |
| [RTSP](04-rtsp.md)     | UDP, UDP-Multicast, TCP, RTSPS             | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec |
| [RTMP](05-rtmp.md)     | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [HLS](06-hls.md)       | Low-Latency HLS, MP4-based HLS, legacy HLS | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC)                                                                                                                                                                                |

We provide instructions for reading with the following software:

- [FFmpeg](07-ffmpeg.md)
- [GStreamer](08-gstreamer.md)
- [VLC](09-vlc.md)
- [OBS Studio](10-obs-studio.md)
- [Unity](11-unity.md)
- [Web browsers](12-web-browsers.md)
