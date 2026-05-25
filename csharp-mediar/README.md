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
| Mux Ogg (Opus / Vorbis / FLAC)           | ✅                    |
| Demux Matroska / WebM (SimpleBlock)      | ✅                    |
| Mux Matroska / WebM (SimpleBlock)        | ✅                    |
| Read + write SRT                          | ✅                    |
| Read + write WebVTT                       | ✅                    |
| Read + write ASS / SSA v4+                | ✅                    |
| H.264 / H.265 Annex-B ↔ AVCC / HVCC      | ✅                    |
| Emulation-prevention add / strip          | ✅                    |
| Extract audio (MP4 → M4A)                | ✅                    |
| Mux a/v together                         | ✅ (passthrough)      |
| Embed SRT as `tx3g` into MP4             | ✅                    |
| Extract `tx3g` SRT from MP4              | ✅                    |
| PCM sample-format conversion (s16/s24/s32/f32) | ✅              |
| **PCM decoder (via `IAudioDecoder`)**     | ✅                    |
| **G.711 µ-law / A-law decoder**           | ✅                    |
| **FLAC decoder (RFC 9639)**               | ✅                    |
| **Seek API (`SeekAsync`)**                | ✅                    |
| BenchmarkDotNet micro-benchmarks         | ✅                    |
| AAC / AC-3 / E-AC-3 decoder              | ❌ (patent encumbered) |
| H.264 / H.265 / AV1 decoder              | ❌ (out of scope)     |
| Vorbis / Opus decoder                    | 🟡 (deferred, royalty-free but large) |
| MP3 / ALAC decoder                       | 🟡 (deferred)         |
| Matroska lacing (XIPH/EBML/FIXED)        | ❌ (not yet)          |

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
│   ├── Mediar.Containers.Ogg/          Ogg page reader + muxer
│   ├── Mediar.Containers.Matroska/     Matroska / WebM demuxer + muxer (EBML)
│   ├── Mediar.Codecs/                  IAudioDecoder / IVideoDecoder abstractions
│   ├── Mediar.Codecs.Pcm/              PCM sample-format conversion + decoder
│   ├── Mediar.Codecs.G711/             G.711 µ-law / A-law encode + decode
│   ├── Mediar.Codecs.Flac.Decoder/     Native FLAC decoder (RFC 9639)
│   ├── Mediar.Bitstream/               H.264 / H.265 Annex-B ↔ AVCC/HVCC helpers
│   ├── Mediar.Subtitles.Srt/           SRT read + write
│   ├── Mediar.Subtitles.WebVtt/        WebVTT read + write
│   ├── Mediar.Subtitles.Ass/           ASS / SSA v4+ read + write
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

### Decoders

Decoders implement `Mediar.Codecs.IAudioDecoder` / `IVideoDecoder` and produce
interleaved float PCM frames out of compressed samples. Mediar ships
implementations for codecs whose specifications are unencumbered.

```csharp
using Mediar.Codecs;
using Mediar.Codecs.Flac.Decoder;

var decoder = new FlacDecoderFactory().Create(track.Codec);
foreach (var sample in samples)
{
    using var frame = decoder.Decode(sample.Data.Span);
    // frame.Pcm is ReadOnlySpan<float>, interleaved L,R,L,R,...
}
```

A `DecoderRegistry` lets you plug in your own (or a third-party
permissively-licensed) decoder for codecs Mediar doesn't ship:

```csharp
DecoderRegistry.Default.RegisterAudio(CodecId.Aac, new MyOwnAacDecoderFactory());
```

### Muxing Ogg / Matroska / WebM

`Mediar.Containers.Ogg.OggMuxer` and `Mediar.Containers.Matroska.MatroskaMuxer`
implement the same `IMediaMuxer` contract as the MP4 muxer:

```csharp
await using var output = File.Create("audio.opus.ogg");
await using var mux = new OggMuxer(output);
mux.AddTrack(track);
await mux.StartAsync();
foreach (var s in samples) await mux.WriteSampleAsync(s);
await mux.FinishAsync();
```

### H.264 / H.265 bitstream conversion

`Mediar.Bitstream.AnnexBAvccConverter` converts NAL units between Annex-B
(`00 00 01` start-code framing used by raw `.h264` / TS / RTP) and the
length-prefixed AVCC / HVCC format expected inside MP4 / Matroska samples:

```csharp
using Mediar.Bitstream;

byte[] avcc = AnnexBAvccConverter.AnnexBToLengthPrefixed(annexB, lengthSize: 4);
byte[] back = AnnexBAvccConverter.LengthPrefixedToAnnexB(avcc, lengthSize: 4);
```

The scanner uses AVX2 when available for fast start-code detection.

### Seeking

Every demuxer exposes `SeekAsync(TimeSpan)`; the next call to
`ReadSamplesAsync` resumes at-or-before the requested time:

```csharp
await using var demuxer = MediarOperations.Open("movie.mp4");
await demuxer.SeekAsync(TimeSpan.FromSeconds(30));
await foreach (var s in demuxer.ReadSamplesAsync()) { /* ... */ }
```

* MP4 / WAV use random-access tables for true O(1) byte-level seek; for
  video tracks the cursor snaps back to the nearest preceding keyframe.
* MP3 / FLAC / ADTS walk frame headers without reading payload bytes.
* Matroska skips whole clusters whose timecodes fall before the target,
  then drops early blocks within the chosen cluster.
* Ogg skips packets whose PTS falls before the target (no-op for Opus
  streams where per-packet sample count is not pre-computed).

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
* ITU-T G.711 (µ-law / A-law PCM companding)
* ITU-T H.264 Annex B / ISO/IEC 14496-15 (AVCC)
* Matroska / EBML specifications (matroska.org)
* SubRip community spec (`.srt`)
* W3C WebVTT
* Advanced SubStation Alpha v4+ (community spec)

No FFmpeg code was copied. Where format layouts were cross-checked against
reference implementations, only the public specifications were used to write
Mediar's parsers.
