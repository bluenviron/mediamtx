# RTSP clients

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption (see [RTSP-specific features](../4-other/23-rtsp-specific-features.md)). In order to publish a stream to the server with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

Some clients that can publish with RTSP are [FFmpeg](15-ffmpeg.md), [GStreamer](16-gstreamer.md), [OBS Studio](17-obs-studio.md), [Python and OpenCV](18-python-opencv.md).
