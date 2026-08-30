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
