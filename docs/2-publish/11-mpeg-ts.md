# MPEG-TS

The server supports ingesting MPEG-TS streams, shipped in two different ways (UDP packets or Unix sockets).

In order to read a UDP MPEG-TS stream, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  mypath:
    source: udp+mpegts://238.0.0.1:1234
```

Where `238.0.0.1` is the IP for listening packets, in this case a multicast IP.

If the listening IP is a multicast IP, _MediaMTX_ will listen for incoming packets on the default multicast interface, picked by the operating system. It is possible to specify the interface manually by using the `interface` parameter:

```yml
paths:
  mypath:
    source: udp+mpegts://238.0.0.1:1234?interface=eth0
```

It is possible to restrict who can send packets by using the `source` parameter:

```yml
paths:
  mypath:
    source: udp+mpegts://0.0.0.0:1234?source=192.168.3.5
```

Some clients that can publish with UDP and MPEG-TS are [FFmpeg](15-ffmpeg.md) and [GStreamer](16-gstreamer.md).

Unix sockets are more efficient than UDP packets and can be used as transport by specifying the `unix+mpegts` scheme:

```yml
paths:
  mypath:
    source: unix+mpegts:///tmp/socket.sock
```
