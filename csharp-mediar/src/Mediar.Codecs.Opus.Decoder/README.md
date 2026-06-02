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
|  2c.2a | CELT init_caps + dyn_alloc + alloc_trim (pre-compute_allocation block)    | ✅ shipped |
|  2c.2b | CELT compute_allocation + intensity / dual stereo + skip flag             | ✅ shipped |
|  2c.3a | CELT `unquant_fine_energy` (fine-energy refinement of coarse spectrum)    | ✅ shipped |
| 2c.3b.1 | CELT PVQ math helpers + pulse cache (`bitexact_cos`, `bitexact_log2tan`, `bits2pulses`, `pulses2bits`, `get_pulses`, `cache_index50`/`cache_bits50`) | ✅ shipped |
| 2c.3b.2 | CELT PVQ shape decode primitives (`decode_pulses`, `cwrsi`, small-footprint `unext`/`uprev` recurrence) | ✅ shipped |
| 2c.3b.3 | CELT energy split decoder (`compute_qn`, `compute_theta`, `quant_band_n1`, `isqrt32`) | ✅ shipped |
| 2c.3b.4 | CELT band shape primitives (`haar1`, `deinterleave_hadamard`, `interleave_hadamard`, `exp_rotation`, `normalise_residual`, `extract_collapse_mask`, `alg_unquant`) | ✅ shipped |
|  2c.3b | CELT PVQ shape decode (`quant_partition`, `quant_band`, `quant_all_bands` integration) | ⏳ planned |
|  2c.4 | CELT anti-collapse + `unquant_energy_finalise` (final energy)              | ⏳ planned |
|    2d | CELT IMDCT + post-filter + window overlap-add → first real PCM            | ⏳ planned |
|     3 | SILK NLSF / LPC stability / LTP scaling / sub-frame gains                 | ⏳ planned |
|     4 | SILK excitation + sub-frame synthesis                                     | ⏳ planned |
|     5 | Hybrid bit-allocation + 8/12/16/24/48 kHz resampler                       | ⏳ planned |
|     6 | Multistream, PLC / FEC, perf tuning, RFC test vectors                     | ⏳ planned |

## Phase 2c.3b.4 behavior (added on top of Phase 2c.3b.3)

Phase 2c.3b.4 ports the **band shape primitives** that sit below the
recursive `quant_partition` / `quant_band` state machine — everything
the leaf decoder needs to turn an entropy-coded PVQ codeword into a
unit-norm spectral shape vector. Together they form the building
blocks the recursion will call into in Phase 2c.3b.5.

New surface area in `CeltShape`:

- **`Haar1(Span<float> X, int N0, int stride)`** — in-place single
  level Haar transform on each of `stride` interleaved length-`N0`
  substreams. Splits adjacent pairs into sum/difference scaled by
  `1/√2`. Used by the encoder/decoder to "fold" stereo pairs or
  short-block groups before bit allocation.
- **`DeinterleaveHadamard(Span<float> X, int N0, int stride, bool hadamard)`**
  and **`InterleaveHadamard(...)`** — inverse pair that gathers
  interleaved substreams into contiguous blocks (and back). When
  `hadamard=true`, applies the inverted Hadamard permutation from
  libopus' `ordery_table` (covers stride 2/4/8/16). The plain
  variant is used by stereo coding; the Hadamard variant is used by
  short-block transient coding.
- **`ExpRotation(Span<float> X, int len, int dir, int stride, int K, int spread)`** —
  pseudo-random Givens rotation that "spreads" PVQ pulse energy
  across the partition dimension to whiten quantisation noise.
  Mirrors libopus `exp_rotation`; in our float build the inner
  primitive is a clean cos/sin rotation between adjacent samples.
  Decoder calls with `dir=-1` to undo what the encoder applied;
  `dir=1` is provided for round-trip testing.
- **`NormaliseResidual(ReadOnlySpan<int> iy, Span<float> X, int N, float ryy, float gain)`** —
  float-build port of libopus `normalise_residual`. Computes
  `g = gain / √ryy` and writes `X[i] = iy[i] * g`. Fixed-point
  shifts collapse to no-ops in the float configuration.
- **`ExtractCollapseMask(ReadOnlySpan<int> iy, int N, int B) → uint`** —
  per-block collapse mask used by anti-collapse. Each set bit
  indicates the corresponding MDCT block received at least one
  non-zero pulse. Returns `1` for the degenerate `B≤1` case.
