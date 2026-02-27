# RTSP

RTSP is a protocol that allows to publish and read streams. It supports different underlying transport protocols and encryption (see [RTSP-specific features](../4-other/23-rtsp-specific-features.md)). In order to read a stream with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

Some clients that can read with RTSP are [FFmpeg](07-ffmpeg.md), [GStreamer](08-gstreamer.md) and [VLC](09-vlc.md).
