# SRT

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to read a stream from the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=read:mystream
```

Replace `mystream` with the path name.

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](../4-other/21-srt-specific-features.md#standard-stream-id-syntax).

Some clients that can read with SRT are [FFmpeg](07-ffmpeg.md), [GStreamer](08-gstreamer.md) and [VLC](09-vlc.md).
