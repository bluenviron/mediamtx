# SRT cameras and servers

|           | supported codecs                                      |
| --------- | ----------------------------------------------------- |
| **video** | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3  |
| **other** | KLV                                                   |

In order to ingest a SRT stream from a remote server, camera or client in listening mode (i.e. with `mode=listener` appended to the URL), add the corresponding URL into the `source` parameter of a path:

```yml
paths:
  proxied:
    source: srt://host:port?streamid=streamid
```
