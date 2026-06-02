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
| **Demux AVI (RIFF, idx1 seeking, LIST/INFO)** | ✅               |
| **Demux AIFF / AIFC (PCM + µ-law / A-law / fl32)** | ✅          |
| **Demux CAF (lpcm / ALAC passthrough / AAC / G.711 / Opus)** | ✅ |
| **Mux CAF (PCM)**                         | ✅                    |
| **Demux 8SVX (Amiga IFF)**                | ✅                    |
| **Mux 8SVX (PCM-S8)**                     | ✅                    |
| **Demux VOC (Creative Voice; v1.x + v1.20+ blocks)** | ✅         |
| **Mux VOC (PCM-U8 / PCM-S16 / G.711)**    | ✅                    |
| **Demux MP2 / MP1 raw streams**           | ✅                    |
| **Demux + mux raw GSM 06.10**             | ✅                    |
| **Demux + mux AMR-NB / AMR-WB**           | ✅                    |
| **RF64 / BW64 (large-WAV ds64 chunk)**    | ✅                    |
| **MIDI File API (SMF format 0/1/2 reader + writer)** | ✅         |
| **Playlist File API (M3U / M3U8 / PLS / XSPF / WPL)** | ✅        |
| **Per-file metadata extraction (title, artist, album, date, genre, track #, geo-location, …)** | ✅ (WAV / MP3 / FLAC / Ogg / MP4 / Matroska / AVI / AIFF / 8SVX / CAF) |
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
| **Vorbis I decoder — full audio synthesis** | ✅ (floor 1 + residue 0/1/2 + IMDCT + sin² window + lap) |
| **Seek API (`SeekAsync`)**                | ✅                    |
| BenchmarkDotNet micro-benchmarks         | ✅                    |
| AAC / AC-3 / E-AC-3 decoder              | ❌ (patent encumbered) |
| H.264 / H.265 / AV1 decoder              | ❌ (out of scope — multi-month implementation) |
| **AV2 transmux (carry-through MP4 / MKV)** | ✅ (`av02` / `V_AV2` codec id; passthrough only — no decoder, spec not yet published) |
| Vorbis audio synthesis (floor 1 + residue + overlap-add) | ✅ (shipped) |
| Opus decoder                              | 🟡 (deferred, royalty-free but large) |
| MP3 decoder                              | 🟢 (pipeline shipped; full status, performance baseline, and conformance/perf TODOs in [`src/Mediar.Codecs.Mp3.Decoder/README.md`](src/Mediar.Codecs.Mp3.Decoder/README.md)) |
| ALAC decoder                             | 🟡 (deferred)         |
| Matroska lacing (XIPH/EBML/FIXED)        | ❌ (not yet)          |

### Imaging — File APIs for ~120 still-image formats

A separate `Mediar.Imaging.*` family of projects exposes a unified
`IImageReader` / `IImageWriter` contract behind a single `MediarImage.Open(path)`
entry point. Magic-byte detection lives in `ImageFormatDetector.Detect(...)`
and an `ImageFormat` enum spans every requested format.

| Tier | Capability | Formats |
| --- | --- | --- |
| Full pixel decode | demux + decode + metadata | BMP / DIB, PNG (+ APNG), GIF (LZW + animation), TIFF (uncompressed / PackBits / Deflate / LZW), TGA (types 1/2/3/9/10/11 + 2.0 footer), PCX (1 / 8 / 24 bpp), HDR (Radiance RGBE), PNM (P1-P6), XPM3, ICNS (sub-image enumerator), DDS (uncompressed + BC1/BC2/BC3/BC4/BC5/BC6H **all 14 modes per Khronos KDF spec 1.4**/BC7), JPEG (baseline SOF0 + progressive SOF2 + lossless SOF3, grayscale + 4:4:4 / 4:2:2 / 4:2:0 YCbCr) |
| Header + metadata only | dimensions / channels / EXIF without pixel decode | HEIF / HEIC / AVIF / CR3 (ISO-BMFF box walker), JP2 / J2K (codestream + container), JXR, BPG, FLIF, MNG, EMF, WMF / APM, DICOM, DJVU, SVS (Aperio TIFF), gzipped vector wrappers |
| Writer | encode | BMP (1 / 4 / 8 / 24 / 32 bpp), PNG (Gray8 / Rgba32 / Palette + APNG), PNM (P4 / P5 / P6, 8 + 16 bpc), TGA (uncompressed + RLE, Gray8 / Rgb24 / Rgba32), HDR (Radiance RGBE + scanline RLE), PCX (version 5, byte RLE), XPM3 (C-array text), ICNS (icp4 / icp5 / icp6 / ic07-ic10, PNG sub-images), TIFF (single-strip baseline; None / Deflate / PackBits / LZW; Gray8 / Gray16 / Rgb24 / Rgba32), GIF89a (single-image Indexed8) |

Pixel decoding is intentionally **never** wired to a third-party codec
binary. JPEG arithmetic-coded variants remain deferred;
HEIF / AVIF / JXL require codec implementations
that are themselves multi-month projects and are out of scope. DDS BC6H
(HDR half-float UF16 / SF16) implements **all 14 BC6H modes** — the four
single-subset modes (3, 7, 11, 15) and the ten two-subset partitioned
modes (0, 1, 2, 6, 10, 14, 18, 22, 26, 30) — per the Khronos Data Format
Specification 1.4 § 20.2. Reserved mode numbers (19, 23, 27, 31) decode
to the spec-mandated transparent black. The header tier returns full
dimensions + tags so callers can route, sort, or build indexes without
touching pixels.

Performance principles applied throughout: `ArrayPool<byte>.Shared`-backed
`ImageFrame.Rent`, `Span<byte>` on every hot path, `BinaryPrimitives` for
endian moves, `FrozenDictionary` for tag tables, `AggressiveInlining` on
tight helpers, `IAsyncEnumerable<ImageFrame>` for streaming multi-frame
sources.

### A note on "all codecs"

Mediar is explicitly a **container-level toolkit + select decoders**, not a
ground-up FFmpeg replacement. The decoders that ship (PCM, G.711, FLAC, Vorbis
I) cover the patent-clean audio codecs that fit a single-person engineering
budget. Patent-encumbered codecs (AAC, AC-3, H.264/H.265/H.266) are
**permanently** out of scope. Royalty-free-but-massive codecs (Opus, AV1, VP9,
AV2) are deferred — each is an independent multi-month project. AV2 in
particular has **no published bitstream specification** as of 2026; AOM is
still finalising the format and there are no AV2 files in the wild to test
against. Mediar carries AV2 samples opaquely through MP4 and Matroska so the
plumbing is in place when files do start to appear.

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
│   ├── Mediar.Containers.Avi/          RIFF / AVI demuxer (idx1 seeking, LIST/INFO)
│   ├── Mediar.Containers.Aiff/         AIFF / AIFC demuxer (PCM, fl32, µ-law / A-law)
│   ├── Mediar.Codecs/                  IAudioDecoder / IVideoDecoder abstractions
│   ├── Mediar.Codecs.Pcm/              PCM sample-format conversion + decoder
│   ├── Mediar.Codecs.G711/             G.711 µ-law / A-law encode + decode
│   ├── Mediar.Codecs.Flac.Decoder/     Native FLAC decoder (RFC 9639)
│   ├── Mediar.Codecs.Mp3.Decoder/      MPEG-1/2/2.5 Layer III decoder (patent-free since 2017)
│   ├── Mediar.Codecs.Vorbis.Decoder/   Vorbis I foundation (headers + bitstream + IMDCT/TDAC)
│   ├── Mediar.Bitstream/               H.264 / H.265 Annex-B ↔ AVCC/HVCC helpers
│   ├── Mediar.Subtitles.Srt/           SRT read + write
│   ├── Mediar.Subtitles.WebVtt/        WebVTT read + write
│   ├── Mediar.Subtitles.Ass/           ASS / SSA v4+ read + write
│   ├── Mediar.Imaging.Core/            IImageReader / IImageWriter + magic-byte detector
│   ├── Mediar.Imaging.Bmp/             BMP / DIB reader + writer + ICO
│   ├── Mediar.Imaging.Png/             PNG / APNG reader + writer (CRC32 inline)
│   ├── Mediar.Imaging.Jpeg/            JPEG baseline + progressive + lossless decoder + EXIF / MPO
│   ├── Mediar.Imaging.Metadata/        Shared EXIF / TIFF tag-stream parser (re-used by JPEG, TIFF, HEIC, AVIF, DNG, CR3, WebP)
│   ├── Mediar.Imaging.Gif/             GIF87a / GIF89a + animation (LZW via Mediar.Codecs.Lzw)
│   ├── Mediar.Imaging.Tiff/            TIFF (uncompressed / Deflate / PackBits / LZW via shared codecs)
│   ├── Mediar.Imaging.Tga/             Truevision TGA (types 1/2/3/9/10/11)
│   ├── Mediar.Imaging.Pcx/             ZSoft PCX (1 / 8 / 24 bpp)
│   ├── Mediar.Imaging.Hdr/             Radiance .hdr (RGBE with RLE)
│   ├── Mediar.Imaging.Pnm/             Portable AnyMap P1..P6
│   ├── Mediar.Imaging.Xpm/             X PixMap (XPM3 text format)
│   ├── Mediar.Imaging.Icns/            Apple .icns sub-image enumerator
│   ├── Mediar.Imaging.Dicom/           DICOM PS 3.10 reader (Implicit / Explicit VR LE; MONOCHROME1/2 + RGB)
│   ├── Mediar.Imaging.Dds/             DirectDraw Surface container (uncompressed surfaces; BCn block decode delegated to Mediar.Codecs.Bcn)
│   ├── Mediar.Codecs.Bcn/              Shared BC1 / BC2 / BC3 / BC4 / BC5 / BC6H (all 14 modes) / BC7 block decoders, container-agnostic (DDS / KTX / KTX2 / PVR)
│   ├── Mediar.Codecs.Lzw/              Shared variable-width LZW decoder (GIF + TIFF dialects, allocation-free flat dictionary)
│   ├── Mediar.Codecs.PackBits/         Shared Apple PackBits encoder + decoder (TIFF 32773, PSD, MacPaint)
│   ├── Mediar.Acceleration/            Portable kernel-dispatch layer (IAcceleratedKernel, AccelerationDispatcher, scalar fallbacks)
│   ├── Mediar.Acceleration.X86/        SSE2 / AVX2 SIMD kernels; auto-registers via [ModuleInitializer]
│   ├── Mediar.Acceleration.Arm/        AdvSimd / NEON SIMD kernels; auto-registers via [ModuleInitializer]
│   ├── Mediar.Imaging.Probe/           HEIF / AVIF / JXR / BPG / FLIF / MNG / EMF / WMF / DJVU / SVS header probes
│   ├── Mediar.Imaging/                 MediarImage.Open(path) facade
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

### File metadata

Every demuxer exposes a `MediaMetadata` (or equivalent property) populated
from the container's native tag block — no extra pass over the file is needed.
Strongly-typed fields cover the common cases (`Title`, `Artist`, `Album`,
`AlbumArtist`, `Date`, `Genre`, `TrackNumber`, `DiscNumber`, `Composer`,
`Comment`, `Lyrics`, `Copyright`, `Publisher`, `Encoder`,
`GeoLocation` ISO-6709) and anything else lands in a string→string `Tags`
dictionary.

| Container        | Tag format read                          |
|------------------|------------------------------------------|
| WAV              | `LIST INFO` (INAM / IART / ICRD / IGNR / ICMT / ICOP …) |
| MP3              | ID3v1 + ID3v2.2 / v2.3 / v2.4 (text + `WXXX` / `COMM`) |
| FLAC             | `VORBIS_COMMENT` block                   |
| Ogg Vorbis       | `comment` packet (vorbis_comment)        |
| Ogg Opus         | `OpusTags` packet                        |
| MP4 / MOV / M4A  | QuickTime `moov.udta` (©nam / ©ART / ©day / ©xyz …) + iTunes `meta/ilst` + 3GPP `loci` (lat/lon/alt) |
| Matroska / WebM  | `Segment.Tags.Tag.SimpleTag` (recursive) |
| AVI              | `LIST INFO`                              |
| AIFF / AIFC      | `NAME` / `AUTH` / `ANNO` / `COPY` chunks |
| 8SVX             | `NAME` / `AUTH` / `ANNO` / `(c) ` chunks |
| CAF              | `info` chunk (key/value pairs)           |

```csharp
await using var demuxer = MediarOperations.Open("trip.mp4");
Console.WriteLine(demuxer.Metadata.Title);
Console.WriteLine(demuxer.Metadata.Artist);
if (demuxer.Metadata.GeoLocation is { } g)
    Console.WriteLine($"{g.Latitude},{g.Longitude} @ {g.Altitude}m");
```

### Decoders
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

### MIDI File API

`Mediar.Midi` reads + writes Standard MIDI Files (SMF formats 0 / 1 / 2)
as strongly-typed events. It is independent of the streaming media-demuxer
abstraction because MIDI is an event score, not a continuous sample stream:

```csharp
using Mediar.Midi;

var file = MidiReader.ReadFile("song.mid");
foreach (var track in file.Tracks)
foreach (var ev in track.Events)
{
    if (ev.Type == MidiMessageType.NoteOn)
        Console.WriteLine($"t={ev.Tick} ch={ev.Channel} note={ev.Data1} vel={ev.Data2}");
}

MidiWriter.WriteFile("out.mid", file);
```

Running-status compression is applied automatically on write; SysEx and
all meta-event subtypes (track name, set-tempo, time-signature, SMPTE
offset, …) are round-tripped losslessly.

### Playlist File API

`Mediar.Playlists` reads and writes the common audio-playlist formats
through a single immutable `Playlist` record:

```csharp
using Mediar.Playlists;

Playlist mix = M3uPlaylist.ReadFile("favorites.m3u8");
foreach (var e in mix.Entries)
    Console.WriteLine($"{e.Artist} - {e.Title} ({e.Uri})");

PlsPlaylist.WriteFile("favorites.pls", mix);
XmlPlaylist.WriteXspfFile("favorites.xspf", mix);
XmlPlaylist.WriteWplFile("favorites.wpl", mix);
```

Supported formats: M3U / M3U8 (extended `#EXTINF` / `#PLAYLIST`),
PLS (INI-style), XSPF (`http://xspf.org/ns/0/`), WPL (SMIL).

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
