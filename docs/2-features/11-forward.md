# Forward streams

Incoming streams can be natively forwarded to other servers with the following protocols:

- [RTSP](#rtsp)
- [RTMP](#rtmp)
- [SRT](#srt)

It is also possible to use [FFmpeg](#ffmpeg) to perform the forwarding.

## RTSP

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: rtsp://user:pass@host:port/path
```

## RTMP

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: rtmp://user:pass@host:port/path#streamKey
```

## SRT

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: srt://host:port?streamid=streamid
```

## FFmpeg

When the destination requires transcoding, filtering or a protocol that is not supported by `forward`, use _FFmpeg_ inside the `runOnAvailable` parameter instead:

```yml
pathDefaults:
  runOnAvailable: >
    ffmpeg -i rtsp://localhost:$RTSP_PORT/$MTX_PATH
    -c copy
    -f rtsp rtsp://other-server:8554/another-path
  runOnAvailableRestart: yes
```
