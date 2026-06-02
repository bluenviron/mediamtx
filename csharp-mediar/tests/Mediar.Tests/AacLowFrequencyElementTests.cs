using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacLowFrequencyElementTests
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

    private static byte[] BuildCanonicalLfeBytes(int tag, byte globalGain, int maxSfb)
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

    [Fact]
    public void TryParse_CanonicalLongLfe_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 0, globalGain: 0x40, maxSfb: 6);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(0, lfe!.ElementInstanceTag);
        Assert.NotNull(lfe.Stream);
        Assert.Equal(0x40, lfe.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.OnlyLong, lfe.Stream.IcsInfo.WindowSequence);
        Assert.Equal(6, lfe.Stream.IcsInfo.MaxSfb);
        // 4 (tag) + 8 (global_gain) + 11 (long ics_info) + 9 (section) + 0 (no SF) + 3 (flags) = 35
        Assert.Equal(35, lfe.BitsConsumed);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(15)]
    public void TryParse_RoundTripsElementInstanceTag(int tag)
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag, globalGain: 0x80, maxSfb: 4);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(tag, lfe!.ElementInstanceTag);
    }

    [Fact]
    public void TryParse_RoundTripsGlobalGain()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 5, globalGain: 0xA5, maxSfb: 4);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.Equal(0xA5, lfe!.Stream.GlobalGain);
    }

    [Fact]
    public void TryParse_EmptyBuffer_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        Assert.False(AacLowFrequencyElement.TryParse(ReadOnlySpan<byte>.Empty, book, out var lfe));
        Assert.Null(lfe);
    }

    [Fact]
    public void TryParse_TruncatedAfterTagOnly_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(7u, 4);
        Assert.False(AacLowFrequencyElement.TryParse(w.ToArray(), book, out var lfe));
        Assert.Null(lfe);
    }

    [Fact]
    public void TryParse_TruncatedMidBody_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var full = BuildCanonicalLfeBytes(tag: 0, globalGain: 0x80, maxSfb: 6);
        var truncated = full.AsSpan(0, full.Length - 1).ToArray();
        Assert.False(AacLowFrequencyElement.TryParse(truncated, book, out var lfe));
        Assert.Null(lfe);
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

        Assert.True(AacLowFrequencyElement.TryParse(w.ToArray(), book, out var lfe));
        Assert.NotNull(lfe);
        Assert.True(lfe!.Stream.GainControlDataPresent);
        Assert.NotNull(lfe.Stream.GainControlData);
        Assert.Equal(0, lfe.Stream.GainControlData!.MaxBand);
        Assert.Empty(lfe.Stream.GainControlData.Bands);
    }

    [Fact]
    public void TryParse_NullCodebook_Throws()
    {
        var bytes = new byte[] { 0x00 };
        Assert.Throws<ArgumentNullException>(() =>
            AacLowFrequencyElement.TryParse(bytes, null!, out _));
    }

    [Fact]
    public void TryParse_ExposesUnderlyingIcsInfoAndSectionData()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 2, globalGain: 0x30, maxSfb: 8);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.NotNull(lfe);
        Assert.NotNull(lfe!.Stream.OwnIcsInfo);
        Assert.Single(lfe.Stream.SectionData.Sections);
        Assert.Equal(8, lfe.Stream.ScaleFactorData.Entries.Count);
        Assert.All(lfe.Stream.ScaleFactorData.Entries,
            e => Assert.Equal(AacScaleFactorKind.None, e.Kind));
    }

    [Fact]
    public void MaxElementInstanceTag_IsFifteen()
    {
        Assert.Equal(15, AacLowFrequencyElement.MaxElementInstanceTag);
    }

    [Fact]
    public void TryParse_BitstreamIsIdenticalToSce()
    {
        // LFE and SCE share the exact same bitstream shape per ISO/IEC
        // 14496-3 Table 4.4 / Table 4.6. The same byte sequence must
        // produce equivalent ElementInstanceTag / Stream.GlobalGain /
        // Stream.IcsInfo.MaxSfb / BitsConsumed when parsed as either.
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 4, globalGain: 0x55, maxSfb: 5);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));

        Assert.NotNull(lfe);
        Assert.NotNull(sce);
        Assert.Equal(sce!.ElementInstanceTag, lfe!.ElementInstanceTag);
        Assert.Equal(sce.Stream.GlobalGain, lfe.Stream.GlobalGain);
        Assert.Equal(sce.Stream.IcsInfo.MaxSfb, lfe.Stream.IcsInfo.MaxSfb);
        Assert.Equal(sce.BitsConsumed, lfe.BitsConsumed);
    }

    // ----- EightShort window LFE coverage -----
    //
    // Real encoders never emit EightShort on an LFE channel because LFE
    // is intrinsically low-bandwidth/long-window, but the LFE bitstream
    // is syntactically identical to SCE and the spec does not forbid it
    // at the parse layer. These tests verify the syntactic path stays
    // open and stays parity-equivalent with SCE.

    private static byte[] BuildCanonicalShortLfeBytes(int tag, byte globalGain, int maxSfb, byte grouping)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);          // element_instance_tag
        w.Write(globalGain, 8);         // global_gain
        WriteShortIcsInfo(w, maxSfb, grouping);
        WriteOneZeroShortSection(w, maxSfb);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);                 // pulse_data_present (must stay 0 for short)
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        return w.ToArray();
    }

    [Fact]
    public void TryParse_CanonicalShortLfe_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalShortLfeBytes(tag: 0, globalGain: 0x40, maxSfb: 4, grouping: 0x7F);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(AacWindowSequence.EightShort, lfe!.Stream.IcsInfo.WindowSequence);
        Assert.Equal(4, lfe.Stream.IcsInfo.MaxSfb);
        Assert.Equal(1, lfe.Stream.IcsInfo.WindowGroupCount);
        // 4 + 8 + 15 (short ics) + 7 (short section) + 0 + 3 = 37 bits.
        Assert.Equal(37, lfe.BitsConsumed);
    }

    [Fact]
    public void TryParse_ShortLfe_BitstreamIsIdenticalToSce()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalShortLfeBytes(tag: 6, globalGain: 0x77, maxSfb: 4, grouping: 0x7F);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.True(AacSingleChannelElement.TryParse(bytes, book, out var sce));

        Assert.NotNull(lfe);
        Assert.NotNull(sce);
        Assert.Equal(sce!.ElementInstanceTag, lfe!.ElementInstanceTag);
        Assert.Equal(sce.Stream.IcsInfo.WindowSequence, lfe.Stream.IcsInfo.WindowSequence);
        Assert.Equal(sce.Stream.IcsInfo.MaxSfb, lfe.Stream.IcsInfo.MaxSfb);
        Assert.Equal(sce.Stream.IcsInfo.WindowGroupCount, lfe.Stream.IcsInfo.WindowGroupCount);
        Assert.Equal(sce.BitsConsumed, lfe.BitsConsumed);
    }

    [Theory]
    [InlineData(0x00)]
    [InlineData(0x55)]
    [InlineData(0xAA)]
    [InlineData(0xFF)]
    public void TryParse_GlobalGain_RoundTripsAll4Bytes(byte gain)
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 0, globalGain: gain, maxSfb: 4);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.Equal(gain, lfe!.Stream.GlobalGain);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(8)]
    [InlineData(20)]
    [InlineData(30)]
    public void TryParse_LongLfe_HandlesAllMaxSfbValues(int maxSfb)
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 0, globalGain: 0x40, maxSfb: maxSfb);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.Equal(maxSfb, lfe!.Stream.IcsInfo.MaxSfb);
    }

    [Fact]
    public void TryParse_PulseDataPresent_IsRejectedOrAccepted_ConsistentlyWithSce()
    {
        // For long windows, pulse_data_present is legal. LFE bitstream
        // parity with SCE means the same syntax should be accepted in
        // both — verify that a minimal pulse_data parses through.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0x40u, 8);
        WriteLongIcsInfo(w, maxSfb: 4);
        WriteOneZeroSection(w, len: 4);
        w.Write(1u, 1); // pulse_data_present
        // pulse_data: number_pulse(2) + pulse_start_sfb(6) + 1*(offset(5)+amp(4))
        w.Write(0u, 2); // 1 pulse
        w.Write(0u, 6);
        w.Write(0u, 5);
        w.Write(0u, 4);
        w.Write(0u, 1); // tns
        w.Write(0u, 1); // gain control

        Assert.True(AacLowFrequencyElement.TryParse(w.ToArray(), book, out var lfe));
        Assert.True(AacSingleChannelElement.TryParse(w.ToArray(), book, out var sce));
        Assert.Equal(sce!.BitsConsumed, lfe!.BitsConsumed);
    }

    [Fact]
    public void TryParse_PreservesScaleFactorEntries_Length_EqualsMaxSfb()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCanonicalLfeBytes(tag: 1, globalGain: 0x42, maxSfb: 10);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, book, out var lfe));
        Assert.Equal(10, lfe!.Stream.ScaleFactorData.Entries.Count);
    }

    [Fact]
    public void TryParse_LfeHas_NoSeparateConstants_BeyondElementInstanceTag()
    {
        // MaxElementInstanceTag = 15 ⇒ a 4-bit field; SCE has the same bound.
        Assert.Equal(15, AacLowFrequencyElement.MaxElementInstanceTag);
        Assert.Equal(AacSingleChannelElement.MaxElementInstanceTag,
            AacLowFrequencyElement.MaxElementInstanceTag);
    }
}
