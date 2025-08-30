# Forward streams to other servers

To forward incoming streams to another server, use _FFmpeg_ inside the `runOnReady` parameter:

```yml
pathDefaults:
  runOnReady: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnReadyRestart: yes
```
