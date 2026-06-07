# HLS

|           | supported codecs               |
| --------- | ------------------------------ |
| **video** | AV1, VP9, H265, H264           |
| **audio** | Opus, FLAC, MPEG-4 Audio (AAC) |
| **other** | KLV (MPEG-TS-based HLS only)   |

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate an HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Some clients that can read with HLS are [FFmpeg](08-ffmpeg.md), [GStreamer](09-gstreamer.md), [VLC](10-vlc.md) and [web browsers](07-web-browsers.md).

_MediaMTX_ supports generating HLS in several variants (including Low-Latency mode), and provides various parameters to tune HLS generation. These are listed in the [configuration file](../5-references/1-configuration-file.md).

HLS can also be used to [scale the server](../2-features/20-scalability.md) through a CDN.
