# FFmpeg

FFmpeg can publish a stream to the server by acting as a [RTSP client](07-rtsp-clients.md), [RTMP client](09-rtmp-clients.md), [SRT client](02-srt-clients.md), [WebRTC client](05-webrtc-clients.md) or by sending [MPEG-TS packets](12-mpeg-ts.md) or [RTP packets](13-rtp.md). The recommended way is acting as a RTSP client.

## FFmpeg as a RTSP client

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f rtsp rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

## FFmpeg as a RTMP client

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f flv rtmp://localhost:1935/mystream
```

## FFmpeg and MPEG-TS over UDP

In _MediaMTX_ configuration, add a path with `source: udp+mpegts://238.0.0.1:1234`. Then:

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f mpegts 'udp://238.0.0.1:1234?pkt_size=1316'
```

## FFmpeg and MPEG-TS over Unix socket

In _MediaMTX_ configuration, add a path with `source: unix+mpegts:///tmp/socket.sock`. Then:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f mpegts unix:/tmp/socket.sock
```

## FFmpeg and RTP over UDP

In _MediaMTX_ configuration, add a path with `source: udp+rtp://238.0.0.1:1234` and a valid `rtpSDP` (read [RTP](13-rtp.md)). Then:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f rtp udp://238.0.0.1:1234?pkt_size=1316
```

## FFmpeg as a SRT client

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f mpegts 'srt://localhost:8890?streamid=publish:stream&pkt_size=1316'
```

## FFmpeg as a WebRTC client

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-f lavfi -i "sine=frequency=1000:sample_rate=48000" \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a libopus -ar 48000 -ac 2 -b:a 128k \
-f whip http://localhost:8889/stream/whip
```

WARNING: in case of FFmpeg 8.0, a video track and an audio track must both be present.
