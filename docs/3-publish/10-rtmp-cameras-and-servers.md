# RTMP cameras and servers

|           | supported codecs                                                                    |
| --------- | ----------------------------------------------------------------------------------- |
| **video** | AV1, VP9, H265, H264                                                                |
| **audio** | Opus, FLAC, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM |

You can use _MediaMTX_ to connect to one or several existing RTMP servers and read their media streams:

```yml
paths:
  proxied:
    source: rtmp://user:pass@host:port/path#streamKey
```

If username or password contain special characters (like ?, :, etc), they need to be [url-encoded](https://www.urlencoder.org/).

The resulting stream will be available on path `/proxied`.
