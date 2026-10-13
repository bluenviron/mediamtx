# WebRTC servers

|           | supported codecs              |
| --------- | ----------------------------- |
| **video** | AV1, VP9, VP8, H265, H264     |
| **audio** | Opus, G722, G711 (PCMA, PCMU) |

In order to ingest a WebRTC stream from a remote server, add the corresponding WHEP URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # Use whep:// for HTTP and wheps:// for HTTPS.
    source: whep://host:port/path
    # If the source is HTTPS and the source TLS certificate is self-signed
    # or invalid, you can provide the fingerprint of the certificate in order to
    # validate it anyway. It can be obtained by running:
    # openssl s_client -connect source_ip:source_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
    # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
    sourceFingerprint:
    # Token to insert in the Authorization: Bearer header.
    whepBearerToken: ""
```

If host is a stateless IPv6 address, wrap the address in square brackets and write the zone separator `%` as `%25`, for example `whep://[fe80::1234:5678:9abc:def0%25eth0]:8889/path`.

If the remote server is a _MediaMTX_ instance, remember to add a `/whep` suffix after the stream name, since in _MediaMTX_ [it's part of the WHEP URL](../4-read/03-webrtc.md):

```yml
paths:
  proxied:
    source: whep://host:port/mystream/whep
```
