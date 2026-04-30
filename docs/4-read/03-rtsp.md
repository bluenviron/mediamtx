# RTSP clients

|       | supported codecs                                                                          |
| ----- | ----------------------------------------------------------------------------------------- |
| video | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG              |
| audio | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM |
| other | KLV, MPEG-TS, any RTP-compatible codec                                                    |

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption (read [RTSP-specific features](../2-features/27-rtsp-specific-features.md)). In order to read a stream with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

Some clients that can read with RTSP are [FFmpeg](06-ffmpeg.md), [GStreamer](07-gstreamer.md) and [VLC](08-vlc.md).
