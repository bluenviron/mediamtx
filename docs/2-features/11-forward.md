# Forward streams

Incoming streams can be natively forwarded to other servers with the following protocols:

- [Media-over-QUIC](#media-over-quic)
- [SRT](#srt)
- [WebRTC](#webrtc)
- [RTSP](#rtsp)
- [RTMP](#rtmp)

It is also possible to use [FFmpeg](#ffmpeg) to perform the forwarding.

We provide instructions to forward streams to the following services:

- [YouTube](#youtube)
- [Twitch](#twitch)

## Media-over-QUIC

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: moqt://user:pass@host:port/path
        # Transport protocol used to forward the stream. available values are "quic", "webtransport".
        moqTransport: quic
        # If the destination TLS certificate is self-signed or invalid, you can provide the
        # fingerprint of the certificate in order to validate it anyway. It can be obtained by running:
        # openssl s_client -connect dest_ip:dest_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
        # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
        destFingerprint:
```

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
        # Token to insert in the Authorization: Bearer header.
        whipBearerToken: ""
        # If the destination is HTTPS and the destination TLS certificate is self-signed
        # or invalid, you can provide the fingerprint of the certificate in order to
        # validate it anyway. It can be obtained by running:
        # openssl s_client -connect dest_ip:dest_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
        # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
        destFingerprint:
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

## YouTube

Open YouTube, sign in with a valid account, search for the _Create_, _Go Live_ button, click it. In the next page, search for _Default stream key_, copy the key somewhere, then search for the _Stream URL_ section and copy the URL. Insert the stream URL and the stream key in _MediaMTX_ in this way:

```yml
paths:
  mypath:
    forward:
      # Use an hashtag to separate the URL from the stream key.
      # "a.rtmp.youtube.com/live2" was the URL YouTube was reporting last time this documentation was updated. Check that it is still correct.
      # Also replace rtmp:// with rtmps:// in order to enable encryption in-transit.
      - dest: rtmps://a.rtmp.youtube.com/live2#streamKey
        # Last time we checked, the TLS certificate was invalid. Use destFingerprint to validate it anyway.
        destFingerprint: 131409734af825d8e994cce4cb205bc5de6c657271673650aabd8c504c27e891
```

**Warning**: YouTube requires streams to have both a video and an audio track. Video-only streams are silently rejected.

## Twitch

Open Twitch, sign in with a valid account, go to _Settings_, _Stream_. Search for the _Primary Stream Key_, copy it somewhere. Then search for _Recommended Ingest Endpoints_, that should be in [this page](https://help.twitch.tv/s/twitch-ingest-recommendation?language=en_US). Copy one of the URLs. Insert the URL and the stream key in _MediaMTX_ in this way:

```yml
paths:
  mypath:
    forward:
      # Use an hashtag to separate the URL from the stream key.
      # "rtmp://ingest.global-contribute.live-video.net/app" was the URL Twitch was reporting last time this documentation was updated. Check that it is still correct.
      # Also replace rtmp:// with rtmps:// in order to enable encryption in-transit.
      - dest: rtmps://ingest.global-contribute.live-video.net/app#streamKey
```

**Warning**: Enhanced Broadcasting (2K / HEVC / Dual Format).
Although MediaMTX supports Enhanced RTMP (including H.265/HEVC and multitrack video), the simple `forward` mechanism above only works with classic single-track H.264.
Twitch Enhanced Broadcasting requires a special Automatic Stream Configuration (ASC) handshake that returns a dynamic ERTMP ingest endpoint. The static URL listed in the Recommended Ingest Endpoints page does not support this flow and will reject HEVC / multitrack streams.
For Enhanced Broadcasting use a compatible client (OBS Studio with Enhanced Broadcasting enabled) that performs the ASC handshake itself.
