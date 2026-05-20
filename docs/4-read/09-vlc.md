# VLC

VLC can read a stream from the server by using the [RTSP](03-rtsp.md), [RTMP](04-rtmp.md), [HLS](05-hls.md) and [SRT](01-srt.md) protocols. The recommended one is RTSP.

In order to minimize latency, it is also suggested to change the `Network caching` parameter of VLC:

1. Open VLC, _Tools_, _Preferences_, _Show Settings_, _All_, page _Input / Codecs_.
2. Find the _Network caching (ms)_ parameter, set it to `50`. Save

## VLC and RTSP

Open the following URL:

```
rtsp://localhost:8554/mystream
```

### RTSP and Ubuntu

The VLC shipped with Ubuntu 21.10 doesn't support playing RTSP due to a license issue (read [here](https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=982299) and [here](https://stackoverflow.com/questions/69766748/cvlc-cannot-play-rtsp-omxplayer-instead-can)). To fix the issue, remove the default VLC instance and install the snap version:

```sh
sudo apt purge -y vlc
snap install vlc
```

### Encrypted RTSP

At the moment VLC doesn't support reading encrypted RTSP streams (RTSPS). However, you can use a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/) or a local _MediaMTX_ instance to decrypt streams before reading them.

## VLC and RTMP

Open the following URL:

```
rtmp://localhost/mystream
```

## VLC and SRT

Open the following URL:

```
srt://localhost:8890?streamid=read:mystream
```
