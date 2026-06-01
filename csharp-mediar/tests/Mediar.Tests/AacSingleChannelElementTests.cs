using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSingleChannelElementTests
{
    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static void WriteLongIcsInfo(AacBitWriter w, int maxSfb)
    {
        w.Write(0u, 1);                                     // ics_reserved_bit
        w.Write((uint)AacWindowSequence.OnlyLong, 2);       // window_sequence
        w.Write(0u, 1);                                     // window_shape (Sine)
        w.Write((uint)maxSfb, 6);                           // max_sfb
        w.Write(0u, 1);                                     // predictor_data_present
    }

    private static void WriteShortIcsInfo(AacBitWriter w, int maxSfb, byte grouping)
    {
        w.Write(0u, 1);                                     // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2);     // window_sequence
        w.Write(0u, 1);                                     // window_shape (Sine)
        w.Write((uint)maxSfb, 4);                           // max_sfb (4 bits for short)
        w.Write(grouping, 7);                               // scale_factor_grouping
    }

    private static void WriteOneZeroSection(AacBitWriter w, int len)
    {
        w.Write(0u, 4);                 // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)len, 5);          // sect_len (long: 5-bit increment)
    }

    private static void WriteOneZeroShortSection(AacBitWriter w, int len)
    {
        w.Write(0u, 4);                 // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)len, 3);          // sect_len (short: 3-bit increment, < escape 7)
    }

    private static byte[] BuildCanonicalSceBytes(int tag, byte globalGain, int maxSfb)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);          // element_instance_tag
        w.Write(globalGain, 8);         // global_gain
        WriteLongIcsInfo(w, maxSfb);
        WriteOneZeroSection(w, maxSfb);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        return w.ToArray();
    }

    private static byte[] BuildCanonicalShortSceBytes(int tag, byte globalGain, int maxSfb, byte grouping)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);          // element_instance_tag
        w.Write(globalGain, 8);         // global_gain
        WriteShortIcsInfo(w, maxSfb, grouping);
        WriteOneZeroShortSection(w, maxSfb);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        return w.ToArray();
    }

    [Fact]
    public void TryParse_CanonicalLongSce_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalSceBytes(tag: 0, globalGain: 0x40, maxSfb: 6);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(0, sce!.ElementInstanceTag);
        Assert.NotNull(sce.Stream);
        Assert.Equal(0x40, sce.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.OnlyLong, sce.Stream.IcsInfo.WindowSequence);
        Assert.Equal(6, sce.Stream.IcsInfo.MaxSfb);
        // 4 (tag) + 8 (global_gain) + 11 (long ics_info) + 9 (section) + 0 (no SF) + 3 (flags) = 35
        Assert.Equal(35, sce.BitsConsumed);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(15)]
    public void TryParse_RoundTripsElementInstanceTag(int tag)
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalSceBytes(tag, globalGain: 0x80, maxSfb: 4);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(tag, sce!.ElementInstanceTag);
    }

    [Fact]
    public void TryParse_RoundTripsGlobalGain()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalSceBytes(tag: 5, globalGain: 0xA5, maxSfb: 4);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.Equal(0xA5, sce!.Stream.GlobalGain);
    }

    [Fact]
    public void TryParse_EmptyBuffer_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        Assert.False(AacSingleChannelElement.TryParse(ReadOnlySpan<byte>.Empty, book, out var sce));
        Assert.Null(sce);
    }

    [Fact]
    public void TryParse_TruncatedAfterTagOnly_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        // Only 4 bits of payload: element_instance_tag, but no body
        // bits. The body needs global_gain (8 bits) next so this must fail.
        var w = new AacBitWriter();
        w.Write(7u, 4);
        Assert.False(AacSingleChannelElement.TryParse(w.ToArray(), book, out var sce));
        Assert.Null(sce);
    }

    [Fact]
    public void TryParse_TruncatedMidBody_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var full = BuildCanonicalSceBytes(tag: 0, globalGain: 0x80, maxSfb: 6);
        // Slice off the final byte to cut into the trailing flag block.
        var truncated = full.AsSpan(0, full.Length - 1).ToArray();
        Assert.False(AacSingleChannelElement.TryParse(truncated, book, out var sce));
        Assert.Null(sce);
    }

    [Fact]
    public void TryParse_GainControlDataPresent_ParsesEmptyGainControlData()
    {
        // gain_control_data_present = 1 with an empty (max_band = 0) gcd body parses cleanly.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(3u, 4);                 // element_instance_tag
        w.Write(0x40u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 4);
        WriteOneZeroSection(w, len: 4);
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(1u, 1);                 // gain_control_data_present = 1
        w.Write(0u, 2);                 // gain_control_data(): max_band = 0 (empty)

        Assert.True(AacSingleChannelElement.TryParse(w.ToArray(), book, out var sce));
        Assert.NotNull(sce);
        Assert.True(sce!.Stream.GainControlDataPresent);
        Assert.NotNull(sce.Stream.GainControlData);
        Assert.Equal(0, sce.Stream.GainControlData!.MaxBand);
        Assert.Empty(sce.Stream.GainControlData.Bands);
    }

    [Fact]
    public void TryParse_NullCodebook_Throws()
    {
        var bytes = new byte[] { 0x00 };
        Assert.Throws<ArgumentNullException>(() =>
            AacSingleChannelElement.TryParse(bytes, null!, out _));
    }

    [Fact]
    public void TryParse_ExposesUnderlyingIcsInfoAndSectionData()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalSceBytes(tag: 2, globalGain: 0x30, maxSfb: 8);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.NotNull(sce);
        Assert.NotNull(sce!.Stream.OwnIcsInfo);
        Assert.Single(sce.Stream.SectionData.Sections);
        Assert.Equal(8, sce.Stream.ScaleFactorData.Entries.Count);
        Assert.All(sce.Stream.ScaleFactorData.Entries,
            e => Assert.Equal(AacScaleFactorKind.None, e.Kind));
    }

    [Fact]
    public void MaxElementInstanceTag_IsFifteen()
    {
        Assert.Equal(15, AacSingleChannelElement.MaxElementInstanceTag);
    }

    // ----- EightShort window ICS path -----

    [Fact]
    public void TryParse_CanonicalShortSce_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        // grouping 0x7F = all 8 windows in one group → one section covering all SFBs.
        var bytes = BuildCanonicalShortSceBytes(tag: 0, globalGain: 0x40, maxSfb: 4, grouping: 0x7F);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(0, sce!.ElementInstanceTag);
        Assert.Equal(0x40, sce.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.EightShort, sce.Stream.IcsInfo.WindowSequence);
        Assert.Equal(4, sce.Stream.IcsInfo.MaxSfb);
        // 4 (tag) + 8 (gain) + 15 (short ics_info) + 7 (short section) + 0 (no SF) + 3 (flags) = 37
        Assert.Equal(37, sce.BitsConsumed);
    }

    [Fact]
    public void TryParse_ShortSce_OneGroup_ExposesSingleGroupGeometry()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalShortSceBytes(tag: 3, globalGain: 0x80, maxSfb: 4, grouping: 0x7F);

        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(1, sce!.Stream.IcsInfo.WindowGroupCount);
        Assert.Equal(new byte[] { 8 }, sce.Stream.IcsInfo.WindowsPerGroup.ToArray());
        // One ZERO_HCB section covering the only group.
        Assert.Single(sce.Stream.SectionData.Sections);
        Assert.Equal(0, sce.Stream.SectionData.Sections[0].CodebookNumber);
        Assert.Equal(0, sce.Stream.SectionData.Sections[0].StartSfb);
        Assert.Equal(4, sce.Stream.SectionData.Sections[0].EndSfb);
    }

    [Fact]
    public void TryParse_ShortSce_AllSeparateGroups_RequiresEightSections()
    {
        var book = BuildSyntheticSfCodebook();
        // grouping = 0 → eight singleton groups → section_data() must
        // emit one section per group.
        var w = new AacBitWriter();
        w.Write(2u, 4);                                     // element_instance_tag
        w.Write(0x10u, 8);                                  // global_gain
        WriteShortIcsInfo(w, maxSfb: 4, grouping: 0x00);
        for (int g = 0; g < 8; g++) WriteOneZeroShortSection(w, len: 4);
        w.Write(0u, 1);                                     // pulse_data_present
        w.Write(0u, 1);                                     // tns_data_present
        w.Write(0u, 1);                                     // gain_control_data_present

        Assert.True(AacSingleChannelElement.TryParse(w.ToArray(), book, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(8, sce!.Stream.IcsInfo.WindowGroupCount);
        Assert.Equal(8, sce.Stream.SectionData.Sections.Count);
    }

    [Fact]
    public void TryParse_ShortSce_PulseDataPresent_IsRejected()
    {
        // pulse_data() is forbidden when window_sequence == EIGHT_SHORT.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);                                     // element_instance_tag
        w.Write(0u, 8);                                     // global_gain
        WriteShortIcsInfo(w, maxSfb: 4, grouping: 0x7F);
        WriteOneZeroShortSection(w, len: 4);
        w.Write(1u, 1);                                     // pulse_data_present = 1 (illegal for short)
        // The aggregator rejects before reading the pulse_data() body.

        Assert.False(AacSingleChannelElement.TryParse(w.ToArray(), book, out var sce));
        Assert.Null(sce);
    }
}
