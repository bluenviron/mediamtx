# WebRTC

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

If the remote server is a _MediaMTX_ instance, remember to add a `/whip` suffix after the stream name, since in _MediaMTX_ [it's part of the WHIP URL](../3-publish/03-webrtc.md).
