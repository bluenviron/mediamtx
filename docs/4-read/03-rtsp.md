# RTSP clients

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption (read [RTSP-specific features](../2-features/26-rtsp-specific-features.md)). In order to read a stream with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

Some clients that can read with RTSP are [FFmpeg](06-ffmpeg.md), [GStreamer](07-gstreamer.md) and [VLC](08-vlc.md).
