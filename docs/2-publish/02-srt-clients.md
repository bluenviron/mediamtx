# SRT clients

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to publish a stream to the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=publish:mystream&pkt_size=1316
```

Replace `mystream` with any name you want. The resulting stream will be available on path `/mystream`.

If you need to use the standard stream ID syntax instead of the custom one in use by this server, see [Standard stream ID syntax](../4-other/21-srt-specific-features.md#standard-stream-id-syntax).

If you want to publish a stream by using a client in listening mode (i.e. with `mode=listener` appended to the URL), read the next section.

Some clients that can publish with SRT are [FFmpeg](15-ffmpeg.md), [GStreamer](16-gstreamer.md), [OBS Studio](17-obs-studio.md).
