# Media-over-QUIC servers

|           | supported codecs          |
| --------- | ------------------------- |
| **video** | AV1, VP9, VP8, H265, H264 |
| **audio** | Opus, MPEG-4 Audio (AAC)  |

In order to ingest a Media-over-QUIC stream from a remote server, add the corresponding URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    source: moqt://user:pass@host:port/path
    # Transport protocol used to pull the stream. available values are "quic", "webtransport".
    moqTransport: quic
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

If host is a stateless IPv6 address, wrap the address in square brackets and write the zone separator `%` as `%25`, for example `moqt://[fe80::1234:5678:9abc:def0%25eth0]:8892/path`.
