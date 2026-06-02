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
| 2c.3b.5a | CELT `quant_partition` recursive mono splitter + `renormalise_vector` + `celt_lcg_rand` | ✅ shipped |
| 2c.3b.5b | CELT `quant_band` mono wrapper (Haar1 recombine / time-divide + Hadamard reorganisation + √N lowband_out scaling) | ✅ shipped |
| 2c.3b.5c | CELT `quant_band_stereo` (stereo mid/side band wrapper + `stereo_merge`)              | ✅ shipped |
| 2c.3b.5d | CELT `quant_all_bands` top-level band-iteration integration                            | ✅ shipped |
|  2c.4 | CELT anti-collapse + `unquant_energy_finalise` (final energy)              | ⏳ planned |
|    2d | CELT IMDCT + post-filter + window overlap-add → first real PCM            | ⏳ planned |
|     3 | SILK NLSF / LPC stability / LTP scaling / sub-frame gains                 | ⏳ planned |
|     4 | SILK excitation + sub-frame synthesis                                     | ⏳ planned |
|     5 | Hybrid bit-allocation + 8/12/16/24/48 kHz resampler                       | ⏳ planned |
|     6 | Multistream, PLC / FEC, perf tuning, RFC test vectors                     | ⏳ planned |

## Phase 2c.3b.5d behavior (added on top of Phase 2c.3b.5c)

Phase 2c.3b.5d ports `quant_all_bands` — the **top-level band-iteration
driver** that walks every CELT band, computes its per-band bit budget,
dispatches to the mono or stereo PVQ wrapper, threads the LCG seed
through the no-pulse fold path, and accumulates the conservative
collapse-mask source for fold seeding of subsequent bands. With this
slice the entire PVQ block (Phase 2c.3b) is end-to-end usable; the
remaining CELT work (anti-collapse + IMDCT + post-filter + window
overlap-add) builds on top of the normalised band coefficients this
driver writes.

`QuantAllBands(...)` ports the libopus decoder branch (no QEXT, no
theta_rdo, no encoder resynth state) of the band-iteration loop:

1. **Per-iteration budget** — `tell = ec_tell_frac(ec)` is captured at
   the start of every band. After band 0 the running `balance` is
   adjusted by `balance -= tell` to share leftover bits with later
   bands. `remaining_bits = total_bits - tell - 1` is written into the
   `BandContext` for the recursive wrappers. The per-band budget
   `b = max(0, min(16383, min(remaining_bits + 1, pulses[i] + currBalance)))`
   uses the `celt_sudiv`-style integer division
   `currBalance = balance / min(3, codedBands - i)`. Bands beyond
   `codedBands` get `b = 0` (fold-only).
2. **Folding source bookkeeping** — `lowband_offset` advances with the
   loop, but only when (a) the previous lowband has fully crossed the
   start band edge or we're at `start+1`, and (b) the previous band's
   budget exceeded `N << BITRES`. At `i == start + 1` libopus calls
   `special_hybrid_folding`, which duplicates the first band's tail
   data into the second band's fold-source slot (no-op for CELT-only
   widths, mandatory for hybrid widths where band-2's width exceeds
   band-1's).
3. **Conservative collapse-mask accumulator** — when `lowband_offset != 0`
   and (`spread != AGGRESSIVE || B > 1 || tf_change < 0`), the wrapper
   walks back through prior bands' `collapse_masks` and `OR`s them
   together into the fold seed for both channels. Otherwise both seeds
   default to `(1<<B)-1` (all blocks treated as non-zero, signalling
   the LCG-fold path inside the recursive wrappers).
4. **Dual-stereo intensity crossover** — when `dualStereo == true` and
   the loop reaches band `intensity`, the two parallel `norm`/`norm2`
   buffers are averaged in-place over the bands decoded so far
   (`norm[j] = 0.5·(norm[j] + norm2[j])`) and `dualStereo` is flipped
   off — subsequent bands use the joint-stereo `quant_band_stereo`
   path.
