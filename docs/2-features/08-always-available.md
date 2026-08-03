# Always-available

When the publisher or source of a stream is offline, the server can be configured to fill gaps in the stream with an offline segment that is played on repeat until a publisher comes back online. This allows readers to stay connected regardless of the state of the stream. The offline segment and online stream are concatenated without re-encoding any frame, using the original codec.

This feature can be enabled by toggling the `alwaysAvailable` flag and filling `alwaysAvailableTracks`:

```yml
paths:
  mypath:
    alwaysAvailable: true
    alwaysAvailableTracks:
      # Available values are: AV1, VP9, H265, H264, Opus, MPEG4Audio, G711, LPCM
      - codec: H264
        # in case of MPEG4Audio, G711, LPCM, sampleRate and ChannelCount must be provided too.
        #  sampleRate: 48000
        #  channelCount: 2
        #  in case of G711, muLaw must be provided too.
        #  muLaw: false
```

By default, the server uses a default offline segment with the text "STREAM IS OFFLINE". The segment can be replaced with an external MP4 file:

```yml
paths:
  mypath:
    alwaysAvailable: true
    alwaysAvailableFile: "./h264.mp4"
```

Any alternative source -- whether an offline media file or a fallback stream -- must be format-compatible with the primary live stream:

* The number of tracks and their codec types must match exactly (e.g. one H264 video track and one MPEG4Audio audio track).
* Audio parameters (sample rate, channel count, AAC profile) must match exactly. A mismatch causes the source to be rejected at connection time.
* Video resolution and bitrate may differ; SPS/PPS are swapped automatically on source switch.

The safest approach is to configure all sources with the same encoder settings.
