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

    [Fact]
    public void TryParse_EightShort_Sets_ScaleFactorGrouping_NonNull()
    {
        // Long sequences leave ScaleFactorGrouping null; EIGHT_SHORT
        // always populates it.
        var longBytes = ParseHelper_BuildLong(maxSfb: 5, windowShape: 0, predictor: false);
        Assert.True(RunParse(longBytes, out var longInfo));
        Assert.Null(longInfo!.ScaleFactorGrouping);

        var shortBytes = ParseHelper_BuildShort(maxSfb: 3, windowShape: 0, sfg: 0b0101_0101);
        Assert.True(RunParse(shortBytes, out var shortInfo));
        Assert.NotNull(shortInfo!.ScaleFactorGrouping);
    }

    [Fact]
    public void TryParse_EightShort_PredictorDataPresent_Is_Always_False()
    {
        // Predictor is forced false for the EIGHT_SHORT branch (no
        // predictor field exists for it in the spec).
        var data = ParseHelper_BuildShort(maxSfb: 3, windowShape: 0, sfg: 0b0011_1100);
        Assert.True(RunParse(data, out var info));
        Assert.False(info!.PredictorDataPresent);
    }

    [Fact]
    public void TryParse_LongStart_Sets_PredictorDataPresent_False()
    {
        // Predictor bit = 0 -> info populated, PredictorDataPresent stays false.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.LongStart, 2);
        w.Write(0u, 1);
        w.Write(20u, 6);
        w.Write(0u, 1);
        Assert.True(RunParse(w.ToArray(), out var info));
        Assert.False(info!.PredictorDataPresent);
    }

    [Fact]
    public void TryParse_LongStop_Predictor_Rejected_Like_OnlyLong()
    {
        // Phase-1 rejects predictor for *every* long window sequence,
        // not just OnlyLong.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.LongStop, 2);
        w.Write(0u, 1);
        w.Write(20u, 6);
        w.Write(1u, 1); // predictor = 1
        Assert.False(RunParse(w.ToArray(), out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_LongStart_Predictor_Rejected_Like_OnlyLong()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.LongStart, 2);
        w.Write(0u, 1);
        w.Write(20u, 6);
        w.Write(1u, 1);
        Assert.False(RunParse(w.ToArray(), out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_Zero_Length_Stream_Returns_False()
    {
        var reader = new BitReader(Array.Empty<byte>());
        Assert.False(AacIcsInfo.TryParse(ref reader, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void Record_With_Init_Replaces_WindowSequence()
    {
        var data = ParseHelper_BuildLong(maxSfb: 10, windowShape: 0, predictor: false);
        Assert.True(RunParse(data, out var info));
        var altered = info! with { WindowSequence = AacWindowSequence.EightShort };
        Assert.Equal(AacWindowSequence.OnlyLong, info!.WindowSequence);
        Assert.Equal(AacWindowSequence.EightShort, altered.WindowSequence);
    }

    [Fact]
    public void TryParse_EightShort_MaxSfb_Limit_Is_15()
    {
        // The 4-bit field caps at 15 (already enforced by the field
        // width); ensure that value is accepted.
        var data = ParseHelper_BuildShort(maxSfb: 15, windowShape: 0, sfg: 0);
        Assert.True(RunParse(data, out var info));
        Assert.Equal(15, info!.MaxSfb);
    }

    [Fact]
    public void TryParse_Advances_Reader_To_Expected_Position_For_Long()
    {
        var data = ParseHelper_BuildLong(maxSfb: 49, windowShape: 0, predictor: false);
        var reader = new BitReader(data);
        int beforeRemaining = reader.Remaining;
        Assert.True(AacIcsInfo.TryParse(ref reader, out _));
        // ics_info() long = 1 + 2 + 1 + 6 + 1 = 11 bits consumed.
        Assert.Equal(beforeRemaining - 11, reader.Remaining);
    }

    [Fact]
    public void TryParse_Advances_Reader_To_Expected_Position_For_EightShort()
    {
        var data = ParseHelper_BuildShort(maxSfb: 5, windowShape: 0, sfg: 0);
        var reader = new BitReader(data);
        int beforeRemaining = reader.Remaining;
        Assert.True(AacIcsInfo.TryParse(ref reader, out _));
        // ics_info() short = 1 + 2 + 1 + 4 + 7 = 15 bits consumed.
        Assert.Equal(beforeRemaining - 15, reader.Remaining);
    }

    [Fact]
    public void TryParse_WindowsPerGroup_Memory_Length_Matches_GroupCount()
    {
        for (int sfg = 0; sfg < 128; sfg++)
        {
            var data = ParseHelper_BuildShort(maxSfb: 0, windowShape: 0, sfg: (byte)sfg);
            Assert.True(RunParse(data, out var info));
            Assert.Equal(info!.WindowGroupCount, info.WindowsPerGroup.Length);
        }
    }
}
