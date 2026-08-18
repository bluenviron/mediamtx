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
      # use whip:// for HTTP and whips:// for HTTPS.
      - dest: whip://host:port/mystream/whip
        # If the destination is HTTPS and the destination TLS certificate is self-signed
        # or invalid, you can provide the fingerprint of the certificate in order to
        # validate it anyway. It can be obtained by running:
        # openssl s_client -connect dest_ip:dest_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
        # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
        destFingerprint:
        # Token to insert in the Authorization: Bearer header.
        whipBearerToken: ""
```

If the remote server is a _MediaMTX_ instance, remember to add a `/whip` suffix after the stream name, since in _MediaMTX_ [it's part of the WHIP URL](../3-publish/05-webrtc-clients.md).

## RTSP

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      # Use rtsp:// for plain RTSP and rtsps:// for encrypted RTSP.
      - dest: rtsp://user:pass@host:port/path
        # If the destination is RTSPS and the destination TLS certificate is self-signed
        # or invalid, you can provide the fingerprint of the certificate in order to
        # validate it anyway. It can be obtained by running:
        # openssl s_client -connect dest_ip:dest_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
        # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
        destFingerprint:
```

## RTMP

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      # Use rtmp:// for plain RTMP and rtmps:// for encrypted RTMP.
      - dest: rtmp://user:pass@host:port/path#streamKey
        # If the destination is RTMPS and the destination TLS certificate is self-signed
        # or invalid, you can provide the fingerprint of the certificate in order to
        # validate it anyway. It can be obtained by running:
        # openssl s_client -connect dest_ip:dest_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
        # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
        destFingerprint:
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
