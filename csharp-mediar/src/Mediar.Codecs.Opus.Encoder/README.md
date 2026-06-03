# Mediar.Codecs.Opus.Encoder

Clean-room Opus encoder. Mirror of `Mediar.Codecs.Opus.Decoder` —
together the two libraries form a complete RFC 6716 round-trip pipeline.

## Status

| Phase | Component | Status |
| ----- | --------- | ------ |
| B1 | Range encoder (`OpusRangeEncoder`) | placeholder, bit-exact for the writers Phase B2 needs (`Encode`, `EncodeBin`, `EncodeBitLogP`, `EncodeIcdf`, `EncodeUint`, `EncodeBitsRaw`, `EncodeLaplace`); validated by round-trip against `OpusRangeDecoder` |
| B1 | TOC builder | placeholder — to be unified when B1 merges |
| B1 | Frame assembler | placeholder — to be unified when B1 merges |
| B1 | `OpusEncoder` skeleton | placeholder — `EncodeCeltOnlyFrame` throws |
| **B2** | **Forward MDCT** (`CeltMdct`) | **shipped** — textbook O(N²) reference impl with sine window; MDCT∘IMDCT round-trip + TDAC verified |
| **B2** | **Energy quant** (`CeltEnergyQuant`) | **shipped** — `QuantCoarseEnergy` / `QuantFineEnergy` / `QuantEnergyFinalise` mirror the decoder's `unquant_*` line-for-line |
| **B2** | **PVQ search** (`CeltPvqSearch`) | **shipped** — `AlgQuant` (greedy + complexity≥6 local-swap refinement), `Icwrs` (encoder side of `cwrsi`); round-trips through `CeltShape.AlgUnquant` |
| **B2** | **Band quant** (`CeltBandQuant`) | **partial** — `QuantBandSimple` (mono, no TF / no Haar / no split / no folding). Full `QuantBand` + `QuantBandStereo` + `QuantPartition` deferred to B2.1 |
| **B2** | **Allocator** (`CeltAllocator`) | **partial** — `FlatAllocation` placeholder. Full `compute_allocation` + TF/spread/dyn-alloc analysis deferred to B2.1 |
| **B2** | **Encoder state** (`CeltEncoder`) | **partial** — owns overlap / `oldLogE` / anti-collapse seed; `Encode(...)` top-level wiring deferred to B2.1 |
| **B2** | **Pitch search** (`CeltPitchSearch`) | **stub** — post-filter disabled (matches libopus `postfilter_period == 0`); full `pitch_search` port deferred to B2.2 |
| B3 | SILK encoder | not in scope |

## CELT encoder behaviour (target per-frame pipeline)

```
PCM in
  │
  ▼
[ window + 50% overlap with prior frame ]                 ← CeltMdct.BuildSineWindow
  │
  ▼
[ forward MDCT (length-N/2 spectrum) ]                    ← CeltMdct.Forward
  │
  ▼
[ per-band energy magnitudes → quant ]
   ├── coarse (signed Laplace, e_prob_model[NB_BANDS])    ← CeltEnergyQuant.QuantCoarseEnergy
   └── fine   (Q14 raw bits)                              ← CeltEnergyQuant.QuantFineEnergy
  │
  ▼
[ TF / spread / dyn_alloc / intensity decisions ]         ← deferred (B2.1)
  │
  ▼
[ compute_allocation → pulses_per_band[NB_BANDS] ]        ← CeltAllocator (B2.1)
  │
  ▼
[ per-band PVQ search + write codeword index ]            ← CeltBandQuant + CeltPvqSearch.AlgQuant
  │
  ▼
[ finalise leftover bits ]                                ← CeltEnergyQuant.QuantEnergyFinalise
  │
  ▼
Packet (range-coded prefix + raw-bit suffix)
```

## Notable design choices in Phase B2 v1

- **MDCT scaling.** The reference IMDCT uses `1/N` (not libopus's `2/N`)
  so `MDCT ∘ IMDCT = I` cleanly. The fast FFT-based variant landing in
  the decoder's `CeltImdct` (Phase 2d) uses a different scale that's
  compensated by windowed OLA; when wiring into the full encoder
  pipeline this needs to be reconciled.
- **Range coder.** `OpusRangeEncoder` is a `ref struct` mirror of
  `OpusRangeDecoder`. The carry-out shift is `EC_CODE_BITS - EC_SYM_BITS - 1`
  (i.e. `>> 23`) — getting this off-by-one wrong silently halves every
  encoded value. `Finish` explicitly flushes the remaining `_rem` /
  carry chain so a tight buffer round-trips without relying on libopus's
  "caller zero-pads the gap" convention. Validated by encode → decode
  round-trip on synthetic indices in `CeltPvqSearchTests`.
- **PVQ search.** Greedy pulse placement with a warm-start from
  `floor(K · |X[i]| / Σ|X|)` (libopus does the same). For complexity
  ≥ 6 we add up to two passes of single-pulse swap refinement. Pre-rotation
  is applied with `dir = +1` to invert the decoder's `dir = -1`.
- **CWRS storage.** The Phase B2 v1 `Icwrs` uses libopus's small-footprint
  unsigned-32-bit recurrence (matches the decoder's `Cwrsi`). For very
  large `(N, K)` pairs where `V(N, K)` doesn't fit in `uint32`, libopus
  splits the band recursively in `encode_pulses`; that path is deferred
  to B2.1.
- **InternalsVisibleTo.** The encoder reuses the decoder's spec-derived
  tables (`CeltConstants.EProbModel`, `PredCoef`, `BetaCoef`, `DbShift`,
  `SmallEnergyIcdf`, …) and inverse helpers (`CeltShape.ExpRotation`,
  `NormaliseResidual`, `ExtractCollapseMask`, `AlgUnquant`) directly —
  no table duplication. The decoder's csproj has
  `<InternalsVisibleTo Include="Mediar.Codecs.Opus.Encoder" />`.

## References

- **RFC 6716** — *Definition of the Opus Audio Codec.* §4.3 (CELT layer),
  §4.3.2.1 (signed-Laplace coarse energy), §4.3.3 (PVQ shape coding),
  §4.3.7 (MDCT), §4.1 (range coder).
- **libopus** — reference C implementation. Mirrored files:
  `celt/entenc.c` (`OpusRangeEncoder`), `celt/mdct.c`
  (`CeltMdct.Forward`), `celt/quant_bands.c`
  (`CeltEnergyQuant.QuantCoarseEnergy` / `QuantFineEnergy` /
  `QuantEnergyFinalise`), `celt/vq.c` (`CeltPvqSearch.AlgQuant`),
  `celt/cwrs.c` (`CeltPvqSearch.Icwrs`), `celt/bands.c`
  (`CeltBandQuant`), `celt/celt_encoder.c` (`CeltEncoder`),
  `celt/pitch.c` (`CeltPitchSearch`), `celt/rate.c` (`CeltAllocator`).
- **Valin, Vos, Terriberry.** *Definition of the Opus Audio Codec*,
  IETF draft / IETF 88 (2012). Especially the CELT chapter on PVQ
  search, energy quant, and bit allocation.
- **Princen & Bradley, 1986/1987.** Time-domain aliasing cancellation
  underpinning the MDCT.
- **J.-M. Valin et al.** *A Full-Bandwidth Audio Codec with Low
  Complexity and Very Low Delay* (EUSIPCO 2009). The CELT design paper.
