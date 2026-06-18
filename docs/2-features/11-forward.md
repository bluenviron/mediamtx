# Forward

Incoming streams can be forwarded to other servers with `pushTargets`. This forwards the original stream without running external processes.

```yml
pathDefaults:
  pushTargets:
    - url: rtmp://other-server/live/$MTX_PATH
    - url: rtsp://other-server:8554/$MTX_PATH
    - url: srt://other-server:8890?streamid=publish:$MTX_PATH
```

Each target is started when the path is created and reconnects automatically when the stream or remote server becomes unavailable.

The `url` field supports the following schemes: `rtmp`, `rtmps`, `rtsp`, `rtsps` and `srt`. It can contain the variable `$MTX_PATH`. If the path name is a regular expression, regular expression groups can be used with `$G1`, `$G2`, and so on:

```yml
paths:
  '~^camera_(.+)$':
    pushTargets:
      - url: rtsp://other-server:8554/$G1
```

When the destination requires transcoding, filtering or a protocol that is not supported by `pushTargets`, use _FFmpeg_ inside the `runOnReady` parameter instead:

```yml
pathDefaults:
  runOnReady: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnReadyRestart: true
```
