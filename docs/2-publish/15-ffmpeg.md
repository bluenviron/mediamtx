# FFmpeg

FFmpeg can publish a stream to the server in several ways. The recommended one consists in publishing with RTSP.

## FFmpeg and RTSP

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f rtsp rtsp://localhost:8554/mystream
```

The resulting stream will be available on path `/mystream`.

## FFmpeg and RTMP

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

In _MediaMTX_ configuration, add a path with `source: udp+rtp://238.0.0.1:1234` and a valid `rtpSDP` (see [RTP](12-rtp.md)). Then:

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-f rtp udp://238.0.0.1:1234?pkt_size=1316
```

## FFmpeg and SRT

```sh
ffmpeg -re -stream_loop -1 -i file.mp4 -c copy -f mpegts 'srt://localhost:8890?streamid=publish:stream&pkt_size=1316'
```

## FFmpeg and WebRTC

```sh
ffmpeg -re -f lavfi -i testsrc=size=1280x720:rate=30 \
-f lavfi -i "sine=frequency=1000:sample_rate=48000" \
-c:v libx264 -pix_fmt yuv420p -preset ultrafast -b:v 600k \
-c:a libopus -ar 48000 -ac 2 -b:a 128k \
-f whip http://localhost:8889/stream/whip
```

WARNING: in case of FFmpeg 8.0, a video track and an audio track must both be present.
