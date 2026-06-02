# Mediar.Codecs.Alac.Decoder

Native Apple Lossless Audio Codec (ALAC) decoder. Clean-room reimplementation
in C# of the bitstream documented by Apple's Apache-2.0 reference
([macosforge/alac](https://github.com/macosforge/alac)). No native dependencies.

## Status

🟢 **Pipeline shipped.** The full decode chain is in place and exercised by
unit + integration tests in `Mediar.Tests`:

```
ALACSpecificConfig (cookie)
   │
   ▼
top-level element loop  ──┐
   ├─ SCE (mono)          │
   ├─ CPE (stereo)        │  ── per frame ──►  adaptive Rice
   ├─ LFE / CCE           │                       │
   ├─ END                 │                       ▼
   └─ FIL (alignment)     │              adaptive FIR predictor
                          │                       │
                          │                       ▼
                          │                stereo matrix unmix
                          │                       │
                          ▼                       ▼
                    interleaved PCM (int16/24/32 or float32)
```

| Capability | State |
|---|---|
| 16 / 20 / 24 / 32-bit depth | ✅ |
| Mono (SCE) + stereo (CPE) | ✅ |
| Uncompressed escape frames | ✅ |
| Compressed predictor + Rice | ✅ |
| `bytesShifted` low-bit pass-through | ✅ |
| MP4 (`alac` child box) extra-data wiring | ✅ |
| CAF (`kuki` chunk) extra-data wiring | ✅ |
| FIL element (byte alignment) | ❌ — currently rejected |
| LFE / CCE / multi-element 5.1 / 7.1 | ❌ — `NotSupportedException` |
| Real-world fixture regression test | ❌ — no `.m4a` / `.caf` in `TestData/` |

`AlacDecoder` plugs into the standard `IAudioDecoder` factory used by the
container demuxers, so any consumer that goes through `CodecRegistry` will pick
it up automatically once the package is referenced.

## Performance baseline

No baseline yet — the decoder has not been profiled against a real fixture.
Reference numbers (target territory once we have a fixture):

| Reference decoder | Throughput |
|---|---|
| Apple ALAC (scalar C++)   | 80–150× realtime stereo 16-bit |
| FFmpeg `alac` (scalar C)  | 100–200× realtime |
| FFmpeg `alac` (SSE/AVX)   | 250–500× realtime |

Given the predictor / Rice / matrix kernels are all straight scalar loops at
the moment, expect us to land somewhere near Apple's scalar reference — likely
**~2–4× slower than FFmpeg's vectorized path** until we add SIMD.

## TODO / improvements

### 🔴 Conformance — required for full ISO / Apple-reference parity

1. **Real-world fixture coverage** — drop a short `.m4a` (MP4 `alac`) and
   `.caf` clip into `tests/Mediar.Tests/TestData/Alac/` and add integration
   tests that decode them end-to-end. Without this, the compressed
   predictor + Rice + matrix pipeline is only verified in isolation, not as a
   pipeline. This is the single highest-value follow-up.
2. **FIL element (tag 6)** — Apple's encoder emits `FIL` for byte alignment in
   some streams. `AlacDecoder.DecodePacket` currently throws
   `NotSupportedException` for tag 6. Treat it as "skip `count` bytes /
   bit-align and continue" per Apple's reference.
3. **`mode != 0` predictor path validation** — Apple's `unpc_block` runs an
   extra residual pass when `mode != 0`. The code is in place but has never
   been exercised against a stream that uses it; needs a fixture with at least
   one frame in this mode for regression coverage.
4. **LFE / CCE / multi-element layouts (5.1, 6.1, 7.1)** — the top-level
   element loop currently only handles `SCE`, `CPE`, `END`. Add `LFE` and
   `CCE` element dispatch and a channel-layout-aware interleaver so 5.1 and
   7.1 ALAC files (which use multiple stacked elements per frame) decode.
5. **Cookie channel-layout tag** — `AlacSpecificConfig` currently parses the
   24-byte `ALACSpecificConfig` only. The 48-byte form with the trailing
   `ALACChannelLayoutInfo` (used by some Apple files) should be parsed and
   surfaced so the channel order matches Apple's layout tag.
6. **CPE uncoupled escape path** — when both channels go uncompressed *and*
   `mixBits != 0`, Apple's reference still applies the matrix on the raw
   samples in some encoder versions. Our escape path skips the matrix
   entirely; verify against a fixture that this matches Apple's output.

### 🟡 Robustness

7. **Bit reader bounds checking** — `AlacDecoder` trusts the cookie and the
   per-frame headers. Add explicit length validation so a truncated packet
   throws `EndOfStreamException` cleanly instead of underflowing the
   `BitReader`.
8. **Cookie variant tolerance** — accept both the raw 24-byte cookie and the
   28-byte `FullBox`-prefixed form transparently in `AlacSpecificConfig.Parse`
   (currently handled by `NormalizeCookie` at the call sites — push it down
   into the parser itself).
9. **Sample-rate / channel-count consistency check** — when the container
   reports a sample-rate or channel count that disagrees with the cookie,
   prefer the cookie (per Apple's reference) and surface a one-time warning
   rather than silently accepting whichever the caller passes in.

### 🟢 Performance

10. **SIMD predictor** — the FIR predictor inner loop
    (`AlacPredictor.Unpc` general case) is the hot path. With AVX2 / NEON we
    can do 8 / 4 taps per iteration. Specialise `numactive == 4` and
    `numactive == 8` (the overwhelmingly common encoder choices) for an easy
    2–3× win on the predictor.
11. **SIMD matrix unmix** — `AlacMatrix.Unmix` is a trivial
    `mid + (side >> mixRes * mixBits)` per sample; vectorize with `Vector256<int>`
    for a ~4× speedup on stereo content.
12. **Stackalloc residual buffers** — the per-frame `int[]` residual /
    predictor scratch buffers are heap-allocated. For frames up to 4096
    samples (Apple's default) they fit comfortably in `stackalloc Span<int>`,
    eliminating per-frame GC pressure.
13. **Rice decoder branch reduction** — `AlacRice.DecodeBlock` has a hot
    `dyn_jam_noise_block` zero-run trigger. The zero-run branch is taken
    rarely in real content; profile and consider hoisting it out of the inner
    loop with a fast-path for the common "no zero run" case.

## Attribution

Derived from the ALAC bitstream format and reference algorithms published by
Apple Inc. under the Apache License 2.0
([github.com/macosforge/alac](https://github.com/macosforge/alac)). No
Apple code is copied — this is a from-scratch C# reimplementation against the
publicly documented format. The original reference is included as
documentation of the bitstream layout only.

## License

Same as the rest of Mediar.
