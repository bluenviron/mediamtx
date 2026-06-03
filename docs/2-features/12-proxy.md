# Proxy

The server allows to proxy incoming requests to other servers or cameras. This is useful to expose servers or cameras behind a NAT. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  "~^(.+)$":
    # If path name is a regular expression, $G1, $G2, etc will be replaced
    # with regular expression groups.
    source: rtsp://other-server:8554/$G1
    sourceOnDemand: yes
```

All requests addressed to `rtsp://server:8854/a` will be forwarded to `rtsp://other-server:8554/a` and so on.

## Dynamic proxy with regex capture groups

The server supports dynamic proxy paths with regex capture groups for all supported protocols (RTSP, RTMP, HLS, SRT, WebRTC, MPEG-TS, RTP). You can use regex capture groups in the path and substitute them in the `source` field.

### RTSP example

```yml
paths:
  "~^mycam/([^/]+)/([^/]+)/(.+)$":
    source: rtsp://$G1:$G2/$G3
    sourceOnDemand: yes
```

In this example:

- incoming request: `rtsp://proxy:8555/mycam/192.168.1.35/8554/stream1`
- resolved source: `rtsp://192.168.1.35:8554/stream1`

The placeholders `$G1`, `$G2`, etc. are replaced with the regex capture groups from the path.

### Other protocols

The same mechanism works for all other protocols:

```yml
paths:
  "~^rtmp_(.+)$":
    source: rtmp://backend.example.com:1935/$G1
    sourceOnDemand: yes

  "~^hls_(.+)_(.+)$":
    source: http://$G1/$G2/index.m3u8
    sourceOnDemand: yes

  "~^srt_([^_]+)_([^_]+)_(.+)$":
    source: srt://$G1:$G2/$G3
    sourceOnDemand: yes
```

Use `$G1`, `$G2`, ... to reference regex capture groups. You can also use `$MTX_QUERY` to include the query string from the incoming request in the proxied source.
