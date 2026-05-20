# FFmpeg

FFmpeg can read a stream from the server the [RTSP](03-rtsp.md), [RTMP](04-rtmp.md), [HLS](05-hls.md) and [SRT](01-srt.md) protocols. The recommended one is RTSP.

## FFmpeg and RTSP

```sh
ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
```

## FFmpeg and RTMP

```sh
ffmpeg -i rtmp://localhost/mystream -c copy output.mp4
```

In order to read tracks that make use of modern codecs (AV1, VP9, H265, Opus, FLAC, AC-3) and in order to read multiple video or audio tracks, the `-rtmp_enhanced_codecs` flag must be present and filled with the following values:

```sh
ffmpeg -rtmp_enhanced_codecs ac-3,av01,avc1,ec-3,fLaC,hvc1,.mp3,mp4a,Opus,vp09 \
-i rtmp://localhost/mystream -c copy output.mp4
```

## FFmpeg and SRT

```sh
ffmpeg -i 'srt://localhost:8890?streamid=read:test' -c copy output.mp4
```
