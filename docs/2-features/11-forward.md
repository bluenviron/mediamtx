# Forward

To forward incoming streams to another server, use _FFmpeg_ inside the `runOnAvailable` parameter:

```yml
pathDefaults:
  runOnAvailable: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnAvailableRestart: yes
```
