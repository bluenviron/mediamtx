# RTMP clients

RTMP is a protocol that allows to read and publish streams. It supports encryption, see [RTMP-specific features](../4-other/24-rtmp-specific-features.md). Streams can be published to the server by using the URL:

```
rtmp://localhost/mystream
```

The resulting stream will be available on path `/mystream`.

Some clients that can publish with RTMP are [FFmpeg](15-ffmpeg.md), [GStreamer](16-gstreamer.md), [OBS Studio](17-obs-studio.md).
