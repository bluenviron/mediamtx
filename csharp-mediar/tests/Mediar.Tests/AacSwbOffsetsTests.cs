using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSwbOffsetsTests
{
    // Sample rates that are explicitly catalogued by ISO/IEC 14496-3
    // and the expected long-block SWB count, short-block SWB count,
    // and the table-sharing companion sample rate (or 0 if the rate
    // is the sole user of the table).
    [Theory]
    [InlineData(96_000, 41, 12, 88_200)]
    [InlineData(88_200, 41, 12, 96_000)]
    [InlineData(64_000, 47, 12, 0)]
    [InlineData(48_000, 49, 14, 44_100)]
    [InlineData(44_100, 49, 14, 48_000)]
    [InlineData(32_000, 51, 14, 0)]
    [InlineData(24_000, 47, 15, 22_050)]
    [InlineData(22_050, 47, 15, 24_000)]
    [InlineData(16_000, 43, 15, 12_000)]
    [InlineData(12_000, 43, 15, 16_000)]
    [InlineData(11_025, 43, 15, 16_000)]
    [InlineData(8_000, 40, 15, 7_350)]
    [InlineData(7_350, 40, 15, 8_000)]
    public void GetNumSwb_MatchesSpecCatalogue(
        int sampleRate, int expectedLongSwb, int expectedShortSwb, int sharedWith)
    {
        Assert.Equal(expectedLongSwb, AacSwbOffsets.GetNumSwbLong(sampleRate));
        Assert.Equal(expectedShortSwb, AacSwbOffsets.GetNumSwbShort(sampleRate));

        if (sharedWith == 0) return;

        // Verify table-sharing: paired sample rates must yield the
        // identical span (reference-equal underlying array).
        var aLong = AacSwbOffsets.GetLongOffsets(sampleRate);
        var bLong = AacSwbOffsets.GetLongOffsets(sharedWith);
        Assert.True(aLong.SequenceEqual(bLong));

        var aShort = AacSwbOffsets.GetShortOffsets(sampleRate);
        var bShort = AacSwbOffsets.GetShortOffsets(sharedWith);
        Assert.True(aShort.SequenceEqual(bShort));
    }

    [Fact]
    public void GetLongOffsets_AllKnownRates_TerminateAt1024()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            var span = AacSwbOffsets.GetLongOffsets(rate);
            Assert.False(span.IsEmpty, $"sampleRate {rate} produced empty long table");
            Assert.Equal(0, span[0]);
            Assert.Equal(AacSwbOffsets.LongTransformLength, span[^1]);
        }
    }

    [Fact]
    public void GetShortOffsets_AllKnownRates_TerminateAt128()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            var span = AacSwbOffsets.GetShortOffsets(rate);
            Assert.False(span.IsEmpty, $"sampleRate {rate} produced empty short table");
            Assert.Equal(0, span[0]);
            Assert.Equal(AacSwbOffsets.ShortTransformLength, span[^1]);
        }
    }

    [Fact]
    public void GetLongOffsets_AllKnownRates_StrictlyMonotonicIncreasing()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            var span = AacSwbOffsets.GetLongOffsets(rate);
            for (int i = 1; i < span.Length; i++)
            {
                Assert.True(span[i] > span[i - 1],
                    $"sampleRate {rate} long SWB[{i}] = {span[i]} not greater than SWB[{i - 1}] = {span[i - 1]}");
            }
        }
    }

    [Fact]
    public void GetShortOffsets_AllKnownRates_StrictlyMonotonicIncreasing()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            var span = AacSwbOffsets.GetShortOffsets(rate);
            for (int i = 1; i < span.Length; i++)
            {
                Assert.True(span[i] > span[i - 1],
                    $"sampleRate {rate} short SWB[{i}] = {span[i]} not greater than SWB[{i - 1}] = {span[i - 1]}");
            }
        }
    }

    [Fact]
    public void GetLongOffsets_48k_MatchesAnnex4ATable()
    {
        // Spot-check a non-trivial subset of the 48 kHz long table
        // (ISO/IEC 14496-3 Table 4.140) against the canonical values.
        int[] expected =
        [
                0,    4,    8,   12,   16,   20,   24,   28,
               32,   36,   40,   48,   56,   64,   72,   80,
               88,   96,  108,  120,  132,  144,  160,  176,
              196,  216,  240,  264,  292,  320,  352,  384,
              416,  448,  480,  512,  544,  576,  608,  640,
              672,  704,  736,  768,  800,  832,  864,  896,
              928, 1024,
        ];
        Assert.True(AacSwbOffsets.GetLongOffsets(48_000).SequenceEqual(expected));
    }

    [Fact]
    public void GetShortOffsets_48k_MatchesAnnex4ATable()
    {
        // 48 kHz short table (Table 4.140).
        int[] expected = [0, 4, 8, 12, 16, 20, 28, 36, 44, 56, 68, 80, 96, 112, 128];
        Assert.True(AacSwbOffsets.GetShortOffsets(48_000).SequenceEqual(expected));
    }

    [Fact]
    public void GetLongOffsets_8k_MatchesAnnex4ATable()
    {
        // 8 kHz long table (Table 4.146) - the narrowest sample-rate
        // entry with the largest per-SWB widths.
        int[] expected =
        [
                0,   12,   24,   36,   48,   60,   72,   84,
               96,  108,  120,  132,  144,  156,  172,  188,
              204,  220,  236,  252,  268,  288,  308,  328,
              348,  372,  396,  420,  448,  476,  508,  544,
              580,  620,  664,  712,  764,  820,  880,  944,
             1024,
        ];
        Assert.True(AacSwbOffsets.GetLongOffsets(8_000).SequenceEqual(expected));
    }

    [Fact]
    public void GetLongOffsets_UnknownRate_ReturnsEmpty()
    {
        Assert.True(AacSwbOffsets.GetLongOffsets(192_000).IsEmpty);
        Assert.True(AacSwbOffsets.GetLongOffsets(0).IsEmpty);
        Assert.True(AacSwbOffsets.GetLongOffsets(-1).IsEmpty);
    }

    [Fact]
    public void GetShortOffsets_UnknownRate_ReturnsEmpty()
    {
        Assert.True(AacSwbOffsets.GetShortOffsets(192_000).IsEmpty);
        Assert.True(AacSwbOffsets.GetShortOffsets(0).IsEmpty);
        Assert.True(AacSwbOffsets.GetShortOffsets(-1).IsEmpty);
    }

    [Fact]
    public void GetNumSwb_UnknownRate_ReturnsZero()
    {
        Assert.Equal(0, AacSwbOffsets.GetNumSwbLong(192_000));
        Assert.Equal(0, AacSwbOffsets.GetNumSwbShort(192_000));
    }

    [Fact]
    public void GetLongOffsetsForIndex_DispatchesViaSampleRateTable()
    {
        // samplingFrequencyIndex 3 → 48 000 Hz per AacSampleRates.
        var byIndex = AacSwbOffsets.GetLongOffsetsForIndex(3);
        var byRate = AacSwbOffsets.GetLongOffsets(48_000);
        Assert.True(byIndex.SequenceEqual(byRate));
    }

    [Fact]
    public void GetShortOffsetsForIndex_DispatchesViaSampleRateTable()
    {
        // samplingFrequencyIndex 6 → 24 000 Hz per AacSampleRates.
        var byIndex = AacSwbOffsets.GetShortOffsetsForIndex(6);
        var byRate = AacSwbOffsets.GetShortOffsets(24_000);
        Assert.True(byIndex.SequenceEqual(byRate));
    }

    [Fact]
    public void GetLongOffsetsForIndex_EscapeIndex_ReturnsEmpty()
    {
        // Index 15 is the escape value (sample rate provided inline) -
        // there is no static table for it.
        Assert.True(AacSwbOffsets.GetLongOffsetsForIndex(AacSampleRates.EscapeIndex).IsEmpty);
        Assert.True(AacSwbOffsets.GetShortOffsetsForIndex(AacSampleRates.EscapeIndex).IsEmpty);
    }

    [Theory]
    [InlineData(48_000, 0, 4)]      // First long SWB is 4 wide.
    [InlineData(48_000, 11, 8)]     // SWB 11 transitions 40 → 48 (width 8).
    [InlineData(48_000, 48, 96)]    // Last long SWB is 928 → 1024 (width 96).
    [InlineData(8_000, 0, 12)]      // 8 kHz first long SWB is 12 wide.
    [InlineData(96_000, 40, 64)]    // 96 kHz last long SWB is 960 → 1024.
    public void GetLongSwbWidth_KnownValues(int sampleRate, int swb, int expectedWidth)
    {
        Assert.Equal(expectedWidth, AacSwbOffsets.GetLongSwbWidth(sampleRate, swb));
    }

    [Theory]
    [InlineData(48_000, 0, 4)]
    [InlineData(48_000, 13, 16)]    // 48 kHz short SWB 13 is 112 → 128 (width 16).
    [InlineData(24_000, 0, 4)]
    [InlineData(8_000, 14, 20)]     // 8 kHz short SWB 14 is 108 → 128 (width 20).
    public void GetShortSwbWidth_KnownValues(int sampleRate, int swb, int expectedWidth)
    {
        Assert.Equal(expectedWidth, AacSwbOffsets.GetShortSwbWidth(sampleRate, swb));
    }

    [Fact]
    public void GetLongSwbWidth_OutOfRangeOrUnknownRate_ReturnsZero()
    {
        Assert.Equal(0, AacSwbOffsets.GetLongSwbWidth(48_000, -1));
        Assert.Equal(0, AacSwbOffsets.GetLongSwbWidth(48_000, 49));    // num_swb = 49 → max index 48.
        Assert.Equal(0, AacSwbOffsets.GetLongSwbWidth(192_000, 0));
    }

    [Fact]
    public void GetShortSwbWidth_OutOfRangeOrUnknownRate_ReturnsZero()
    {
        Assert.Equal(0, AacSwbOffsets.GetShortSwbWidth(48_000, -1));
        Assert.Equal(0, AacSwbOffsets.GetShortSwbWidth(48_000, 14));   // num_swb = 14 → max index 13.
        Assert.Equal(0, AacSwbOffsets.GetShortSwbWidth(192_000, 0));
    }

    [Fact]
    public void LongSwbWidths_SumToTransformLength_AllRates()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            int sum = 0;
            int num = AacSwbOffsets.GetNumSwbLong(rate);
            for (int s = 0; s < num; s++) sum += AacSwbOffsets.GetLongSwbWidth(rate, s);
            Assert.Equal(AacSwbOffsets.LongTransformLength, sum);
        }
    }

    [Fact]
    public void ShortSwbWidths_SumToTransformLength_AllRates()
    {
        int[] rates = [96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000, 7_350];
        foreach (var rate in rates)
        {
            int sum = 0;
            int num = AacSwbOffsets.GetNumSwbShort(rate);
            for (int s = 0; s < num; s++) sum += AacSwbOffsets.GetShortSwbWidth(rate, s);
            Assert.Equal(AacSwbOffsets.ShortTransformLength, sum);
        }
    }

    [Fact]
    public void ShortTable_32kHz_SharesWith48kHz()
    {
        // 32 kHz long uses its own table (51 SWBs vs 49), but the
        // short table is shared with 48 kHz per libfaad's dispatch.
        Assert.Equal(51, AacSwbOffsets.GetNumSwbLong(32_000));
        Assert.Equal(14, AacSwbOffsets.GetNumSwbShort(32_000));
        Assert.True(AacSwbOffsets.GetShortOffsets(32_000)
            .SequenceEqual(AacSwbOffsets.GetShortOffsets(48_000)));
    }
}
