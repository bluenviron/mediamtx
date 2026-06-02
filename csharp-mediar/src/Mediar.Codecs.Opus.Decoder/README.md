# Mediar.Codecs.Opus.Decoder

A native-C# Opus audio decoder (RFC 6716) for the Mediar pipeline. **Phased
delivery** — Phase 1 is in this commit; Phases 2–6 will land in subsequent
commits.

## Status

| Phase | Scope                                                                     | Status   |
|------:|---------------------------------------------------------------------------|----------|
|     1 | Foundation: TOC parser, frame packing, range coder, silence-emitting skeleton | ✅ shipped |
|    2a | CELT foundation: constants, band-layout `CeltMode`, decoder skeleton, OpusDecoder routing | ✅ shipped |
|    2b | CELT energy: silence/transient/post-filter/intra flags + coarse energy (Laplace) | ✅ shipped |
|  2c.1 | CELT tf_decode + spread decision (front of PVQ block)                      | ✅ shipped |
|  2c.2 | CELT bit allocation: dyn_alloc, alloc_trim, skip, stereo, compute_allocation | ⏳ planned |
|  2c.3 | CELT fine energy + PVQ shape decode (`decode_pulses`, `cwrsi`)            | ⏳ planned |
|  2c.4 | CELT anti-collapse + final energy                                          | ⏳ planned |
|    2d | CELT IMDCT + post-filter + window overlap-add → first real PCM            | ⏳ planned |
|     3 | SILK NLSF / LPC stability / LTP scaling / sub-frame gains                 | ⏳ planned |
|     4 | SILK excitation + sub-frame synthesis                                     | ⏳ planned |
|     5 | Hybrid bit-allocation + 8/12/16/24/48 kHz resampler                       | ⏳ planned |
|     6 | Multistream, PLC / FEC, perf tuning, RFC test vectors                     | ⏳ planned |

## Phase 2c.1 behavior (added on top of Phase 2b)

CELT-only frames now also decode the two symbols that live between
coarse energy and the bit-allocation block:

- **tf_decode (`LastTfResolution`)** — per-band time-frequency
  resolution offsets. Each band in `[StartBand, EndBand)` is read as a
  toggling 1-bit (logp = 2 or 4 depending on transient, then 4 or 5),
  optionally followed by a reserved `tf_select` bit. The raw 0/1
  resolution flags are then mapped through `TfSelectTable` into the
  signed offsets the IMDCT will consume in Phase 2d.
- **spread decision (`LastSpreadDecision`)** — 4-outcome ICDF
  (`{25, 23, 2, 0}`, ftb=5) selecting one of `SpreadNone (0)`,
  `SpreadLight (1)`, `SpreadNormal (2)`, `SpreadAggressive (3)`.
  Defaults to `SpreadNormal` when the bit budget is exhausted.

Output is still silence — Phases 2c.2 (allocation), 2c.3 (PVQ shape +
fine energy), 2c.4 (anti-collapse + final energy), and 2d (IMDCT) ship
the rest.

## Phase 2b behavior (added on top of Phase 2a)

CELT-only frames now parse the front-of-packet flag set and the
Laplace-coded coarse band energies. Output is still silence — the
spectral synthesis path (PVQ + IMDCT) lands in Phase 2c/2d — but the
decoded state is observable from tests via:

- `LastFrameWasSilent` — silence shortcut triggered.
- `LastPostFilter` — octave, period, gain, tapset (parsed, not yet applied).
- `LastFrameWasTransient` — `true` when the frame is split into
  `ShortBlocksPerFrame` short MDCTs.
- `LastFrameUsedIntra` — coarse-energy predictor used the intra path.
- `OldLogE` — per-band log-energy state in DB_SHIFT (Q10) units; this
  is the same storage layout libopus uses.

The decoder consumes range bits in the exact order specified by RFC
6716 §4.3.2, so plugging in Phase 2c (PVQ + fine energy) just means
reading the next set of symbols from the same range decoder.

## Phase 2a behavior (added on top of Phase 1)

CELT-only configs (TOC config 16..31) now route through a dedicated
`CeltDecoder` per (mode, channels) tuple. The decoder:

- Resolves the band layout via `CeltMode.ForCeltOnly` (StartBand=0,
  EndBand from bandwidth, ShortBlocksPerFrame from frame size).
- Still emits silence today — Phase 2b begins consuming entropy.
- Increments its own `SamplesProduced` counter so progress is observable.

The structural payoff: every part of the CELT pipeline now knows its
band edges, frame sizes, and channel layout. Phases 2b-2d fill in the
DSP without touching the surrounding wiring.

## Phase 1 behavior

The current decoder parses the bit-stream structure of every Opus packet,
constructs a range decoder for each frame (so any malformed entropy header
surfaces immediately), and returns a correctly-shaped
`DecodedAudioFrame`:

