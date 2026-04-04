# RTMP clients

RTMP is a protocol that allows to read and publish streams. It supports encryption, read [RTMP-specific features](../2-features/27-rtmp-specific-features.md). Streams can be published to the server by using the URL:

```
rtmp://localhost/mystream
```

The resulting stream will be available on path `/mystream`.

Some clients that can publish with RTMP are [FFmpeg](14-ffmpeg.md), [GStreamer](15-gstreamer.md), [OBS Studio](16-obs-studio.md).
