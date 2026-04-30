# HLS

|           | supported codecs             |
| --------- | ---------------------------- |
| **video** | AV1, VP9, H265, H264         |
| **audio** | Opus, MPEG-4 Audio (AAC)     |
| **other** | KLV (MPEG-TS-based HLS only) |

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate an HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Some clients that can read with HLS are [FFmpeg](06-ffmpeg.md), [GStreamer](07-gstreamer.md), [VLC](08-vlc.md) and [web browsers](13-web-browsers.md).

HLS content can be generated in several variants:

- MPEG-TS: uses MPEG-TS segments, for maximum compatibility.
- fMP4: uses fragmented MP4 segments, more efficient.
- Low-Latency: based on blocking requests that are unlocked as soon as content is available.

All HLS pameters are listed in the [configuration file](../5-references/1-configuration-file.md).

HLS can also be used to [scale the server](../2-features/20-scaling.md) through a CDN.
