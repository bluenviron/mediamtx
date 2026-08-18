# HLS cameras and servers

|           | supported codecs               |
| --------- | ------------------------------ |
| **video** | AV1, VP9, H265, H264           |
| **audio** | Opus, FLAC, MPEG-4 Audio (AAC) |
| **other** | KLV (MPEG-TS-based HLS only)   |

HLS is a streaming protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to connect to one or several existing HLS servers and read their media streams:

```yml
paths:
  proxied:
    # Use http:// for plain HTTP and https:// for HTTPS.
    source: http://user:pass@host:port/path
    # If the source is HTTPS and the source TLS certificate is self-signed
    # or invalid, you can provide the fingerprint of the certificate in order to
    # validate it anyway. It can be obtained by running:
    # openssl s_client -connect source_ip:source_port </dev/null 2>/dev/null | sed -n '/BEGIN/,/END/p' > server.crt
    # openssl x509 -in server.crt -noout -fingerprint -sha256 | cut -d "=" -f2 | tr -d ':'
    sourceFingerprint:
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

The resulting stream will be available on path `/proxied`.
