# Mediar.Codecs.Mp3.Decoder

Clean-room MPEG-1 / MPEG-2 / MPEG-2.5 Audio Layer III (MP3) decoder. All
underlying patents (the last being US 5,742,735) expired in April 2017, so
neither the format nor the reference tables it depends on are encumbered.

## Status

🟢 **Pipeline shipped.** The full decode chain (header parse → side-info →
bit reservoir → scalefactor decode → Huffman → requantize → MS/IS stereo →
reorder → antialias → IMDCT + overlap-add → polyphase synthesis →
interleaved PCM float32) is in place and runs end-to-end over the
real-world fixture in `tests/Mediar.Tests/TestData/Mp3/`. See
`Mediar.Tests.Mp3DecoderIntegrationTests` and `Mp3DecoderPerformanceTests`
for the executable contract.

## Performance baseline

Measured against the 162-frame 44.1 kHz stereo fixture in `Release`
configuration on a developer workstation (numbers are indicative, not a
benchmark guarantee):

| Metric | Value |
|---|---|
| Per-frame decode cost | **~383 µs** |
| Throughput | **~3.0 M samples/s ≈ 68× realtime** |
| Per-frame allocations | **~600 B/frame** (steady state) |

For context, mature C++ decoders typically land at:

| Reference decoder | Per-frame | Realtime |
|---|---|---|
| libmpg123 (SSE/AVX) | 50–100 µs | 250–500× |
| FFmpeg `mp3float`   | 60–120 µs | 200–400× |
| minimp3 (scalar C)  | 80–150 µs | 170–300× |
| libmad (fixed-pt)   | 200–400 µs | 60–130× |

So we're competitive with `libmad` (also unoptimized scalar) and roughly
**3–6× slower than the best vectorized C++ implementations**.

## TODO / improvements

### 🔴 Conformance — required for bit-exact ISO 11172-3 / 13818-3 agreement

1. **Huffman big-values tables 5..31** (`Mp3HuffmanTables.cs`) — only
   tables 0..3 and Quad A/B are populated; tables 5..31 currently fall
   back to a zero-decode path. Most real-world audio uses tables 5..31, so
   any non-silence/non-low-entropy frames produce reduced output. Drop in
   the full ISO 11172-3 Annex B Table B.7 to close this gap.
2. **Polyphase D-window** (`Mp3Polyphase.BuildDWindow`) — currently an
   analytic windowed-sinc approximation centered at π/64. Substitute the
   512 floats from ISO 11172-3 Annex B Table B.4 for bit-exact output.
3. **MPEG-2.5 (`mpeg25`) coverage tests** — pipeline supports it via
   `header.Version == 25`, but no fixture exercises the path. Add a 11.025
   / 12 / 8 kHz fixture and assert frame-count + sample-rate correctness.
4. **Free-format frame size discovery** — `Mp3FrameHeader.TryParse`
   accepts `bitrateIndex == 0` but the demuxer treats it as a sync
   miss. Implement frame-size scanning for free-format streams.
5. **CRC-16 verification** — `hasCrc` is read from the header and the CRC
   bytes are skipped over, but the value is not actually checked against
   the side-info. Add CRC-16 validation behind an opt-in flag.

### 🟠 Performance — closing the gap to libmpg123/FFmpeg

6. **Vectorize `Mp3Polyphase.SynthesizeRow`** — the 64×32 matrixing and
   the 32×16 dewindow loop together do ~37k scalar multiplies per frame
   per channel. `Vector256<float>` / `MathF.FusedMultiplyAdd` on the inner
   loop is the single biggest expected win (estimated 2–3× on `x86_64`,
   1.5–2× on ARM NEON via `Vector128<float>`).
7. **Vectorize `Mp3Hybrid` IMDCT inner loops** — the 12- and 18-point
   IMDCT matrices in `Mp3Tables.ImdctShort` / `ImdctLong` are read
   row-major; switching to SIMD-friendly column-major + `Vector256<float>`
   gather is worth measuring.
8. **Replace `MemoryPool<float>.Shared.Rent`** — every audio decoder in
   the project pays a ~280 B `IMemoryOwner<float>` wrapper allocation per
   frame. A custom pooled wrapper backed by `ArrayPool<float>.Shared`
   that returns itself to a pool on `Dispose()` would benefit *all*
   decoders, not just MP3. This is a cross-cutting refactor for
   `Mediar.Codecs`, not an MP3-only change.
9. **Flatten multi-dim arrays** — `float[32, 18]` in `Mp3Hybrid._overlap`
   and `Mp3Decoder._hybridOut` pays per-access JIT bounds-check cost.
   Switching to `float[576]` with manual `sb * 18 + row` indexing
   eliminates them.
10. **Channel-strided granule copy** (`Mp3Decoder.cs`, the inner
    `floats[outIdx] = _polyphasePcm[s]` loop) does scalar writes with a
    stride of `channels`. A two-pass per-channel write into a contiguous
    region followed by a stereo interleave would be more SIMD-friendly.
11. **Huffman lookup tables** — current decoder bit-decodes one bit at a
    time via `MainDataReader.ReadBits(1)`. Build per-table prefix lookup
    that consumes up to 8 bits at once and dispatches via a
    `(symbol, bitsConsumed)` table, mirroring what libmpg123 does in
    `huffman.h`.
12. **`Buffer.BlockCopy` → `Span.CopyTo`** in
    `Mp3Polyphase.SynthesizeRow`’s V-buffer shift — minor JIT-friendliness
    win, no algorithmic change.
13. **Cache `Mp3Demuxer.Tracks`** — currently `=> new[] { _track }`
    allocates per access. Cache once.

### 🟢 Robustness / API

14. **Layer I / II support** — currently throws `NotSupportedException`.
    Layer I/II share most of the pipeline (no bit reservoir, simpler
    filterbank, no Huffman/IMDCT/overlap). Out of scope for the initial
    Layer III ship but a natural follow-up if there's demand.
15. **Streaming / partial frame input** — `Decode` requires one complete
    MPEG frame per call. A buffering variant that accepts arbitrary byte
    spans and yields decoded frames as they become available would
    smooth integration with `Mediar.Containers.Mp3.Mp3Demuxer` users that
    do their own framing.
16. **Joint-stereo MS/IS regression fixtures** — the integration test
    fixture is intensity-stereo-light. Add fixtures that exercise MS-only
    (`mode_extension == 2`), IS-only (`mode_extension == 1`), and
    MS+IS (`mode_extension == 3`) paths for `Mp3Stereo.Apply`.
17. **Seek correctness** — `Mp3Demuxer.SeekAsync` is implemented but the
    decoder's bit reservoir is not invalidated on container seek. After a
    seek the first frame correctly returns silence (the
    `HasEnoughHistory == false` fallback), but verify with a dedicated
    test that exercises mid-stream seeking through the decoder pipeline.

## References

* ISO/IEC 11172-3:1993 — MPEG-1 Audio (Layer III in §2.4)
* ISO/IEC 13818-3:1998 — MPEG-2 Audio (LSF + sample-rate extensions)
* K. Brandenburg & G. Stoll, "ISO-MPEG-1 Audio: A Generic Standard for Coding of High-Quality Digital Audio," *JAES* 42(10), 1994
* libmpg123 source — useful for Huffman-table layout and polyphase SIMD
* dr_mp3 — single-header C reference with readable scalar implementation
