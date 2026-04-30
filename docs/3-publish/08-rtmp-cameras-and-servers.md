# RTMP cameras and servers

|       | supported codecs                                                              |
| ----- | ----------------------------------------------------------------------------- |
| video | AV1, VP9, H265, H264                                                          |
| audio | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G711 (PCMA, PCMU), LPCM |

You can use _MediaMTX_ to connect to one or several existing RTMP servers and read their media streams:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtmp://user:pass@host:port/path
    source: rtmp://original-url
```

The resulting stream will be available on path `/proxied`.
