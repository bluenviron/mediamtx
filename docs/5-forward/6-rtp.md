# RTP

|           | supported codecs                                                                          |
| --------- | ----------------------------------------------------------------------------------------- |
| **video** | AV1, VP9, VP8, H265, H264, MPEG-4 Video (H263, Xvid), MPEG-1/2 Video, M-JPEG              |
| **audio** | Opus, MPEG-4 Audio (AAC), MPEG-1/2 Audio (MP3), AC-3, G726, G722, G711 (PCMA, PCMU), LPCM |
| **other** | KLV, MPEG-TS, any RTP-compatible codec                                                    |

_MediaMTX_ supports forwarding streams as raw UDP/RTP packets targeted to a specific IP and port. This streaming technique has two main use cases:

- forwarding streams to legacy software/hardware that supports only raw RTP packets.
- forwarding streams as multicast RTP packets, to save bandwidth on local networks.

Add the target URL inside `dest` of a `forward` entry:

```yml
paths:
  mypath:
    forward:
      - dest: udp+rtp://238.0.0.1:1234
```

All the tracks of the stream are sent as RTP packets, through a single UDP socket, and are distinguished by their payload type.

If the destination IP is a multicast IP, packets are sent through the default multicast interface, picked by the operating system. It is possible to specify the interface manually by using the `interface` parameter:

```yml
paths:
  mypath:
    forward:
      - dest: udp+rtp://238.0.0.1:1234?interface=eth0
```
