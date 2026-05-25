# Real-file fixture sources

The integration tests in `RealFileIntegrationTests.cs` exercise every Mediar
demuxer against actual media files. The fixtures themselves are **not**
committed (size + licensing); supply them locally and point the
`MEDIAR_FIXTURES` environment variable at the directory.

When `MEDIAR_FIXTURES` is unset (e.g. on CI without fixtures), the tests
silently skip.

## Expected layout

```
$MEDIAR_FIXTURES/
  sample.wav    sample.flac   sample.mp3    sample.aac
  sample.ogg    sample.opus   sample.aiff   sample.avi
  sample.mp4    sample.m4a    sample.mkv    sample.webm
```

## A known-working set (Creative-Commons / public domain)

| File         | Source                                                                                                 |
|--------------|--------------------------------------------------------------------------------------------------------|
| `sample.mp3` | <https://www.soundhelix.com/examples/mp3/SoundHelix-Song-1.mp3>                                        |
| `sample.flac`| <https://helpguide.sony.net/high-res/sample1/v1/data/Sample_BeeMoved_96kHz24bit.flac.zip> (unzip)       |
| `sample.mp4` | <https://download.samplelib.com/mp4/sample-5s.mp4>                                                     |
| `sample.m4a` | <https://filesamples.com/samples/audio/m4a/sample3.m4a>                                                |
| `sample.aac` | <https://filesamples.com/samples/audio/aac/sample3.aac>                                                |
| `sample.wav` | <https://download.samplelib.com/wav/sample-3s.wav>                                                     |
| `sample.aiff`| <https://filesamples.com/samples/audio/aiff/sample3.aiff>                                              |
| `sample.ogg` | <https://filesamples.com/samples/audio/ogg/sample3.ogg>                                                |
| `sample.opus`| <https://filesamples.com/samples/audio/opus/sample3.opus>                                              |
| `sample.mkv` | <https://filesamples.com/samples/video/mkv/sample_640x360.mkv>                                         |
| `sample.avi` | <https://filesamples.com/samples/video/avi/sample_640x360.avi>                                         |
| `sample.webm`| <https://download.samplelib.com/webm/sample-5s.webm>                                                   |

## Running

```pwsh
$env:MEDIAR_FIXTURES = "C:\path\to\fixtures"
dotnet test --filter "FullyQualifiedName~RealFileIntegrationTests"
```