5. **Dispatch** — three branches, all consuming
   `effectiveLowband = max(0, M·eBands[lowband_offset] - norm_offset - N)`
   for the fold source slice:
   - **Dual stereo**: two `QuantBand` calls, one per channel, each with
     half the budget (`b/2`) and gain 1.0.
   - **Joint stereo** (`Y` non-empty, not dual): a single
     `QuantBandStereo` call with `(xCm | yCm)` as the combined fill.
   - **Mono** (`Y` empty): a single `QuantBand` call.
6. **Output bookkeeping** — `collapseMasks[i*C + c]` is written for
   each channel, `balance += pulses[i] + tell` accumulates for the
   next iteration, `update_lowband = b > (N << BITRES)` controls
   whether the next band may advance `lowband_offset`.

The decoder branch never touches `theta_round`, `bandE`,
`avoid_split_noise`, or any `ENABLE_QEXT`-gated state. The
"`i >= effEBands`" branch from libopus is unreachable in our port
because our `CeltMode.EndBand` already caps the iteration at the
bands that produce real output. The encoder-only buffers (`X_save`,
`Y_save`, `X_save2`, `Y_save2`, `norm_save2`, `bytes_save`) are not
allocated.

`SpecialHybridFolding(eBands, norm, norm2, start, M, dualStereo)` is
exposed as a public static helper for unit testing — it just does the
`OPUS_COPY(&norm[n1], &norm[2*n1 - n2], n2 - n1)` duplication (and
mirrors it for `norm2` when `dualStereo` is true) when `n2 > n1`.

### Files added this phase

- `csharp-mediar/src/Mediar.Codecs.Opus.Decoder/Celt/CeltBands.cs`:
  +`SpecialHybridFolding` (public static, ~12 lines) and
  +`QuantAllBands` (public static, ~150 lines) appended to the
  existing partial class. The new method's `normWorkspace` parameter
  takes a caller-allocated scratch span — sized for the worst case as
  `C * (M·eBands[end-1] - M·eBands[start])` floats. The integration
  layer (Phase 2c.4 / `CeltDecoder.Decode`) will allocate this once
  per decoder instance.
- `csharp-mediar/tests/Mediar.Tests/CeltBandsTests.cs`: 15 new tests
  covering:
  - `SpecialHybridFolding`: CELT-only no-op / hybrid duplicate /
    dual-stereo norm2 mirror / `dualStereo=false` leaves norm2 alone.
  - `QuantAllBands`: mono FB / joint-stereo WB / dual-stereo (no
    intensity crossover) / dual-stereo with intensity crossover /
    short blocks / hybrid start / aggressive spread non-transient /
    low codedBands / seed mutation / two argument validations.

### Test results after Phase 2c.3b.5d

