# Mediar — high-performance .NET media toolkit

Mediar is a from-scratch C# library for **container-level** media operations on
common file formats. It is designed for high throughput, zero unnecessary
allocations and AOT-friendly deployment, and ships **without any dependency on
FFmpeg or other LGPL/GPL native code**.

The library targets `net10.0` and is MIT licensed.

## Why?

Everyday media tasks — extracting audio from a video, embedding subtitles,
remuxing — are pure container operations. They do **not** require decoding or
re-encoding the underlying audio/video, and therefore do **not** require an
implementation of patented or license-encumbered codecs. Mediar focuses on
exactly this layer:

* Parse containers, walk sample tables, expose tracks/samples.
* Re-mux samples into a new container without touching their bytes.
* Round-trip subtitle text formats.

Anything that genuinely requires decoding (e.g. converting MP3 to WAV PCM,
producing a JPEG thumbnail) is **explicitly out of scope** of this version and
documented as such — patches that add codec implementations are welcome but
must come with their own licensing analysis.

## What it can do today

| Capability                                | Status                |
|------------------------------------------|-----------------------|
| Demux MP4 / MOV / M4A / M4V / 3GP        | ✅                    |
| Mux MP4 (passthrough samples)            | ✅                    |
| Demux WAV (PCM 16/24/32 + IEEE float)    | ✅                    |
| Mux WAV (PCM + IEEE float)               | ✅                    |
| Demux MP3 (with ID3v1/v2 skipping)       | ✅                    |
| Mux MP3 (frame concatenation)            | ✅                    |
| Demux FLAC (native, frame-level)         | ✅                    |
| Mux FLAC (native, frame passthrough)     | ✅                    |
| Demux + mux raw AAC ADTS                 | ✅                    |
| Demux Ogg (Opus / Vorbis / FLAC)         | ✅                    |
| Demux Matroska / WebM (SimpleBlock)      | ✅                    |
| Read + write SRT                          | ✅                    |
| Read + write WebVTT                       | ✅                    |
| Extract audio (MP4 → M4A)                | ✅                    |
| Mux a/v together                         | ✅ (passthrough)      |
| Embed SRT as `tx3g` into MP4             | ✅                    |
| Extract `tx3g` SRT from MP4              | ✅                    |
| PCM sample-format conversion (s16/s24/s32/f32) | ✅              |
| BenchmarkDotNet micro-benchmarks         | ✅                    |
| AAC / Opus / Vorbis decoder              | ❌ (out of scope)     |
| H.264 / H.265 / AV1 decoder              | ❌ (out of scope)     |
| Matroska lacing (XIPH/EBML/FIXED)        | ❌ (not yet)          |
| Mux Matroska / WebM / Ogg                | ❌ (planned)          |

## Project layout

```
csharp-mediar/
├── src/
│   ├── Mediar.Core/                    abstractions + IO primitives
│   ├── Mediar.Containers.IsoBmff/      MP4 / MOV / M4A demux + mux
│   ├── Mediar.Containers.Wav/          WAV read + write
│   ├── Mediar.Containers.Mp3/          MP3 + ID3 demuxer + frame muxer
│   ├── Mediar.Containers.Flac/         FLAC demuxer + native muxer
│   ├── Mediar.Containers.Adts/         AAC ADTS demuxer + muxer
│   ├── Mediar.Containers.Ogg/          Ogg page reader + logical-stream demuxer
│   ├── Mediar.Containers.Matroska/     Matroska / WebM demuxer (EBML)
│   ├── Mediar.Codecs.Pcm/              PCM sample-format conversion helpers
│   ├── Mediar.Subtitles.Srt/           SRT read + write
│   ├── Mediar.Subtitles.WebVtt/        WebVTT read + write
│   └── Mediar/                         high-level facade
├── tests/Mediar.Tests/                 xUnit round-trip + parser tests
├── bench/Mediar.Bench/                 BenchmarkDotNet micro-benchmarks
└── samples/Mediar.Cli/                 mediar CLI (info / extract / mux / embed)
```

## Quick start

```bash
cd csharp-mediar
dotnet build Mediar.slnx
dotnet test  Mediar.slnx

# Run micro-benchmarks
dotnet run -c Release --project bench/Mediar.Bench -- --filter '*'
```

### CLI

```bash
# Inspect a file
mediar info input.mp4

# Extract audio (no re-encoding)
mediar extract-audio input.mp4 audio.m4a

# Mux a video and an audio source together
mediar mux-av video.mp4 audio.m4a out.mp4

# Embed an SRT file as a tx3g subtitle track inside an MP4
mediar embed-srt input.mp4 subs.srt out.mp4 eng

# Extract a tx3g subtitle track back to SRT
mediar extract-srt input.mp4 subs.srt
```

### Library

```csharp
using Mediar;

await MediarOperations.ExtractAudioAsync("input.mp4", "audio.m4a");
await MediarOperations.EmbedSrtAsync("video.mp4", "subs.srt", "with-subs.mp4", "eng");

await using var demuxer = MediarOperations.Open("track.flac");
foreach (var t in demuxer.Tracks)
    Console.WriteLine(t);
```

## Performance characteristics

Hot-path conventions used throughout the library:

* `Span<byte>` / `ReadOnlySpan<byte>` ref-struct readers/writers (no allocations).
* `ArrayPool<byte>.Shared` / `MemoryPool<byte>.Shared` for transient buffers.
* `System.IO.RandomAccess` positioned reads — no shared `Stream.Position` state.
* `BinaryPrimitives` for endian conversions; no boxing or LINQ on hot paths.
* MP4 metadata is parsed once; samples are read lazily on enumeration.
* Demuxers expose `IAsyncEnumerable<MediaSample>` so callers can stream
  through gigabytes of media without buffering the whole file.

> **Honest disclaimer.** The codec/container scope here is intentionally
> narrow. Building something feature-comparable to FFmpeg is hundreds of
> person-years of work; Mediar gives you the parts that don't require codec
> implementations and a clean place to slot in your own (or a third-party
> permissively-licensed) decoder when you need it.

## Licensing

Mediar itself is MIT. The codec **identifiers** (`CodecId.H264`, `CodecId.Aac`,
…) are just enum constants and have no licensing implications. The library
does **not** ship any implementation of those codecs, and so does not transfer
any patent license obligations to consumers. If you intend to *decode* AAC or
H.264 samples carried by an MP4 that Mediar produced, the patent landscape for
those codecs applies to whichever decoder you plug in — that decision is
yours.

Reference specs used:

* ISO/IEC 14496-12 (ISO BMFF)
* ISO/IEC 14496-14 (MP4 file format)
* Microsoft RIFF/WAVE
* ISO/IEC 11172-3 (MPEG-1 audio)
* ISO/IEC 13818-7 (MPEG-2 / ADTS AAC)
* RFC 9639 (FLAC)
* RFC 3533 (Ogg)
* Matroska / EBML specifications (matroska.org)
* SubRip community spec (`.srt`)
* W3C WebVTT

No FFmpeg code was copied. Where format layouts were cross-checked against
reference implementations, only the public specifications were used to write
Mediar's parsers.
