using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for the Phase 2a CELT foundation — <see cref="CeltMode"/>
/// band-layout derivation. The DSP itself lands in Phase 2b-d.
/// </summary>
public sealed class CeltModeTests
{
    // ----- ForCeltOnly: bandwidth → EndBand -----

    [Theory]
    [InlineData(OpusBandwidth.Narrowband,     13)]
    [InlineData(OpusBandwidth.Wideband,       17)]
    [InlineData(OpusBandwidth.SuperWideband,  19)]
    [InlineData(OpusBandwidth.Fullband,       21)]
    public void ForCeltOnly_Sets_EndBand_From_Bandwidth(OpusBandwidth bw, int expected)
    {
        var mode = CeltMode.ForCeltOnly(bw, 20_000);
        Assert.Equal(0, mode.StartBand);
        Assert.Equal(expected, mode.EndBand);
        Assert.Equal(expected, mode.BandCount);
        Assert.False(mode.IsHybrid);
    }

    [Fact]
    public void ForCeltOnly_Rejects_Mediumband()
    {
        Assert.Throws<ArgumentException>(() => CeltMode.ForCeltOnly(OpusBandwidth.Mediumband, 20_000));
    }

    // ----- ForCeltOnly: frame size → SamplesPerFrame / ShortBlocks -----

    [Theory]
    [InlineData(2_500,  120, 1)]
    [InlineData(5_000,  240, 2)]
    [InlineData(10_000, 480, 4)]
    [InlineData(20_000, 960, 8)]
    public void ForCeltOnly_Sets_Frame_Layout_From_FrameSize(int frameUs, int samples, int shortBlocks)
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, frameUs);
        Assert.Equal(samples, mode.SamplesPerFrame);
        Assert.Equal(shortBlocks, mode.ShortBlocksPerFrame);
    }

    [Fact]
    public void ForCeltOnly_Rejects_Unsupported_FrameSize()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 60_000));
    }

    // ----- ForHybrid -----

    [Theory]
    [InlineData(10_000, 480, 4)]
    [InlineData(20_000, 960, 8)]
    public void ForHybrid_Has_Hybrid_StartBand_And_Layout(int frameUs, int samples, int shortBlocks)
    {
        var mode = CeltMode.ForHybrid(frameUs);
        Assert.Equal(17, mode.StartBand);
        Assert.Equal(21, mode.EndBand);
        Assert.Equal(4, mode.BandCount);
        Assert.Equal(samples, mode.SamplesPerFrame);
        Assert.Equal(shortBlocks, mode.ShortBlocksPerFrame);
        Assert.True(mode.IsHybrid);
    }

    [Theory]
    [InlineData(2_500)]
    [InlineData(5_000)]
    [InlineData(40_000)]
    [InlineData(60_000)]
    public void ForHybrid_Rejects_NonHybrid_FrameSizes(int frameUs)
    {
        Assert.Throws<ArgumentException>(() => CeltMode.ForHybrid(frameUs));
    }

    // ----- EBands / BinsAtLongBlock -----

    [Fact]
    public void EBands_Has_MaxBands_Plus_One_Entries()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        Assert.Equal(22, mode.EBands.Length);
        Assert.Equal(0, mode.EBands[0]);
        Assert.Equal(100, mode.EBands[21]); // upper bin edge at 20 kHz (short-block units)
    }

    [Fact]
    public void EBands_Are_Strictly_Monotonic()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        for (int i = 1; i < mode.EBands.Length; i++)
        {
            Assert.True(mode.EBands[i] > mode.EBands[i - 1],
                $"EBands must be strictly increasing: EBands[{i - 1}]={mode.EBands[i - 1]}, EBands[{i}]={mode.EBands[i]}.");
        }
    }

    [Theory]
    [InlineData(2_500,  1)]
    [InlineData(5_000,  2)]
    [InlineData(10_000, 4)]
    [InlineData(20_000, 8)]
    public void BinsAtLongBlock_Scales_With_FrameSize(int frameUs, int scale)
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, frameUs);
        // EBands[21] = 100 short-block bins → 100 × scale long-block bins.
        Assert.Equal(100 * scale, mode.BinsAtLongBlock(100));
        Assert.Equal(0, mode.BinsAtLongBlock(0));
    }
}
