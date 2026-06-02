using Mediar.Codecs.Opus.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Smoke tests for the CELT Laplace decoder added in Phase 2b. These
/// exercise <see cref="OpusRangeDecoder.DecodeLaplace"/> in isolation so
/// that the higher-level coarse-energy code can rely on it.
/// </summary>
public sealed class OpusRangeDecoderLaplaceTests
{
    [Fact]
    public void DecodeLaplace_Returns_Zero_When_Centre_Bin_Wins()
    {
        // After init with all-zero buffer the range coder lands at the
        // top of its window (val ≈ rng - 1). DecodeBin(15) therefore
        // returns 0, which is below any positive fs — so Laplace must
        // return 0 (centre bin) with no further range reads.
        byte[] buffer = new byte[64];
        var rd = new OpusRangeDecoder(buffer);
        int v = rd.DecodeLaplace(fs: 16384, decay: 8192);
        Assert.Equal(0, v);
    }

    [Fact]
    public void DecodeLaplace_Can_Be_Called_Repeatedly_Without_Underflow()
    {
        // Issue 21 consecutive Laplace decodes — matches the worst case
        // the CELT coarse-energy loop drives. The decoder must not
        // crash, must not over-consume, and must produce values within
        // the legal signed range.
        byte[] buffer = new byte[128];
        var rd = new OpusRangeDecoder(buffer);
        for (int i = 0; i < 21; i++)
        {
            int v = rd.DecodeLaplace(fs: 5376, decay: 7744);
            Assert.InRange(v, short.MinValue, short.MaxValue);
        }
        Assert.False(rd.HasError);
    }

    [Fact]
    public void DecodeLaplace_Advances_Tell_Counter()
    {
        byte[] buffer = new byte[32];
        var rd = new OpusRangeDecoder(buffer);
        int tellBefore = rd.Tell();
        rd.DecodeLaplace(fs: 8192, decay: 16000);
        int tellAfter = rd.Tell();
        Assert.True(tellAfter > tellBefore,
            $"Tell did not advance: before={tellBefore} after={tellAfter}");
    }
}
