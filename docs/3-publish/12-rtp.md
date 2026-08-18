# RTP

|           | supported codecs                                                                          |
| --------- | ----------------------------------------------------------------------------------------- |
| **video** | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG              |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM |
| **other** | KLV, MPEG-TS, any RTP-compatible codec                                                    |

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
```

`rtpSDP` must contain a valid SDP, that is a description of the RTP session.

If the listening IP is a multicast IP, MediaMTX will listen for incoming packets on the default multicast interface, picked by the operating system. It is possible to specify the interface manually by using the interface parameter:

```yml
paths:
  mypath:
    source: udp+rtp://238.0.0.1:1234?interface=eth0
    rtpSDP: |
      v=0
      o=- 123456789 123456789 IN IP4 192.168.1.100
      s=H264 Video Stream
      c=IN IP4 192.168.1.100
      t=0 0
      m=video 5004 RTP/AVP 96
      a=rtpmap:96 H264/90000
```

It is possible to restrict who can send packets by using the source parameter:

```yml
paths:
  mypath:
    source: udp+rtp://238.0.0.1:1234?source=192.168.3.5
    rtpSDP: |
      v=0
      o=- 123456789 123456789 IN IP4 192.168.1.100
      s=H264 Video Stream
      c=IN IP4 192.168.1.100
      t=0 0
      m=video 5004 RTP/AVP 96
      a=rtpmap:96 H264/90000
```

Some clients that can publish with UDP and MPEG-TS are [FFmpeg](16-ffmpeg.md) and [GStreamer](17-gstreamer.md).
