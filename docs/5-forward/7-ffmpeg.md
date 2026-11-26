# FFmpeg

When the destination requires transcoding, filtering or a protocol that is not supported by `forward`, use _FFmpeg_ inside the `runOnAvailable` parameter instead:

```yml
pathDefaults:
  runOnAvailable: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnAvailableRestart: yes
```
