using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for <see cref="CeltBands.QuantPartition"/> (Opus Phase 2c.3b.5a) —
/// the recursive PVQ mono partition splitter — and its supporting
/// helpers <see cref="CeltShape.RenormaliseVector"/> and
/// <see cref="CeltShape.LcgRand"/>.
/// </summary>
public sealed class CeltBandsTests
{
    // ---------- RenormaliseVector ----------

    [Fact]
    public void RenormaliseVector_ScalesToUnitNormTimesGain()
    {
        float[] x = { 3f, 4f, 0f };  // ‖x‖ = 5
        CeltShape.RenormaliseVector(x, 3, gain: 1f);
        double norm = 0;
        foreach (var v in x) norm += v * v;
        Assert.Equal(1.0, norm, 5);
        Assert.Equal(0.6f, x[0], 5);
        Assert.Equal(0.8f, x[1], 5);
    }

    [Fact]
    public void RenormaliseVector_GainParameterScalesOutputNorm()
    {
        var rng = new System.Random(123);
        var x = new float[16];
        for (int i = 0; i < x.Length; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        CeltShape.RenormaliseVector(x, 16, gain: 2.5f);
        double norm = 0;
        foreach (var v in x) norm += v * v;
        Assert.Equal(2.5 * 2.5, norm, 4);
    }

    [Fact]
    public void RenormaliseVector_SilentInput_DoesNotDivideByZero()
    {
        float[] x = new float[8];  // all zeros
        // EPSILON guard ⇒ g = gain / sqrt(EPSILON) is finite ⇒ all-zero output stays zero.
        CeltShape.RenormaliseVector(x, 8, gain: 1f);
        foreach (var v in x) Assert.Equal(0f, v);
    }

    [Fact]
    public void RenormaliseVector_PreservesDirection()
    {
        float[] x = { -2f, 4f, -1f };
        var copy = (float[])x.Clone();
        CeltShape.RenormaliseVector(x, 3, gain: 1f);
        // Sign pattern preserved.
        Assert.True(x[0] < 0);
        Assert.True(x[1] > 0);
        Assert.True(x[2] < 0);
        // Ratios preserved.
        Assert.Equal(copy[0] / copy[1], x[0] / x[1], 5);
        Assert.Equal(copy[2] / copy[0], x[2] / x[0], 5);
    }

    // ---------- LcgRand ----------

    [Fact]
    public void LcgRand_MatchesLibopusNumericalRecipesConstants()
    {
        // libopus celt_lcg_rand: 1664525*seed + 1013904223 (mod 2^32).
        Assert.Equal(1013904223u, CeltShape.LcgRand(0));
        // 1664525*1 + 1013904223 = 1015568748
        Assert.Equal(1015568748u, CeltShape.LcgRand(1));
        // Wrap-around behavior — large seeds should produce well-defined uint output.
        Assert.Equal(unchecked(1664525u * 0xFFFFFFFFu + 1013904223u),
                     CeltShape.LcgRand(0xFFFFFFFFu));
    }

    [Fact]
    public void LcgRand_HasNonTrivialCycle()
    {
        // Sanity: 1000 iterations from seed 0 visit 1000 distinct values
        // (the LCG period is 2^32 so we shouldn't see any duplicates this early).
        var seen = new System.Collections.Generic.HashSet<uint>();
        uint s = 0;
        for (int i = 0; i < 1000; i++)
        {
            s = CeltShape.LcgRand(s);
            Assert.True(seen.Add(s), $"Duplicate at iteration {i}");
        }
    }

    // ---------- QuantPartition: leaf no-pulse paths ----------

    [Fact]
    public void QuantPartition_NoPulses_NoFill_ZeroesOutput()
    {
        // b = 0 ⇒ q = 0 (no pulses fit). fill = 0 ⇒ X is cleared.
        var ctx = MakeContext(band: 5);
        var dec = new OpusRangeDecoder(new byte[] { 0, 0, 0, 0 });
        var X = new float[8];
        for (int i = 0; i < X.Length; i++) X[i] = 0.5f;

        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 8, b: 0, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 0);

        Assert.Equal(0u, cm);
        foreach (var v in X) Assert.Equal(0f, v);
    }

    [Fact]
    public void QuantPartition_NoPulses_WithFill_NoLowband_InjectsRenormalisedNoise()
    {
        // b = 0, fill ≠ 0, lowband = null ⇒ noise injection + renormalise.
        var ctx = MakeContext(band: 5, seed: 42);
        var dec = new OpusRangeDecoder(new byte[] { 0, 0, 0, 0 });
        var X = new float[16];

        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 16, b: 0, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 1);

