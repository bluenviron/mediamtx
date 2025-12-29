# Remuxing, re-encoding, compression

To change the format, codec or compression of a stream, use _FFmpeg_ or _GStreamer_ together with _MediaMTX_. For instance, to re-encode an existing stream, that is available in the `/original` path, and publish the resulting stream in the `/compressed` path, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  compressed:
  original:
    runOnReady: >
      ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
        -c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k
        -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    runOnReadyRestart: yes
```
