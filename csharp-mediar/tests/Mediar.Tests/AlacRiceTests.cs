using Mediar.Codecs.Alac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AlacRiceTests
{
    // Mirrors Apple's `dyn_code_32bit` constants used by the decoder. Kept
    // private here so the test encoder lives in the test assembly only.
    private const int QbShift = 9;
    private const int Qb = 1 << QbShift;
    private const int MmulShift = 2;
    private const int MdenShift = QbShift - MmulShift - 1;
    private const int Moff = 1 << (MdenShift - 2);
    private const int Bitoff = 24;
    private const int MaxPrefix32 = 9;
    private const int MaxPrefix16 = 9;
    private const int MaxDataTypeBits16 = 16;
    private const int NMaxMeanClamp = 0xFFFF;
    private const int NMeanClampVal = 0xFFFF;

    [Fact]
    public void RoundTrip_AllZeros_TriggersZeroRunMode()
    {
        // All-zero residuals should compress aggressively. The decoder must
        // enter zero-run mode and emit the run correctly.
        var residuals = new int[64];
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    [Fact]
    public void RoundTrip_Constant1_NoZeroRun()
    {
        // Constant non-zero residuals keep mb high and don't trigger zero-run.
        var residuals = new int[64];
        Array.Fill(residuals, 1);
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    [Fact]
    public void RoundTrip_AlternatingSigns_NoZeroRun()
    {
        var residuals = new int[32];
        for (int i = 0; i < residuals.Length; i++)
            residuals[i] = (i & 1) == 0 ? 7 : -3;
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    [Fact]
    public void RoundTrip_LargeValues_TriggersEscape()
    {
        // Values larger than 2^9 * m will overflow the unary prefix and hit
        // the escape path. Drive a sequence of large positive values to
        // force at least one escape.
        var residuals = new int[16];
        Array.Fill(residuals, 30000);
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    [Fact]
    public void RoundTrip_SparseSignal_ExercisesZeroRunAndRegular()
    {
        // Mix of large impulses and stretches of zeros — this should toggle
        // between the regular and zero-run paths repeatedly.
        var residuals = new int[128];
        residuals[0] = 50;
        residuals[10] = -25;
        residuals[40] = 100;
        residuals[90] = -50;
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    [Fact]
    public void RoundTrip_RandomResiduals_24Bit()
    {
        // Larger bit width and a longer random stream stress all branches.
        var rng = new Random(12345);
        var residuals = new int[256];
        for (int i = 0; i < residuals.Length; i++)
        {
            int range = 1 << 23;
            int v = rng.Next(-range, range);
            residuals[i] = v;
        }
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 24);
    }

    [Fact]
    public void RoundTrip_NearZeroDrivesPbIntoSmallK()
    {
        // Residuals just barely above zero keep mb small, exercising the
        // k=1 branch (pure unary, no truncated-binary suffix).
        var residuals = new int[64];
        for (int i = 0; i < residuals.Length; i++)
            residuals[i] = (i % 3 == 0) ? 1 : 0;
        AssertRoundTrip(residuals, mb: 40, pb: 40, kb: 14, maxSize: 16);
    }

    private static void AssertRoundTrip(int[] residuals, int mb, int pb, int kb, int maxSize)
    {
        byte[] encoded = EncodeBlock(residuals, mb, pb, kb, maxSize);

        var br = new Mediar.IO.BitReader(encoded);
        var decoded = new int[residuals.Length];
        AlacRice.DecodeBlock(ref br, decoded, residuals.Length, mb, pb, kb, maxSize);

        Assert.Equal(residuals, decoded);
    }

    // Encoder mirror of the decoder state machine in AlacRice.DecodeBlock.
    // Mirrors Apple's `dyn_comp` plus `dyn_code_32bit`.
    private static byte[] EncodeBlock(int[] residuals, int mbInitial, int pb, int kb, int maxSize)
    {
        var bytes = new byte[residuals.Length * 8 + 64];
        var bw = new Mediar.IO.BitWriter(bytes);

        int mb = mbInitial;
        int wb = (1 << kb) - 1;
        int zmode = 0;

        int c = 0;
        while (c < residuals.Length)
        {
            int m = mb >> QbShift;
            int k = Lg3a(m);
            if (k > kb) k = kb;
            m = (1 << k) - 1;

            int del = residuals[c++];
            uint nDecode = del > 0
                ? (uint)(2 * del)
                : (uint)(-2 * del - 1);
            if (del == 0) nDecode = 0;
            uint n = nDecode - (uint)zmode;

            DynCode32Bit(ref bw, m, k, maxSize, n);

            mb = pb * (int)(n + (uint)zmode) + mb - ((pb * mb) >> QbShift);
            if (n > NMaxMeanClamp) mb = NMeanClampVal;

            zmode = 0;

            if (((mb << MmulShift) < Qb) && (c < residuals.Length))
            {
                zmode = 1;
                int leadBits = Lead((uint)mb);
                int kz = leadBits - Bitoff + ((mb + Moff) >> MdenShift);
                if (kz < 0) kz = 0;
                int mz = ((1 << kz) - 1) & wb;

                // Count the number of consecutive zeros at the current position
                // (the encoder agreed-upon "run length" to write).
                int runLen = 0;
                while (c < residuals.Length && residuals[c] == 0 && runLen < 65535)
                {
                    runLen++;
                    c++;
                }

                DynCode16Bit(ref bw, mz, kz, (uint)runLen);

                if (runLen >= 65535) zmode = 0;
                mb = 0;
            }
        }

        return bytes.AsSpan(0, bw.BytesWritten).ToArray();
    }

    private static void DynCode32Bit(ref Mediar.IO.BitWriter bw, int m, int k, int maxbits, uint n)
    {
        uint div = m == 0 ? n : (uint)(n / (uint)m);
        if (div >= MaxPrefix32)
        {
            // Escape: MAX_PREFIX_32 ones, no terminator, then `maxbits` raw bits.
            bw.WriteBits(((1u << MaxPrefix32) - 1u), MaxPrefix32);
            bw.WriteBits(n, maxbits);
            return;
        }

        if (k == 1)
        {
            // Pure unary: n ones followed by a terminator 0.
            int numBits = (int)div + 1;
            uint value = ((1u << numBits) - 2u);
            bw.WriteBits(value, numBits);
            return;
        }

        uint mod = n - (uint)m * div;
        uint de = (mod == 0) ? 1u : 0u;
        int numBits2 = (int)div + k + 1 - (int)de;
        uint value2 = (((1u << (int)div) - 1u) << (numBits2 - (int)div)) + mod + 1u - de;
        bw.WriteBits(value2, numBits2);
    }

    private static void DynCode16Bit(ref Mediar.IO.BitWriter bw, int m, int k, uint n)
    {
        uint div = m == 0 ? n : (uint)(n / (uint)m);
        if (div >= MaxPrefix16)
        {
            bw.WriteBits(((1u << MaxPrefix16) - 1u), MaxPrefix16);
            bw.WriteBits(n, MaxDataTypeBits16);
            return;
        }

        if (k == 1)
        {
            int numBits = (int)div + 1;
            uint value = ((1u << numBits) - 2u);
            bw.WriteBits(value, numBits);
            return;
        }

        uint mod = n - (uint)m * div;
        uint de = (mod == 0) ? 1u : 0u;
        int numBits2 = (int)div + k + 1 - (int)de;
        uint value2 = (((1u << (int)div) - 1u) << (numBits2 - (int)div)) + mod + 1u - de;
        bw.WriteBits(value2, numBits2);
    }

    private static int Lg3a(int x)
    {
        long v = (long)x + 3;
        if (v <= 0) return 0;
        int leadingZeros = System.Numerics.BitOperations.LeadingZeroCount((uint)v);
        return 31 - leadingZeros;
    }

    private static int Lead(uint x) =>
        x == 0 ? 32 : System.Numerics.BitOperations.LeadingZeroCount(x);
}
