# Media-over-QUIC clients

|           | supported codecs                                        |
| --------- | ------------------------------------------------------- |
| **video** | AV1, VP9, VP8, H265, H264                               |
| **audio** | Opus, FLAC, MPEG-4 Audio (AAC), G711 (PCMA, PCMU), LPCM |

Media-over-QUIC is a streaming protocol built upon cutting edge protocols (QUIC, HTTP3) and browser APIs (WebTransport, WebCodecs) that can be used to publish and read live media streams. It's slightly faster than WebRTC, has an advanced data recovery mechanism, supports additional codecs (FLAC and future ones), supports B-frames and is less complicated to route.

There are some limitations and requirements that are listed in [Publish with Media-over-QUIC clients](../3-publish/01-moq-clients.md).

You can read a stream with Media-over-QUIC and a web browser by visiting:

```
https://localhost:8892/mystream
```

The only clients that can currently read with Media-over-QUIC are [Web browsers](07-web-browsers.md).
