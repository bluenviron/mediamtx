namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Mutable per-band state shared across the recursive PVQ shape
/// decoder. Mirrors libopus <c>struct band_ctx</c> with the
/// encoder-only and fixed-point-only fields stripped — what remains
/// is exactly the state read or mutated by <see cref="CeltBands.QuantPartition"/>
/// and (in later phases) <c>quant_band</c> / <c>quant_band_stereo</c>.
/// </summary>
public struct BandContext
{
    /// <summary>Current band index (libopus <c>ctx->i</c>).</summary>
    public int Band;

    /// <summary>Intensity-stereo cut-off band (libopus <c>ctx->intensity</c>).</summary>
    public int Intensity;

    /// <summary>Spread mode 0..3 (<c>SPREAD_NONE</c>..<c>SPREAD_AGGRESSIVE</c>).</summary>
    public int Spread;

    /// <summary>Per-band time-frequency change flag (libopus <c>ctx->tf_change</c>).</summary>
    public int TfChange;

    /// <summary>
    /// Bits remaining in the budget, in 1/8-bit units. Mutated as the
    /// recursion consumes entropy.
    /// </summary>
    public int RemainingBits;

    /// <summary>
    /// LCG seed for the no-pulse noise / folded-spectrum fill path.
    /// Mutated in-place by <see cref="CeltShape.LcgRand"/> calls.
    /// </summary>
    public uint Seed;

    /// <summary>
    /// Set when the bitstream explicitly disables the stereo inversion
    /// flag (libopus <c>ctx->disable_inv</c>).
    /// </summary>
    public bool DisableInv;
}

/// <summary>
/// CELT PVQ shape decoder — recursive mono partition splitter
/// (libopus <c>quant_partition</c>). Phase 2c.3b.5a.
/// </summary>
/// <remarks>
/// Decoder branch only. Encoder paths, fixed-point branches, and the
/// <c>ENABLE_QEXT</c> extension are not ported. Stereo handling lives
/// in <c>quant_band_stereo</c> (later phase) which wraps this mono
/// function via the same mid/side decomposition that
/// <see cref="CeltSplit.ComputeTheta"/> produces.
/// </remarks>
public static class CeltBands
{
    /// <summary>
    /// Recursive PVQ shape decoder for a single mono partition.
    /// Either splits the band in half via mid/side decomposition and
    /// recurses, or hits the leaf path that calls
    /// <see cref="CeltShape.AlgUnquant"/> (when pulses fit) or the
    /// no-pulse fill (noise / folded / zero). Mirrors the float-build
    /// decoder branch of libopus <c>quant_partition</c>.
    /// </summary>
    /// <param name="ctx">Mutable per-band state (see <see cref="BandContext"/>).</param>
    /// <param name="dec">Range decoder.</param>
    /// <param name="X">Output partition vector (length ≥ N).</param>
    /// <param name="N">Partition size in samples.</param>
    /// <param name="b">Bit budget for this partition (1/8-bit units).</param>
    /// <param name="blocks">MDCT block count for this partition.</param>
    /// <param name="lowband">
    /// Lowband prediction source for the folded-spectrum no-pulse
    /// branch. Pass an empty span to indicate "null" (triggers the
    /// noise branch instead).
    /// </param>
    /// <param name="LM">Log-2 of frame size (0..3); −1 disables splitting.</param>
    /// <param name="gain">Output gain.</param>
    /// <param name="fill">Per-block fill mask propagated through the recursion.</param>
    /// <returns>Collapse mask — one bit per MDCT block.</returns>
    public static uint QuantPartition(
        ref BandContext ctx,
        ref OpusRangeDecoder dec,
        Span<float> X,
        int N,
        int b,
        int blocks,
        ReadOnlySpan<float> lowband,
        int LM,
        float gain,
        int fill)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        if (X.Length < N) throw new System.ArgumentException("X must hold at least N samples.", nameof(X));

        int band = ctx.Band;
        int spread = ctx.Spread;
        int blocks0 = blocks;

        // Pulse cache for this (LM, band). `cache[0]` is the highest
        // pulse index; `cache[cache[0]]` is its bit cost.
        int cacheStart = CeltPvqMath.CacheIndex50[(LM + 1) * CeltConstants.MaxBands + band];
        ReadOnlySpan<byte> cache = CeltPvqMath.CacheBits50.Slice(cacheStart);

