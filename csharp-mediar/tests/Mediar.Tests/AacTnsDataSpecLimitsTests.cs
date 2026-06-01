using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsDataSpecLimitsTests
{
    private static byte[] BuildLongSingleFilter(int order, int length = 16, bool coefRes = false)
    {
        var w = new AacBitWriter();
        w.Write(1u, 2);
        w.Write(coefRes ? 1u : 0u, 1);
        w.Write((uint)length, 6);
        w.Write((uint)order, 5);
        if (order > 0)
        {
            w.Write(0u, 1);
            w.Write(0u, 1);
            int coefBits = (coefRes ? 4 : 3);
            for (int i = 0; i < order; i++) w.Write(0u, coefBits);
        }
        return w.ToArray();
    }

    private static byte[] BuildShortNoFilters()
    {
        var w = new AacBitWriter();
        for (int win = 0; win < 8; win++) w.Write(0u, 1);
        return w.ToArray();
    }

    private static byte[] BuildShortSingleFilterFirstWindow(int order, int length = 5)
    {
        var w = new AacBitWriter();
        w.Write(1u, 1);
        w.Write(0u, 1);
        w.Write((uint)length, 4);
        w.Write((uint)order, 3);
        if (order > 0)
        {
            w.Write(0u, 1);
            w.Write(0u, 1);
            for (int i = 0; i < order; i++) w.Write(0u, 3);
        }
        for (int win = 1; win < 8; win++) w.Write(0u, 1);
        return w.ToArray();
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain, 0)]
    [InlineData(AacAudioObjectType.AacMain, 1)]
    [InlineData(AacAudioObjectType.AacMain, 20)]
    public void TryParse_LongMain_AcceptsOrderUpToMaxOrderLongMain(AacAudioObjectType aot, int order)
    {
        var bytes = BuildLongSingleFilter(order);
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(order, data!.Windows[0].Filters[0].Order);
    }

    [Theory]
    [InlineData(21)]
    [InlineData(22)]
    [InlineData(31)]
    public void TryParse_LongMain_RejectsOrderAboveMaxOrderLongMain(int order)
    {
        var bytes = BuildLongSingleFilter(order);
        Assert.False(AacTnsData.TryParse(
            bytes,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacMain,
            out var data));
        Assert.Null(data);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacLc, 0)]
    [InlineData(AacAudioObjectType.AacLc, 1)]
    [InlineData(AacAudioObjectType.AacLc, 12)]
    [InlineData(AacAudioObjectType.AacLtp, 12)]
    [InlineData(AacAudioObjectType.ErAacLc, 12)]
    public void TryParse_LongOther_AcceptsOrderUpToMaxOrderLongOther(AacAudioObjectType aot, int order)
    {
        var bytes = BuildLongSingleFilter(order);
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(order, data!.Windows[0].Filters[0].Order);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacLc, 13)]
    [InlineData(AacAudioObjectType.AacLc, 20)]
    [InlineData(AacAudioObjectType.AacLc, 31)]
    [InlineData(AacAudioObjectType.AacLtp, 13)]
    [InlineData(AacAudioObjectType.ErAacLc, 31)]
    public void TryParse_LongOther_RejectsOrderAboveMaxOrderLongOther(AacAudioObjectType aot, int order)
    {
        var bytes = BuildLongSingleFilter(order);
        Assert.False(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, aot, out var data));
        Assert.Null(data);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void TryParse_LongStart_HappyPath(AacAudioObjectType aot)
    {
        var bytes = BuildLongSingleFilter(7);
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.LongStart, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(AacWindowSequence.LongStart, data!.WindowSequence);
        Assert.Equal(7, data.Windows[0].Filters[0].Order);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void TryParse_LongStop_HappyPath(AacAudioObjectType aot)
    {
        var bytes = BuildLongSingleFilter(5);
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.LongStop, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(AacWindowSequence.LongStop, data!.WindowSequence);
        Assert.Equal(5, data.Windows[0].Filters[0].Order);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain, 0)]
    [InlineData(AacAudioObjectType.AacMain, 7)]
    [InlineData(AacAudioObjectType.AacLc, 0)]
    [InlineData(AacAudioObjectType.AacLc, 7)]
    public void TryParse_EightShort_AcceptsAllInRangeOrders(AacAudioObjectType aot, int order)
    {
        var bytes = BuildShortSingleFilterFirstWindow(order);
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(order, data!.Windows[0].Filters[0].Order);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacMain)]
    [InlineData(AacAudioObjectType.AacLc)]
    [InlineData(AacAudioObjectType.AacLtp)]
    [InlineData(AacAudioObjectType.ErAacLc)]
    public void TryParse_EightShort_NoFilters_AlwaysAccepted(AacAudioObjectType aot)
    {
        var bytes = BuildShortNoFilters();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, aot, out var data));
        Assert.NotNull(data);
        Assert.Equal(8, data!.Windows.Length);
        foreach (var w in data.Windows) Assert.Empty(w.Filters);
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    public void TryParse_UnsupportedAot_Throws(AacAudioObjectType aot)
    {
        var bytes = BuildLongSingleFilter(3);
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, aot, out _));
    }

    [Fact]
    public void TryParse_AotOverload_ParityWithBaseOverloadWhenAllOrdersInRange()
    {
        var bytes = BuildLongSingleFilter(10);

        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var baseData));
        Assert.True(AacTnsData.TryParse(
            bytes,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out var aotData));

        Assert.NotNull(baseData);
        Assert.NotNull(aotData);
        Assert.Equal(baseData!.BitsConsumed, aotData!.BitsConsumed);
        Assert.Equal(baseData.Windows.Length, aotData.Windows.Length);
        Assert.Equal(
            baseData.Windows[0].Filters[0].Order,
            aotData.Windows[0].Filters[0].Order);
        Assert.Equal(
            baseData.Windows[0].Filters[0].Length,
            aotData.Windows[0].Filters[0].Length);
    }

    [Fact]
    public void TryParse_AotOverload_OverLimitClearsDataToNull()
    {
        var bytes = BuildLongSingleFilter(15);

        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out _));
        Assert.False(AacTnsData.TryParse(
            bytes,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out var aotData));
        Assert.Null(aotData);
    }

    [Fact]
    public void TryParse_AotOverload_MainBoundary20Accepted_21Rejected()
    {
        var bytes20 = BuildLongSingleFilter(20);
        var bytes21 = BuildLongSingleFilter(21);

        Assert.True(AacTnsData.TryParse(
            bytes20,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacMain,
            out _));
        Assert.False(AacTnsData.TryParse(
            bytes21,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacMain,
            out _));
    }

    [Fact]
    public void TryParse_AotOverload_LcBoundary12Accepted_13Rejected()
    {
        var bytes12 = BuildLongSingleFilter(12);
        var bytes13 = BuildLongSingleFilter(13);

        Assert.True(AacTnsData.TryParse(
            bytes12,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out _));
        Assert.False(AacTnsData.TryParse(
            bytes13,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out _));
    }

    [Fact]
    public void TryParse_AotOverload_TwoFiltersSecondOverLimitRejects()
    {
        var w = new AacBitWriter();
        w.Write(2u, 2);
        w.Write(0u, 1);
        w.Write(10u, 6);
        w.Write(5u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1);
        for (int i = 0; i < 5; i++) w.Write(0u, 3);
        w.Write(8u, 6);
        w.Write(15u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1);
        for (int i = 0; i < 15; i++) w.Write(0u, 3);
        var bytes = w.ToArray();

        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out _));
        Assert.False(AacTnsData.TryParse(
            bytes,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out var aotData));
        Assert.Null(aotData);
    }

    [Fact]
    public void TryParse_AotOverload_TruncationStillReturnsFalse()
    {
        Assert.False(AacTnsData.TryParse(
            ReadOnlySpan<byte>.Empty,
            AacWindowSequence.OnlyLong,
            AacAudioObjectType.AacLc,
            out var data));
        Assert.Null(data);
    }
}
