# Media-over-QUIC

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
