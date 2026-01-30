# Always-available streams

When the publisher or source of a stream is offline, the server can be configured to fill gaps in the stream with a video that is played on repeat until a publisher comes back online. This allows readers to stay connected regardless of the state of the stream. The offline video and any future online stream are concatenated without decoding or re-encoding packets, using the original codec.

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

By default, the server uses a default offline video with the text "STREAM IS OFFLINE". This can be changed by importing the video from a MP4 file:

```yml
paths:
  mypath:
    alwaysAvailable: true
    # Path to the MP4 file that is played on repeat. If not provided, a default video will be used instead.
    alwaysAvailableFile: "./h264.mp4"
```
