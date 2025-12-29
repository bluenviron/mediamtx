# Route absolute timestamps

Some streaming protocols allow to route absolute timestamps, associated with each frame, that are useful for synchronizing several video or data streams together. In particular, _MediaMTX_ supports receiving absolute timestamps with the following protocols and devices:

- HLS
- RTSP
- WebRTC
- Raspberry Pi Camera

and supports sending absolute timestamps with the following protocols:

- HLS
- RTSP
- WebRTC

By default, absolute timestamps of incoming frames are not used, instead they are replaced with the system timestamp. This prevents users from arbitrarily changing recording dates, and also allows to support sources that do not send absolute timestamps. It is possible to preserve original absolute timestamps by toggling the `useAbsoluteTimestamp` parameter:

```yml
pathDefaults:
  # Use absolute timestamp of frames, instead of replacing them with the current time.
  useAbsoluteTimestamp: false
```

## Absolute timestamp in HLS

In the HLS protocol, absolute timestamps are routed by adding a `EXT-X-PROGRAM-DATE-TIME` tag before each segment:

```
#EXTM3U
#EXT-X-VERSION:9
#EXT-X-MEDIA-SEQUENCE:20
#EXT-X-TARGETDURATION:2
#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:02:00Z
#EXTINF:2,
segment1.mp4
#EXT-X-PROGRAM-DATE-TIME:2015-02-05T01:04:00Z
#EXTINF:2,
segment2.mp4
```

The `EXT-X-PROGRAM-DATE-TIME` value is the absolute timestamp that corresponds to the first frame of the segment. The absolute timestamp of following frames can be obtained by summing `EXT-X-PROGRAM-DATE-TIME` with the relative frame timestamp.

A library that can read absolute timestamps with HLS is [gohlslib](https://github.com/bluenviron/gohlslib).

## Absolute timestamp in RTSP and WebRTC

In RTSP and WebRTC, absolute timestamps are routed through periodic RTCP sender reports:

```
        0                   1                   2                   3
        0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
       +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
header |V=2|P|    RC   |   PT=SR=200   |             length            |
       +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
       |                         SSRC of sender                        |
       +=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+=+
sender |              NTP timestamp, most significant word             |
info   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
       |             NTP timestamp, least significant word             |
       +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
       |                         RTP timestamp                         |
...
```

The sender report contains a reference absolute timestamp (NTP timestamp) and a reference relative timestamp (RTP timestamp). The absolute timestamp of each frame can be computed by using these values together with the RTP timestamp of the frame (shipped with each frame), through the formula:

```
frame_abs_timestamp = ref_ntp_timestamp + (frame_rtp_timestamp - ref_rtp_timestamp) / clock_rate
```

A library that can read absolute timestamps with RTSP is [gortsplib](https://github.com/bluenviron/gortsplib).

A browser can read absolute timestamps with WebRTC if it exposes the [estimatedPlayoutTimestamp](https://www.w3.org/TR/webrtc-stats/#dom-rtcinboundrtpstreamstats-estimatedplayouttimestamp) statistic.