- **`AlgUnquant(Span<float> X, int N, int K, int spread, int B, ref OpusRangeDecoder, float gain) → uint`** —
  the leaf decoder orchestration. Calls `CeltPvq.DecodePulses`,
  `ExtractCollapseMask`, `NormaliseResidual`, then `ExpRotation`
  (with `dir=-1`). Returns the collapse mask. Mirrors the
  float-build, non-QEXT branch of libopus `alg_unquant`.

Why this slice exists separately: each of these helpers is
self-contained (no entropy decode beyond `AlgUnquant`'s wrap of
`DecodePulses`), individually testable, and only ~165 lines of code
together. Shipping them now means the recursive `quant_partition` /
`quant_band` work in Phase 2c.3b.5 can focus on the splitting state
machine alone, calling these primitives as black boxes.

Test coverage (`CeltShapeTests`, 34 new tests):

- **Haar1**: explicit sum/difference math at the 1/√2 scale,
  self-inverse property after two applications, energy preservation,
  and stride>1 per-substream behavior.
- **Hadamard helpers**: deinterleave∘interleave round trip identity
  for both plain and `ordery` permutations across strides 2/4/8/16;
  explicit byte-level verification of the plain split and the
  stride-2 ordery permutation against hand-computed expected output.
- **ExpRotation**: early-out checks (`spread=NONE`, `2K≥len`),
  forward-then-inverse identity sweep across multiple `(len, stride,
  K, spread)` configurations, energy preservation (orthonormal
  rotation invariant), invalid `spread` argument validation.
- **NormaliseResidual**: unit-norm output for unit gain, gain
  scaling, sign-preservation, zero-`ryy` guard.
- **ExtractCollapseMask**: `B=1` returns 1, per-block bits
  correctly mark sparse / dense slots, signed entries trip their
  block bit (no `abs()` applied).
- **AlgUnquant**: side-channel verification — decode a byte
  stream once with raw `DecodePulses` to learn the integer
  codeword, then verify `AlgUnquant` over a fresh decoder of the
  same stream produces the matching `NormaliseResidual` output and
  collapse mask, both with and without `ExpRotation`.

Implementation gotchas:

- `ref struct OpusRangeDecoder` cannot be captured in lambdas (no
  `Assert.Throws(() => ...)`), and combining `stackalloc` `Span<int>`
  with `ref` parameters trips CS8350. `AlgUnquant` therefore
  heap-allocates its `iy` buffer.
- Hadamard scratch buffers stack-allocate for `N0·stride ≤ 512`
  floats (~2 KB) and fall back to the heap above that — CELT's
  worst case is `176 · 8 = 1408` floats.
- Float build collapses all fixed-point shifts (`PSHR32`, `SHR16`,
  `norm_scaledown` / `norm_scaleup`) to no-ops, so `ExpRotation1`
  is just two Givens-rotation loops with `float` cos/sin
  coefficients.

## Phase 2c.3b.3 behavior (added on top of Phase 2c.3b.2)

Phase 2c.3b.3 ports the **energy split decoder** that sits in front of
the recursive `quant_band` / `quant_partition` work: libopus
`compute_qn`, `compute_theta`, and `quant_band_n1` from `celt/bands.c`.
Together these decide *how* a band's energy gets split between the
two halves of a mid/side (or stereo) decomposition before any PVQ
shape decode happens.

New surface area in `CeltSplit`:

- **`ComputeQn(int n, int b, int offset, int pulseCap, bool stereo) → int`** —
  pure integer math: given the partition size, the available bit
  budget, and the band's pulse cap, returns the number of theta
  quantisation levels `qn` (always 1 or an even value ≤ 256).
  Reproduces libopus byte-for-byte against an inline reference port
  in the test suite.
- **`ComputeTheta(ref OpusRangeDecoder, int logNAtBand, int bandIndex, int intensity, int n, ref int b, int blocks, int blocks0, int LM, bool stereo, ref int fill, bool disableInv, int remainingBits, out BandSplitContext sctx)`** —
  decoder-side port of libopus `compute_theta`. Decodes `itheta` from
  one of three pdfs (step / uniform / triangular) depending on the
  partition's geometry, then derives `imid`, `iside`, and the
  mid-vs-side bit-allocation `delta`. Updates the caller's bit
  budget `b` and per-block `fill` mask in-place. The encoder-only
  branches (`theta_round`, `avoid_split_noise`, `ENABLE_QEXT`) are
  not ported.
- **`BandSplitContext`** — public output struct mirroring libopus
  `struct split_ctx`: `Inv`, `IMid`, `ISide`, `Delta`, `ITheta`,
  `QAlloc`.
