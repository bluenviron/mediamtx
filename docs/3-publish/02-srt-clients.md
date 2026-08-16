# SRT clients

|           | supported codecs                                      |
| --------- | ----------------------------------------------------- |
| **video** | H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3  |
| **other** | KLV                                                   |

SRT is a protocol that allows to publish and read live data stream, providing encryption, integrity and a retransmission mechanism. It is usually used to transfer media streams encoded with MPEG-TS. In order to publish a stream to the server with the SRT protocol, use this URL:

```
srt://localhost:8890?streamid=publish:mystream&pkt_size=1316
```

Replace `mystream` with any name you want. The resulting stream will be available on path `/mystream`.

If you need to use the standard stream ID syntax instead of the custom one in use by this server, read [Standard stream ID syntax](../2-features/25-srt-specific-features.md#standard-stream-id-syntax).

If you want to publish a stream by using a client in listening mode (i.e. with `mode=listener` appended to the URL), read the next section.

Some clients that can publish with SRT are [FFmpeg](17-ffmpeg.md), [GStreamer](18-gstreamer.md), [OBS Studio](19-obs-studio.md).

## Publishing with SRTLA

SRTLA (SRT Link Aggregation) allows bonding multiple network connections for improved reliability during mobile streaming. SRTLA-capable clients (BELABOX, IRL Pro, Moblin) can connect to the SRTLA port (default `:8891`) and register multiple links that are aggregated into a single SRT stream.

To publish via SRTLA, point the sender to the SRTLA port instead of the SRT port:

```
srtla://yourserver:8891?streamid=publish:mystream
```

The SRTLA receiver bonds all registered connections and forwards the aggregated stream to the local SRT server. No additional SRT configuration is needed — path selection, authentication, and codec handling work identically to direct SRT publishing.

See [SRTLA configuration](../2-features/25-srt-specific-features.md#srtla-srt-link-aggregation) for server-side setup.
