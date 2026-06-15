# GStreamer

GStreamer can publish a stream to the server by acting as a [RTSP client](07-rtsp-clients.md), [RTMP client](09-rtmp-clients.md), [SRT client](02-srt-clients.md), [WebRTC client](05-webrtc-clients.md) or by sending [MPEG-TS packets](12-mpeg-ts.md). The recommended way is acting as a RTSP client.

## GStreamer as a RTSP client

```sh
gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream \
filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! queue ! s.sink_0 \
d.audio_0 ! queue ! s.sink_1
```

If the stream is video only:

```sh
gst-launch-1.0 filesrc location=file.mp4 ! qtdemux name=d \
d.video_0 ! rtspclientsink location=rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

## GStreamer as a RTMP client

```sh
gst-launch-1.0 -v flvmux name=mux ! rtmpsink location=rtmp://localhost/stream \
videotestsrc ! video/x-raw,width=1280,height=720,format=I420 ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60 ! video/x-h264,profile=high ! mux. \
audiotestsrc ! audioconvert ! avenc_aac ! mux.
```

## GStreamer as a SRT client

```sh
gst-launch-1.0 -v mpegtsmux name=mux ! srtsink uri="srt://localhost:8890?streamid=publish:mystream&pkt_size=1316" \
videotestsrc ! video/x-raw,width=1280,height=720,format=I420 ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60 ! video/x-h264,profile=high ! mux. \
audiotestsrc ! audioconvert ! avenc_aac ! mux.
```

The resulting stream will be available on path `/mystream`.

## GStreamer and MPEG-TS over UDP

```sh
gst-launch-1.0 -v mpegtsmux name=mux alignment=1 ! udpsink host=238.0.0.1 port=1234 \
videotestsrc ! video/x-raw,width=1280,height=720,format=I420 ! x264enc speed-preset=ultrafast bitrate=3000 key-int-max=60 ! video/x-h264,profile=high ! mux. \
audiotestsrc ! audioconvert ! avenc_aac ! mux.
```

## GStreamer as a WebRTC client

Make sure that GStreamer version is at least 1.22, and that if the codec is H264, the profile is baseline. Use the `whipclientsink` element:

```sh
gst-launch-1.0 videotestsrc \
! video/x-raw,width=1920,height=1080,format=I420 \
! x264enc speed-preset=ultrafast bitrate=2000 \
! video/x-h264,profile=baseline \
! whipclientsink signaller::whip-endpoint=http://localhost:8889/mystream/whip
```
