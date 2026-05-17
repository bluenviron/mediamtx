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

## Dynamic RTSP proxy with capture groups

You can also use regex capture groups in the path and substitute them in `source`.

```yml
paths:
  "~^mycam/([^/]+)/([^/]+)/(.+)$":
    source: rtsp://$1:$2/$3
    sourceOnDemand: yes
```

In this example:

- incoming request: `rtsp://proxy:8555/mycam/192.168.1.35/8554/stream1`
- resolved source: `rtsp://192.168.1.35:8554/stream1`

The placeholders `$1`, `$2`, etc. are replaced with the regex capture groups from the path.
