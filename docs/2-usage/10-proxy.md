# Proxy requests to other servers

The server allows to proxy incoming requests to other servers or cameras. This is useful to expose servers or cameras behind a NAT. Edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  "~^proxy_(.+)$":
    # If path name is a regular expression, $G1, G2, etc will be replaced
    # with regular expression groups.
    source: rtsp://other-server:8554/$G1
    sourceOnDemand: yes
```

All requests addressed to `rtsp://server:8854/proxy_a` will be forwarded to `rtsp://other-server:8854/a` and so on.
