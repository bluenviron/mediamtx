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
}
