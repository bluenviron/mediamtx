# Mediar.Codecs.Opus.Encoder

A native-C# Opus audio encoder (RFC 6716) for the Mediar pipeline. **Phased
delivery** — this commit ships the SILK encoder analysis foundation;
later phases add the closed-loop noise-shaping quantiser, NLSF MSVQ
encoder, LTP search, CELT encoder, and end-to-end packet emission.

## Status

| Phase            | Scope                                                                    | Status     |
| ---------------- | ------------------------------------------------------------------------ | ---------- |
| B1               | Encoder foundation: range encoder, TOC packer, OpusEncoder skeleton      | ⏳ planned  |
| B2               | CELT encoder (transform, PVQ search, energy quant, allocation)           | ⏳ planned  |
| **B3-foundation**| **SILK analysis: Q-math, Burg autocorr + LPC, LPC bandwidth expansion, VAD** | **✅ shipped** |
| B3-quant         | SILK NLSF MSVQ encoder (depends on decoder Phase 3a NLSF tables)         | ⏳ planned  |
| B3-ltp           | SILK open-loop + closed-loop LTP pitch search                            | ⏳ planned  |
| B3-shape         | SILK noise-shaping analysis                                              | ⏳ planned  |
| B3-nsq           | SILK closed-loop noise-shaping quantiser (NSQ) + shell-coder encode      | ⏳ planned  |
| B3-top           | SilkEncoder top-level + stereo prediction + OpusEncoder wire-up          | ⏳ planned  |
| B4               | Hybrid mode mixer (SILK low-band + CELT high-band)                       | ⏳ planned  |

### What's blocked, and why

The remaining B3 slices (NLSF MSVQ encoder, LTP search, NSQ, shell
encode, top-level SilkEncoder, stereo prediction) **cannot ship
honestly until the SILK *decoder* lands**. The decoder is currently
at Phase 2c.4 (CELT-only); Phase 3 (NLSF / LPC stability / LTP
scaling / sub-frame gains) and Phase 4 (excitation + sub-frame
synthesis) are still planned. Without the decoder:

- The MSVQ NLSF codebooks (RFC §A.5 Tables 22-25) are not yet
  ported to a shared location for `InternalsVisibleTo` reuse.
- The "round-trip" tests in the original B3 brief
  (`SilkLsfQuantTests` via `SilkNlsf`, `SilkQuantiserTests` via
  `SilkShellCoder` + `SilkExcitationReconstruction`,
  `SilkEncoderRoundTripTests` via `OpusDecoder`) have no decoder
  to validate against and would amount to shipping unverified
  fixed-point DSP.

Producing those slices without a decoder would mean ~6 500 lines of
intricate Q-format DSP that cannot be checked against the reference
behaviour. We've deliberately left them deferred rather than write
plausible-looking code that nobody can prove correct.

## Phase B3-foundation behavior

This phase ports the SILK encoder's **front-end analysis** — the
stages that do not depend on the SILK decoder being available because
they operate purely on the input PCM and produce intermediate values
consumed downstream by later encoder slices.

### Pipeline position

```
        ┌──────────────┐   ┌────────────────┐   ┌─────────────────┐
PCM ──▶│   SilkVad    │──▶│ SilkAutocorr   │──▶│ SilkLpcAnalysis │──▶ LPC
        │ subframe SNR │   │  + Burg LPC    │   │ bw-expand + chk │     vector
        └──────────────┘   └────────────────┘   └─────────────────┘
                ▲                                       │
                │                                       ▼
        (consumed by                            (consumed by
         B3-quant rate                           B3-quant NLSF
         control)                                MSVQ, B3-ltp
                                                 LTP search,
                                                 B3-nsq quantiser)
```

### File-by-file

- **`Silk/SilkInt.cs`** — Fixed-point Q-math primitives ported from
  libopus `silk/macros.h` and `silk/SigProc_FIX.h`
  (`silk_SMULWB`, `silk_SMULWW`, `silk_SMULL`, `silk_SMLAWB`,
  `silk_ADD_SAT32`, `silk_SUB_SAT32`, `silk_RSHIFT_ROUND`,
  `silk_CLZ32`, plus `Q ↔ float` converters used by the
  float-build analysis stages). All marked
  `[MethodImpl(MethodImplOptions.AggressiveInlining)]`.

