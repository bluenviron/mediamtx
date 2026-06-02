using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

public class CeltShapeTests
{
    // ---------- Haar1 ----------

    [Fact]
    public void Haar1_StrideOne_SumDifferenceAtScaleSqrt1_2()
    {
        const float k = 0.70710678118654752440f;
        float[] x = { 1f, 3f, 5f, 7f };
        CeltShape.Haar1(x, 4, 1);

        Assert.Equal(k * 1 + k * 3, x[0], 5);
        Assert.Equal(k * 1 - k * 3, x[1], 5);
        Assert.Equal(k * 5 + k * 7, x[2], 5);
        Assert.Equal(k * 5 - k * 7, x[3], 5);
    }

    [Fact]
    public void Haar1_AppliedTwice_RecoversInput_StrideOne()
    {
        // haar1 is its own inverse up to a permutation: applying it
        // twice to (a, b) gives ((a+b)/2 + (a-b)/2, (a+b)/2 - (a-b)/2)
        // = (a, b). The 1/sqrt(2) factors compose to 1/2 * 2 = 1.
        var rng = new System.Random(0xDEAD);
        var x = new float[16];
        for (int i = 0; i < x.Length; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        var copy = (float[])x.Clone();

        CeltShape.Haar1(x, 16, 1);
        CeltShape.Haar1(x, 16, 1);

        for (int i = 0; i < x.Length; i++)
            Assert.Equal(copy[i], x[i], 5);
    }

    [Fact]
    public void Haar1_PreservesEnergy()
    {
        var rng = new System.Random(0xC0FFEE);
        var x = new float[32];
        for (int i = 0; i < x.Length; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        double e0 = 0; foreach (var v in x) e0 += v * v;

        CeltShape.Haar1(x, 32, 1);
        double e1 = 0; foreach (var v in x) e1 += v * v;
        Assert.Equal(e0, e1, 4);
    }

    [Fact]
    public void Haar1_StrideTwo_OperatesPerSubstream()
    {
        // With stride=2 the two interleaved substreams are
        // (X[0], X[2], X[4], X[6]) and (X[1], X[3], X[5], X[7]).
        const float k = 0.70710678118654752440f;
        float[] x = { 1f, 10f, 2f, 20f, 3f, 30f, 4f, 40f };
        CeltShape.Haar1(x, 4, 2);

        // Substream 0 (indices 0,2,4,6 = 1,2,3,4):
        Assert.Equal(k * 1 + k * 2, x[0], 5);
        Assert.Equal(k * 1 - k * 2, x[2], 5);
        Assert.Equal(k * 3 + k * 4, x[4], 5);
        Assert.Equal(k * 3 - k * 4, x[6], 5);
        // Substream 1 (indices 1,3,5,7 = 10,20,30,40):
        Assert.Equal(k * 10 + k * 20, x[1], 5);
        Assert.Equal(k * 10 - k * 20, x[3], 5);
        Assert.Equal(k * 30 + k * 40, x[5], 5);
        Assert.Equal(k * 30 - k * 40, x[7], 5);
    }

    // ---------- DeinterleaveHadamard / InterleaveHadamard ----------

    [Theory]
    [InlineData(2)]
    [InlineData(4)]
    [InlineData(8)]
    [InlineData(16)]
    public void Hadamard_DeinterleaveThenInterleave_IsIdentity_PlainOrder(int stride)
    {
        const int N0 = 6;
        int n = N0 * stride;
        var rng = new System.Random(stride * 31);
        var x = new float[n];
        for (int i = 0; i < n; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        var copy = (float[])x.Clone();

        CeltShape.DeinterleaveHadamard(x, N0, stride, hadamard: false);
        CeltShape.InterleaveHadamard(x, N0, stride, hadamard: false);

        for (int i = 0; i < n; i++) Assert.Equal(copy[i], x[i], 6);
    }

    [Theory]
    [InlineData(2)]
    [InlineData(4)]
    [InlineData(8)]
    [InlineData(16)]
    public void Hadamard_DeinterleaveThenInterleave_IsIdentity_OrderyPermutation(int stride)
    {
        const int N0 = 5;
        int n = N0 * stride;
        var rng = new System.Random(stride * 17 + 1);
        var x = new float[n];
        for (int i = 0; i < n; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        var copy = (float[])x.Clone();

        CeltShape.DeinterleaveHadamard(x, N0, stride, hadamard: true);
        CeltShape.InterleaveHadamard(x, N0, stride, hadamard: true);

        for (int i = 0; i < n; i++) Assert.Equal(copy[i], x[i], 6);
    }

    [Fact]
    public void Deinterleave_PlainOrder_SplitsInterleavedSubstreams()
    {
        // With X = [a0, b0, c0, a1, b1, c1, a2, b2, c2] (stride=3, N0=3)
        // hadamard=false gathers each substream contiguously:
        // tmp[i*N0+j] = X[j*stride+i] ⇒ [a0,a1,a2, b0,b1,b2, c0,c1,c2].
        float[] x = { 10, 20, 30, 11, 21, 31, 12, 22, 32 };
        CeltShape.DeinterleaveHadamard(x, N0: 3, stride: 3, hadamard: false);
        Assert.Equal(new float[] { 10, 11, 12, 20, 21, 22, 30, 31, 32 }, x);
    }

    [Fact]
    public void Deinterleave_OrderyOrder_AppliesStride2Permutation()
    {
        // For stride=2 the ordery table is {1, 0}: substream 0 lands at
        // ord=1 (second half), substream 1 lands at ord=0 (first half).
        // X = [a0, b0, a1, b1, a2, b2] ⇒ result = [b0,b1,b2, a0,a1,a2].
        float[] x = { 10, 20, 11, 21, 12, 22 };
        CeltShape.DeinterleaveHadamard(x, N0: 3, stride: 2, hadamard: true);
        Assert.Equal(new float[] { 20, 21, 22, 10, 11, 12 }, x);
    }

    // ---------- ExpRotation ----------

    [Fact]
    public void ExpRotation_SpreadNone_IsNoOp()
    {
        var x = new float[16];
        for (int i = 0; i < x.Length; i++) x[i] = i;
        var copy = (float[])x.Clone();
        CeltShape.ExpRotation(x, 16, dir: -1, stride: 1, K: 3, spread: CeltConstants.SpreadNone);
        Assert.Equal(copy, x);
    }

    [Fact]
    public void ExpRotation_TwoKGreaterEqualLen_IsNoOp()
    {
        var x = new float[8];
        for (int i = 0; i < x.Length; i++) x[i] = i + 1;
        var copy = (float[])x.Clone();
        // 2K = 8 ≥ len = 8 ⇒ early-out.
        CeltShape.ExpRotation(x, 8, dir: -1, stride: 1, K: 4, spread: CeltConstants.SpreadNormal);
        Assert.Equal(copy, x);
    }

    [Theory]
    [InlineData(16, 1, 3, CeltConstants.SpreadLight)]
    [InlineData(16, 1, 3, CeltConstants.SpreadNormal)]
    [InlineData(16, 1, 3, CeltConstants.SpreadAggressive)]
    [InlineData(32, 2, 5, CeltConstants.SpreadNormal)]
    [InlineData(64, 4, 7, CeltConstants.SpreadNormal)]
    [InlineData(128, 8, 9, CeltConstants.SpreadNormal)]
    public void ExpRotation_ForwardThenInverse_RecoversInput(int len, int stride, int K, int spread)
    {
        var rng = new System.Random(len * 7 + stride * 31 + K * 11 + spread);
        var x = new float[len];
        for (int i = 0; i < len; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        var copy = (float[])x.Clone();

        CeltShape.ExpRotation(x, len, dir: 1, stride, K, spread);
        CeltShape.ExpRotation(x, len, dir: -1, stride, K, spread);

        for (int i = 0; i < len; i++)
            Assert.Equal(copy[i], x[i], 3);
    }

    [Fact]
    public void ExpRotation_ApproximatelyPreservesEnergy()
    {
        // Givens rotations are orthonormal so ‖X‖² is invariant.
        var rng = new System.Random(99);
        var x = new float[64];
        for (int i = 0; i < x.Length; i++) x[i] = (float)(rng.NextDouble() * 2 - 1);
        double e0 = 0; foreach (var v in x) e0 += v * v;

        CeltShape.ExpRotation(x, 64, dir: -1, stride: 4, K: 5, spread: CeltConstants.SpreadNormal);
        double e1 = 0; foreach (var v in x) e1 += v * v;
        Assert.Equal(e0, e1, 3);
    }

    [Fact]
    public void ExpRotation_InvalidSpread_Throws()
    {
        var x = new float[8];
        Assert.Throws<System.ArgumentOutOfRangeException>(() =>
            CeltShape.ExpRotation(x, 8, dir: -1, stride: 1, K: 1, spread: 4));
    }

    // ---------- NormaliseResidual ----------

    [Fact]
    public void NormaliseResidual_UnitGain_ProducesUnitNorm()
    {
        int[] iy = { 3, -4, 0, 0 }; // ‖iy‖² = 9 + 16 = 25
        var X = new float[4];
        CeltShape.NormaliseResidual(iy, X, 4, ryy: 25f, gain: 1f);
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(1.0, norm, 5);
        // Direction preserved.
        Assert.Equal(0.6f, X[0], 5);
        Assert.Equal(-0.8f, X[1], 5);
    }

    [Fact]
    public void NormaliseResidual_WithGain_ScalesOutputNorm()
    {
        int[] iy = { 1, 0, -1, 0, 1, -1 };
        int ryy = 0; foreach (var v in iy) ryy += v * v;
        var X = new float[6];
        CeltShape.NormaliseResidual(iy, X, 6, ryy: ryy, gain: 2.5f);
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(2.5 * 2.5, norm, 4);
    }

    [Fact]
    public void NormaliseResidual_ZeroRyy_Throws()
    {
        int[] iy = { 0, 0 };
        var X = new float[2];
        Assert.Throws<System.ArgumentOutOfRangeException>(() =>
            CeltShape.NormaliseResidual(iy, X, 2, ryy: 0f, gain: 1f));
    }

    // ---------- ExtractCollapseMask ----------

    [Fact]
    public void ExtractCollapseMask_BEqualsOne_ReturnsOne()
    {
        int[] iy = { 0, 0, 0, 0 };
        Assert.Equal(1u, CeltShape.ExtractCollapseMask(iy, 4, 1));
    }

    [Fact]
    public void ExtractCollapseMask_PerBlockBitsReflectNonZeroSlots()
    {
        // N=8, B=4 ⇒ N0=2. Blocks: [0,0] [0,1] [2,0] [0,0]
        // → block 0 empty, block 1 non-empty, block 2 non-empty, block 3 empty.
        int[] iy = { 0, 0, 0, 1, 2, 0, 0, 0 };
        uint mask = CeltShape.ExtractCollapseMask(iy, 8, 4);
        Assert.Equal(0b_0110u, mask);
    }

    [Fact]
    public void ExtractCollapseMask_NegativePulseSetsBit()
    {
        // |·| is not used — the OR of signed ints includes the sign bit.
        // Any non-zero (positive or negative) trips the block bit.
        int[] iy = { 0, -1, 0, 0 };
        uint mask = CeltShape.ExtractCollapseMask(iy, 4, 2);
        Assert.Equal(0b_01u, mask);
    }

    // ---------- AlgUnquant (integration) ----------

    [Fact]
    public void AlgUnquant_DecodesUnitNormShape_AndMatchesDecodePulsesSideChannel()
    {
        // Use side-channel verification: decode the same byte stream twice.
        // First pass with the raw DecodePulses primitive gives us the
        // "ground truth" iy and ryy; second pass through AlgUnquant should
        // produce X equal to NormaliseResidual(iy, ryy, gain) (no rotation
        // since spread=NONE) and a matching collapse mask.
        const int N = 8, K = 4, B = 2;
        // Arbitrary bytes — must be long enough to satisfy decode_pulses.
        byte[] buf = { 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0x11, 0x22 };

        var decRef = new OpusRangeDecoder(buf);
        var iy = new int[N];
        int ryy = CeltPvq.DecodePulses(ref decRef, N, K, iy);

        // Expected post-normalisation vector (spread=NONE ⇒ no exp_rotation).
        var expectedX = new float[N];
        CeltShape.NormaliseResidual(iy, expectedX, N, ryy, gain: 1f);
        uint expectedMask = CeltShape.ExtractCollapseMask(iy, N, B);

        var decTest = new OpusRangeDecoder(buf);
        var X = new float[N];
        uint mask = CeltShape.AlgUnquant(X, N, K, spread: CeltConstants.SpreadNone, B: B, ref decTest, gain: 1f);

        Assert.Equal(expectedMask, mask);
        for (int i = 0; i < N; i++)
            Assert.Equal(expectedX[i], X[i], 6);

        // Output is unit norm.
        double norm = 0;
        foreach (var v in X) norm += v * v;
        Assert.Equal(1.0, norm, 5);
    }

    [Fact]
    public void AlgUnquant_WithSpread_AppliesInverseRotation()
    {
        // With spread enabled, AlgUnquant should = NormaliseResidual + ExpRotation(dir=-1).
        // Verify by reproducing the pipeline manually on a side-channel decode.
        const int N = 16, K = 3, B = 1;
        byte[] buf = { 0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD };

        var decRef = new OpusRangeDecoder(buf);
        var iy = new int[N];
        int ryy = CeltPvq.DecodePulses(ref decRef, N, K, iy);

        var expectedX = new float[N];
        CeltShape.NormaliseResidual(iy, expectedX, N, ryy, gain: 1f);
        CeltShape.ExpRotation(expectedX, N, dir: -1, stride: B, K, CeltConstants.SpreadNormal);

        var decTest = new OpusRangeDecoder(buf);
        var X = new float[N];
        CeltShape.AlgUnquant(X, N, K, CeltConstants.SpreadNormal, B, ref decTest, gain: 1f);

        for (int i = 0; i < N; i++)
            Assert.Equal(expectedX[i], X[i], 5);
    }

    [Fact]
    public void AlgUnquant_KZero_Throws()
    {
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[4];
        bool threw = false;
        try { CeltShape.AlgUnquant(X, 4, K: 0, spread: 0, B: 1, ref dec, gain: 1f); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }

    [Fact]
    public void AlgUnquant_NTooSmall_Throws()
    {
        var dec = new OpusRangeDecoder(new byte[] { 0 });
        var X = new float[1];
        bool threw = false;
        try { CeltShape.AlgUnquant(X, N: 1, K: 1, spread: 0, B: 1, ref dec, gain: 1f); }
        catch (System.ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw);
    }
}
