# Route absolute timestamps

Some streaming protocols allow to route absolute timestamps, associated with each frame, that are useful for synchronizing several video or data streams together. In particular, _MediaMTX_ supports receiving absolute timestamps with the following protocols and devices:

- HLS (through the `EXT-X-PROGRAM-DATE-TIME` tag in playlists)
- RTSP (through RTCP reports, when `useAbsoluteTimestamp` is `true` in settings)
- WebRTC (through RTCP reports, when `useAbsoluteTimestamp` is `true` in settings)
- Raspberry Pi Camera

and supports sending absolute timestamps with the following protocols:

- HLS (through the `EXT-X-PROGRAM-DATE-TIME` tag in playlists)
- RTSP (through RTCP reports)
- WebRTC (through RTCP reports)

A library that can read absolute timestamps with HLS is [gohlslib](https://github.com/bluenviron/gohlslib).

A library that can read absolute timestamps with RTSP is [gortsplib](https://github.com/bluenviron/gortsplib).

A browser can read absolute timestamps with WebRTC if it exposes the [estimatedPlayoutTimestamp](https://www.w3.org/TR/webrtc-stats/#dom-rtcinboundrtpstreamstats-estimatedplayouttimestamp) statistic.