- **`Silk/SilkAutocorr.cs`** — Autocorrelation + Burg-method LPC
  analysis. Port of the algorithmic core of libopus
  `silk/burg_modified.c` (`silk_burg_modified`) plus the
  autocorrelation accumulator from `silk/autocorrelation.c`.
  Float-precision build matching the rest of the Mediar Opus
  pipeline; the fixed-point Q-chain (residual energy + Q-headroom
  shift, Q16 reflection coefficients) gets wired in alongside the
  bit-exact NLSF MSVQ encoder in `B3-quant`. Also ships a
  reference `LevinsonDurbin` implementation used by tests.

- **`Silk/SilkLpcAnalysis.cs`** — Wraps Burg with chirp
  bandwidth expansion (`silk/bwexpander_32.c`) and a Schur-recursion
  stability check. The richer `silk_LPC_inverse_pred_gain` check
  lands with the NLSF quantiser.

- **`Silk/SilkVad.cs`** — Voice activity detector. **Behaviourally
  equivalent simplification** of libopus `silk/VAD.c`
  (`silk_VAD_GetSA_Q8`): sub-frame energy + 5-frame minimum-statistics
  noise floor → per-sub-frame SNR (Q7) + voice-active flag. The VAD
  output is not written to the bitstream directly (it only influences
  the encoder's internal rate/distortion path), so replacing this
  simplification with the bit-exact 4-band IIR port in a later slice
  is behaviour-preserving.

### Q-format chain (foundation slice)

| Stage                 | In                | Out                                         |
| --------------------- | ----------------- | ------------------------------------------- |
| `SilkVad.Analyze`     | `float` PCM       | per-sub-frame SNR Q7 + bool                 |
| `SilkAutocorr.Burg`   | `float` window    | `float` LPC vector + `float` residual energy|
| `SilkLpcAnalysis`     | `float` LPC vec   | `float` LPC vec (bandwidth-expanded)        |

Later slices (B3-quant onward) add the fixed-point Q chain that the
SILK bitstream layer requires (Q16 reflection coefficients → Q15 NLSFs
via `silk_NLSF2A` inverse → Q8 quantised NLSF indices via MSVQ).

### Test results after Phase B3-foundation

New tests added in `csharp-mediar/tests/Mediar.Tests/`:

- `SilkAutocorrTests` — Burg vs Levinson-Durbin agreement on a
  synthetic AR(2) process; autocorrelation symmetry/positivity;
  bandwidth-expansion factor invariants.
- `SilkVadTests` — silence → VAD off; speech-shaped (filtered noise)
  → VAD on after the noise-floor history fills.
- `SilkLpcAnalysisTests` — stability check on hand-crafted LPC
  vectors (unit-circle root → unstable; safe interior root → stable).

## References

- IETF RFC 6716, "Definition of the Opus Audio Codec," §4.2 (the
  SILK *decoder* is the spec; the encoder must produce a stream
  this decoder will reconstruct).
- libopus source (xiph.org / mozilla / IETF reference): `silk/VAD.c`,
  `silk/burg_modified.c`, `silk/autocorrelation.c`,
  `silk/find_LPC.c`, `silk/bwexpander_32.c`, `silk/macros.h`,
  `silk/SigProc_FIX.h`. (Later encoder slices will additionally
  reference `silk/NSQ.c`, `silk/noise_shape_analysis.c`,
  `silk/pitch_analysis_core.c`, `silk/enc_API.c`,
  `silk/encode_frame.c`.)
- K. Vos, S. Jensen, K. Sørensen, "SILK Speech Codec,"
  draft-vos-silk-02 (IETF Internet-Draft, 2010).
- J. P. Burg, *Maximum entropy spectral analysis*, PhD thesis,
  Stanford University, 1975.
- B. S. Atal, "Predictive coding of speech at low bit rates,"
  *IEEE Trans. Communications*, vol. 30, no. 4, pp. 600-614, 1982.
- R. Martin, "Noise power spectral density estimation based on
  optimal smoothing and minimum statistics," *IEEE Trans. Speech
  and Audio Processing*, vol. 9, no. 5, pp. 504-512, 2001 (origin
  of the minimum-statistics noise-floor estimator used in
  `SilkVad`).