Test suite total after Phase 2c.3b.5d: **7777 / 7777 pass**
(+15 vs. Phase 2c.3b.5c's 7762). No flaky failures on this run.

## Phase 2c.3b.5c behavior (added on top of Phase 2c.3b.5b)

Phase 2c.3b.5c ports `quant_band_stereo` — the **stereo mid/side band
wrapper** that pairs the Phase 2c.3b.5b mono wrapper with the
`compute_theta` energy split from Phase 2c.3b.3 to decode coupled
left/right CELT bands. It also adds `stereo_merge`, the inverse rotation
that re-derives L/R channel norms from the mid/side pair after the two
recursive mono decodes return.

`QuantBandStereo(ref BandContext, ref OpusRangeDecoder, Span<float> X,
Span<float> Y, int N, int b, int blocks, Span<float> lowband, int LM,
Span<float> lowbandOut, Span<float> lowbandScratch, int fill) → uint`
follows the libopus float-build decoder branch closely:

1. **N==1 short-circuit** — routes to `CeltSplit.QuantBandN1` (Phase
   2c.3b.3) with a non-empty Y span, decoding two sign bits.
2. **`ComputeTheta`** is called with `stereo: true` and `B0 = blocks`
   (same as B for the stereo wrapper), producing `inv` / `imid` /
   `iside` / `delta` / `itheta` / `qalloc`. The `mid` and `side`
   scaling factors are derived as Q15 → float (`imid * 1/32768`,
   `iside * 1/32768`).
3. **N==2 special case** — for width-2 bands the side becomes the only
   unit vector orthogonal to the mid (up to sign), so libopus skips the
   PVQ recursion for the side and just decodes a 1-bit sign when
   `itheta != 0 && itheta != 16384`. The wrapper:
   - splits the budget as `mbits = b; sbits = sign-bit-present ? 1<<BITRES : 0; mbits -= sbits;`
   - swaps which channel is decoded as the mid based on `c = itheta > 8192`;
   - decodes the chosen channel via the mono `QuantBand` recursion;
   - constructs the orthogonal side as `y2[0] = -sign*x2[1]; y2[1] = sign*x2[0]`;
   - applies an inline 2-point mid/side → L/R butterfly
     (`X *= mid; Y *= side; tmp = X[i]; X[i] = tmp - Y[i]; Y[i] = tmp + Y[i]`).
4. **N>2 normal split** — splits the budget between mid and side, with
   the larger half decoded first. The mid is decoded with `gain = 1.0`
   (libopus needs the normalised mid for folding), the side is decoded
   with `gain = side` (the cosine-derived scaling) and `fill >> blocks`
   (the high bits of fill are always zero for the side, so no folding).
   The mid keeps the caller's `lowband` / `lowbandOut` /
   `lowbandScratch`; the side passes empty spans for all three. After
   the first call, leftover bits above `3<<BITRES` are rolled into the
   second call's budget (only on the appropriate side of `itheta`).
5. **Resynth** — for N>2 the wrapper calls `CeltShape.StereoMerge`
   (which is *not* called for N==2 because the inline butterfly already
   produced L/R). If `ComputeTheta` set `inv != 0`, every sample of Y
   is negated as the last step (matches the libopus φ-rotation
   inversion).

`StereoMerge(Span<float> X, Span<float> Y, float mid, int N)` ports the
libopus float-build of `stereo_merge` (which is in `celt/bands.c` in
recent libopus, not `celt/vq.c`):

- Computes `xp = Σ Y[i]·X[i]` and `side = Σ Y[i]²` (the two
  `celt_inner_prod_norm_shift` calls — degenerate to plain dot products
  in the float build).
- Compensates the mid normalisation: `xp = mid · xp`.
- Reconstructs left/right energies as
  `El = mid² + side − 2·xp` and `Er = mid² + side + 2·xp`
  (the libopus `SHR32(mid², 3)` term and the `kl`/`kr` block-floating
  shifts are no-ops in the float build, so the formula simplifies).
- Applies the silent-merge guard: when `El < 6e-4f || Er < 6e-4f`,
  copies X into Y and returns — this matches the libopus
  near-zero-energy clamp that protects the rsqrt from blowing up to
  NaN/Inf when the recovered channel is silent.
- Otherwise computes `lgain = 1/√El`, `rgain = 1/√Er`, and runs the
  per-sample inverse rotation:
  `l = mid·X[j]; r = Y[j]; X[j] = lgain·(l−r); Y[j] = rgain·(l+r)`.

The 14 `QuantPartition` LUTs and bit-interleave tables from Phases
2c.3b.5a / 2c.3b.5b are re-used unchanged.

### Files added this phase

- `csharp-mediar/src/Mediar.Codecs.Opus.Decoder/Celt/CeltBands.cs`:
  +`StereoMerge` (public static, allocation-free, ~30 lines including
  the silent-merge guard) and +`QuantBandStereo` (public static,
  ~120 lines covering N==1, N==2 inline-merge and N>2 normal-split
  branches) appended to the existing partial class.
- `csharp-mediar/tests/Mediar.Tests/CeltBandsTests.cs`: 15 new tests
  covering `StereoMerge` (zero-side / orthogonal mid+side / low-energy
  clamp / random-input finite-output / two argument validations) and
  `QuantBandStereo` (N==1 stereo short-circuit, N==2 mid+side decode,
  N>2 split with and without lowband, disable_inv path, tight-budget
  path, three argument validations).

### Test results after Phase 2c.3b.5c

Test suite total after Phase 2c.3b.5c: **7762 / 7762 pass**
(+15 vs. Phase 2c.3b.5b's 7747). The flaky
`Mp3DecoderPerformanceTests.Repeated_Full_Decode_Passes_Do_Not_Grow_Heap_Unboundedly`
test (timing-sensitive GC heap measurement, unrelated to Opus) still
fails on cold-cache runs but passes on retry — its flakiness is
pre-existing and tracked separately.

## Phase 2c.3b.5b behavior (added on top of Phase 2c.3b.5a)

Phase 2c.3b.5b ports `quant_band` (the **mono band wrapper** around the
Phase 2c.3b.5a recursive splitter) — everything libopus does to a band
*before* and *after* calling `quant_partition`, except the stereo
half (which becomes Phase 2c.3b.5c). With this slice a decoder can
take the per-band bit allocation produced by Phase 2c.2b, plus the
shared LM/spread/intensity context, and produce a fully-folded
unit-norm spectral shape for a mono band — the exact output that
`anti_collapse` (Phase 2c.4) and the IMDCT (Phase 2d) consume.

Added to `Celt/CeltBands.cs`:

- Two static lookup tables — `BitInterleaveTable` (16 bytes,
  `{0,1,1,1,2,3,3,3,2,3,3,3,2,3,3,3}`) and `BitDeinterleaveTable`
  (16 bytes, `{0x00,0x03,0x0C,0x0F,…,0xFF}`) — copied verbatim from
  libopus `celt/bands.c`. The fill mask runs through the first table
  during the recombine pass and the collapse mask runs through the
  second during the inverse.
- `CeltBands.QuantBand(ref BandContext ctx, ref OpusRangeDecoder dec,
  Span<float> X, int N, int b, int blocks, Span<float> lowband,
  int LM, Span<float> lowbandOut, float gain,
  Span<float> lowbandScratch, int fill) → uint` — full port of the
  mono decoder branch of libopus `quant_band` (float build). Steps:

  1. **N == 1 short-circuit** routes to `CeltSplit.QuantBandN1`
     (Phase 2c.3b.3) for the single-sample sign-bit special case.
  2. **`tf_change > 0` recombine**: for each step, apply `Haar1` to
     the lowband (if non-empty) and bit-interleave the fill mask.
     `B >>= recombine` and `N_B <<= recombine` shrink the partition's
     block count to the post-recombine value.
  3. **`tf_change < 0` time-divide**: while `N_B` is even and
     `tf_change < 0`, apply `Haar1` to the lowband, double `fill` with
     itself shifted by `B`, double `B`, halve `N_B`, and advance
     `tf_change`. This expands the partition into more, smaller blocks.
  4. **Block-reorganisation**: when post-transform `B0 > 1`, apply
     `DeinterleaveHadamard` to the lowband so the partition decoder
     sees time-ordered samples.
  5. **`QuantPartition`** decodes the partition recursively.
  6. **Resynth (always for the decoder)** — invert every transform in
     reverse order: `InterleaveHadamard` if `B0 > 1`, then per
     `time_divide` step halve B + double `N_B` + propagate the
     collapse mask (`cm |= cm >> B`) + inverse `Haar1`, then per
     `recombine` step bit-deinterleave the collapse mask + inverse
     `Haar1` at the original strides.
  7. **`lowband_out` scaling**: if a non-empty output buffer is
     supplied, write `√N₀ · X[j]` for every sample (libopus's
     `MULT16_32_Q15(celt_sqrt(N0<<22), X[j])` reduced to its float
     form). This is the source the *next* band folds against.
  8. **`cm &= (1<<B)-1`** clamps the returned collapse mask to the
     per-block range.

  The wrapper is allocation-free — every transform reuses the caller's
  buffers. The `lowband_scratch` argument lets the caller opt in to
  the same scratch-copy behaviour libopus uses (avoids mutating the
  caller's lowband when transforms are applied); passing
  `Span<float>.Empty` disables it. Both `lowband` and `lowband_out`
  follow the established empty-span-equals-null convention.

Ten new tests added to `tests/Mediar.Tests/CeltBandsTests.cs`:

- N==1 short-circuit produces ±1 with the `X[0]/16` lowband_out, and
  the routed `QuantBandN1` mask of 1.
- No-transform case (tf_change=0, blocks=1) gives identical output to
  a direct `QuantPartition` call with the same bytes and decoder seed,
  plus √N-scaled lowband_out.
- √N scaling assertion (`‖lowband_out‖² == N · ‖X‖²`).
- Empty `lowband_out` no-ops the scaling step.
- Scratch-copy isolation — caller's lowband stays untouched when
  blocks > 1 forces deinterleave_hadamard.
- `tf_change > 0` recombine path (blocks=2) and `tf_change < 0`
  time-divide path (blocks=1, N=64) both produce finite unit-norm
  output without crashing.
- `blocks=4` triggers the Hadamard reorganisation bracket; the
  returned collapse mask stays within `(1<<blocks)-1`.
- Argument validation: `N < 1` and `blocks < 1` both throw.

Test suite total after Phase 2c.3b.5b: **7747 / 7747 pass**
(+10 vs. Phase 2c.3b.5a's 7737).

The next slice **2c.3b.5c** will deliver `quant_band_stereo` (mid/side
band wrapper with the N==2 single-bit-sign special case, the
`stereo_merge` resynth step, and the inversion-disable plumbing).
Slice **2c.3b.5d** then assembles `quant_all_bands`, which iterates
over bands, manages the norm / norm2 / prev1 / prev2 buffers that
feed `special_hybrid_folding` and the anti-collapse step, and routes
each band to the mono or stereo wrapper.

## Phase 2c.3b.5a behavior (added on top of Phase 2c.3b.4)

Phase 2c.3b.5a ports `quant_partition`, the **recursive mono PVQ
partition splitter** at the heart of CELT band-shape decoding, plus the
two small primitives it leans on: `renormalise_vector` and
`celt_lcg_rand`. With this slice the decoder can take a band's bit
budget, descend the binary energy-split tree it shares with the
encoder, decode a PVQ codeword at each leaf, and assemble a normalised
spectral shape vector for the band — everything *between* the
`compute_theta` energy split (Phase 2c.3b.3) and the per-band
post-processing (`quant_band` Hadamard recombination, anti-collapse) that
the next slice will deliver.

The new pieces (`Celt/CeltBands.cs` and additions to `Celt/CeltShape.cs`)
are pure, allocation-free static functions:

- `CeltShape.RenormaliseVector(Span<float> X, int N, float gain)` —
  float-build port of libopus `renormalise_vector` (`celt/vq.c`).
  Computes `E = EPSILON + Σ X²` with EPSILON = 1e-15f, then scales
  every coefficient by `gain / √E`. The EPSILON guard prevents a
  divide-by-zero on silent partitions: the result is still well-defined
  (gain · zero-vector = zero-vector), no NaNs.

- `CeltShape.LcgRand(uint seed) → uint` — Numerical-Recipes linear
  congruential generator (`1664525·seed + 1013904223` mod 2³²).
  Drives the noise-fill / dither paths in the leaf below. Unchecked
  arithmetic matches the C wrap-around exactly.

- `CeltBands.BandContext` — public mutable struct carrying the
  per-band state that flows through the recursion: `Band`, `Spread`,
  `Intensity`, `TfChange`, `RemainingBits`, `Seed`, `DisableInv`. The
  struct is deliberately passed `ref` so the recursive callee can
  decrement `RemainingBits` and advance `Seed` in place, just like
  libopus mutates `ctx` through the pointer it passes.

- `CeltBands.QuantPartition(ref BandContext ctx, ref OpusRangeDecoder
  dec, Span<float> X, int N, int b, int blocks, ReadOnlySpan<float>
  lowband, int LM, float gain, int fill) → uint` — full port of the
  decoder branch of libopus `quant_partition` (`celt/bands.c`). It
  has two paths:

  **Split path** (`LM != -1 && b > cache[cache[0]] + 12 && N > 2`):
  the partition is divided in half, an angle/sign split is decoded via
  `CeltSplit.ComputeTheta` (Phase 2c.3b.3), the bit budget and fill
  mask are partitioned between the mid and side halves with libopus's
  rebalance heuristic, and the two halves are decoded recursively at
  `LM-1` with `gain * mid` and `gain * side`. The pre-echo / forward-
  masking delta adjustment that libopus applies when a partition spans
  multiple MDCT blocks (`B0 > 1 && (itheta & 0x3FFF) != 0`) is
  reproduced bit-for-bit (`delta -= delta >> (4 - splitLM)` above the
  90° mark, additive clamp below).

  **Leaf path**: the bit budget is converted to a pulse count via
  `CeltPvqMath.Bits2Pulses`, the bit-busting prevention loop shrinks
  the pulse count until `RemainingBits >= 0`, and the result is either
  decoded with `CeltShape.AlgUnquant` (q ≠ 0), filled with renormalised
  noise from `LcgRand` (q = 0, no lowband), filled with the lowband
  plus ±1/256 dither and renormalised (q = 0, lowband present), or
  zeroed (q = 0, fill = 0 after masking by `(1<<blocks)-1`). The
  function returns the **collapse mask** of the partition: in the
  split case the recursive submasks are OR-combined with the side mask
  shifted by `blocks0 >> 1`; in the leaf case it's either the AlgUnquant
  result, `(1<<blocks)-1`, the masked fill, or 0.

The function is allocation-free — `Span<float>` slicing and
`stackalloc` inside `AlgUnquant` handle all scratch. `OpusRangeDecoder`
is passed by `ref` because it's a `ref struct`. The lowband-null
sentinel from libopus (`celt_norm *lowband == NULL`) is modelled as
`ReadOnlySpan<float>.IsEmpty`, which both `default` and a true empty
span satisfy.

Sixteen new tests in `tests/Mediar.Tests/CeltBandsTests.cs` cover:

- `RenormaliseVector` unit-norm output (3-4-5 right triangle), gain
  scaling at arbitrary norms, EPSILON guard on the all-zero vector,
  and direction-preservation (signs / ratios unchanged).
- `LcgRand` matching the libopus constants on seeds 0, 1, and
  0xFFFFFFFF (wrap-around), plus a 1000-iteration uniqueness sanity
  check (no early cycle).
- `QuantPartition` leaf no-pulse paths: zero output when
  `fill & ((1<<blocks)-1) == 0`, gain·unit-norm noise injection when
  `fill ≠ 0` and lowband is empty (with seed advancement verified),
  folded-lowband injection when both `fill ≠ 0` and lowband is
  populated (direction preservation), and the `blocks=2` fill-masking
  edge case where `fill = 0b1100` clears the partition.
- `QuantPartition` leaf with pulses: unit-norm output on a tight `N=2,
  LM=0, band=8` configuration; bit-busting guard tolerated when
  `RemainingBits` starts negative.
- `QuantPartition` recursive split: smoke tests at `LM=2, N=16,
  band=13, b=400` (recursion descends to LM=-1) and `LM=3, N=64,
  band=17, b=800, blocks=2` (deep recursion, two-block fill). The
  collapse mask is validated against `(1<<blocks0) - 1` and every
  output sample is verified to be finite.
- Argument validation: `N < 1` and `X.Length < N` throw
  `ArgumentOutOfRangeException` / `ArgumentException`.

Test suite total after Phase 2c.3b.5a: 7737 / 7737 pass (+16 vs.
Phase 2c.3b.4's 7721).

The remaining Phase 2c.3b slice is **2c.3b.5b**: the `quant_band`
mono/stereo wrappers (which add the Hadamard time-recombination
around `QuantPartition` and the `quant_band_stereo` mid/side merge),
plus `quant_all_bands` (which iterates over bands, manages norm
buffers and the `prev1` / `prev2` tracking that feeds the
`anti-collapse` step).



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