        // Split path: if even the max pulse count costs > 1.5 bits less than
        // the budget (and the partition is splittable), recurse.
        if (LM != -1 && b > cache[cache[0]] + 12 && N > 2)
        {
            int halfN = N >> 1;
            Span<float> Y = X.Slice(halfN, halfN);
            Span<float> Xhalf = X.Slice(0, halfN);
            int splitLM = LM - 1;
            if (blocks == 1) fill = (fill & 1) | (fill << 1);
            int splitBlocks = (blocks + 1) >> 1;

            int splitBudget = b;
            int splitFill = fill;
            CeltSplit.ComputeTheta(
                ref dec,
                logNAtBand: CeltConstants.LogN400[band],
                bandIndex: band,
                intensity: ctx.Intensity,
                n: halfN,
                b: ref splitBudget,
                blocks: splitBlocks,
                blocks0: blocks0,
                LM: splitLM,
                stereo: false,
                fill: ref splitFill,
                disableInv: ctx.DisableInv,
                remainingBits: ctx.RemainingBits,
                sctx: out CeltSplit.BandSplitContext sctx);
            b = splitBudget;
            fill = splitFill;

            int imid = sctx.IMid;
            int iside = sctx.ISide;
            int delta = sctx.Delta;
            int itheta = sctx.ITheta;
            int qalloc = sctx.QAlloc;

            // Float-build mid/side coefficients (Q15 → float).
            float mid = imid * (1f / 32768f);
            float side = iside * (1f / 32768f);

            // Pre-echo / forward-masking adjustment when the partition
            // spans multiple MDCT blocks and the angle isn't on-axis.
            if (blocks0 > 1 && (itheta & 0x3FFF) != 0)
            {
                if (itheta > 8192)
                    delta -= delta >> (4 - splitLM);
                else
                    delta = System.Math.Min(0, delta + ((halfN << CeltConstants.BitRes) >> (5 - splitLM)));
            }
            int mbits = System.Math.Max(0, System.Math.Min(b, (b - delta) / 2));
            int sbits = b - mbits;
            ctx.RemainingBits -= qalloc;

            ReadOnlySpan<float> nextLowband = lowband.IsEmpty ? default : lowband.Slice(0, halfN);
            ReadOnlySpan<float> nextLowband2 = lowband.IsEmpty ? default : lowband.Slice(halfN, halfN);

            int rebalance = ctx.RemainingBits;
            uint cm;
            if (mbits >= sbits)
            {
                cm = QuantPartition(ref ctx, ref dec, Xhalf, halfN, mbits, splitBlocks,
                    nextLowband, splitLM, gain * mid, fill);
                rebalance = mbits - (rebalance - ctx.RemainingBits);
                if (rebalance > 3 << CeltConstants.BitRes && itheta != 0)
                    sbits += rebalance - (3 << CeltConstants.BitRes);
                cm |= QuantPartition(ref ctx, ref dec, Y, halfN, sbits, splitBlocks,
                    nextLowband2, splitLM, gain * side, fill >> blocks) << (blocks0 >> 1);
            }
            else
            {
                cm = QuantPartition(ref ctx, ref dec, Y, halfN, sbits, splitBlocks,
                    nextLowband2, splitLM, gain * side, fill >> blocks) << (blocks0 >> 1);
                rebalance = sbits - (rebalance - ctx.RemainingBits);
                if (rebalance > 3 << CeltConstants.BitRes && itheta != 16384)
                    mbits += rebalance - (3 << CeltConstants.BitRes);
                cm |= QuantPartition(ref ctx, ref dec, Xhalf, halfN, mbits, splitBlocks,
                    nextLowband, splitLM, gain * mid, fill);
            }
            return cm;
        }

        // Leaf path.
        int q = CeltPvqMath.Bits2Pulses(band, LM, b);
        int currBits = CeltPvqMath.Pulses2Bits(band, LM, q);
        ctx.RemainingBits -= currBits;

        // Bit-busting prevention loop: shrink q until we fit in the budget.
        while (ctx.RemainingBits < 0 && q > 0)
        {
            ctx.RemainingBits += currBits;
            q--;
            currBits = CeltPvqMath.Pulses2Bits(band, LM, q);
            ctx.RemainingBits -= currBits;
        }

        if (q != 0)
        {
            int K = CeltPvqMath.GetPulses(q);
            return CeltShape.AlgUnquant(X, N, K, spread, blocks, ref dec, gain);
        }

        // No-pulse fill — fold the lowband, inject noise, or zero out.
        uint cmMask = ((1u << blocks) - 1);
        fill &= (int)cmMask;
        if (fill == 0)
        {
            X.Slice(0, N).Clear();
            return 0;
        }

