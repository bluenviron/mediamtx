# Read a stream

Live streams can be read from the server with the following protocols and codecs:

| protocol                                 | variants                                   | codecs                                                                                                                                                                                                                                                 |
| ---------------------------------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [SRT clients](../4-read/01-srt.md)       |                                            | **Video**: H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3<br/>**Other**: KLV                                                                                                |
| [WebRTC clients](../4-read/02-webrtc.md) | WHEP                                       | **Video**: AV1, VP9, VP8, H265, H264<br/>**Audio**: Opus, G722, G711 (PCMA, PCMU)<br/>**Other**: KLV                                                                                                                                                   |
| [RTSP clients](../4-read/03-rtsp.md)     | UDP, UDP-Multicast, TCP, RTSPS             | **Video**: AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM<br/>**Other**: KLV, MPEG-TS, any RTP-compatible codec |
| [RTMP clients](../4-read/04-rtmp.md)     | RTMP, RTMPS, Enhanced RTMP                 | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM                                                                                                                           |
| [HLS](../4-read/05-hls.md)               | Low-Latency HLS, MP4-based HLS, legacy HLS | **Video**: AV1, VP9, H265, H264<br/>**Audio**: Opus, MPEG-4 Audio (AAC)                                                                                                                                                                                |

We provide instructions for reading with the following software:

- [FFmpeg](../4-read/06-ffmpeg.md)
- [GStreamer](../4-read/07-gstreamer.md)
- [VLC](../4-read/08-vlc.md)
- [OBS Studio](../4-read/09-obs-studio.md)
- [Python and OpenCV](../4-read/10-python-opencv.md)
- [Golang](../4-read/11-golang.md)
- [Unity](../4-read/12-unity.md)
- [Web browsers](../4-read/13-web-browsers.md)
