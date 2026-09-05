# RTSP clients

|           | supported codecs                                                                          |
| --------- | ----------------------------------------------------------------------------------------- |
| **video** | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, MJPEG               |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM |
| **other** | KLV, MPEG-TS, any RTP-compatible codec                                                    |

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption. In order to publish a stream to the server with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

Some clients that can publish with RTSP are [FFmpeg](17-ffmpeg.md), [GStreamer](18-gstreamer.md), [OBS Studio](19-obs-studio.md), [Python and OpenCV](20-python-opencv.md).

Advanced RTSP features and settings are described in [RTSP-specific features](../2-features/26-rtsp-specific-features.md).

## MPEG-TS inside RTSP

Some RTSP clients encode tracks with MPEG-TS before sending them to the server, causing the server to see a single "MPEG-TS" track, and preventing track conversion from a protocol to another.

It is possible to automatically demux these MPEG-TS-encoded streams by toggling `rtspDemuxMpegts`:

```yml
pathDefaults:
  rtspDemuxMpegts: true
```