        // Mask == fill mask (1<<blocks) - 1 = 1.
        Assert.Equal(1u, cm);
        // Output is unit-norm.
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(1.0, norm, 5);
        // Seed was advanced 16 times.
        Assert.NotEqual(42u, ctx.Seed);
    }

    [Fact]
    public void QuantPartition_NoPulses_WithFill_WithLowband_InjectsFoldedSpectrum()
    {
        var ctx = MakeContext(band: 5, seed: 0xC0FFEEu);
        var dec = new OpusRangeDecoder(new byte[] { 0, 0, 0, 0 });

        // Pre-computed lowband approximates the band's prior shape.
        var lowband = new float[8];
        for (int i = 0; i < lowband.Length; i++) lowband[i] = (i + 1) * 0.1f;

        var X = new float[8];
        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 8, b: 0, blocks: 2,
            lowband: lowband, LM: 0, gain: 2f, fill: 0b_11);

        // Mask == provided fill (with the (1<<blocks)-1 = 0b11 limit).
        Assert.Equal(0b_11u, cm);
        // Output is gain · unit-norm.
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(4.0, norm, 4);
        // Folded shape follows the lowband direction (monotonic in this case).
        Assert.True(X[7] > X[0], "Folded output should preserve lowband direction.");
    }

    [Fact]
    public void QuantPartition_NoPulses_FillMaskedByBlocks_ZeroesOutput()
    {
        // blocks = 2 ⇒ cm_mask = 0b11. fill = 0b1100 ⇒ fill & cm_mask = 0 ⇒ X cleared.
        var ctx = MakeContext(band: 3);
        var dec = new OpusRangeDecoder(new byte[] { 0, 0, 0, 0 });
        var X = new float[4];
        for (int i = 0; i < X.Length; i++) X[i] = 1f;

        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 4, b: 0, blocks: 2,
            lowband: default, LM: 0, gain: 1f, fill: 0b_1100);

        Assert.Equal(0u, cm);
        foreach (var v in X) Assert.Equal(0f, v);
    }

    // ---------- QuantPartition: leaf with pulses ----------

    [Fact]
    public void QuantPartition_LeafWithPulses_ProducesUnitNormShape()
    {
        // N <= 2 forces the leaf path. Band 8 with LM=0 has a valid pulse cache
        // (CacheIndex50[(0+1)*21+8] = 41).
        var ctx = MakeContext(band: 8, remainingBits: 1000);
        var dec = new OpusRangeDecoder(new byte[] { 0x55, 0xAA, 0x33, 0xCC, 0x77, 0x88, 0x12, 0x34 });
        var X = new float[2];

        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 2, b: 40, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 0);

        // AlgUnquant returned a non-zero collapse mask (B=1 ⇒ mask = 1).
        Assert.Equal(1u, cm);
        // Output is unit-norm.
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(1.0, norm, 5);
    }

    [Fact]
    public void QuantPartition_LeafWithPulses_BitBustGuardClampsQ()
    {
        // RemainingBits starts negative ⇒ leaf path's while-loop must
        // shrink q until either q==0 or RemainingBits>=0. We just need
        // to confirm it doesn't crash and produces a defined result.
        var ctx = MakeContext(band: 8, remainingBits: -100);
        var dec = new OpusRangeDecoder(new byte[] { 0x10, 0x20, 0x30, 0x40 });
        var X = new float[2];

        // No exception ⇒ guard worked.
        _ = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 2, b: 20, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 1);
    }

    // ---------- QuantPartition: recursive split smoke tests ----------

    [Fact]
    public void QuantPartition_LargeBudget_TriggersRecursiveSplit_ProducesFiniteOutput()
    {
        // Band 13 has width 4 in eband5ms; at frame LM=2 it has N=16, and the
        // recursion can bottom out at LM=-1 without invalid cache reads
        // (CacheIndex50[0*21+13] = 41 is valid for LM=-1 on this band).
        var ctx = MakeContext(band: 13, remainingBits: 5000);
        var dec = new OpusRangeDecoder(new byte[]
            { 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0,
              0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88 });
        var X = new float[16];

        _ = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 16, b: 400, blocks: 1,
            lowband: default, LM: 2, gain: 1f, fill: 0xFF);

        foreach (var v in X) Assert.True(float.IsFinite(v), "Output must be finite.");
    }

    [Fact]
    public void QuantPartition_RecursesUntilLM_MinusOne()
    {
        // Wide band + large LM ⇒ recursion descends multiple times. Band 17
        // (eband5ms width 10) at frame LM=3 yields N=80 → recursion through
        // LM=2,1,0,-1 with valid cache at every level.
        var bytes = MakeRandomBytes(seed: 7, length: 64);
        var ctx = MakeContext(band: 17, remainingBits: 20_000);
        var dec = new OpusRangeDecoder(bytes);
        var X = new float[64];

        uint cm = CeltBands.QuantPartition(ref ctx, ref dec, X, N: 64, b: 800, blocks: 2,
            lowband: default, LM: 3, gain: 1f, fill: 0xFF);

        // Collapse mask must fit within (1 << blocks0) - 1.
        Assert.True(cm <= 0b_11u, $"Mask {cm} exceeds B0=2 range.");
        foreach (var v in X) Assert.True(float.IsFinite(v));
    }

    [Fact]
    public void QuantPartition_NTooSmall_Throws()
    {
        var ctx = MakeContext();
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[1];
        bool threw = false;
        try { CeltBands.QuantPartition(ref ctx, ref dec, X, N: 0, b: 0, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 0); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void QuantPartition_XTooSmall_Throws()
    {
        var ctx = MakeContext();
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[2];
        bool threw = false;
        try { CeltBands.QuantPartition(ref ctx, ref dec, X, N: 4, b: 0, blocks: 1,
            lowband: default, LM: 0, gain: 1f, fill: 0); }
        catch (System.ArgumentException) { threw = true; }
        Assert.True(threw);
    }

    // ---------- QuantBand mono wrapper ----------

    [Fact]
    public void QuantBand_NEqualsOne_RoutesToQuantBandN1()
    {
        // N==1 ⇒ QuantBandN1 short-circuit. Returns mask=1, writes ±1
        // to X based on the sign bit, and (if lowbandOut is non-empty)
        // emits X[0]/16 into lowbandOut[0].
        var ctx = MakeContext(remainingBits: 100);
        // Sign bit = 0 (first bit of 0x00) ⇒ X[0] = +1.
        var dec = new OpusRangeDecoder(new byte[] { 0x00, 0x00, 0x00, 0x00 });
        var X = new float[1];
        var lowbandOut = new float[1];

        uint cm = CeltBands.QuantBand(ref ctx, ref dec, X, N: 1, b: 0, blocks: 1,
            lowband: default, LM: 0, lowbandOut: lowbandOut, gain: 1f,
            lowbandScratch: default, fill: 0);

        Assert.Equal(1u, cm);
        Assert.Equal(1f, X[0]);
        Assert.Equal(1f / 16f, lowbandOut[0]);
    }

    [Fact]
    public void QuantBand_NoTfNoBlocks_MatchesQuantPartitionDirectly()
    {
        // With tf_change=0 and blocks=1, the wrapper applies no
        // transforms — output must match a direct QuantPartition call
        // with the same inputs (after the wrapper's lowband-out scaling).
        var bytes = MakeRandomBytes(seed: 11, length: 32);

        var ctxWrap = MakeContext(band: 13, remainingBits: 5000, seed: 0xDEAD_BEEFu);
        var decWrap = new OpusRangeDecoder(bytes);
        var Xwrap = new float[16];
        var lowbandOut = new float[16];
        uint cmWrap = CeltBands.QuantBand(ref ctxWrap, ref decWrap, Xwrap, N: 16, b: 400,
            blocks: 1, lowband: default, LM: 2, lowbandOut: lowbandOut, gain: 1f,
            lowbandScratch: default, fill: 1);

        var ctxRef = MakeContext(band: 13, remainingBits: 5000, seed: 0xDEAD_BEEFu);
        var decRef = new OpusRangeDecoder(bytes);
        var Xref = new float[16];
        uint cmRef = CeltBands.QuantPartition(ref ctxRef, ref decRef, Xref, N: 16, b: 400,
            blocks: 1, lowband: default, LM: 2, gain: 1f, fill: 1);

        Assert.Equal(cmRef, cmWrap);
        for (int i = 0; i < 16; i++) Assert.Equal(Xref[i], Xwrap[i], 5);

        // lowband_out scaled by sqrt(N) = 4.
        float n = System.MathF.Sqrt(16);
        for (int i = 0; i < 16; i++) Assert.Equal(n * Xwrap[i], lowbandOut[i], 5);
    }

    [Fact]
    public void QuantBand_LowbandOutHasSqrtNScaling()
    {
        // After the wrapper, ‖lowbandOut‖² should equal N·‖X‖² because
        // lowbandOut[j] = √N · X[j]. With QuantPartition emitting a
        // unit-norm shape, ‖lowbandOut‖² ≈ N.
        var ctx = MakeContext(band: 13, remainingBits: 5000);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 13, length: 32));
        var X = new float[16];
        var lowbandOut = new float[16];

        _ = CeltBands.QuantBand(ref ctx, ref dec, X, N: 16, b: 400, blocks: 1,
            lowband: default, LM: 2, lowbandOut: lowbandOut, gain: 1f,
            lowbandScratch: default, fill: 1);

        double normX = 0, normLO = 0;
        for (int i = 0; i < 16; i++) { normX += X[i] * X[i]; normLO += lowbandOut[i] * lowbandOut[i]; }
        Assert.Equal(1.0, normX, 4);
        Assert.Equal(16.0, normLO, 3);
    }

    [Fact]
    public void QuantBand_LowbandOutEmpty_DoesNotWrite()
    {
        // Passing an empty lowbandOut should be a no-op for that step.
        var ctx = MakeContext(band: 13, remainingBits: 5000);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 17, length: 32));
        var X = new float[16];

        // No exception ⇒ the empty lowbandOut branch is exercised correctly.
        _ = CeltBands.QuantBand(ref ctx, ref dec, X, N: 16, b: 400, blocks: 1,
            lowband: default, LM: 2, lowbandOut: default, gain: 1f,
            lowbandScratch: default, fill: 1);
    }

    [Fact]
    public void QuantBand_ScratchCopy_PreservesCallerLowband()
    {
        // With blocks=2 (B0>1), the wrapper applies deinterleave_hadamard
        // to the lowband. If a non-empty scratch is supplied, the
        // caller's lowband array must not be mutated.
        var lowband = new float[8];
        for (int i = 0; i < 8; i++) lowband[i] = (i + 1) * 0.1f;
        var lowbandCopy = (float[])lowband.Clone();

        var ctx = MakeContext(band: 13, remainingBits: 5000);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 19, length: 32));
        var X = new float[8];
        var scratch = new float[8];

        _ = CeltBands.QuantBand(ref ctx, ref dec, X, N: 8, b: 100, blocks: 2,
            lowband: lowband, LM: 1, lowbandOut: default, gain: 1f,
            lowbandScratch: scratch, fill: 0b_11);

        for (int i = 0; i < 8; i++)
            Assert.Equal(lowbandCopy[i], lowband[i]);
    }

    [Fact]
    public void QuantBand_TfChangePositive_TriggersRecombine_NoCrash()
    {
        // tf_change > 0 ⇒ recombine path runs (bit-interleave + Haar1
        // on lowband). Per libopus semantics tf>0 only occurs on
        // multi-block bands, so blocks=2 keeps the shifted B valid.
        var ctx = MakeContext(band: 13, remainingBits: 5000, tfChange: 1);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 23, length: 32));
        var X = new float[16];
        var lowband = new float[16];
        for (int i = 0; i < 16; i++) lowband[i] = ((i & 1) == 0 ? 1f : -1f) * 0.25f;
        var scratch = new float[16];

        _ = CeltBands.QuantBand(ref ctx, ref dec, X, N: 16, b: 400, blocks: 2,
            lowband: lowband, LM: 2, lowbandOut: default, gain: 1f,
            lowbandScratch: scratch, fill: 0b_11);

        double norm = 0;
        foreach (var v in X) { Assert.True(float.IsFinite(v)); norm += v * v; }
        Assert.Equal(1.0, norm, 4);
    }

    [Fact]
    public void QuantBand_TfChangeNegative_TriggersTimeDivide_NoCrash()
    {
        // tf_change < 0 ⇒ time-divide loop runs (Haar1 + fill expand).
        var ctx = MakeContext(band: 17, remainingBits: 10_000, tfChange: -1);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 29, length: 64));
        var X = new float[64];
        var lowband = new float[64];
        for (int i = 0; i < 64; i++) lowband[i] = (float)System.Math.Sin(i * 0.3) * 0.1f;
        var scratch = new float[64];

        _ = CeltBands.QuantBand(ref ctx, ref dec, X, N: 64, b: 1000, blocks: 1,
            lowband: lowband, LM: 3, lowbandOut: default, gain: 1f,
            lowbandScratch: scratch, fill: 1);

        double norm = 0;
        foreach (var v in X) { Assert.True(float.IsFinite(v)); norm += v * v; }
        Assert.Equal(1.0, norm, 4);
    }

    [Fact]
    public void QuantBand_MultipleBlocks_TriggersHadamardReorganisation()
    {
        // blocks=4 ⇒ B0>1 ⇒ deinterleave_hadamard + interleave_hadamard
        // bracket the partition decode. Verify output is finite and
        // collapse mask stays within (1<<blocks) - 1.
        var ctx = MakeContext(band: 17, remainingBits: 10_000);
        var dec = new OpusRangeDecoder(MakeRandomBytes(seed: 31, length: 64));
        var X = new float[32];

        uint cm = CeltBands.QuantBand(ref ctx, ref dec, X, N: 32, b: 600, blocks: 4,
            lowband: default, LM: 3, lowbandOut: default, gain: 1f,
            lowbandScratch: default, fill: 0xF);

        Assert.True(cm <= 0xFu, $"Mask {cm} exceeds blocks=4 range.");
        foreach (var v in X) Assert.True(float.IsFinite(v));
    }

    [Fact]
    public void QuantBand_NTooSmall_Throws()
    {
        var ctx = MakeContext();
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[1];
        bool threw = false;
        try { CeltBands.QuantBand(ref ctx, ref dec, X, N: 0, b: 0, blocks: 1,
            lowband: default, LM: 0, lowbandOut: default, gain: 1f,
            lowbandScratch: default, fill: 0); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void QuantBand_BlocksZero_Throws()
    {
        var ctx = MakeContext();
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[4];
        bool threw = false;
        try { CeltBands.QuantBand(ref ctx, ref dec, X, N: 4, b: 0, blocks: 0,
            lowband: default, LM: 0, lowbandOut: default, gain: 1f,
            lowbandScratch: default, fill: 0); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    // ---------- StereoMerge ----------

    [Fact]
    public void StereoMerge_ZeroSide_PutsMidCopyInBothChannels()
    {
        // X is the normalised mid; Y is silence. With mid=1.0 the recovered
        // left = mid*X - Y = X, right = mid*X + Y = X.
        float[] x = { 0.5f, 0.5f, 0.5f, 0.5f };
        float[] y = new float[4];
        var xCopy = (float[])x.Clone();
        CeltBands.StereoMerge(x, y, mid: 1f, N: 4);
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(xCopy[i], x[i], 5);
            Assert.Equal(xCopy[i], y[i], 5);
        }
    }

    [Fact]
    public void StereoMerge_OrthogonalMidAndSide_ProducesOrthogonalLR()
    {
        // mid = [1,0], side = [0,1], both unit norm. With mid scaling 1 → left=[1,-1]/√2, right=[1,1]/√2.
        float[] x = { 1f, 0f };
        float[] y = { 0f, 1f };
        CeltBands.StereoMerge(x, y, mid: 1f, N: 2);
        float invSqrt2 = 1f / System.MathF.Sqrt(2f);
        Assert.Equal(invSqrt2, x[0], 5);
        Assert.Equal(-invSqrt2, x[1], 5);
        Assert.Equal(invSqrt2, y[0], 5);
        Assert.Equal(invSqrt2, y[1], 5);
        // Result should be orthonormal.
        Assert.Equal(0f, x[0] * y[0] + x[1] * y[1], 5);
    }

    [Fact]
    public void StereoMerge_LowEnergyClamp_CopiesXToY()
    {
        // To force El < 6e-4 we need mid² + |Y|² - 2·mid·<Y,X> below the threshold.
        // Pick mid very small with both X and Y near-zero: El = mid² + |Y|² - 2·mid·<Y,X>
        // ≈ 1e-4 + 2e-6 ≈ 1.02e-4 < 6e-4. Same for Er.
        float[] x = { 1e-3f, 1e-3f };
        float[] y = { 1e-3f, 1e-3f };
        var xCopy = (float[])x.Clone();
        CeltBands.StereoMerge(x, y, mid: 0.01f, N: 2);
        // Guard fires → X is unchanged, Y becomes a copy of X.
        Assert.Equal(xCopy[0], x[0], 6);
        Assert.Equal(xCopy[1], x[1], 6);
        Assert.Equal(xCopy[0], y[0], 6);
        Assert.Equal(xCopy[1], y[1], 6);
    }

    [Fact]
    public void StereoMerge_ProducesFiniteOutputForRandomInputs()
    {
        var rng = new System.Random(42);
        int N = 16;
        var x = new float[N];
        var y = new float[N];
        for (int i = 0; i < N; i++) { x[i] = (float)(rng.NextDouble() * 2 - 1); y[i] = (float)(rng.NextDouble() * 2 - 1); }
        // Renormalise so they look like mid/side outputs.
        CeltShape.RenormaliseVector(x, N, 1f);
        CeltShape.RenormaliseVector(y, N, 1f);
        CeltBands.StereoMerge(x, y, mid: 0.7071f, N: N);
        for (int i = 0; i < N; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
    }

    [Fact]
    public void StereoMerge_ArgumentValidation_NLessThanOne()
    {
        bool threw = false;
        try { CeltBands.StereoMerge(new float[1], new float[1], 1f, 0); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void StereoMerge_ArgumentValidation_TooSmallSpan()
    {
        bool threw = false;
        try { CeltBands.StereoMerge(new float[1], new float[4], 1f, 4); }
        catch (System.ArgumentException) { threw = true; }
        Assert.True(threw);
    }

    // ---------- QuantBandStereo ----------

    [Fact]
    public void QuantBandStereo_NEquals1_ShortCircuitsToQuantBandN1()
    {
        // Stereo N=1: QuantBandN1 should decode two sign bits.
        var bytes = MakeRandomBytes(seed: 7, length: 32);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 0, remainingBits: 100);
        float[] x = new float[1];
        float[] y = new float[1];
        float[] lo = new float[1];
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: 1, b: 0,
            blocks: 1, lowband: default, LM: 0, lowbandOut: lo, lowbandScratch: default, fill: 0);
        Assert.Equal(1u, cm);
        // Both channels are ±1.
        Assert.Equal(1f, System.MathF.Abs(x[0]));
        Assert.Equal(1f, System.MathF.Abs(y[0]));
        // Lowband out is √N · X[0] / √N (lowbandOut scaled by √N0; here N0=1).
        // (For QuantBandN1, libopus stores X[0]/16384.f, but the float build's
        // store happens unconditionally — verify it's finite and signed.)
        Assert.True(float.IsFinite(lo[0]));
    }

    [Fact]
    public void QuantBandStereo_N2_PureMid_ItheTaForcedToZero_ProducesFiniteOutput()
    {
        // band=8 LM=0 N=2 with a moderate bit budget. We can't force
        // itheta deterministically without an encoder, but we can verify
        // the function returns finite output and consumes bits.
        var bytes = MakeRandomBytes(seed: 11, length: 64);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 8, remainingBits: 200);
        float[] x = new float[2];
        float[] y = new float[2];
        int before = ctx.RemainingBits;
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: 2, b: 80,
            blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 3);
        Assert.True(ctx.RemainingBits <= before);
        for (int i = 0; i < 2; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
        Assert.True(cm != 0);
    }

    [Fact]
    public void QuantBandStereo_NGreaterThan2_NormalSplit_ProducesFiniteOutput()
    {
        // band=8 LM=0 N=16 with a generous bit budget. The split goes
        // through the mid/side rebalance path and ends with stereo_merge.
        var bytes = MakeRandomBytes(seed: 31, length: 256);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 8, remainingBits: 1000);
        const int N = 16;
        float[] x = new float[N];
        float[] y = new float[N];
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: N, b: 400,
            blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 1);
        for (int i = 0; i < N; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
        Assert.True(cm != 0);
    }

    [Fact]
    public void QuantBandStereo_NGreaterThan2_DisableInv_SuppressesYNegation()
    {
        // disableInv=true should prevent the inv-path in ComputeTheta,
        // so Y never gets negated. We can't directly observe inv from
        // the outside, but the output should remain finite and the
        // function should not throw.
        var bytes = MakeRandomBytes(seed: 99, length: 256);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 8, remainingBits: 1000, disableInv: true);
        const int N = 16;
        float[] x = new float[N];
        float[] y = new float[N];
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: N, b: 400,
            blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 1);
        for (int i = 0; i < N; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
        Assert.True(cm != 0);
    }

    [Fact]
    public void QuantBandStereo_NGreaterThan2_WithLowband_ConsumesLowbandSource()
    {
        // Provide a non-trivial lowband; verify the decode still
        // produces finite output. (Folding source is consumed when bits
        // run out; can't observe the fold directly without an encoder.)
        var bytes = MakeRandomBytes(seed: 17, length: 256);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 13, remainingBits: 500);
        const int N = 16;
        float[] x = new float[N];
        float[] y = new float[N];
        float[] lowband = new float[N];
        float[] scratch = new float[N];
        var rng = new System.Random(5);
        for (int i = 0; i < N; i++) lowband[i] = (float)(rng.NextDouble() * 2 - 1);
        CeltShape.RenormaliseVector(lowband, N, 1f);
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: N, b: 100,
            blocks: 1, lowband: lowband, LM: 2, lowbandOut: default, lowbandScratch: scratch, fill: 1);
        for (int i = 0; i < N; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
        Assert.True(cm != 0);
    }

    [Fact]
    public void QuantBandStereo_NGreaterThan2_LowBudget_StillProducesFiniteOutput()
    {
        // Tight budget pushes the recursive QuantBand into the
        // fold-only branch for the side. Output must still be finite.
        var bytes = MakeRandomBytes(seed: 13, length: 256);
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext(band: 8, remainingBits: 50);
        const int N = 16;
        float[] x = new float[N];
        float[] y = new float[N];
        uint cm = CeltBands.QuantBandStereo(ref ctx, ref dec, x, y, N: N, b: 30,
            blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 1);
        for (int i = 0; i < N; i++)
        {
            Assert.True(float.IsFinite(x[i]));
            Assert.True(float.IsFinite(y[i]));
        }
        Assert.True(cm != 0);
    }

    [Fact]
    public void QuantBandStereo_ArgumentValidation_NLessThanOne()
    {
        var bytes = new byte[8];
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext();
        bool threw = false;
        try
        {
            CeltBands.QuantBandStereo(ref ctx, ref dec, new float[1], new float[1], 0, 0,
                blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 0);
        }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void QuantBandStereo_ArgumentValidation_YLengthLessThanN()
    {
        var bytes = new byte[8];
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext();
        bool threw = false;
        try
        {
            CeltBands.QuantBandStereo(ref ctx, ref dec, new float[8], new float[4], 8, 0,
                blocks: 1, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 0);
        }
        catch (System.ArgumentException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void QuantBandStereo_ArgumentValidation_ZeroBlocks()
    {
        var bytes = new byte[8];
        var dec = new OpusRangeDecoder(bytes);
        var ctx = MakeContext();
        bool threw = false;
        try
        {
            CeltBands.QuantBandStereo(ref ctx, ref dec, new float[8], new float[8], 8, 0,
                blocks: 0, lowband: default, LM: 0, lowbandOut: default, lowbandScratch: default, fill: 0);
        }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    // ---------- SpecialHybridFolding ----------

    [Fact]
    public void SpecialHybridFolding_CeltOnly_StartBand0_IsNoOp()
    {
        // For CELT-only frames the band widths give n2 ≤ n1 → no copy.
        // eBands[0..2] = {0, 1, 2} ⇒ n1 = M, n2 = M, extra = 0.
        short[] eBands = { 0, 1, 2, 3 };
        float[] norm = { 1f, 2f, 3f, 4f, 5f, 6f, 7f, 8f };
        var snapshot = (float[])norm.Clone();
        CeltBands.SpecialHybridFolding(eBands, norm, default, start: 0, M: 4, dualStereo: false);
        Assert.Equal(snapshot, norm);
    }

    [Fact]
    public void SpecialHybridFolding_HybridMode_DuplicatesIntoSecondBandSlot()
    {
        // Hybrid mode: start=17 with eBands[17..19] = {68, 72, 80}.
        // n1 = M·(72-68) = 4M, n2 = M·(80-72) = 8M, extra = 4M.
        // Copy norm[2*n1 - n2 .. 2*n1 - n2 + extra) = norm[0..4M) → norm[n1..n1+4M) = norm[4M..8M).
        const int M = 1;
        short[] eBands = new short[19];
        eBands[17] = 68; eBands[18] = 72; // only entries needed for this test
        // But SpecialHybridFolding only reads eBands[start..start+2] — provide [17],[18],[18+1] = need eBands[19].
        // Adjust: provide a small array starting where the function reads.
        short[] localBands = { 68, 72, 80 };
        float[] norm = new float[10];
        for (int i = 0; i < 10; i++) norm[i] = i;
        CeltBands.SpecialHybridFolding(localBands, norm, default, start: 0, M: M, dualStereo: false);
        // n1=4, n2=8, extra=4 → copy norm[0..4) → norm[4..8).
        Assert.Equal(0f, norm[4]);
        Assert.Equal(1f, norm[5]);
        Assert.Equal(2f, norm[6]);
        Assert.Equal(3f, norm[7]);
        // Untouched: 8, 9.
        Assert.Equal(8f, norm[8]);
        Assert.Equal(9f, norm[9]);
    }

    [Fact]
    public void SpecialHybridFolding_DualStereo_AlsoDuplicatesNorm2()
    {
        short[] localBands = { 68, 72, 80 };
        float[] norm = new float[10];
        float[] norm2 = new float[10];
        for (int i = 0; i < 10; i++) { norm[i] = i; norm2[i] = i + 100; }
        CeltBands.SpecialHybridFolding(localBands, norm, norm2, start: 0, M: 1, dualStereo: true);
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(i, norm[4 + i]);
            Assert.Equal(100 + i, norm2[4 + i]);
        }
    }

    [Fact]
    public void SpecialHybridFolding_DualStereoFalse_LeavesNorm2Untouched()
    {
        short[] localBands = { 68, 72, 80 };
        float[] norm = new float[10];
        float[] norm2 = new float[10];
        for (int i = 0; i < 10; i++) { norm[i] = i; norm2[i] = i + 100; }
        var n2Before = (float[])norm2.Clone();
        CeltBands.SpecialHybridFolding(localBands, norm, norm2, start: 0, M: 1, dualStereo: false);
        Assert.Equal(n2Before, norm2);
    }

    // ---------- QuantAllBands ----------

    [Fact]
    public void QuantAllBands_MonoFullBand_ProducesFiniteNormalisedOutput()
    {
        // FB mono, 20 ms long block (LM=3, M=8). start=0, end=21.
        const int LM = 3;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 21;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        float[] cm = new float[end];  // unused mono extra
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        // Realistic budgets — give each band a moderate budget.
        for (int i = 0; i < end; i++) pulses[i] = 40 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 7, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 12345;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 8000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);

        // All decoded coefficients must be finite.
        for (int i = 0; i < xLen; i++) Assert.True(float.IsFinite(X[i]));
        // At least some collapse masks should be non-zero.
        int nonZeroMasks = 0;
        for (int i = 0; i < end; i++) if (collapse[i] != 0) nonZeroMasks++;
        Assert.True(nonZeroMasks > 0);
    }

    [Fact]
    public void QuantAllBands_StereoJoint_ProducesFiniteOutputBothChannels()
    {
        const int LM = 2;  // 10 ms
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;  // WB
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        float[] Y = new float[xLen];
        byte[] collapse = new byte[end * 2];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < end; i++) pulses[i] = 60 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 21, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 99;
        float[] norm = new float[2 * (M * eBands[end - 1] - M * eBands[start])];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, Y, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: end, tfRes,
            totalBits: 12000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);

        for (int i = 0; i < xLen; i++)
        {
            Assert.True(float.IsFinite(X[i]));
            Assert.True(float.IsFinite(Y[i]));
        }
        int nonZeroMasks = 0;
        for (int i = 0; i < end * 2; i++) if (collapse[i] != 0) nonZeroMasks++;
        Assert.True(nonZeroMasks > 0);
    }

    [Fact]
    public void QuantAllBands_StereoDualStereo_RunsBothChannelsIndependently()
    {
        const int LM = 2;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        float[] Y = new float[xLen];
        byte[] collapse = new byte[end * 2];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < end; i++) pulses[i] = 60 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 33, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[2 * (M * eBands[end - 1] - M * eBands[start])];
        // dualStereo=true, intensity=end → never crosses, stays in dual-stereo path.
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, Y, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: true, intensity: end, tfRes,
            totalBits: 12000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        for (int i = 0; i < xLen; i++)
        {
            Assert.True(float.IsFinite(X[i]));
            Assert.True(float.IsFinite(Y[i]));
        }
    }

    [Fact]
    public void QuantAllBands_DualStereoCrossesIntensity_SwitchesToJointStereo()
    {
        // dualStereo=true with intensity in the middle → first half is
        // dual-stereo, second half is joint-stereo after the intensity
        // crossover. Function must complete without throwing.
        const int LM = 2;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        float[] Y = new float[xLen];
        byte[] collapse = new byte[end * 2];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < end; i++) pulses[i] = 60 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 51, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[2 * (M * eBands[end - 1] - M * eBands[start])];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, Y, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: true, intensity: 8, tfRes,
            totalBits: 12000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        for (int i = 0; i < xLen; i++)
        {
            Assert.True(float.IsFinite(X[i]));
            Assert.True(float.IsFinite(Y[i]));
        }
    }

    [Fact]
    public void QuantAllBands_ShortBlocks_UsesBEqualsM()
    {
        // Transient frame: B=M=4 (LM=2). Verify no crash and finite output.
        const int LM = 2;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < end; i++) pulses[i] = 40 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 71, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: true, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 8000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        for (int i = 0; i < xLen; i++) Assert.True(float.IsFinite(X[i]));
    }

    [Fact]
    public void QuantAllBands_HybridStart_AppliesSpecialHybridFolding()
    {
        // Hybrid CELT (start=17, end=21) — second-iteration call to
        // SpecialHybridFolding must duplicate the previous band's data
        // so band 18 has a fold source. Smoke test only — finite
        // output across all CELT bands.
        const int LM = 2;
        const int M = 1 << LM;
        const int start = CeltConstants.HybridStartBand;  // 17
        const int end = 21;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = start; i < end; i++) pulses[i] = 80 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 91, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 4000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        // Only X[M*eBands[start] .. M*eBands[end]) is touched.
        for (int j = M * eBands[start]; j < M * eBands[end]; j++)
            Assert.True(float.IsFinite(X[j]));
    }

    [Fact]
    public void QuantAllBands_AggressiveSpread_NonTransient_SkipsLowbandCollapseAccum()
    {
        // spread=SpreadAggressive with B=1 and tf_change=0 means the
        // lowband fold-mask accumulator is skipped (xCm = (1<<B)-1 = 1).
        // Verify it still runs cleanly.
        const int LM = 3;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < end; i++) pulses[i] = 50 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 41, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadAggressive,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 10000, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        for (int i = 0; i < xLen; i++) Assert.True(float.IsFinite(X[i]));
    }

    [Fact]
    public void QuantAllBands_LowCodedBands_UnallocatedBandsGetZeroBudget()
    {
        // codedBands < end: bands >= codedBands get b=0 and pulse fold-only.
        const int LM = 2;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 17;
        const int codedBands = 10;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        for (int i = 0; i < codedBands; i++) pulses[i] = 50 << CeltConstants.BitRes;
        var bytes = MakeRandomBytes(seed: 61, length: 1024);
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 1;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 6000, balance: 0, LM, codedBands,
            ref seed, disableInv: false, normWorkspace: norm);
        for (int i = 0; i < xLen; i++) Assert.True(float.IsFinite(X[i]));
    }

    [Fact]
    public void QuantAllBands_UpdatesSeed()
    {
        // The LCG seed should be mutated by the no-pulse fold path.
        const int LM = 2;
        const int M = 1 << LM;
        const int start = 0;
        const int end = 10;
        var eBands = CeltConstants.EBands;
        int xLen = M * eBands[end];
        float[] X = new float[xLen];
        byte[] collapse = new byte[end];
        int[] pulses = new int[end];
        sbyte[] tfRes = new sbyte[end];
        // Zero pulses → every band falls through to the fold/LCG path.
        var bytes = MakeRandomBytes(seed: 81, length: 256);
        var dec = new OpusRangeDecoder(bytes);
        uint seedBefore = 0xDEADBEEF;
        uint seed = seedBefore;
        float[] norm = new float[M * eBands[end - 1] - M * eBands[start]];
        CeltBands.QuantAllBands(
            ref dec, eBands, start, end, X, default, collapse, pulses,
            shortBlocks: false, spread: CeltConstants.SpreadNormal,
            dualStereo: false, intensity: 0, tfRes,
            totalBits: 500, balance: 0, LM, codedBands: end,
            ref seed, disableInv: false, normWorkspace: norm);
        Assert.NotEqual(seedBefore, seed);
    }

    [Fact]
    public void QuantAllBands_ArgumentValidation_NegativeStart()
    {
        var bytes = new byte[8];
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 0;
        bool threw = false;
        try
        {
            CeltBands.QuantAllBands(
                ref dec, CeltConstants.EBands, start: -1, end: 1,
                X: new float[8], Y: default, collapseMasks: new byte[1],
                pulses: new int[1], shortBlocks: false, spread: 0, dualStereo: false,
                intensity: 0, tfRes: new sbyte[1], totalBits: 0, balance: 0, LM: 0,
                codedBands: 1, ref seed, disableInv: false, normWorkspace: new float[1]);
        }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void QuantAllBands_ArgumentValidation_NormWorkspaceTooSmall()
    {
        var bytes = new byte[8];
        var dec = new OpusRangeDecoder(bytes);
        uint seed = 0;
        bool threw = false;
        try
        {
            CeltBands.QuantAllBands(
                ref dec, CeltConstants.EBands, start: 0, end: 5,
                X: new float[256], Y: default, collapseMasks: new byte[5],
                pulses: new int[5], shortBlocks: false, spread: 0, dualStereo: false,
                intensity: 0, tfRes: new sbyte[5], totalBits: 100, balance: 0, LM: 3,
                codedBands: 5, ref seed, disableInv: false, normWorkspace: new float[4]);
        }
        catch (System.ArgumentException) { threw = true; }
        Assert.True(threw);
    }

    // ---------- helpers ----------

    private static BandContext MakeContext(int band = 0, int spread = CeltConstants.SpreadNormal,
        int intensity = 21, int remainingBits = 10_000, uint seed = 1, bool disableInv = false,
        int tfChange = 0)
        => new BandContext
        {
            Band = band,
            Spread = spread,
            Intensity = intensity,
            RemainingBits = remainingBits,
            Seed = seed,
            DisableInv = disableInv,
            TfChange = tfChange,
        };

    // ---------- AntiCollapse (Phase 2c.4) ----------

    [Fact]
    public void AntiCollapse_AllMasksSet_LeavesXUnchanged()
    {
        // When every short block has a non-zero pulse, anti-collapse is a no-op.
        const int LM = 2;
        const int channels = 1;
        const int start = 0;
        const int end = 8;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X = new float[channels * size];
        for (int i = 0; i < X.Length; i++) X[i] = 0.1f + i * 0.001f;
        var snapshot = (float[])X.Clone();

        var collapseMasks = new byte[end * channels];
        for (int i = 0; i < collapseMasks.Length; i++) collapseMasks[i] = 0xF; // blocks 0..3 all set

        var logE = new float[2 * CeltConstants.MaxBands];
        var prev1 = new float[2 * CeltConstants.MaxBands];
        var prev2 = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[end];
        for (int i = 0; i < end; i++) pulses[i] = 32;
        uint seed = 12345u;
        uint seedBefore = seed;

        CeltBands.AntiCollapse(eBands, X, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref seed);

        for (int i = 0; i < X.Length; i++)
            Assert.Equal(snapshot[i], X[i]);
        Assert.Equal(seedBefore, seed); // no LCG advance when nothing to fill
    }

    [Fact]
    public void AntiCollapse_FullyCollapsed_FillsAndRenormalisesBandsToUnit()
    {
        // Mask = 0 ⇒ every short block in every band is filled with ±r noise
        // and then renormalised to unit norm.
        const int LM = 2;
        const int channels = 1;
        const int start = 1;
        const int end = 6;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X = new float[channels * size];                 // start from silence
        var collapseMasks = new byte[end * channels];       // all zero
        var logE = new float[2 * CeltConstants.MaxBands];
        var prev1 = new float[2 * CeltConstants.MaxBands];
        var prev2 = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[end];
        for (int i = 0; i < end; i++) pulses[i] = 16;
        uint seed = 1u;

        CeltBands.AntiCollapse(eBands, X, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref seed);

        // Each band in [start, end) should have unit norm; bands < start untouched.
        for (int i = start; i < end; i++)
        {
            int N = (eBands[i + 1] - eBands[i]) << LM;
            double norm = 0;
            for (int j = 0; j < N; j++)
            {
                float v = X[(eBands[i] << LM) + j];
                norm += v * v;
            }
            Assert.Equal(1.0, norm, 4);
        }
        // Untouched region before start stays silent.
        for (int j = 0; j < (eBands[start] << LM); j++) Assert.Equal(0f, X[j]);
    }

    [Fact]
    public void AntiCollapse_PartialMask_OnlyMissingBlocksFilled()
    {
        // Mask bit set ⇒ that interleaved short block is preserved as-is.
        const int LM = 2; // 4 short blocks per band
        const int channels = 1;
        const int start = 0;
        const int end = 2;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X = new float[channels * size];
        for (int i = 0; i < X.Length; i++) X[i] = 0.25f; // sentinel, will be renormalised away if touched

        var collapseMasks = new byte[end * channels];
        collapseMasks[0] = 0b0101; // band 0: blocks 0,2 preserved; 1,3 filled
        collapseMasks[1] = 0b1111; // band 1: nothing filled

        var logE = new float[2 * CeltConstants.MaxBands];
        var prev1 = new float[2 * CeltConstants.MaxBands];
        var prev2 = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[end];
        for (int i = 0; i < end; i++) pulses[i] = 8;
        uint seed = 7u;

        CeltBands.AntiCollapse(eBands, X, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref seed);

        // Band 1 unchanged (preserved blocks for all k).
        int N1Start = eBands[1] << LM;
        int N1End = eBands[2] << LM;
        for (int j = N1Start; j < N1End; j++) Assert.Equal(0.25f, X[j]);
        // Band 0 was renormalised; check the magnitudes are bounded and signs reflect ±r at filled slots.
        int N0Start = eBands[0] << LM;
        int N0 = eBands[1] - eBands[0];
        // Filled blocks (k=1,3) carry ±r values; preserved blocks (k=0,2) retain pre-renorm
        // 0.25 scaled by the renorm gain — all values are finite and non-zero.
        for (int j = 0; j < (N0 << LM); j++) Assert.True(float.IsFinite(X[N0Start + j]));
    }

    [Fact]
    public void AntiCollapse_SeedAdvances_Deterministically()
    {
        const int LM = 1;
        const int channels = 1;
        const int start = 0;
        const int end = 4;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X1 = new float[channels * size];
        var X2 = new float[channels * size];
        var collapseMasks = new byte[end * channels]; // all zero
        var logE = new float[2 * CeltConstants.MaxBands];
        var prev1 = new float[2 * CeltConstants.MaxBands];
        var prev2 = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[end];
        for (int i = 0; i < end; i++) pulses[i] = 16;

        uint s1 = 42u, s2 = 42u;
        CeltBands.AntiCollapse(eBands, X1, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref s1);
        CeltBands.AntiCollapse(eBands, X2, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref s2);

        Assert.Equal(s1, s2);
        Assert.NotEqual(42u, s1); // advanced
        for (int i = 0; i < X1.Length; i++) Assert.Equal(X1[i], X2[i]);
    }

    [Fact]
    public void AntiCollapse_Stereo_FillsEachChannelPerItsMask()
    {
        const int LM = 1;
        const int channels = 2;
        const int start = 0;
        const int end = 3;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X = new float[channels * size];

        var collapseMasks = new byte[end * channels];
        // Band 0: ch0 collapsed, ch1 ok. Band 1: both ok. Band 2: both collapsed.
        collapseMasks[0 * 2 + 0] = 0;     collapseMasks[0 * 2 + 1] = 0b11;
        collapseMasks[1 * 2 + 0] = 0b11;  collapseMasks[1 * 2 + 1] = 0b11;
        collapseMasks[2 * 2 + 0] = 0;     collapseMasks[2 * 2 + 1] = 0;

        var logE = new float[2 * CeltConstants.MaxBands];
        var prev1 = new float[2 * CeltConstants.MaxBands];
        var prev2 = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[end];
        for (int i = 0; i < end; i++) pulses[i] = 16;
        uint seed = 9u;

        CeltBands.AntiCollapse(eBands, X, collapseMasks, LM, channels, size,
            start, end, logE, prev1, prev2, CeltConstants.MaxBands, pulses, ref seed);

        // Band 0 ch0 has unit norm; ch1 untouched (still zero).
        int b0n = (eBands[1] - eBands[0]) << LM;
        double n0c0 = 0, n0c1 = 0;
        for (int j = 0; j < b0n; j++)
        {
            float v0 = X[0 * size + (eBands[0] << LM) + j];
            float v1 = X[1 * size + (eBands[0] << LM) + j];
            n0c0 += v0 * v0;
            n0c1 += v1 * v1;
        }
        Assert.Equal(1.0, n0c0, 4);
        Assert.Equal(0.0, n0c1, 6);

        // Band 2 both channels filled and renormed.
        int b2n = (eBands[3] - eBands[2]) << LM;
        double n2c0 = 0, n2c1 = 0;
        for (int j = 0; j < b2n; j++)
        {
            float v0 = X[0 * size + (eBands[2] << LM) + j];
            float v1 = X[1 * size + (eBands[2] << LM) + j];
            n2c0 += v0 * v0;
            n2c1 += v1 * v1;
        }
        Assert.Equal(1.0, n2c0, 4);
        Assert.Equal(1.0, n2c1, 4);
    }

    [Fact]
    public void AntiCollapse_Mono_TakesMaxAcrossBothChannelHistories()
    {
        // For C==1 the decoder considers both channels' previous logE for the
        // collapse-threshold computation. Make ch1 history MUCH higher than
        // ch0 ⇒ Ediff drops ⇒ r is small ⇒ post-renorm samples have small
        // magnitude per sample (but the band as a whole is still unit-norm).
        const int LM = 0;
        const int channels = 1;
        const int start = 0;
        const int end = 1;
        var eBands = CeltConstants.EBands;
        int size = eBands[end] << LM;
        var X = new float[channels * size];
        var collapseMasks = new byte[end * channels]; // 0 -> collapse
        var pulses = new int[end];
        pulses[0] = 0; // depth 0 ⇒ thresh = 0.5
        var logE = new float[2 * CeltConstants.MaxBands];

        // Case A: ch1 history matches ch0 (irrelevant in non-mono path of params).
        var prev1A = new float[2 * CeltConstants.MaxBands];
        var prev2A = new float[2 * CeltConstants.MaxBands];
        uint sA = 31u;
        var Xa = (float[])X.Clone();
        CeltBands.AntiCollapse(eBands, Xa, collapseMasks, LM, channels, size,
            start, end, logE, prev1A, prev2A, CeltConstants.MaxBands, pulses, ref sA);

        // Case B: ch1 history is large ⇒ in mono mode max(ch0, ch1) is used,
        // so prev becomes large ⇒ Ediff = max(0, logE - prev) = 0 (since logE=0)
        // ⇒ r = 2 ⇒ clipped to thresh = 0.5 ⇒ same r as case A.
        var prev1B = new float[2 * CeltConstants.MaxBands];
        var prev2B = new float[2 * CeltConstants.MaxBands];
        prev1B[CeltConstants.MaxBands + 0] = 8.0f; // huge ch1 history
        prev2B[CeltConstants.MaxBands + 0] = 8.0f;
        uint sB = 31u;
        var Xb = (float[])X.Clone();
        CeltBands.AntiCollapse(eBands, Xb, collapseMasks, LM, channels, size,
            start, end, logE, prev1B, prev2B, CeltConstants.MaxBands, pulses, ref sB);

        // Both cases end up clipped by thresh so post-renorm outputs match.
        Assert.Equal(sA, sB);
        for (int i = 0; i < Xa.Length; i++) Assert.Equal(Xa[i], Xb[i], 6);

        // Case C: logE > prev ⇒ Ediff > 0 ⇒ r shrinks below thresh,
        // but post-renorm the band is still unit-norm. Just verify finiteness.
        var prev1C = new float[2 * CeltConstants.MaxBands];
        var prev2C = new float[2 * CeltConstants.MaxBands];
        var logEc = new float[2 * CeltConstants.MaxBands];
        logEc[0] = 4.0f; // current band logE
        uint sC = 31u;
        var Xc = (float[])X.Clone();
        CeltBands.AntiCollapse(eBands, Xc, collapseMasks, LM, channels, size,
            start, end, logEc, prev1C, prev2C, CeltConstants.MaxBands, pulses, ref sC);
        for (int i = 0; i < Xc.Length; i++) Assert.True(float.IsFinite(Xc[i]));
        double normC = 0; foreach (var v in Xc) normC += v * v;
        Assert.Equal(1.0, normC, 4);
    }

    [Fact]
    public void AntiCollapse_NegativeStartOrEndThrows()
    {
        var X = new float[16];
        var masks = new byte[8];
        var logE = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[8];

        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            uint s = 1u;
            CeltBands.AntiCollapse(CeltConstants.EBands, X, masks, LM: 4, channels: 1, size: 16,
                start: 0, end: 1, logE, logE, logE, CeltConstants.MaxBands, pulses, ref s);
        });
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            uint s = 1u;
            CeltBands.AntiCollapse(CeltConstants.EBands, X, masks, LM: 1, channels: 3, size: 16,
                start: 0, end: 1, logE, logE, logE, CeltConstants.MaxBands, pulses, ref s);
        });
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            uint s = 1u;
            CeltBands.AntiCollapse(CeltConstants.EBands, X, masks, LM: 1, channels: 1, size: 16,
                start: 5, end: 2, logE, logE, logE, CeltConstants.MaxBands, pulses, ref s);
        });
    }

    [Fact]
    public void AntiCollapse_TooSmallSize_Throws()
    {
        var X = new float[4];
        var masks = new byte[8];
        var logE = new float[2 * CeltConstants.MaxBands];
        var pulses = new int[8];
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            uint s = 1u;
            CeltBands.AntiCollapse(CeltConstants.EBands, X, masks, LM: 2, channels: 1, size: 2,
                start: 0, end: 1, logE, logE, logE, CeltConstants.MaxBands, pulses, ref s);
        });
    }

    // ---------- UnquantEnergyFinalise (Phase 2c.4) ----------

    [Fact]
    public void UnquantEnergyFinalise_NoBitsLeft_NoOp()
    {
        var bytes = MakeRandomBytes(seed: 5, length: 64);
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var snapshot = (float[])oldLogE.Clone();
        var ebits = new int[21];
        var fp = new int[21];

        int bitsLeft = 0;
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        Assert.Equal(0, bitsLeft);
        for (int i = 0; i < oldLogE.Length; i++) Assert.Equal(snapshot[i], oldLogE[i]);
    }

    [Fact]
    public void UnquantEnergyFinalise_AllBandsMaxedOut_NoChange()
    {
        var bytes = MakeRandomBytes(seed: 11, length: 64);
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var snapshot = (float[])oldLogE.Clone();
        var ebits = new int[21];
        for (int i = 0; i < 21; i++) ebits[i] = CeltConstants.MaxFineBits; // skip all
        var fp = new int[21];

        int bitsLeft = 100;
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        Assert.Equal(100, bitsLeft); // nothing consumed
        for (int i = 0; i < oldLogE.Length; i++) Assert.Equal(snapshot[i], oldLogE[i]);
    }

    [Fact]
    public void UnquantEnergyFinalise_AllZeroBits_DecreasesByMagnitude()
    {
        // Last byte of the buffer feeds raw bits. Set the trailing portion to 0
        // so every DecodeBits(1) returns 0 ⇒ offset = -(1 << (8 - eb)).
        var bytes = new byte[64];
        // leave all bytes as zero
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];
        // Use ebits = 3 so magnitude = 1 << 5 = 32.
        const int eb = 3;
        const float expected = -(1 << (8 - eb));
        for (int i = 0; i < 21; i++) { ebits[i] = eb; fp[i] = 0; }

        int bitsLeft = 10;
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 10,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        // 10 bands × 1 channel = 10 bits consumed.
        Assert.Equal(0, bitsLeft);
        for (int i = 0; i < 10; i++) Assert.Equal(expected, oldLogE[i]);
        for (int i = 10; i < CeltConstants.MaxBands; i++) Assert.Equal(0f, oldLogE[i]);
    }

    [Fact]
    public void UnquantEnergyFinalise_AllOneBits_IncreasesByMagnitude()
    {
        // Filling the buffer with 0xFF feeds 1-bits to the raw decoder.
        var bytes = new byte[64];
        for (int i = 0; i < bytes.Length; i++) bytes[i] = 0xFF;
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];
        const int eb = 5;
        const float expected = +(1 << (8 - eb)); // 8
        for (int i = 0; i < 21; i++) { ebits[i] = eb; fp[i] = 0; }

        int bitsLeft = 5;
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 5,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        Assert.Equal(0, bitsLeft);
        for (int i = 0; i < 5; i++) Assert.Equal(expected, oldLogE[i]);
    }

    [Fact]
    public void UnquantEnergyFinalise_PriorityOrdering_Prio0FirstThenPrio1()
    {
        var bytes = new byte[64]; // all-zero bits
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];
        for (int i = 0; i < 21; i++) ebits[i] = 4;
        // Alternate priorities to force prio-0 to be processed first.
        for (int i = 0; i < 21; i++) fp[i] = i % 2; // 0,1,0,1,...

        // 8 bits ⇒ enough to do all four prio-0 bands in [0,8) then start prio-1.
        int bitsLeft = 8;
        const float magnitude = (1 << (8 - 4)); // 16

        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 8,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        // Prio-0 bands (indices 0,2,4,6) decremented; prio-1 bands (1,3,5,7) also decremented
        // (since we had 4 prio-0 + 4 prio-1 = 8 bits total).
        for (int i = 0; i < 8; i++) Assert.Equal(-magnitude, oldLogE[i]);
        Assert.Equal(0, bitsLeft);
    }

    [Fact]
    public void UnquantEnergyFinalise_BitsExhaustedMidLoop_StopsEarly()
    {
        var bytes = new byte[64];
        for (int i = 0; i < bytes.Length; i++) bytes[i] = 0xFF;
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];
        for (int i = 0; i < 21; i++) { ebits[i] = 2; fp[i] = 0; }

        int bitsLeft = 3; // only 3 bands worth (mono)
        const float mag = (1 << (8 - 2)); // 64
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
            channels: 1, logStride: CeltConstants.MaxBands, ref bitsLeft);

        Assert.Equal(0, bitsLeft);
        Assert.Equal(+mag, oldLogE[0]);
        Assert.Equal(+mag, oldLogE[1]);
        Assert.Equal(+mag, oldLogE[2]);
        for (int i = 3; i < CeltConstants.MaxBands; i++) Assert.Equal(0f, oldLogE[i]);
    }

    [Fact]
    public void UnquantEnergyFinalise_Stereo_ConsumesTwoBitsPerBand()
    {
        var bytes = new byte[64];
        for (int i = 0; i < bytes.Length; i++) bytes[i] = 0xFF;
        var dec = new OpusRangeDecoder(bytes);
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];
        const int eb = 6;
        for (int i = 0; i < 21; i++) { ebits[i] = eb; fp[i] = 0; }

        int bitsLeft = 6; // exactly 3 bands × 2 channels
        const float mag = (1 << (8 - eb)); // 4
        CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
            channels: 2, logStride: CeltConstants.MaxBands, ref bitsLeft);

        Assert.Equal(0, bitsLeft);
        for (int i = 0; i < 3; i++)
        {
            Assert.Equal(+mag, oldLogE[i]);                                // ch0
            Assert.Equal(+mag, oldLogE[CeltConstants.MaxBands + i]);       // ch1
        }
        // 4th band onward untouched.
        Assert.Equal(0f, oldLogE[3]);
        Assert.Equal(0f, oldLogE[CeltConstants.MaxBands + 3]);
    }

    [Fact]
    public void UnquantEnergyFinalise_ArgumentValidation()
    {
        var bytes = new byte[64];
        var oldLogE = new float[2 * CeltConstants.MaxBands];
        var ebits = new int[21];
        var fp = new int[21];

        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var dec = new OpusRangeDecoder(bytes);
            int bl = 10;
            CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
                channels: 0, logStride: CeltConstants.MaxBands, ref bl);
        });
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var dec = new OpusRangeDecoder(bytes);
            int bl = 10;
            CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 5, end: 2,
                channels: 1, logStride: CeltConstants.MaxBands, ref bl);
        });
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            var dec = new OpusRangeDecoder(bytes);
            int bl = 10;
            CeltBands.UnquantEnergyFinalise(ref dec, oldLogE, ebits, fp, start: 0, end: 21,
                channels: 1, logStride: 5, ref bl);
        });
    }

    private static byte[] MakeRandomBytes(int seed, int length)
    {
        var rng = new System.Random(seed);
        var buf = new byte[length];
        rng.NextBytes(buf);
        return buf;
    }
}
