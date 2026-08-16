# Forward streams

Incoming streams can be natively forwarded to other servers with the following protocols:

- [SRT](#srt)
- [WebRTC](#webrtc)
- [RTSP](#rtsp)
- [RTMP](#rtmp)

It is also possible to use [FFmpeg](#ffmpeg) to perform the forwarding.

## SRT

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: srt://host:port?streamid=streamid
```

## WebRTC

We support forwarding streams by using the WebRTC protocol and the WHIP extension. Add the target URL inside `dest` of a `forward` entry. Use `whip://` for HTTP and `whips://` for HTTPS:

```yml
paths:
  mypath:
    forward:
      - dest: whip://host:port/mystream/whip
        whipBearerToken: mytoken
```

If the remote server is a _MediaMTX_ instance, remember to add a `/whip` suffix after the stream name, since in _MediaMTX_ [it's part of the WHIP URL](../3-publish/05-webrtc-clients.md).

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
