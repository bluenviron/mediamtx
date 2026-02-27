# RTP

The server supports ingesting RTP streams, transmitted with UDP packets.

In order to read a UDP RTP stream, edit `mediamtx.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  mypath:
    source: udp+rtp://238.0.0.1:1234
    rtpSDP: |
      v=0
      o=- 123456789 123456789 IN IP4 192.168.1.100
      s=H264 Video Stream
      c=IN IP4 192.168.1.100
      t=0 0
      m=video 5004 RTP/AVP 96
      a=rtpmap:96 H264/90000
      a=fmtp:96 profile-level-id=42e01e;packetization-mode=1;sprop-parameter-sets=Z0LAHtkDxWhAAAADAEAAAAwDxYuS,aMuMsg==
```

`rtpSDP` must contain a valid SDP, that is a description of the RTP session.

Some clients that can publish with UDP and MPEG-TS are [FFmpeg](15-ffmpeg.md) and [GStreamer](16-gstreamer.md).

## Devices
