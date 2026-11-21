# RTSP

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

The stream can also be forwarded through a WebSocket, which is useful to pass through HTTP proxies and gateways:

```yml
paths:
  mypath:
    forward:
      # Use rtsp+ws:// for a plain WebSocket and rtsps+ws:// for a secure WebSocket.
      - dest: rtsps+ws://user:pass@host:port/path
        # HTTP headers to send when opening the WebSocket.
        destHeaders:
          X-Custom-Header: value
```