- **`QuantBandN1(ref OpusRangeDecoder, ref int remainingBits, Span<float> X, Span<float> Y, Span<float> lowbandOut) → uint`** —
  the degenerate-partition fast path: when `N==1`, the entire band
  reduces to a single sign bit per channel followed by a `±1.0f`
  resynth. Optionally writes a 1/16-scaled value to `lowband_out`
  for the next band's lowband prediction. Returns the codeword
  mask (always 1).
- **`FinaliseSplit`** (internal) — the `imid`/`iside`/`delta`
  computation extracted from `compute_theta` so the pure-math part
  is independently testable. Identical to libopus' tail of
  `compute_theta`.
- **`IsqrtU32`** (internal) — bit-exact port of libopus `isqrt32`
  (digit-by-digit binary square root) used by the triangular pdf
  decoder. Returns `floor(sqrt(v))` for any uint input.

Why this slice exists separately: `compute_qn` / `compute_theta` /
`quant_band_n1` are self-contained — they take a few primitives
(range decoder, `bitexact_cos`, `bitexact_log2tan`) and return
caller-consumable results. Shipping them now means the recursive
`quant_partition` work in Phase 2c.3b.4 can focus on the splitting
state machine alone, calling these helpers as building blocks.

Test coverage (`CeltSplitTests`, 35 new tests):

- **`ComputeQn_Matches_LibOpus_Reference`** — runs both our port and
  an inline libopus reference port on shared inputs; any drift fails
  the test.
- **`ComputeQn_Always_Returns_Even_Or_One`** — invariant sweep over
  thousands of `(n, b, offset)` combinations.
- **`ComputeQn_Caps_At_256`** — qn upper bound.
- **`FinaliseSplit_With_Middle_Theta_Matches_BitexactCos`** — sweeps
  itheta over [64, 16320] and verifies `imid`/`iside`/`delta`
  against the bit-exact cosine helpers from Phase 2c.3b.1.
- **`QuantBandN1_*`** — sign-bit decode, stereo decode, zero-budget
  no-op, lowband_out scaling, argument validation.
- **`ComputeTheta_With_*_Qn_One_*`** — exercises the degenerate qn=1
  branches (mono and stereo, with and without the `disableInv` mask).
- **`IsqrtU32_*`** — known-value table plus a 50 000-input sweep
  asserting the invariant `r² ≤ v < (r+1)²`.

The entropy-pdf branches of `ComputeTheta` (step / uniform /
triangular) will get end-to-end coverage in Phase 2c.3b.5 once
`quant_all_bands` is wired against real Opus packets.

Output is still silence — `CeltSplit` is not yet invoked from
`CeltDecoder.Decode`. That wiring lands once the recursive
`quant_band` logic ships in Phase 2c.3b.4 / 2c.3b.5.

## Phase 2c.3b.2 behavior (added on top of Phase 2c.3b.1)

Phase 2c.3b.2 ports the **PVQ shape codeword decoder** itself —
libopus `cwrsi` plus its `decode_pulses` range-coder wrapper. Given a
codeword index `i ∈ [0, V(N, K))` the decoder reconstructs the unique
N-dimensional signed integer vector whose absolute values sum to K.
This is the inverse of CELT's spherical-vector quantiser and runs
once per band on every CELT frame.

New surface area in `CeltPvq`:

- **`DecodePulses(ref OpusRangeDecoder, int n, int k, Span<int> y) → int yy`** —
  reads `ceil(log2(V(n,k)))` bits from the range coder via
  `ec_dec_uint`, decodes the resulting index into `y[0..n)`, and
  returns the sum-of-squares `yy = Σ y[j]²`. Pure stack allocations;
  no GC pressure.
- **`ComputeV(int n, int k) → uint`** — returns the codebook size
  `V(N, K) = U(N, K) + U(N, K+1)`, i.e. the number of length-N integer
  vectors with `Σ |y| = K`. Used by both the decoder and the bit
  allocator. Cross-checked against the closed forms
  `V(2,K)=4K`, `V(3,K)=4K²+2`, `V(4,K)=8K(K²+2)/3`, and against the
  libopus static `U_DATA` table at the upper edge (`V(5,5)=1002`,
  `V(6,6)=5336`).
- **`Cwrsi`** (internal) — small-footprint port of libopus `cwrsi`.
  Uses two `O(K)` recurrences (`Unext`, `Uprev`) instead of the 1272-
  entry static `U` table, trading a few kilocycles per band for ~5 KB
  of binary size.