        uint resultMask;
        if (lowband.IsEmpty)
        {
            // Noise injection: 12-bit signed integers from the LCG, then
            // renormalise to gain · unit-norm.
            for (int j = 0; j < N; j++)
            {
                ctx.Seed = CeltShape.LcgRand(ctx.Seed);
                X[j] = (int)ctx.Seed >> 20;
            }
            resultMask = cmMask;
        }
        else
        {
            // Folded-spectrum copy with ±1/256 dither (~48 dB below the
            // normal folding level).
            const float dither = 1f / 256f;
            for (int j = 0; j < N; j++)
            {
                ctx.Seed = CeltShape.LcgRand(ctx.Seed);
                float tmp = (ctx.Seed & 0x8000u) != 0 ? dither : -dither;
                X[j] = lowband[j] + tmp;
            }
            resultMask = (uint)fill;
        }
        CeltShape.RenormaliseVector(X, N, gain);
        return resultMask;
    }

    // Bit-interleave / bit-deinterleave tables for the Haar1 recombine
    // wrapper. Lifted verbatim from libopus celt/bands.c quant_band.
    private static ReadOnlySpan<byte> BitInterleaveTable => new byte[]
        { 0, 1, 1, 1, 2, 3, 3, 3, 2, 3, 3, 3, 2, 3, 3, 3 };

    private static ReadOnlySpan<byte> BitDeinterleaveTable => new byte[]
    {
        0x00, 0x03, 0x0C, 0x0F, 0x30, 0x33, 0x3C, 0x3F,
        0xC0, 0xC3, 0xCC, 0xCF, 0xF0, 0xF3, 0xFC, 0xFF,
    };

    /// <summary>
    /// Decoder-side mono band wrapper around
    /// <see cref="QuantPartition"/>. Performs the time/frequency
    /// recombine / time-divide transforms based on <c>ctx.TfChange</c>
    /// (Haar1 + bit-interleave for short-block widening, Hadamard
    /// deinterleave when <paramref name="blocks"/> &gt; 1) before
    /// decoding the partition, then inverts every transform after the
    /// PVQ decode so the output sits in the same orientation libopus
    /// emits. Also produces a √N-scaled copy in
    /// <paramref name="lowbandOut"/> for the next band's folding
    /// source. Routes N==1 to <see cref="CeltSplit.QuantBandN1"/>.
    /// Mirrors libopus <c>quant_band</c> (mono case, decoder branch,
    /// float build).
    /// </summary>
    /// <param name="ctx">Mutable per-band state.</param>
    /// <param name="dec">Range decoder.</param>
    /// <param name="X">Output partition vector (length ≥ N).</param>
    /// <param name="N">Partition size in samples.</param>
    /// <param name="b">Bit budget (1/8-bit units).</param>
    /// <param name="blocks">MDCT block count.</param>
    /// <param name="lowband">Lowband fold source (empty = null).</param>
    /// <param name="LM">Log-2 of frame size (0..3).</param>
    /// <param name="lowbandOut">Optional √N-scaled output buffer for the
    ///   next band's folding, or empty to skip.</param>
    /// <param name="gain">Output gain.</param>
    /// <param name="lowbandScratch">Optional scratch buffer (length ≥ N)
    ///   used to avoid mutating the caller's lowband; pass empty to
    ///   disable the scratch copy.</param>
    /// <param name="fill">Per-block fill mask.</param>
    /// <returns>Collapse mask — bits 0..(blocks-1).</returns>
    public static uint QuantBand(
        ref BandContext ctx,
        ref OpusRangeDecoder dec,
        Span<float> X,
        int N,
        int b,
        int blocks,
        Span<float> lowband,
        int LM,
        Span<float> lowbandOut,
        float gain,
        Span<float> lowbandScratch,
        int fill)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        if (X.Length < N) throw new System.ArgumentException("X must hold at least N samples.", nameof(X));
        System.ArgumentOutOfRangeException.ThrowIfLessThan(blocks, 1);

        if (N == 1)
        {
            return CeltSplit.QuantBandN1(ref dec, ref ctx.RemainingBits, X, default, lowbandOut);
        }

        int N0 = N;
        int NB = N / blocks;
        int B0 = blocks;
        int B = blocks;
        bool longBlocks = B0 == 1;
        int timeDivide = 0;
        int recombine = 0;

        int tfChange = ctx.TfChange;
        if (tfChange > 0) recombine = tfChange;

        // Optionally route the lowband through a scratch buffer so we
        // can safely apply Haar1 / deinterleave_hadamard without
        // mutating the caller's data.
        bool needScratch = !lowbandScratch.IsEmpty && !lowband.IsEmpty
            && (recombine != 0 || ((NB & 1) == 0 && tfChange < 0) || B0 > 1);
        if (needScratch)
        {
            lowband.Slice(0, N).CopyTo(lowbandScratch);
            lowband = lowbandScratch.Slice(0, N);
        }

        // Recombine — bit-interleave for tf>0.
        for (int k = 0; k < recombine; k++)
        {
            if (!lowband.IsEmpty) CeltShape.Haar1(lowband, N >> k, 1 << k);
            fill = BitInterleaveTable[fill & 0xF] | (BitInterleaveTable[(fill >> 4) & 0xF] << 2);
        }
        B >>= recombine;
        NB <<= recombine;

        // Time-divide — increase time resolution when tf<0.
        while ((NB & 1) == 0 && tfChange < 0)
        {
            if (!lowband.IsEmpty) CeltShape.Haar1(lowband, NB, B);
            fill |= fill << B;
            B <<= 1;
            NB >>= 1;
            timeDivide++;
            tfChange++;
        }
        B0 = B;
        int NB0 = NB;

        // Reorganise lowband from frequency- to time-order so the
        // partition decoder sees the same layout libopus does.
        if (B0 > 1 && !lowband.IsEmpty)
        {
            CeltShape.DeinterleaveHadamard(lowband, NB >> recombine, B0 << recombine, longBlocks);
        }

        uint cm = QuantPartition(ref ctx, ref dec, X, N, b, B, lowband, LM, gain, fill);

        // ---- Resynthesis: undo every transform we applied. ----

        // Time-order → frequency-order.
        if (B0 > 1)
        {
            CeltShape.InterleaveHadamard(X, NB >> recombine, B0 << recombine, longBlocks);
        }

        // Undo time-divide: each step halves B, doubles N_B, copies the
        // collapse mask down, and applies inverse Haar1.
        NB = NB0;
        B = B0;
        for (int k = 0; k < timeDivide; k++)
        {
            B >>= 1;
            NB <<= 1;
            cm |= cm >> B;
            CeltShape.Haar1(X, NB, B);
        }

        // Undo recombine: deinterleave the collapse mask and apply
        // inverse Haar1 at the original strides.
        for (int k = 0; k < recombine; k++)
        {
            cm = BitDeinterleaveTable[(int)(cm & 0x0Fu)];
            CeltShape.Haar1(X, N0 >> k, 1 << k);
        }
        B <<= recombine;

        // Produce the √N-scaled lowband copy the next band will fold against.
        if (!lowbandOut.IsEmpty)
        {
            float n = System.MathF.Sqrt(N0);
            for (int j = 0; j < N0; j++) lowbandOut[j] = n * X[j];
        }

        // Clamp the collapse mask back into the per-block range.
        cm &= (uint)((1 << B) - 1);
        return cm;
    }

    /// <summary>
    /// Re-derives left / right channel norms from a decoded mid/side
    /// pair. <paramref name="X"/> holds the unit-norm mid (in Q14 float
    /// form, ‖X‖ = 1) before the call; <paramref name="Y"/> holds the
    /// unit-norm side. After the call X is the left and Y the right
    /// channel, both scaled so their joint energy matches what an
    /// inverse rotation by <paramref name="mid"/> would produce. Float-
    /// build port of libopus <c>stereo_merge</c>. When either output
    /// channel's reconstructed energy drops below 6e-4, the side
    /// component is silenced by copying X to Y (matches the libopus
    /// near-zero clamp).
    /// </summary>
    /// <param name="X">Mid in, left out (length ≥ N).</param>
    /// <param name="Y">Side in, right out (length ≥ N).</param>
    /// <param name="mid">Mid scaling factor (Q15 float, ≈cos θ).</param>
    /// <param name="N">Vector length.</param>
    public static void StereoMerge(Span<float> X, Span<float> Y, float mid, int N)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        if (X.Length < N) throw new System.ArgumentException("X must hold at least N samples.", nameof(X));
        if (Y.Length < N) throw new System.ArgumentException("Y must hold at least N samples.", nameof(Y));

        // Compute the norm of X+Y and X-Y as |X|² + |Y|² ± Σ XY.
        // In float build celt_inner_prod_norm_shift is just Σ x·y.
        float xp = 0f, side = 0f;
        for (int j = 0; j < N; j++)
        {
            xp += Y[j] * X[j];
            side += Y[j] * Y[j];
        }
        xp = mid * xp;
        float midSq = mid * mid;
        float El = midSq + side - 2f * xp;
        float Er = midSq + side + 2f * xp;

        const float minEnergy = 6e-4f;
        if (Er < minEnergy || El < minEnergy)
        {
            X.Slice(0, N).CopyTo(Y);
            return;
        }

        float lgain = 1f / System.MathF.Sqrt(El);
        float rgain = 1f / System.MathF.Sqrt(Er);
        for (int j = 0; j < N; j++)
        {
            float l = mid * X[j];
            float r = Y[j];
            X[j] = lgain * (l - r);
            Y[j] = rgain * (l + r);
        }
    }

    /// <summary>
    /// Decoder-side stereo band wrapper. Decodes the energy / angle
    /// split shared by both channels via
    /// <see cref="CeltSplit.ComputeTheta"/>, recurses into the mono
    /// <see cref="QuantBand"/> twice (mid first or side first depending
    /// on the bit balance), then runs the
    /// <see cref="StereoMerge"/> inversion to produce left/right
    /// outputs. Handles the <c>N==1</c> sign-bit special case
    /// (<see cref="CeltSplit.QuantBandN1"/>) and the <c>N==2</c>
    /// single-bit-sign special case that exploits mid/side
    /// orthogonality at width 2. Mirrors libopus
    /// <c>quant_band_stereo</c> (decoder branch, float build).
    /// </summary>
    /// <param name="ctx">Mutable per-band state.</param>
    /// <param name="dec">Range decoder.</param>
    /// <param name="X">Left channel output (length ≥ N).</param>
    /// <param name="Y">Right channel output (length ≥ N).</param>
    /// <param name="N">Partition size in samples.</param>
    /// <param name="b">Bit budget (1/8-bit units).</param>
    /// <param name="blocks">MDCT block count.</param>
    /// <param name="lowband">Lowband fold source (empty = null).</param>
    /// <param name="LM">Log-2 of frame size.</param>
    /// <param name="lowbandOut">√N-scaled output for the next band, or empty.</param>
    /// <param name="lowbandScratch">Optional lowband scratch (length ≥ N).</param>
    /// <param name="fill">Per-block fill mask.</param>
    /// <returns>Combined collapse mask for X and Y.</returns>
    public static uint QuantBandStereo(
        ref BandContext ctx,
        ref OpusRangeDecoder dec,
        Span<float> X,
        Span<float> Y,
        int N,
        int b,
        int blocks,
        Span<float> lowband,
        int LM,
        Span<float> lowbandOut,
        Span<float> lowbandScratch,
        int fill)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        if (X.Length < N) throw new System.ArgumentException("X must hold at least N samples.", nameof(X));
        if (Y.Length < N) throw new System.ArgumentException("Y must hold at least N samples.", nameof(Y));
        System.ArgumentOutOfRangeException.ThrowIfLessThan(blocks, 1);

        if (N == 1)
        {
            return CeltSplit.QuantBandN1(ref dec, ref ctx.RemainingBits, X, Y, lowbandOut);
        }

        int origFill = fill;

        CeltSplit.ComputeTheta(
            ref dec,
            logNAtBand: CeltConstants.LogN400[ctx.Band],
            bandIndex: ctx.Band,
            intensity: ctx.Intensity,
            n: N,
            b: ref b,
            blocks: blocks,
            blocks0: blocks,
            LM: LM,
            stereo: true,
            fill: ref fill,
            disableInv: ctx.DisableInv,
            remainingBits: ctx.RemainingBits,
            sctx: out CeltSplit.BandSplitContext sctx);
        int inv = sctx.Inv;
        int imid = sctx.IMid;
        int iside = sctx.ISide;
        int delta = sctx.Delta;
        int itheta = sctx.ITheta;
        int qalloc = sctx.QAlloc;

        // Float-build mid / side coefficients (Q15 → float).
        float mid = imid * (1f / 32768f);
        float side = iside * (1f / 32768f);

        uint cm;
        int mbits, sbits;
        if (N == 2)
        {
            // N==2 special case: exploit mid/side orthogonality. Side
            // takes at most one bit (just a sign), mid takes the rest.
            mbits = b;
            sbits = 0;
            if (itheta != 0 && itheta != 16384) sbits = 1 << CeltConstants.BitRes;
            mbits -= sbits;
            int c = itheta > 8192 ? 1 : 0;
            ctx.RemainingBits -= qalloc + sbits;

            Span<float> x2 = c != 0 ? Y : X;
            Span<float> y2 = c != 0 ? X : Y;

            int sign = 0;
            if (sbits != 0) sign = (int)dec.DecodeBits(1);
            sign = 1 - 2 * sign;

            // Decode the mid (x2) as a normal mono band. Use orig_fill —
            // we want to fold the side too, but ComputeTheta may have
            // cleared the low bits when itheta hit 16384.
            cm = QuantBand(ref ctx, ref dec, x2, N, mbits, blocks, lowband, LM,
                lowbandOut, gain: 1f, lowbandScratch, origFill);
            // Side is the 90°-rotated mid (the only unit-norm vector
            // orthogonal to a 2-D unit vector, up to sign).
            y2[0] = -sign * x2[1];
            y2[1] = sign * x2[0];

            // Mid/side → L/R 2-point butterfly.
            X[0] *= mid; X[1] *= mid;
            Y[0] *= side; Y[1] *= side;
            float tmp0 = X[0];
            X[0] = tmp0 - Y[0];
            Y[0] = tmp0 + Y[0];
            float tmp1 = X[1];
            X[1] = tmp1 - Y[1];
            Y[1] = tmp1 + Y[1];
        }
        else
        {
            // Normal stereo split: divide bits between mid (X) and side
            // (Y) per the ComputeTheta delta, decode both with QuantBand,
            // and rebalance any leftover budget.
            mbits = System.Math.Max(0, System.Math.Min(b, (b - delta) / 2));
            sbits = b - mbits;
            ctx.RemainingBits -= qalloc;

            int rebalance = ctx.RemainingBits;
            if (mbits >= sbits)
            {
                // Mid gets gain 1.0 — we want the normalised mid for folding later.
                cm = QuantBand(ref ctx, ref dec, X, N, mbits, blocks, lowband, LM,
                    lowbandOut, gain: 1f, lowbandScratch, fill);
                rebalance = mbits - (rebalance - ctx.RemainingBits);
                if (rebalance > 3 << CeltConstants.BitRes && itheta != 0)
                    sbits += rebalance - (3 << CeltConstants.BitRes);
                // For a stereo split, the high bits of fill are always
                // zero, so no folding will be done to the side.
                cm |= QuantBand(ref ctx, ref dec, Y, N, sbits, blocks, default, LM,
                    default, gain: side, lowbandScratch: default, fill >> blocks);
            }
            else
            {
                cm = QuantBand(ref ctx, ref dec, Y, N, sbits, blocks, default, LM,
                    default, gain: side, lowbandScratch: default, fill >> blocks);
                rebalance = sbits - (rebalance - ctx.RemainingBits);
                if (rebalance > 3 << CeltConstants.BitRes && itheta != 16384)
                    mbits += rebalance - (3 << CeltConstants.BitRes);
                cm |= QuantBand(ref ctx, ref dec, X, N, mbits, blocks, lowband, LM,
                    lowbandOut, gain: 1f, lowbandScratch, fill);
            }
        }

        // Resynth: re-derive L/R from mid/side. The N==2 path already
        // ran the inverse butterfly inline above.
        if (N != 2) StereoMerge(X, Y, mid, N);
        if (inv != 0)
        {
            for (int j = 0; j < N; j++) Y[j] = -Y[j];
        }
        return cm;
    }

    /// <summary>
    /// Duplicates enough of the first band's folding data into the
    /// second band's slot so the second band has something to fold
    /// from. Hybrid mode only — for CELT-only frames the band widths
    /// are such that <c>n2 ≤ n1</c> and the copy is a no-op. Mirrors
    /// libopus <c>special_hybrid_folding</c>.
    /// </summary>
    /// <param name="eBands">Band-edge table (length ≥ start+3).</param>
    /// <param name="norm">Per-band normalised buffer for X (or merged
    /// mid in dual-stereo). Indexed from 0 (caller already offset by
    /// <c>norm_offset</c>).</param>
    /// <param name="norm2">Per-band normalised buffer for Y when
    /// dual stereo is active, otherwise an empty span.</param>
    /// <param name="start">First CELT band.</param>
    /// <param name="M">Long-block multiplier (<c>1 &lt;&lt; LM</c>).</param>
    /// <param name="dualStereo">When true the parallel norm2 buffer
    /// also has its second-band slot duplicated.</param>
    public static void SpecialHybridFolding(
        ReadOnlySpan<short> eBands,
        Span<float> norm,
        Span<float> norm2,
        int start,
        int M,
        bool dualStereo)
    {
        int n1 = M * (eBands[start + 1] - eBands[start]);
        int n2 = M * (eBands[start + 2] - eBands[start + 1]);
        int extra = n2 - n1;
        if (extra <= 0) return;  // CELT-only: nothing to copy.

        // Duplicate norm[2*n1 - n2 .. 2*n1 - n2 + extra) → norm[n1 .. n1 + extra).
        int srcStart = 2 * n1 - n2;
        norm.Slice(srcStart, extra).CopyTo(norm.Slice(n1, extra));
        if (dualStereo && !norm2.IsEmpty)
            norm2.Slice(srcStart, extra).CopyTo(norm2.Slice(n1, extra));
    }

    /// <summary>
    /// Decoder-side band-iteration driver. Walks bands
    /// <c>start..end</c>, dispatches each band to
    /// <see cref="QuantBand"/> (mono / dual-stereo) or
    /// <see cref="QuantBandStereo"/> (joint stereo), threads
    /// <paramref name="balance"/> through the per-band budget
    /// calculation, threads <see cref="BandContext.Seed"/> through the
    /// LCG fold path, and accumulates a conservative collapse mask
    /// from prior bands for fold-source seeding. Mirrors libopus
    /// <c>quant_all_bands</c> — decoder branch only, no QEXT, no
    /// theta_rdo, no encoder resynth paths.
    /// </summary>
    /// <param name="dec">Range decoder.</param>
    /// <param name="eBands">Band-edge table (≥ end+1 entries).</param>
    /// <param name="start">First band index.</param>
    /// <param name="end">One past last band index.</param>
    /// <param name="X">L-channel normalised output, length ≥ M·eBands[end].</param>
    /// <param name="Y">R-channel normalised output (empty for mono), length ≥ M·eBands[end] when non-empty.</param>
    /// <param name="collapseMasks">Per-band per-channel collapse mask
    /// output. Length ≥ <c>end·C</c> where C is 2 when Y is non-empty
    /// else 1. Indexed as <c>collapseMasks[i*C + c]</c>.</param>
    /// <param name="pulses">Per-band PVQ pulse budget in 1/8-bit units.</param>
    /// <param name="shortBlocks">True for transient frames (band split
    /// into M short blocks); false for one long block per band.</param>
    /// <param name="spread">Spread mode 0..3.</param>
    /// <param name="dualStereo">True when stereo is encoded as two
    /// independent mono streams (allocator output). Switched off when
    /// the loop crosses the <paramref name="intensity"/> threshold.</param>
    /// <param name="intensity">Intensity-stereo cut-off band.</param>
    /// <param name="tfRes">Per-band tf change array (length ≥ end).</param>
    /// <param name="totalBits">Total bit budget in 1/8-bit units
    /// (<c>ec_total_bits &lt;&lt; BitRes</c>).</param>
    /// <param name="balance">Running balance from the allocator.</param>
    /// <param name="LM">Log-2 of MDCT block size.</param>
    /// <param name="codedBands">Number of bands receiving non-zero
    /// allocation (allocator output).</param>
    /// <param name="seed">LCG seed in/out — updated as the no-pulse
    /// fold path consumes random numbers.</param>
    /// <param name="disableInv">True when the bitstream disables the
    /// qn==1 stereo inversion bit.</param>
    /// <param name="normWorkspace">Scratch buffer for per-channel
    /// normalised bands. Must hold ≥
    /// <c>C * (M·eBands[end-1] − M·eBands[start])</c> floats; sized for
    /// the worst case the recursion may fold from.</param>
    public static void QuantAllBands(
        ref OpusRangeDecoder dec,
        ReadOnlySpan<short> eBands,
        int start,
        int end,
        Span<float> X,
        Span<float> Y,
        Span<byte> collapseMasks,
        ReadOnlySpan<int> pulses,
        bool shortBlocks,
        int spread,
        bool dualStereo,
        int intensity,
        ReadOnlySpan<sbyte> tfRes,
        int totalBits,
        int balance,
        int LM,
        int codedBands,
        ref uint seed,
        bool disableInv,
        Span<float> normWorkspace)
    {
        System.ArgumentOutOfRangeException.ThrowIfNegative(start);
        System.ArgumentOutOfRangeException.ThrowIfLessThan(end, start + 1);
        System.ArgumentOutOfRangeException.ThrowIfLessThan(LM, 0);
        System.ArgumentOutOfRangeException.ThrowIfGreaterThan(LM, 3);

        int M = 1 << LM;
        int B = shortBlocks ? M : 1;
        int normOffset = M * eBands[start];
        int channels = !Y.IsEmpty ? 2 : 1;
        int normPerChannel = M * eBands[end - 1] - normOffset;
        // Workspace layout: [norm | norm2] each of length normPerChannel.
        if (normWorkspace.Length < channels * normPerChannel)
            throw new System.ArgumentException(
                $"normWorkspace must hold at least {channels * normPerChannel} samples.",
                nameof(normWorkspace));
        normWorkspace.Slice(0, channels * normPerChannel).Clear();
        Span<float> norm = normWorkspace.Slice(0, normPerChannel);
        Span<float> norm2 = channels == 2
            ? normWorkspace.Slice(normPerChannel, normPerChannel)
            : default;
        // Use the tail of X as scratch (libopus decoder trick — that
        // region won't be touched until we decode the last band).
        Span<float> lowbandScratchBase = X.Slice(M * eBands[end - 1]);

        var ctx = new BandContext
        {
            Intensity = intensity,
            Spread = spread,
            DisableInv = disableInv,
            Seed = seed,
        };

        int lowbandOffset = 0;
        bool updateLowband = true;
        bool currentDualStereo = dualStereo;

        for (int i = start; i < end; i++)
        {
            bool last = i == end - 1;
            int N = M * (eBands[i + 1] - eBands[i]);
            int tell = (int)dec.TellFrac();

            if (i != start) balance -= tell;
            int remainingBits = totalBits - tell - 1;
            ctx.RemainingBits = remainingBits;

            int b;
            if (i <= codedBands - 1)
            {
                int currBalance = balance / System.Math.Min(3, codedBands - i);
                b = System.Math.Max(0,
                    System.Math.Min(16383,
                        System.Math.Min(remainingBits + 1, pulses[i] + currBalance)));
            }
            else
            {
                b = 0;
            }

            // Update folding source (DISABLE_UPDATE_DRAFT branch from libopus).
            if ((M * eBands[i] - N >= M * eBands[start] || i == start + 1)
                && (updateLowband || lowbandOffset == 0))
            {
                lowbandOffset = i;
            }
            if (i == start + 1)
                SpecialHybridFolding(eBands, norm, norm2, start, M, currentDualStereo);

            int tfChange = tfRes[i];
            ctx.Band = i;
            ctx.TfChange = tfChange;

            Span<float> bandX = X.Slice(M * eBands[i], N);
            Span<float> bandY = !Y.IsEmpty ? Y.Slice(M * eBands[i], N) : default;
            Span<float> scratch = last ? default : lowbandScratchBase;

            // Compute conservative collapse masks for fold-source seeding.
            int effectiveLowband = -1;
            uint xCm, yCm;
            if (lowbandOffset != 0 && (spread != CeltConstants.SpreadAggressive || B > 1 || tfChange < 0))
            {
                effectiveLowband = System.Math.Max(0, M * eBands[lowbandOffset] - normOffset - N);
                int foldStart = lowbandOffset;
                while (M * eBands[--foldStart] > effectiveLowband + normOffset) { }
                int foldEnd = lowbandOffset - 1;
                while (++foldEnd < i && M * eBands[foldEnd] < effectiveLowband + normOffset + N) { }
                xCm = 0; yCm = 0;
                int foldI = foldStart;
                do
                {
                    xCm |= collapseMasks[foldI * channels + 0];
                    yCm |= collapseMasks[foldI * channels + channels - 1];
                } while (++foldI < foldEnd);
            }
            else
            {
                xCm = yCm = (uint)((1 << B) - 1);
            }

            // Cross intensity threshold: merge norm2 into norm and stop dual stereo.
            if (currentDualStereo && i == intensity)
            {
                currentDualStereo = false;
                int merged = M * eBands[i] - normOffset;
                for (int j = 0; j < merged; j++)
                    norm[j] = 0.5f * (norm[j] + norm2[j]);
            }

            // Lowband source slice (norm/norm2 at effectiveLowband, N samples).
            Span<float> lowbandX = effectiveLowband >= 0 ? norm.Slice(effectiveLowband, N) : default;
            Span<float> lowbandY = effectiveLowband >= 0 && !norm2.IsEmpty ? norm2.Slice(effectiveLowband, N) : default;

            // Lowband out slice — writes √N · X[..] into norm at i's slot.
            int outOffset = M * eBands[i] - normOffset;
            Span<float> lowbandOutX = last ? default : norm.Slice(outOffset, N);
            Span<float> lowbandOutY = last || norm2.IsEmpty ? default : norm2.Slice(outOffset, N);

            uint xCmOut;
            uint yCmOut;
            if (currentDualStereo)
            {
                xCmOut = QuantBand(ref ctx, ref dec, bandX, N, b / 2, B,
                    lowbandX, LM, lowbandOutX, gain: 1f, scratch, (int)xCm);
                yCmOut = QuantBand(ref ctx, ref dec, bandY, N, b / 2, B,
                    lowbandY, LM, lowbandOutY, gain: 1f, scratch, (int)yCm);
            }
            else if (!bandY.IsEmpty)
            {
                xCmOut = QuantBandStereo(ref ctx, ref dec, bandX, bandY, N, b, B,
                    lowbandX, LM, lowbandOutX, scratch, (int)(xCm | yCm));
                yCmOut = xCmOut;
            }
            else
            {
                xCmOut = QuantBand(ref ctx, ref dec, bandX, N, b, B,
                    lowbandX, LM, lowbandOutX, gain: 1f, scratch, (int)(xCm | yCm));
                yCmOut = xCmOut;
            }

            collapseMasks[i * channels + 0] = (byte)xCmOut;
            collapseMasks[i * channels + channels - 1] = (byte)yCmOut;
            balance += pulses[i] + tell;
            updateLowband = b > (N << CeltConstants.BitRes);
        }
        seed = ctx.Seed;
    }

    /// <summary>
    /// Detect bands that collapsed to zero energy in any short MDCT block
    /// and inject pseudo-random noise so the post-IMDCT signal does not lose
    /// its high-frequency content. Mirrors libopus <c>anti_collapse</c> for
    /// the decoder + float build only (no encoder, no fixed-point).
    /// </summary>
    /// <param name="eBands">Critical-band boundary table (length end + 1).</param>
    /// <param name="X">Decoded normalised PVQ samples. Layout
    /// <c>X[c * size + (eBands[i] &lt;&lt; LM) .. + (N0 &lt;&lt; LM))</c>.</param>
    /// <param name="collapseMasks">Per-band collapse bitmask written by the
    /// PVQ shape decoder. One byte per band per channel; bit <c>k</c> is set
    /// when short block <c>k</c> received any non-zero pulse. Layout
    /// <c>collapseMasks[i * channels + c]</c>.</param>
    /// <param name="LM">Frame-size log2 multiplier (0..3).</param>
    /// <param name="channels">1 (mono) or 2 (stereo).</param>
    /// <param name="size">Per-channel stride into <paramref name="X"/>
    /// (must be at least <c>eBands[end] &lt;&lt; LM</c>).</param>
    /// <param name="start">First band processed by this frame.</param>
    /// <param name="end">One past last band processed by this frame.</param>
    /// <param name="logE">Current frame's coarse+fine log2 band energy, Q10,
    /// indexed <c>logE[c * stride + i]</c> where <c>stride</c> spans both
    /// channels (i.e. always 2 × stride entries).</param>
    /// <param name="prev1LogE">Previous frame's band energy (Q10, same
    /// layout as <paramref name="logE"/>).</param>
    /// <param name="prev2LogE">Frame-before-previous band energy (Q10).</param>
    /// <param name="logStride">Per-channel stride for <paramref name="logE"/>,
    /// <paramref name="prev1LogE"/> and <paramref name="prev2LogE"/>
    /// (libopus uses <c>nbEBands</c>; our decoder uses
    /// <see cref="CeltConstants.MaxBands"/>).</param>
    /// <param name="pulses">Per-band pulse count (libopus <c>pulses[i]</c>)
    /// in 1/1 pulse units, length at least <paramref name="end"/>.</param>
    /// <param name="seed">LCG state for the noise generator (libopus
    /// <c>dec->rng</c>). Updated in place.</param>
    public static void AntiCollapse(
        ReadOnlySpan<short> eBands,
        Span<float> X,
        ReadOnlySpan<byte> collapseMasks,
        int LM,
        int channels,
        int size,
        int start,
        int end,
        ReadOnlySpan<float> logE,
        ReadOnlySpan<float> prev1LogE,
        ReadOnlySpan<float> prev2LogE,
        int logStride,
        ReadOnlySpan<int> pulses,
        ref uint seed)
    {
        if (channels is not (1 or 2))
            throw new ArgumentOutOfRangeException(nameof(channels), channels, "Channels must be 1 or 2.");
        if (LM is < 0 or > 3)
            throw new ArgumentOutOfRangeException(nameof(LM), LM, "LM must be 0..3.");
        if (start < 0 || end < start)
            throw new ArgumentOutOfRangeException(nameof(end), end, "end must be >= start >= 0.");
        if (eBands.Length < end + 1)
            throw new ArgumentException("eBands must have at least end + 1 entries.", nameof(eBands));
        if (pulses.Length < end)
            throw new ArgumentException("pulses must have at least end entries.", nameof(pulses));
        if (collapseMasks.Length < end * channels)
            throw new ArgumentException("collapseMasks must have at least end * channels entries.", nameof(collapseMasks));
        if (logStride < end)
            throw new ArgumentOutOfRangeException(nameof(logStride), logStride, "logStride must be at least end.");
        int logMin = 2 * logStride;
        if (logE.Length < logMin || prev1LogE.Length < logMin || prev2LogE.Length < logMin)
            throw new ArgumentException("logE / prev1LogE / prev2LogE must each cover 2 * logStride entries.");
        if (size < (eBands[end] << LM))
            throw new ArgumentOutOfRangeException(nameof(size), size, "size must cover eBands[end] << LM.");
        if (X.Length < channels * size)
            throw new ArgumentException("X must hold channels * size samples.", nameof(X));

        for (int i = start; i < end; i++)
        {
            int N0 = eBands[i + 1] - eBands[i];

            // depth is in units of 1/8 bits per sample (libopus celt_udiv).
            int depth = ((1 + pulses[i]) / N0) >> LM;

            // thresh = 0.5 * 2^(-depth/8); float-build matches libopus.
            float thresh = 0.5f * MathF.Pow(2.0f, -0.125f * depth);

            int Nshift = N0 << LM;
            float sqrt1 = 1.0f / MathF.Sqrt(Nshift);

            for (int c = 0; c < channels; c++)
            {
                float prev1 = prev1LogE[c * logStride + i];
                float prev2 = prev2LogE[c * logStride + i];
                if (channels == 1)
                {
                    // Mono-decode safety: an up-mixed-from-stereo file may carry
                    // history for both channels. Take the louder side.
                    prev1 = MathF.Max(prev1, prev1LogE[logStride + i]);
                    prev2 = MathF.Max(prev2, prev2LogE[logStride + i]);
                }

                float Ediff = logE[c * logStride + i] - MathF.Min(prev1, prev2);
                if (Ediff < 0.0f) Ediff = 0.0f;

                // float build: r = 2 * 2^(-Ediff); for 20ms frames the extra
                // sqrt(2) compensates for the longer block having less energy
                // per short slot.
                float r = 2.0f * MathF.Pow(2.0f, -Ediff);
                if (LM == 3)
                    r *= 1.41421356f;
                if (r > thresh) r = thresh;
                r *= sqrt1;

                Span<float> Xc = X.Slice(c * size + (eBands[i] << LM), Nshift);
                byte mask = collapseMasks[i * channels + c];
                bool renormalize = false;

                int blocks = 1 << LM;
                for (int k = 0; k < blocks; k++)
                {
                    if ((mask & (1 << k)) != 0)
                        continue;

                    // Block k is empty — fill its interleaved slots with ±r noise.
                    for (int j = 0; j < N0; j++)
                    {
                        seed = CeltShape.LcgRand(seed);
                        Xc[(j << LM) + k] = (seed & 0x8000u) != 0 ? r : -r;
                    }
                    renormalize = true;
                }

                if (renormalize)
                    CeltShape.RenormaliseVector(Xc, Nshift, 1.0f);
            }
        }
    }

    /// <summary>
    /// Spend any bits left in the range coder after PVQ + anti-collapse on
    /// extra fine-energy precision. Mirrors libopus
    /// <c>unquant_energy_finalise</c> for the decoder + float build.
    /// </summary>
    /// <param name="dec">Live range decoder.</param>
    /// <param name="oldLogE">Per-band energy (Q10) to refine. Layout
    /// <c>oldLogE[c * logStride + i]</c>.</param>
    /// <param name="fineQuant">Per-band fine-energy bit count already
    /// decoded (libopus <c>fine_quant[i]</c>).</param>
    /// <param name="finePriority">Per-band priority (0 first, 1 second).</param>
    /// <param name="start">First band processed by this frame.</param>
    /// <param name="end">One past last band processed by this frame.</param>
    /// <param name="channels">1 (mono) or 2 (stereo).</param>
    /// <param name="logStride">Per-channel stride for <paramref name="oldLogE"/>.</param>
    /// <param name="bitsLeft">Remaining bits available for finalise; consumed in place.</param>
    public static void UnquantEnergyFinalise(
        ref OpusRangeDecoder dec,
        Span<float> oldLogE,
        ReadOnlySpan<int> fineQuant,
        ReadOnlySpan<int> finePriority,
        int start,
        int end,
        int channels,
        int logStride,
        ref int bitsLeft)
    {
        if (channels is not (1 or 2))
            throw new ArgumentOutOfRangeException(nameof(channels), channels, "Channels must be 1 or 2.");
        if (start < 0 || end < start)
            throw new ArgumentOutOfRangeException(nameof(end), end, "end must be >= start >= 0.");
        if (logStride < end)
            throw new ArgumentOutOfRangeException(nameof(logStride), logStride, "logStride must be at least end.");
        if (fineQuant.Length < end || finePriority.Length < end)
            throw new ArgumentException("fineQuant / finePriority must each have at least end entries.");
        if (oldLogE.Length < channels * logStride)
            throw new ArgumentException("oldLogE must hold channels * logStride entries.", nameof(oldLogE));

        for (int prio = 0; prio < 2; prio++)
        {
            for (int i = start; i < end && bitsLeft >= channels; i++)
            {
                int eb = fineQuant[i];
                if (eb >= CeltConstants.MaxFineBits || finePriority[i] != prio)
                    continue;

                for (int c = 0; c < channels; c++)
                {
                    int q2 = (int)dec.DecodeBits(1);
                    // libopus float build:
                    //   offset = (q2 - 0.5) * (1 << (14 - eb - 1)) / 16384
                    //          = (q2 - 0.5) / (1 << (eb + 1))
                    // In our Q10 layout that becomes:
                    //   offset_q10 = (q2 - 0.5) * DbUnit / (1 << (eb + 1))
                    //              = (q2 == 1 ? +1 : -1) * (1 << (8 - eb))
                    // and 8 - eb is always >= 1 because eb < MaxFineBits == 8.
                    int magnitude = 1 << (8 - eb);
                    float offset = q2 == 1 ? magnitude : -magnitude;
                    oldLogE[c * logStride + i] += offset;
                    bitsLeft--;
                }
            }
        }
    }
}
