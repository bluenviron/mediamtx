# HLS

HLS is a protocol that works by splitting streams into segments, and by serving these segments and a playlist with the HTTP protocol. You can use _MediaMTX_ to generate a HLS stream, that is accessible through a web page:

```
http://localhost:8888/mystream
```

and can also be accessed without using the browsers, by software that supports the HLS protocol (for instance VLC or _MediaMTX_ itself) by using this URL:

```
http://localhost:8888/mystream/index.m3u8
```

Some clients that can read with HLS are [FFmpeg](07-ffmpeg.md), [GStreamer](08-gstreamer.md), [VLC](09-vlc.md) and [web browsers](12-web-browsers.md).
