# GStreamer

GStreamer can read a stream from the server in several ways. The recommended one consists in reading with RTSP.

## GStreamer and RTSP

```sh
gst-launch-1.0 rtspsrc location=rtsp://127.0.0.1:8554/mystream latency=0 ! decodebin ! autovideosink
```

## GStreamer and RTMP

GStreamer supports reading streams with the RTMP protocol, but the path must be composed by at least two elements, for instance `mypath/mysubpath`:

```sh
gst-launch-1.0 rtmpsrc location=rtmp://localhost/mypath/mysubpath ! flvdemux name=d \
d.video ! queue ! decodebin ! autovideosink
```

## GStreamer and SRT

```sh
gst-launch-1.0 srtsrc uri="srt://localhost:8890?streamid=read:mystream" ! tsdemux ! decodebin ! autovideosink
```

## GStreamer and WebRTC

GStreamer supports reading streams with WebRTC/WHEP, although track codecs must be specified in advance through the `video-caps` and `audio-caps` parameters. Furthermore, if audio is not present, `audio-caps` must be set anyway and must point to a PCMU codec. For instance, the command for reading a video-only H264 stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=PCMU,payload=0,clock-rate=8000" \
! rtph264depay ! decodebin ! autovideosink
```

While the command for reading an audio-only Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint="http://127.0.0.1:8889/stream/whep" use-link-headers=true \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! rtpopusdepay ! decodebin ! autoaudiosink
```

While the command for reading a H264 and Opus stream is:

```sh
gst-launch-1.0 whepsrc whep-endpoint=http://127.0.0.1:8889/stream/whep use-link-headers=true \
video-caps="application/x-rtp,media=video,encoding-name=H264,payload=127,clock-rate=90000" \
audio-caps="application/x-rtp,media=audio,encoding-name=OPUS,payload=111,clock-rate=48000,encoding-params=(string)2" \
! decodebin ! autovideosink
```
