# VLC

VLC can read a stream from the server in several ways. The recommended one consists in reading with RTSP:

```sh
vlc --network-caching=50 rtsp://localhost:8554/mystream
```

## RTSP and Ubuntu compatibility

The VLC shipped with Ubuntu 21.10 doesn't support playing RTSP due to a license issue (see [here](https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=982299) and [here](https://stackoverflow.com/questions/69766748/cvlc-cannot-play-rtsp-omxplayer-instead-can)). To fix the issue, remove the default VLC instance and install the snap version:

```sh
sudo apt purge -y vlc
snap install vlc
```

## Encrypted RTSP compatibility

At the moment VLC doesn't support reading encrypted RTSP streams. However, you can use a proxy like [stunnel](https://www.stunnel.org) or [nginx](https://nginx.org/) or a local _MediaMTX_ instance to decrypt streams before reading them.
