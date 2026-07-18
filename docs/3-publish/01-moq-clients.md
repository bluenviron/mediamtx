# Media-over-QUIC clients

|           | supported codecs          |
| --------- | ------------------------- |
| **video** | AV1, VP9, VP8, H265, H264 |
| **audio** | Opus, MPEG-4 Audio (AAC)  |

Media-over-QUIC is a streaming protocol built upon cutting edge protocols (QUIC, HTTP3) and browser APIs (WebTransport, WebCodecs) that can be used to publish and read live media streams. It's slightly faster than WebRTC, has an advanced data recovery mechanism, supports additional codecs (FLAC and future ones), supports B-frames and is less complicated to route.

Media-over-QUIC has a wide range of features and variants, most of them in active development. We currently support the following:

- We support `draft-18` of the [main specification](https://datatracker.ietf.org/doc/html/draft-ietf-moq-transport-18).
- We only support using Media-over-QUIC through browsers and in particular through the WebTransport API. We do not support using QUIC directly.
- We support the `PUBLISH` and `SUBSCRIBE` messages only, which are the ones meant to be used with a routing solution like _MediaMTX_.
- We use the MOQT Streaming Format (MSF) to advertise tracks, described in [this specification](https://datatracker.ietf.org/doc/html/draft-ietf-moq-msf-00).
- We use the Low Overhead Media Container (LOC) to ship frames, described in [this specification](https://datatracker.ietf.org/doc/draft-ietf-moq-loc/).
- We host web pages through a HTTP/2 listener and host the WebTransport endpoint through a HTTP/3 listener. This hybrid setup allows to use self-signed certificates, that are normally forbidden in pure HTTP/3.

There are some server requirements:

- HTTPS is mandatory.
- Clients must be able to access both the HTTP/2 listener (`:8892`) and the HTTP/3 listener (`:8892`), the latter of which runs over UDP.

And there are some client (browser) requirements:

- If the server certificate is self-signed, browser must support the [serverCertificatesHashes option](https://caniuse.com/mdn-api_webtransport_webtransport_options_servercertificatehashes_parameter) (all except iOS Safari do).
- Browser must support [WebTransport](https://caniuse.com/webtransport) and [WebCodecs](https://caniuse.com/webcodecs) (all modern browsers do)
- When publishing tracks, the browser to support [MediaStreamTrackProcessor](https://caniuse.com/mdn-api_mediastreamtrackprocessor) (only Chrome does).

You can publish a stream with Media-over-QUIC and a web browser by visiting:

```
https://localhost:8892/mystream/publish
```

The only clients that can currently publish with Media-over-QUIC are [Web browsers](16-web-browsers.md).
