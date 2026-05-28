using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacIcsInfoTests
{
    private static readonly byte[] LongGroupSingleton = new byte[] { 1 };
    private static readonly byte[] EightShortUngrouped = new byte[] { 1, 1, 1, 1, 1, 1, 1, 1 };
    private static readonly byte[] EightShortAllGrouped = new byte[] { 8 };
    private static readonly byte[] EightShortMixed = new byte[] { 3, 1, 2, 1, 1 };

    private static byte[] ParseHelper_BuildLong(int maxSfb, int windowShape, bool predictor)
    {
        var w = new AacBitWriter();
        w.Write(0u, 1); // ics_reserved_bit
        w.Write((uint)AacWindowSequence.OnlyLong, 2);
        w.Write((uint)windowShape, 1);
        w.Write((uint)maxSfb, 6);
        w.Write(predictor ? 1u : 0u, 1);
        return w.ToArray();
    }

    private static byte[] ParseHelper_BuildShort(int maxSfb, int windowShape, byte sfg)
    {
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.EightShort, 2);
        w.Write((uint)windowShape, 1);
        w.Write((uint)maxSfb, 4);
        w.Write(sfg, 7);
        return w.ToArray();
    }

    private static bool RunParse(byte[] data, out AacIcsInfo? info)
    {
        var reader = new BitReader(data);
        return AacIcsInfo.TryParse(ref reader, out info);
    }

    [Fact]
    public void TryParse_LongSineWindow_Roundtrip()
    {
        var data = ParseHelper_BuildLong(maxSfb: 49, windowShape: 0, predictor: false);
        Assert.True(RunParse(data, out var info));
        Assert.NotNull(info);
        Assert.Equal(AacWindowSequence.OnlyLong, info!.WindowSequence);
        Assert.Equal(AacWindowShape.Sine, info.WindowShape);
        Assert.Equal(49, info.MaxSfb);
        Assert.Null(info.ScaleFactorGrouping);
        Assert.Equal(1, info.WindowGroupCount);
        Assert.Equal(LongGroupSingleton, info.WindowsPerGroup.ToArray());
        Assert.False(info.PredictorDataPresent);
    }

    [Fact]
    public void TryParse_LongKbdWindow_Roundtrip()
    {
        var data = ParseHelper_BuildLong(maxSfb: 0, windowShape: 1, predictor: false);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(AacWindowShape.KaiserBesselDerived, info!.WindowShape);
        Assert.Equal(0, info.MaxSfb);
    }

    [Fact]
    public void TryParse_LongMaxSfb_Roundtrip()
    {
        var data = ParseHelper_BuildLong(maxSfb: 63, windowShape: 0, predictor: false);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(63, info!.MaxSfb);
    }

    [Fact]
    public void TryParse_LongStart_Roundtrip()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.LongStart, 2);
        w.Write(0u, 1);
        w.Write(20u, 6);
        w.Write(0u, 1);
        Assert.True(RunParse(w.ToArray(), out var info));
        Assert.Equal(AacWindowSequence.LongStart, info!.WindowSequence);
        Assert.Equal(1, info.WindowGroupCount);
    }

    [Fact]
    public void TryParse_LongStop_Roundtrip()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.LongStop, 2);
        w.Write(0u, 1);
        w.Write(20u, 6);
        w.Write(0u, 1);
        Assert.True(RunParse(w.ToArray(), out var info));
        Assert.Equal(AacWindowSequence.LongStop, info!.WindowSequence);
    }

    [Fact]
    public void TryParse_EightShort_UngroupedGrouping_ProducesEightSingletonGroups()
    {
        // scale_factor_grouping = 0 -> every transition is "new group" -> 8 singletons.
        var data = ParseHelper_BuildShort(maxSfb: 5, windowShape: 0, sfg: 0b0000_0000);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(AacWindowSequence.EightShort, info!.WindowSequence);
        Assert.Equal(5, info.MaxSfb);
        Assert.Equal((byte)0b0000_0000, info.ScaleFactorGrouping);
        Assert.Equal(8, info.WindowGroupCount);
        Assert.Equal(EightShortUngrouped, info.WindowsPerGroup.ToArray());
    }

    [Fact]
    public void TryParse_EightShort_AllGroupedGrouping_ProducesSingleGroupOfEight()
    {
        // scale_factor_grouping = 0b1111111 -> every transition is "stay" -> one group of 8.
        var data = ParseHelper_BuildShort(maxSfb: 15, windowShape: 1, sfg: 0b0111_1111);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(AacWindowShape.KaiserBesselDerived, info!.WindowShape);
        Assert.Equal(15, info.MaxSfb);
        Assert.Equal(1, info.WindowGroupCount);
        Assert.Equal(EightShortAllGrouped, info.WindowsPerGroup.ToArray());
    }

    [Fact]
    public void TryParse_EightShort_MixedGrouping_DerivesCorrectSizes()
    {
        // scale_factor_grouping bits 6..0 = 1 1 0 0 1 0 0
        // w0 starts group 0; w1 (bit6=1) stay; w2 (bit5=1) stay; w3 (bit4=0) new;
        // w4 (bit3=0) new; w5 (bit2=1) stay; w6 (bit1=0) new; w7 (bit0=0) new.
        // -> sizes [3, 1, 2, 1, 1].
        var data = ParseHelper_BuildShort(maxSfb: 7, windowShape: 0, sfg: 0b0110_0100);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(5, info!.WindowGroupCount);
        Assert.Equal(EightShortMixed, info.WindowsPerGroup.ToArray());
        int sum = 0;
        foreach (var s in info.WindowsPerGroup.Span) sum += s;
        Assert.Equal(8, sum);
    }

    [Fact]
    public void TryParse_Long_PredictorSet_Rejected()
    {
        var data = ParseHelper_BuildLong(maxSfb: 10, windowShape: 0, predictor: true);
        Assert.False(RunParse(data, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_Truncated_Rejected()
    {
        // Single byte only -> 8 bits < 11 minimum.
        var data = new byte[] { 0xFF };
        Assert.False(RunParse(data, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_AllShortGroupingsSumToEight()
    {
        // Property: every possible scale_factor_grouping value yields
        // windows-per-group that sum to 8 and groups in [1, 8].
        for (int sfg = 0; sfg < 128; sfg++)
        {
            var data = ParseHelper_BuildShort(maxSfb: 0, windowShape: 0, sfg: (byte)sfg);
            Assert.True(RunParse(data, out var info), $"sfg={sfg:X2}");
            Assert.NotNull(info);
            Assert.InRange(info!.WindowGroupCount, 1, 8);
            int sum = 0;
            foreach (var s in info.WindowsPerGroup.Span) sum += s;
            Assert.Equal(8, sum);
        }
    }
}