Why the small-footprint variant: the production libopus `cwrsi` reads
from a precomputed `CELT_PVQ_U_DATA` array (`U[NMAX=11][KMAX=128+1]`),
which would balloon this decoder's data segment. The recurrence
variant produces bit-identical output (verified by an exhaustive
visit-every-codeword test for all `(n, k)` with `n ∈ [2, 6]`,
`k ∈ [1, 6]`) and a decode-then-encode round-trip test over
the same grid.

Test coverage (`CeltPvqTests`, 26 new tests):

- **`DecodePulses_Visits_Every_Codeword_Exactly_Once`** — enumerates
  `i ∈ [0, V(n, k))`, decodes each, and asserts that the resulting
  `V(n,k)` vectors are all distinct, all have `Σ |y| == k`, and all
  have `yy == Σ y[j]²`.
- **`Decode_Then_Encode_Round_Trips_For_All_Indices`** — pairs each
  decoder output with a test-only `IcwrsEncode` (small-footprint
  `icwrs` port) and confirms `encode(decode(i)) == i`.
- **`DecodePulses_Reads_From_Range_Coder_And_Produces_Valid_Vector`** —
  smoke test that confirms the range-coder integration is wired
  correctly end-to-end.
- Boundary checks: `n < 2`, `k < 1`, undersized `y` span, and `i ≥ V(n,k)`
  all throw the expected exceptions.

Output is still silence — `CeltPvq.DecodePulses` won't be invoked from
`CeltDecoder.Decode` until `quant_all_bands` lands in Phase 2c.3b.5.

## Phase 2c.3b.1 behavior (added on top of Phase 2c.3a)

Phase 2c.3b ports the CELT PVQ shape decoder, the largest single block
in the CELT layer. We split it into five sub-phases (`2c.3b.1` …
`2c.3b.5`); this commit ships the foundational math + cache helpers
that every later sub-phase depends on. Output is still silence — the
helpers are only invoked by the upcoming `quant_all_bands` integration.

New surface area in `CeltPvqMath`:

- **`BitexactCos(short x)`** — bit-exact `cos(x * π/16384)` approximation
  returning a Q15 value. Reproduces libopus `bitexact_cos` byte-for-byte
  (verified against the self-test vectors `cos(64) == 32767`,
  `cos(8192) == 23171`, `cos(16320) == 200`). Valid for `x ∈ [64, 16320]`.
- **`BitexactLog2Tan(int isin, int icos)`** — bit-exact
  `(11-bit-scaled) log2(tan(asin(isin) - acos(icos)))` approximation.
  Antisymmetric: `BitexactLog2Tan(a, b) == -BitexactLog2Tan(b, a)`.
  Zero when `isin == icos`. Used by the PVQ split decoder to price
  intensity / theta angles.
- **`GetPulses(int i)`** — maps pseudo-pulse index `i ∈ [0, 40]` to the
  real pulse count. Linear in `[0, 8)`, then geometric
  `(8 + i&7) << ((i>>3) - 1)`. Tops out at 128 (= `CeltMaxPulses`)
  at `i == 40` (= `MaxPseudo`).
- **`Bits2Pulses(int band, int lm, int bits)`** — 6-iteration binary
  search over the per-band pulse-cost cache; returns the pseudo-pulse
  index whose cost is closest to the requested bit budget.
- **`Pulses2Bits(int band, int lm, int pulses)`** — inverse lookup:
  cost-in-bits of a given pseudo-pulse count. Returns 0 for `pulses==0`.
- **`CacheIndex50[105]` / `CacheBits50[392]`** — exact copies of the
  libopus 48 kHz pulse-cost cache from
  `celt/static_modes_float.h`. Indexed by
  `(LM+1) * MaxBands + band`; a `-1` cache-index means the band is too
  narrow (`N==1`) for PVQ pulses at that LM.

Why this matters: every `quant_band` / `quant_partition` call in the
remaining sub-phases needs `Bits2Pulses` to convert its bit budget into
a PVQ codeword index space, `BitexactCos` / `BitexactLog2Tan` to split
energy between stereo / theta halves, and `GetPulses` to recover the
actual codebook size. Centralising these here means the recursive
quantiser doesn't have to track any state — it's pure math.

## Phase 2c.3a behavior (added on top of Phase 2c.2b)

`unquant_fine_energy` (libopus `celt/quant_bands.c`) consumes the
`fine_quant[i]` bits computed by Phase 2c.2b's allocator and folds
each band's per-channel refinement into the coarse log-energy
spectrum.

Newly observable on `CeltDecoder`:

- **`LastFineEnergyOffsets`** — per-`(channel, band)` Q10 offset
  applied to `_oldLogE`. Layout: `channel * MaxBands + band`. Bands
  with `LastFineBits[i] == 0` keep offset 0; bands with a non-zero
  fine-bit count store an offset in `[-512, 511]` (i.e. `[-0.5, +0.5)`
  log2 units) computed as
  `(((q2 << DB_SHIFT) + 512) >> ebits) - 512` for the raw `q2 ∈
  [0, 2^ebits)` integer read from the range coder. The same offset is
  also added to `OldLogE` in place, so the refined log-energy is
  immediately consumed by downstream stages.

Output is still silence — Phase 2c.3b ships PVQ shape decode
(`decode_pulses`, `cwrsi`), 2c.4 ships anti-collapse +
`unquant_energy_finalise` (final energy), and 2d ships the IMDCT
pipeline.

## Phase 2c.2a behavior (added on top of Phase 2c.1)

CELT-only frames now also decode the three symbols that lead into bit
allocation:

- **init_caps (`LastBandCaps`)** — pure table lookup, no entropy. Per
  band, in fractional bits (1/8 bit units), computed as
  `(cache.caps[idx] + 64) * C * N >> 2` from libopus
  `cache_caps50[168]`.
- **dyn_alloc (`LastBandBoost`)** — per-band boost loop. Each band
  decodes 1-bit flags at probability `2^-dynalloc_logp` (starts at 6,
  drops to 1 once one bit is paid for a given band, and to a global
  minimum of 2 between bands once any boost has been allocated).
  Boost units are fractional bits, capped at `LastBandCaps[i]`. Stops
  consuming bits as soon as a flag reads 0 or the cap is hit.
- **alloc_trim (`LastAllocTrim`)** — 11-outcome ICDF
  (`{126, 124, 119, 109, 87, 41, 19, 9, 4, 2, 0}`, ftb=7) selecting the
  global trim that biases bit allocation towards low or high bands.
  Defaults to `AllocTrimDefault (5)` when the bit budget is exhausted.

Output is still silence — Phase 2c.2b ships `compute_allocation` (the
multi-pass bit-budget search plus intensity / dual stereo / skip),
Phase 2c.3 ships fine energy + PVQ shape decode, 2c.4 ships
anti-collapse + final energy, and 2d ships the IMDCT pipeline.

## Phase 2c.2b behavior (added on top of Phase 2c.2a)

`compute_allocation` is the multi-pass bit-budget search that converts
the running entropy budget plus the per-band caps / boosts / trim into
a concrete pulse / fine-bit allocation. The C# port follows libopus
`celt/rate.c::clt_compute_allocation` + `interp_bits2pulses`
byte-for-byte.

Newly observable on `CeltDecoder`:

- **`LastAntiCollapseReserved`** — `true` iff 1 fractional bit was
  reserved for the anti-collapse flag (transient frames with `LM >= 2`
  and enough remaining budget).
- **`LastCodedBands`** — number of bands actually carrying PVQ pulses
  (between `StartBand+1` and `EndBand`); the skip-flag loop signals
  high bands away when budget runs out.
- **`LastIntensity`** — intensity-stereo cutoff band index. For mono
  this is always `0`; for stereo it lives in
  `[StartBand, LastCodedBands]` and is read as a uniform integer from
  the range coder when `intensity_rsv > 0`.
- **`LastDualStereo`** — single bit decoded only when intensity stereo
  is active (`LastIntensity > StartBand`) and `dual_stereo_rsv > 0`.
- **`LastPulses`** — per-band PVQ pulse budget (in 1/8-bit fractional
  units, post fine-energy subtraction). Bands outside
  `[StartBand, LastCodedBands)` carry zero.
- **`LastFineBits`** — per-band fine-energy bit count, in `[0, 8]`. Set
  for every band in `[StartBand, EndBand)`; skipped bands receive any
  remaining `pulses[j] >> stereo >> BITRES` budget as fine bits.
- **`LastFinePriority`** — per-band 0/1 priority used by the unused-bit
  redistribution loop in Phase 2c.4.
- **`LastAllocationBalance`** — leftover fractional bits passed to PVQ
  for inter-band rebalancing.

Tables added to `CeltConstants`: `BandAllocation` (the 11×21
`band_allocation` matrix), `LogN400` (per-band `log2(N) + log2_frac`
seed), `Log2FracTable[0..23]` (1/8-bit `log2(1+x/16)` values used to
size the intensity stereo symbol). Constants: `NbAllocVectors=11`,
`AllocSteps=6`, `FineOffset=21`, `MaxFineBits=8`, `QThetaOffset=4`.

Output is still silence — Phase 2c.3 ships fine energy + PVQ shape
decode, 2c.4 ships anti-collapse + final energy, and 2d ships the
IMDCT pipeline.

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
