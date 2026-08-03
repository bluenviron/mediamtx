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
    source: http://user:pass@host:port/path
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

The resulting stream will be available on path `/proxied`.
