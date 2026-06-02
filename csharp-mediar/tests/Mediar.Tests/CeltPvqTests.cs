using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the CELT PVQ shape decoder in <see cref="CeltPvq"/> (Opus
/// Phase 2c.3b.2). The PVQ codebook is enumerable for small N/K, so we
/// can verify the decoder hits every codeword exactly once and that
/// each codeword satisfies the structural invariants (Σ|y|=K, len=n,
/// yy=Σy²) — plus a round-trip test against a test-only encoder port.
/// </summary>
public sealed class CeltPvqTests
{
    /// <summary>
    /// Test-only port of libopus <c>icwrs</c> (small-footprint variant)
    /// — encoder side, used solely to drive round-trip checks. We don't
    /// ship an encoder in the production decoder build.
    /// </summary>
    private static uint IcwrsEncode(int n, int k, ReadOnlySpan<int> y, out uint v)
    {
        Span<uint> u = stackalloc uint[k + 2];
        u[0] = 0;
        for (int kk = 1; kk <= k + 1; kk++) u[kk] = (uint)((kk << 1) - 1);

        uint i = (uint)(y[n - 1] < 0 ? 1 : 0);
        int kCur = Math.Abs(y[n - 1]);
        int j = n - 2;
        i = unchecked(i + u[kCur]);
        kCur += Math.Abs(y[j]);
        if (y[j] < 0) i = unchecked(i + u[kCur + 1]);
        while (j-- > 0)
        {
            UnextInPlace(u, k + 2, 0);
            i = unchecked(i + u[kCur]);
            kCur += Math.Abs(y[j]);
            if (y[j] < 0) i = unchecked(i + u[kCur + 1]);
        }
        v = unchecked(u[kCur] + u[kCur + 1]);
        return i;
    }

    private static void UnextInPlace(Span<uint> u, int len, uint u0)
    {
        for (int j = 1; j < len; j++)
        {
            uint ui1 = unchecked(u[j] + u[j - 1] + u0);
            u[j - 1] = u0;
            u0 = ui1;
        }
        u[len - 1] = u0;
    }

    [Theory]
    [InlineData(2, 2)]
    [InlineData(2, 4)]
    [InlineData(3, 3)]
    [InlineData(4, 4)]
    [InlineData(5, 5)]
    [InlineData(6, 6)]
    [InlineData(8, 3)]
    [InlineData(10, 4)]
    public void DecodePulses_Visits_Every_Codeword_Exactly_Once(int n, int k)
    {
        uint v = CeltPvq.ComputeV(n, k);
        var seen = new HashSet<string>();
        Span<int> y = stackalloc int[n];
        for (uint i = 0; i < v; i++)
        {
            int yy = CeltPvq.DecodePulsesAtIndex(n, k, i, y);

            int absSum = 0, sqSum = 0;
            for (int j = 0; j < n; j++)
            {
                absSum += Math.Abs(y[j]);
                sqSum += y[j] * y[j];
            }
            Assert.Equal(k, absSum);
            Assert.Equal(yy, sqSum);

            string key = string.Join(",", y.ToArray());
            Assert.True(seen.Add(key), $"duplicate codeword {key} at index {i} for (n={n}, k={k})");
        }
        Assert.Equal((int)v, seen.Count);
    }

    [Theory]
    [InlineData(2, 2)]
    [InlineData(3, 3)]
    [InlineData(4, 5)]
    [InlineData(5, 4)]
    [InlineData(8, 6)]
    [InlineData(10, 8)]
    public void Decode_Then_Encode_Round_Trips_For_All_Indices(int n, int k)
    {
        uint v = CeltPvq.ComputeV(n, k);
        Span<int> y = stackalloc int[n];
        for (uint i = 0; i < v; i++)
        {
            CeltPvq.DecodePulsesAtIndex(n, k, i, y);
            uint recoveredV;
            uint recoveredI = IcwrsEncode(n, k, y, out recoveredV);
            Assert.Equal(v, recoveredV);
            Assert.Equal(i, recoveredI);
        }
    }

    [Theory]
    [InlineData(2, 1, 4u)]      // V(2, K) = 4K
    [InlineData(2, 5, 20u)]
    [InlineData(3, 3, 38u)]     // V(3, K) = 4K² + 2 ⇒ V(3,3) = 38
    [InlineData(3, 5, 102u)]
    [InlineData(4, 4, 192u)]    // V(4, K) = 8K(K²+2)/3 ⇒ V(4,4) = 8·4·18/3 = 192
    [InlineData(5, 5, 1002u)]   // U(5,5)+U(5,6) = 321+681 = 1002 — matches static table
    [InlineData(6, 6, 5336u)]   // U(6,6)+U(6,7) = 1683+3653 = 5336
    public void ComputeV_Matches_Known_Values(int n, int k, uint expected)
    {
        Assert.Equal(expected, CeltPvq.ComputeV(n, k));
    }

    [Fact]
    public void DecodePulsesAtIndex_With_Index_Zero_Places_All_Pulses_In_First_Dimension_Positive()
    {
        // Codeword 0 puts all K pulses positively in the first dimension.
        Span<int> y = stackalloc int[8];
        for (int n = 2; n <= 8; n++)
        {
            for (int k = 1; k <= 6; k++)
            {
                var slice = y.Slice(0, n);
                slice.Clear();
                CeltPvq.DecodePulsesAtIndex(n, k, 0, slice);
                Assert.Equal(k, slice[0]);
                for (int j = 1; j < n; j++)
                    Assert.Equal(0, slice[j]);
            }
        }
    }

    [Fact]
    public void DecodePulsesAtIndex_Throws_For_Out_Of_Range_Index()
    {
        Span<int> y = stackalloc int[4];
        uint v = CeltPvq.ComputeV(4, 4);
        // Stackalloc + ref struct prevents `Assert.Throws` lambda; check manually.
        bool threw = false;
        try { CeltPvq.DecodePulsesAtIndex(4, 4, v, y); }
        catch (ArgumentOutOfRangeException) { threw = true; }
        Assert.True(threw, "Expected ArgumentOutOfRangeException for index == V(n,k)");
    }

    [Fact]
    public void NcwrsUrow_Throws_When_Buffer_Too_Small()
    {
        var u = new uint[3]; // need k+2 = 12 for k=10
        Assert.Throws<ArgumentException>(() => CeltPvq.NcwrsUrow(5, 10, u));
    }

    [Fact]
    public void NcwrsUrow_Throws_For_Invalid_Dimensions()
    {
        var u = new uint[16];
        Assert.Throws<ArgumentOutOfRangeException>(() => CeltPvq.NcwrsUrow(1, 5, u));
        Assert.Throws<ArgumentOutOfRangeException>(() => CeltPvq.NcwrsUrow(5, 0, u));
    }

    [Fact]
    public void DecodePulses_Reads_From_Range_Coder_And_Produces_Valid_Vector()
    {
        // Encode a known index by hand into a range-coder payload, then
        // round-trip through DecodePulses + range decoder to verify the
        // entire stack hangs together.
        // We can't easily build a payload from scratch here, but we can
        // at least exercise the API surface to make sure it doesn't
        // throw for a syntactically valid stream. Use a single PVQ
        // codebook draw on a small zero-filled buffer (range coder will
        // pull index 0 effectively when fed all-zero bytes).
        var buffer = new byte[16];
        var dec = new OpusRangeDecoder(buffer);
        var y = new int[4];
        int yy = CeltPvq.DecodePulses(ref dec, 4, 3, y);

        int absSum = 0, sqSum = 0;
        for (int j = 0; j < 4; j++)
        {
            absSum += Math.Abs(y[j]);
            sqSum += y[j] * y[j];
        }
        Assert.Equal(3, absSum);
        Assert.Equal(yy, sqSum);
    }
}
