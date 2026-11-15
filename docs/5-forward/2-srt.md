# SRT

|           | supported codecs                                      |
| --------- | ----------------------------------------------------- |
| **video** | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3  |
| **other** | KLV                                                   |

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: srt://host:port?streamid=streamid
```
