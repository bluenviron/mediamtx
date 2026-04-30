# WebRTC servers

|           | supported codecs              |
| --------- | ----------------------------- |
| **video** | AV1, VP9, VP8, H265, H264     |
| **audio** | Opus, G722, G711 (PCMA, PCMU) |

In order to ingest a WebRTC stream from a remote server, add the corresponding WHEP URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    # url of the source stream, in the format whep://host:port/path (HTTP) or wheps:// (HTTPS)
    source: wheps://host:port/path
```

If the remote server is a _MediaMTX_ instance, remember to add a `/whep` suffix after the stream name, since in _MediaMTX_ [it's part of the WHEP URL](../4-read/02-webrtc.md):

```yml
paths:
  proxied:
    source: whep://host:port/mystream/whep
```
