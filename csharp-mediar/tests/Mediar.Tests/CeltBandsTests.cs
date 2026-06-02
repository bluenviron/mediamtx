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

    private static byte[] MakeRandomBytes(int seed, int length)
    {
        var rng = new System.Random(seed);
        var buf = new byte[length];
        rng.NextBytes(buf);
        return buf;
    }
}
