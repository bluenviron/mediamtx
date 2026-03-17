# RTSP clients

RTSP is a protocol that allows to publish and read streams. It supports several underlying transport protocols and encryption. In order to publish a stream to the server with the RTSP protocol, use this URL:

```
rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

Some clients that can publish with RTSP are [FFmpeg](15-ffmpeg.md), [GStreamer](16-gstreamer.md), [OBS Studio](17-obs-studio.md), [Python and OpenCV](18-python-opencv.md).

Advanced RTSP features and settings are described in [RTSP-specific features](../4-other/23-rtsp-specific-features.md).

## MPEG-TS inside RTSP

Some RTSP clients encode tracks with MPEG-TS before sending them to the server, causing the server to see a single "MPEG-TS" track, and preventing track conversion from a protocol to another.

It's possible to automatically demux these MPEG-TS-encoded streams, by toggling `rtspDemuxMpegts`:

```yml
pathDefaults:
  # Demux MPEG-TS over RTSP into elementary streams.
  # When enabled, RTSP publishers sending MP2T/90000 will be demultiplexed
  # and their elementary streams (H.264, H.265, AAC, etc.) exposed as native tracks.
  # This allows HLS, WebRTC, and other outputs to work transparently with MPEG-TS sources.
  rtspDemuxMpegts: true
```