- Output sample rate: always 48 000 Hz (Opus's internal rate).
- Channels: derived from the TOC stereo bit, or from an `OpusHead` channel
  mapping when `ChannelMappingFamily != 0`.
- Samples per channel: `SamplesPerFrameAt48k × FrameCount`, e.g. a 20 ms
  packet → 960 samples, a code-3 packet with 3 × 10 ms frames → 1440
  samples.
- PTS: passed through unchanged.
- Sample data: zero-filled silence. Phase 2 onwards replaces this with real
  decoded audio without touching the public API.

This is enough for upstream code to wire up the full Mediar pipeline
(probe → demux → transmux → write) on Opus tracks today; downstream code
will see real audio as soon as CELT and SILK ship.

## Pipeline overview (target architecture)

```
                 ┌────────────────────────────────────┐
encoded packet → │ OpusToc.Parse  (1 byte)            │
                 └────────────────┬───────────────────┘
                                  ▼
                 ┌────────────────────────────────────┐
                 │ OpusFramePacker.Unpack             │
                 │   codes 0 / 1 / 2 / 3 (+ padding)  │
                 └────────────────┬───────────────────┘
                                  ▼ per-frame payload
                 ┌────────────────────────────────────┐
                 │ OpusRangeDecoder   (RFC 6716 §4.1) │
                 └────────────────┬───────────────────┘
                                  ▼
        ┌─────────────────────────┴──────────────────────────┐
        ▼                                                    ▼
┌───────────────────┐                              ┌───────────────────┐
│ SILK              │ ── hybrid mode ──►           │ CELT              │
│ Phases 3, 4       │                              │ Phase 2           │
└─────────┬─────────┘                              └─────────┬─────────┘
          │                                                  │
          └──────────────────────┬───────────────────────────┘
                                 ▼
                 ┌──────────────────────────────────┐
                 │ Resampler  (Phase 5)             │
                 │   48 kHz → 8/12/16/24/48 kHz     │
                 └────────────────┬─────────────────┘
                                  ▼
                       DecodedAudioFrame
```

## What's implemented today

### `OpusToc`

Parses RFC 6716 §3.1 — the 1-byte Table of Contents — into a record struct
exposing `Mode`, `Bandwidth`, `FrameSizeMicroseconds`, `IsStereo` and
`FrameCountCode`. The 32-entry config table is the canonical Table 2 from
the spec.

### `OpusFramePacker`

Walks the 4 framing codes:

| Code | Layout                                              |
|------|-----------------------------------------------------|
|  0   | 1 frame of size `packetSize − 1`                    |
|  1   | 2 equal-size frames; payload size must be even (R3) |
|  2   | 2 frames; first frame's length is byte-encoded      |
|  3   | M ∈ [1, 48] frames, optional padding, VBR or CBR    |

All seven structural rejection rules (R1..R7) are enforced. Frame lengths
are encoded with the 1-byte / 2-byte split documented in §3.2.1 (boundary
at 252).

### `OpusRangeDecoder`

A `ref struct` implementation of the range coder shared by both SILK and
CELT. Lives entirely on the caller's stack — no allocations per packet.
Supports the full libopus interface:

- `Decode` / `Update` — primary range-coded path
- `DecodeBin` — power-of-two convenience
- `DecodeBitLogP` — single-bit with probability `2^-logp`
- `DecodeIcdf` — inverse-CDF table lookup (the form CELT/SILK use)
- `DecodeUint` — uniform integer (small or split-coarse-plus-fine)
- `DecodeBits` — raw bits read from the END of the buffer
- `Tell` / `TellFrac` — consumed-bit accounting (1-bit and 1/8-bit)

### `OpusDecoder` / `OpusDecoderFactory`

`IAudioDecoder` skeleton. Accepts either empty `ExtraData` or the
canonical Ogg-form `OpusHead` (via `Mediar.OpusHead.TryReadOgg`). Wires
into `DecoderRegistry` via the factory; register manually if you want
Opus packets resolved by codec id.

## Roadmap

Each subsequent phase adds a self-contained module that the existing
skeleton calls into:

- **Phase 2 – CELT**: `CeltDecoder` → energy dequant, PVQ → IMDCT → output
  buffer. Replaces the zero-fill in `OpusDecoder.Decode` for CELT-only
  configs (16..31).
- **Phase 3 – SILK NLSF / LPC**: `SilkLpc` → NLSF stage 1/2 → LSF→LPC
  conversion with bandwidth expansion and stability fix-up.
- **Phase 4 – SILK excitation / synthesis**: `SilkExcitation` → pulse +
  sign decoding, sub-frame LPC synthesis with LTP. Together with Phase 3,
  this lights up SILK-only configs (0..11).
- **Phase 5 – Hybrid + resampler**: SILK + CELT band split for hybrid
  configs (12..15), `OpusResampler` to deliver 8/12/16/24 kHz output as
  well as native 48 kHz.
- **Phase 6 – Polish**: multistream coupling (`OpusHead.ChannelMappingFamily`
  ≠ 0), packet-loss concealment, forward-error-correction, performance
  tuning (SIMD where it matters), end-to-end tests against the official
  RFC 6716 test vectors.

## References

- IETF RFC 6716 — *Definition of the Opus Audio Codec*
- IETF RFC 8251 — *Updates to the Opus Audio Codec*
- IETF RFC 7845 — *Ogg Encapsulation for the Opus Audio Codec*
- Xiph libopus (BSD-3-Clause) — algorithm reference

## License

Same as the rest of Mediar — see the root `LICENSE`. No third-party Opus
code is statically linked or copied; this is a clean-room port from the
RFC, with libopus consulted only for clarification of ambiguous spec
wording.
