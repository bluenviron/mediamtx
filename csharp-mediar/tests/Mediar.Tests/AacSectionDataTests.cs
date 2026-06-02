using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSectionDataTests
{
    private static AacIcsInfo BuildLongIcsInfo(int maxSfb) => new()
    {
        WindowSequence = AacWindowSequence.OnlyLong,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = null,
        WindowGroupCount = 1,
        WindowsPerGroup = new byte[] { 1 },
        PredictorDataPresent = false,
    };

    private static AacIcsInfo BuildShortIcsInfo(int maxSfb, int groupCount, byte[] windowsPerGroup) => new()
    {
        WindowSequence = AacWindowSequence.EightShort,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = 0,
        WindowGroupCount = groupCount,
        WindowsPerGroup = windowsPerGroup,
        PredictorDataPresent = false,
    };

    private static bool RunParse(byte[] data, AacIcsInfo icsInfo, out AacSectionData? result)
    {
        var reader = new BitReader(data);
        return AacSectionData.TryParse(ref reader, icsInfo, out result);
    }

    [Fact]
    public void TryParse_LongSingleSection_FullSpan()
    {
        // max_sfb = 30, single section spanning [0, 30) with codebook 3.
        var ics = BuildLongIcsInfo(30);
        var w = new AacBitWriter();
        w.Write(3u, 4);   // sect_cb = 3
        w.Write(30u, 5);  // sect_len = 30 (no escape needed since 30 < 31)
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Sections);
        var sec = data.Sections[0];
        Assert.Equal(0, sec.Group);
        Assert.Equal(3, sec.CodebookNumber);
        Assert.Equal(0, sec.StartSfb);
        Assert.Equal(30, sec.EndSfb);
    }

    [Fact]
    public void TryParse_LongMultipleSections_PartitionsMaxSfb()
    {
        // max_sfb = 10, two sections: [0, 4) cb=1, [4, 10) cb=11.
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(1u, 4); w.Write(4u, 5);
        w.Write(11u, 4); w.Write(6u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Equal(2, data!.Sections.Count);
        Assert.Equal(0, data.Sections[0].StartSfb);
        Assert.Equal(4, data.Sections[0].EndSfb);
        Assert.Equal(1, data.Sections[0].CodebookNumber);
        Assert.Equal(4, data.Sections[1].StartSfb);
        Assert.Equal(10, data.Sections[1].EndSfb);
        Assert.Equal(11, data.Sections[1].CodebookNumber);
    }

    [Fact]
    public void TryParse_LongEscapeChain_AccumulatesLength()
    {
        // max_sfb = 40. Section cb=5 with length 40 requires escape:
        //   first chunk = 31 (escape), second chunk = 9 -> total 40.
        var ics = BuildLongIcsInfo(40);
        var w = new AacBitWriter();
        w.Write(5u, 4);
        w.Write(31u, 5); // escape
        w.Write(9u, 5);  // remaining
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(0, data.Sections[0].StartSfb);
        Assert.Equal(40, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_LongMultipleEscapes_AccumulateCorrectly()
    {
        // max_sfb = 63. Length 63 = 31 + 31 + 1.
        var ics = BuildLongIcsInfo(63);
        var w = new AacBitWriter();
        w.Write(2u, 4);
        w.Write(31u, 5);
        w.Write(31u, 5);
        w.Write(1u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(63, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_ShortSequence_UsesThreeBitSectLen()
    {
        // EIGHT_SHORT: sect_len_incr = 3, escape = 7.
        // 2 groups (windows-per-group = [4, 4] hypothetical), max_sfb = 5.
        var ics = BuildShortIcsInfo(5, 2, new byte[] { 4, 4 });
        var w = new AacBitWriter();
        // Group 0: one section cb=7 len=5.
        w.Write(7u, 4); w.Write(5u, 3);
        // Group 1: two sections cb=3 len=2, cb=9 len=3.
        w.Write(3u, 4); w.Write(2u, 3);
        w.Write(9u, 4); w.Write(3u, 3);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Equal(3, data!.Sections.Count);
        Assert.Equal(0, data.Sections[0].Group);
        Assert.Equal(7, data.Sections[0].CodebookNumber);
        Assert.Equal(1, data.Sections[1].Group);
        Assert.Equal(3, data.Sections[1].CodebookNumber);
        Assert.Equal(1, data.Sections[2].Group);
        Assert.Equal(9, data.Sections[2].CodebookNumber);
    }

    [Fact]
    public void TryParse_ShortEscape_ChainsThreeBitChunks()
    {
        // max_sfb = 15. EIGHT_SHORT, one group. sect_len = 15 = 7+7+1.
        var ics = BuildShortIcsInfo(15, 1, new byte[] { 8 });
        var w = new AacBitWriter();
        w.Write(11u, 4);
        w.Write(7u, 3);
        w.Write(7u, 3);
        w.Write(1u, 3);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(15, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_MaxSfbZero_ReturnsEmpty()
    {
        var ics = BuildLongIcsInfo(0);
        var w = new AacBitWriter();
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.NotNull(data);
        Assert.Empty(data!.Sections);
    }

    [Fact]
    public void TryParse_SectionOverrun_Rejected()
    {
        // max_sfb = 10, section claims length 20 -> overruns.
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(3u, 4);
        w.Write(20u, 5);
        Assert.False(RunParse(w.ToArray(), ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_ZeroLengthSection_Rejected()
    {
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(3u, 4);
        w.Write(0u, 5);
        Assert.False(RunParse(w.ToArray(), ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_Truncated_Rejected()
    {
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(3u, 4); // cb only, no length follows
        Assert.False(RunParse(w.ToArray(), ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_TruncatedEscape_Rejected()
    {
        var ics = BuildLongIcsInfo(40);
        var w = new AacBitWriter();
        w.Write(2u, 4);
        w.Write(31u, 5); // escape, but no follow-up chunk
        Assert.False(RunParse(w.ToArray(), ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_MultipleGroupsLong_OnlyOneGroupExpected()
    {
        // A long sequence is always 1 group; if WindowGroupCount = 1
        // we read once. If we passed multiple groups (which shouldn't
        // happen for OnlyLong) the parser should still walk them.
        var ics = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 5,
            ScaleFactorGrouping = null,
            WindowGroupCount = 2,
            WindowsPerGroup = new byte[] { 1, 1 },
            PredictorDataPresent = false,
        };
        var w = new AacBitWriter();
        w.Write(1u, 4); w.Write(5u, 5); // group 0
        w.Write(2u, 4); w.Write(5u, 5); // group 1
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Equal(2, data!.Sections.Count);
        Assert.Equal(0, data.Sections[0].Group);
        Assert.Equal(1, data.Sections[1].Group);
    }

    [Fact]
    public void TryParse_NullIcsInfo_Throws_ArgumentNullException()
    {
        var reader = new BitReader(new byte[8]);
        try
        {
            AacSectionData.TryParse(ref reader, null!, out _);
            Assert.Fail("Expected ArgumentNullException");
        }
        catch (ArgumentNullException)
        {
            // expected
        }
    }

    [Fact]
    public void TryParse_NegativeMaxSfb_Returns_False()
    {
        var ics = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = -1,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        Assert.False(RunParse(new byte[4], ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void AacSection_Record_Equality()
    {
        var a = new AacSection { Group = 0, CodebookNumber = 3, StartSfb = 0, EndSfb = 5 };
        var b = new AacSection { Group = 0, CodebookNumber = 3, StartSfb = 0, EndSfb = 5 };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { EndSfb = 7 };
        Assert.NotEqual(a, c);
        Assert.Equal(7, c.EndSfb);
    }

    [Theory]
    [InlineData(AacWindowSequence.OnlyLong)]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.LongStop)]
    public void TryParse_AllLongWindowSequences_Use_5_Bit_SectLen(AacWindowSequence seq)
    {
        // sect_len_incr is 5 for any non-EightShort sequence.
        // Length 30 fits in 5 bits without escape; with 3-bit sect_len_incr (EightShort) it would not.
        var ics = new AacIcsInfo
        {
            WindowSequence = seq,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 30,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        var w = new AacBitWriter();
        w.Write(3u, 4);
        w.Write(30u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(30, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_EightShort_ThreeGroups_PartitionAsExpected()
    {
        // 3 groups, each with one section of cb=4 length=2 (max_sfb=2).
        var ics = BuildShortIcsInfo(2, 3, new byte[] { 3, 3, 2 });
        var w = new AacBitWriter();
        for (int g = 0; g < 3; g++)
        {
            w.Write(4u, 4);
            w.Write(2u, 3);
        }
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Equal(3, data!.Sections.Count);
        for (int g = 0; g < 3; g++)
        {
            Assert.Equal(g, data.Sections[g].Group);
            Assert.Equal(0, data.Sections[g].StartSfb);
            Assert.Equal(2, data.Sections[g].EndSfb);
            Assert.Equal(4, data.Sections[g].CodebookNumber);
        }
    }

    [Fact]
    public void TryParse_Long_DoubleEscape_PlusTail()
    {
        // max_sfb = 72. Length = 31 + 31 + 10 → 72.
        var ics = BuildLongIcsInfo(72);
        var w = new AacBitWriter();
        w.Write(5u, 4);
        w.Write(31u, 5);
        w.Write(31u, 5);
        w.Write(10u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(72, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_Long_SecondSection_Overrun_Rejected()
    {
        // max_sfb = 10. Section 1 takes [0, 5) cb=2; section 2 claims length 10 → overrun.
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(2u, 4); w.Write(5u, 5);
        w.Write(3u, 4); w.Write(10u, 5);
        Assert.False(RunParse(w.ToArray(), ics, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_Long_CodebookValue_15_Accepted()
    {
        // cb=15 (intensity stereo) is not validated by section parser - raw value passes through.
        var ics = BuildLongIcsInfo(5);
        var w = new AacBitWriter();
        w.Write(15u, 4);
        w.Write(5u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(15, data.Sections[0].CodebookNumber);
    }

    [Fact]
    public void TryParse_Long_CodebookValue_0_Accepted()
    {
        // cb=0 (ZERO_HCB) is also accepted as raw value.
        var ics = BuildLongIcsInfo(5);
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(5u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(0, data.Sections[0].CodebookNumber);
    }

    [Fact]
    public void TryParse_MaxSfb_Of_One_With_Single_Short_Section()
    {
        // Minimal valid EightShort: max_sfb=1, single group, one section length=1.
        var ics = BuildShortIcsInfo(1, 1, new byte[] { 8 });
        var w = new AacBitWriter();
        w.Write(2u, 4);
        w.Write(1u, 3);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Single(data!.Sections);
        Assert.Equal(0, data.Sections[0].StartSfb);
        Assert.Equal(1, data.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_Sections_All_Contiguous_StartEqualsPrevEnd()
    {
        // 3 sections in one group: [0,3) cb=1, [3,7) cb=4, [7,10) cb=11.
        var ics = BuildLongIcsInfo(10);
        var w = new AacBitWriter();
        w.Write(1u, 4); w.Write(3u, 5);
        w.Write(4u, 4); w.Write(4u, 5);
        w.Write(11u, 4); w.Write(3u, 5);
        Assert.True(RunParse(w.ToArray(), ics, out var data));
        Assert.Equal(3, data!.Sections.Count);
        for (int i = 1; i < data.Sections.Count; i++)
        {
            Assert.Equal(data.Sections[i - 1].EndSfb, data.Sections[i].StartSfb);
        }
        Assert.Equal(10, data.Sections[^1].EndSfb);
    }

    [Fact]
    public void TryParse_AacSectionData_Sections_Property_NonNull()
    {
        var ics = BuildLongIcsInfo(0);
        Assert.True(RunParse(Array.Empty<byte>(), ics, out var data));
        Assert.NotNull(data);
        Assert.NotNull(data!.Sections);
        Assert.Empty(data.Sections);
    }
}
