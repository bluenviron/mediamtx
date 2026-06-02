using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsSpecLimitsTests
{
    [Fact]
    public void MaxBandsLong1024_MatchesFFmpegTable()
    {
        var expected = new[] { 31, 31, 34, 40, 42, 51, 46, 46, 42, 42, 42, 39, 39 };
        for (int i = 0; i < expected.Length; i++)
        {
            Assert.Equal(expected[i], AacTnsSpecLimits.GetMaxBandsLong1024(i));
        }
    }

    [Fact]
    public void MaxBandsShort128_MatchesFFmpegTable()
    {
        var expected = new[] { 9, 9, 10, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14 };
        for (int i = 0; i < expected.Length; i++)
        {
            Assert.Equal(expected[i], AacTnsSpecLimits.GetMaxBandsShort128(i));
        }
    }

    [Fact]
    public void MaxBandsLong512_MatchesFFmpegTable()
    {
        var expected = new[] { 0, 0, 0, 31, 32, 37, 31, 31, 0, 0, 0, 0, 0 };
        for (int i = 0; i < expected.Length; i++)
        {
            Assert.Equal(expected[i], AacTnsSpecLimits.GetMaxBandsLong512(i));
        }
    }

    [Fact]
    public void MaxBandsLong480_MatchesFFmpegTable()
    {
        var expected = new[] { 0, 0, 0, 31, 32, 37, 30, 30, 0, 0, 0, 0, 0 };
        for (int i = 0; i < expected.Length; i++)
        {
            Assert.Equal(expected[i], AacTnsSpecLimits.GetMaxBandsLong480(i));
        }
    }

    [Fact]
    public void SampleRateIndexCount_Is13()
    {
        Assert.Equal(13, AacTnsSpecLimits.SampleRateIndexCount);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(13)]
    [InlineData(14)]
    [InlineData(15)]
    [InlineData(int.MaxValue)]
    public void MaxBandsLong1024_OutOfRangeSfi_Throws(int sfi)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBandsLong1024(sfi));
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(13)]
    [InlineData(15)]
    public void MaxBandsShort128_OutOfRangeSfi_Throws(int sfi)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBandsShort128(sfi));
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(13)]
    [InlineData(15)]
    public void MaxBandsLong512_OutOfRangeSfi_Throws(int sfi)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBandsLong512(sfi));
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(13)]
    [InlineData(15)]
    public void MaxBandsLong480_OutOfRangeSfi_Throws(int sfi)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBandsLong480(sfi));
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void GetMaxBands_LongWindow_UsesLong1024Table(AacAudioObjectType aot)
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            int expected = AacTnsSpecLimits.GetMaxBandsLong1024(sfi);
            Assert.Equal(
                expected,
                AacTnsSpecLimits.GetMaxBands(aot, sfi, AacWindowSequence.OnlyLong));
        }
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void GetMaxBands_ShortWindow_UsesShort128Table(AacAudioObjectType aot)
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            int expected = AacTnsSpecLimits.GetMaxBandsShort128(sfi);
            Assert.Equal(
                expected,
                AacTnsSpecLimits.GetMaxBands(aot, sfi, AacWindowSequence.EightShort));
        }
    }

    [Theory]
    [InlineData(AacWindowSequence.OnlyLong)]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.LongStop)]
    public void GetMaxBands_AllLongVariants_RouteToLong1024(AacWindowSequence ws)
    {
        int expected = AacTnsSpecLimits.GetMaxBandsLong1024(4);
        Assert.Equal(
            expected,
            AacTnsSpecLimits.GetMaxBands(AacAudioObjectType.AacLc, 4, ws));
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    [InlineData(AacAudioObjectType.TwinVq)]
    [InlineData(AacAudioObjectType.Celp)]
    [InlineData(AacAudioObjectType.Hvxc)]
    [InlineData(AacAudioObjectType.Ps)]
    [InlineData(AacAudioObjectType.Null)]
    public void GetMaxBands_UnsupportedAot_Throws(AacAudioObjectType aot)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBands(aot, 4, AacWindowSequence.OnlyLong));
    }

    [Fact]
    public void GetMaxBands_SupportedAotOutOfRangeSfi_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBands(
                AacAudioObjectType.AacLc, 15, AacWindowSequence.OnlyLong));
    }

    [Fact]
    public void GetMaxBands_UnknownWindowSequence_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBands(
                AacAudioObjectType.AacLc, 4, (AacWindowSequence)42));
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain, AacWindowSequence.OnlyLong, 20)]
    [InlineData(AacAudioObjectType.AacMain, AacWindowSequence.LongStart, 20)]
    [InlineData(AacAudioObjectType.AacMain, AacWindowSequence.LongStop, 20)]
    [InlineData(AacAudioObjectType.AacMain, AacWindowSequence.EightShort, 7)]
    [InlineData(AacAudioObjectType.AacLc, AacWindowSequence.OnlyLong, 12)]
    [InlineData(AacAudioObjectType.AacLc, AacWindowSequence.LongStart, 12)]
    [InlineData(AacAudioObjectType.AacLc, AacWindowSequence.LongStop, 12)]
    [InlineData(AacAudioObjectType.AacLc, AacWindowSequence.EightShort, 7)]
    [InlineData(AacAudioObjectType.AacLtp, AacWindowSequence.OnlyLong, 12)]
    [InlineData(AacAudioObjectType.AacLtp, AacWindowSequence.EightShort, 7)]
    [InlineData(AacAudioObjectType.ErAacLc, AacWindowSequence.OnlyLong, 12)]
    [InlineData(AacAudioObjectType.ErAacLc, AacWindowSequence.EightShort, 7)]
    public void GetMaxOrder_Cases(
        AacAudioObjectType aot, AacWindowSequence ws, int expected)
    {
        Assert.Equal(expected, AacTnsSpecLimits.GetMaxOrder(aot, ws));
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    [InlineData(AacAudioObjectType.Null)]
    public void GetMaxOrder_UnsupportedAot_Throws(AacAudioObjectType aot)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxOrder(aot, AacWindowSequence.OnlyLong));
    }

    [Fact]
    public void GetMaxOrder_UnknownWindowSequence_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxOrder(
                AacAudioObjectType.AacLc, (AacWindowSequence)42));
    }

    [Fact]
    public void Constants_HaveCanonicalValues()
    {
        Assert.Equal(7, AacTnsSpecLimits.MaxOrderShort);
        Assert.Equal(20, AacTnsSpecLimits.MaxOrderLongMain);
        Assert.Equal(12, AacTnsSpecLimits.MaxOrderLongOther);
    }

    [Fact]
    public void MaxBandsLong512_LdSupportedRates_AreNonZero()
    {
        for (int sfi = 3; sfi <= 7; sfi++)
        {
            Assert.True(AacTnsSpecLimits.GetMaxBandsLong512(sfi) > 0,
                $"LD 512 should support sfi={sfi}");
        }
        for (int sfi = 0; sfi <= 2; sfi++)
        {
            Assert.Equal(0, AacTnsSpecLimits.GetMaxBandsLong512(sfi));
        }
        for (int sfi = 8; sfi <= 12; sfi++)
        {
            Assert.Equal(0, AacTnsSpecLimits.GetMaxBandsLong512(sfi));
        }
    }

    [Fact]
    public void MaxBandsLong480_LdSupportedRates_AreNonZero()
    {
        for (int sfi = 3; sfi <= 7; sfi++)
        {
            Assert.True(AacTnsSpecLimits.GetMaxBandsLong480(sfi) > 0,
                $"LD 480 should support sfi={sfi}");
        }
        for (int sfi = 0; sfi <= 2; sfi++)
        {
            Assert.Equal(0, AacTnsSpecLimits.GetMaxBandsLong480(sfi));
        }
        for (int sfi = 8; sfi <= 12; sfi++)
        {
            Assert.Equal(0, AacTnsSpecLimits.GetMaxBandsLong480(sfi));
        }
    }

    [Fact]
    public void MaxBandsLong1024_NeverExceeds_SfbLimit_52()
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            int v = AacTnsSpecLimits.GetMaxBandsLong1024(sfi);
            Assert.InRange(v, 1, 52);
        }
    }

    [Fact]
    public void MaxBandsShort128_NeverExceeds_SfbLimit_15()
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            int v = AacTnsSpecLimits.GetMaxBandsShort128(sfi);
            Assert.InRange(v, 1, 15);
        }
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void GetMaxBands_LongStart_RoutesToLong1024(AacAudioObjectType aot)
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            Assert.Equal(
                AacTnsSpecLimits.GetMaxBandsLong1024(sfi),
                AacTnsSpecLimits.GetMaxBands(aot, sfi, AacWindowSequence.LongStart));
        }
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void GetMaxBands_LongStop_RoutesToLong1024(AacAudioObjectType aot)
    {
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            Assert.Equal(
                AacTnsSpecLimits.GetMaxBandsLong1024(sfi),
                AacTnsSpecLimits.GetMaxBands(aot, sfi, AacWindowSequence.LongStop));
        }
    }

    [Fact]
    public void GetMaxBands_NegativeSfi_Throws()
    {
        var ex = Assert.Throws<ArgumentOutOfRangeException>(
            () => AacTnsSpecLimits.GetMaxBands(
                AacAudioObjectType.AacLc, -1, AacWindowSequence.OnlyLong));
        Assert.Equal("samplingFrequencyIndex", ex.ParamName);
    }

    [Fact]
    public void GetMaxOrder_AacMain_LongStart_ReturnsMain()
    {
        Assert.Equal(
            AacTnsSpecLimits.MaxOrderLongMain,
            AacTnsSpecLimits.GetMaxOrder(
                AacAudioObjectType.AacMain, AacWindowSequence.LongStart));
    }

    [Fact]
    public void GetMaxOrder_AacMain_LongStop_ReturnsMain()
    {
        Assert.Equal(
            AacTnsSpecLimits.MaxOrderLongMain,
            AacTnsSpecLimits.GetMaxOrder(
                AacAudioObjectType.AacMain, AacWindowSequence.LongStop));
    }

    [Fact]
    public void Long512_Equals_Long480_NonZero_Layout()
    {
        // Long512 and Long480 share the same "supported" sfi window
        // even though the per-band counts differ.
        for (int sfi = 0; sfi < AacTnsSpecLimits.SampleRateIndexCount; sfi++)
        {
            bool a = AacTnsSpecLimits.GetMaxBandsLong512(sfi) > 0;
            bool b = AacTnsSpecLimits.GetMaxBandsLong480(sfi) > 0;
            Assert.Equal(a, b);
        }
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void GetMaxOrder_EightShort_ReturnsShort7_ForEverySupportedAot(
        AacAudioObjectType aot)
    {
        Assert.Equal(
            AacTnsSpecLimits.MaxOrderShort,
            AacTnsSpecLimits.GetMaxOrder(aot, AacWindowSequence.EightShort));
    }
}
