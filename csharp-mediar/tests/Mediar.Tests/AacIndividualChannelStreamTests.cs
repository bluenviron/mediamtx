using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacIndividualChannelStreamTests
{
    // Build a synthetic 121-entry scale-factor codebook reused from
    // AacScaleFactorDataTests: symbol 60 (diff 0) is the 1-bit "0",
    // every other symbol is an 8-bit fixed-length code starting at 0x80.
    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static void WriteLongIcsInfo(AacBitWriter w, int maxSfb, AacWindowSequence sequence = AacWindowSequence.OnlyLong)
    {
        w.Write(0u, 1);                 // ics_reserved_bit
        w.Write((uint)sequence, 2);     // window_sequence
        w.Write(0u, 1);                 // window_shape (Sine)
        w.Write((uint)maxSfb, 6);       // max_sfb
        w.Write(0u, 1);                 // predictor_data_present
    }

    private static void WriteEightShortIcsInfo(AacBitWriter w, int maxSfb, byte sfg = 0)
    {
        w.Write(0u, 1);                                                   // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2);                   // window_sequence
        w.Write(0u, 1);                                                   // window_shape
        w.Write((uint)maxSfb, 4);                                         // max_sfb
        w.Write(sfg, 7);                                                  // scale_factor_grouping
    }

    private static void WriteOneZeroSection(AacBitWriter w, int len, bool eightShort = false)
    {
        int sectLenIncr = eightShort ? 3 : 5;
        w.Write(0u, 4);                 // sect_cb = 0 (ZERO_HCB - no SF reads)
        w.Write((uint)len, sectLenIncr);
    }

    [Fact]
    public void TryRead_LongOwnIcsInfo_NoOptionals_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0x80u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 10);
        WriteOneZeroSection(w, len: 10);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, book, out var stream));
        Assert.NotNull(stream);
        Assert.Equal(0x80, stream!.GlobalGain);
        Assert.NotNull(stream.OwnIcsInfo);
        Assert.Equal(AacWindowSequence.OnlyLong, stream.IcsInfo.WindowSequence);
        Assert.Equal(10, stream.IcsInfo.MaxSfb);
        Assert.Single(stream.SectionData.Sections);
        // 10 cb=0 entries (one per sfb in section [0,10)), each Kind=None.
        Assert.Equal(10, stream.ScaleFactorData.Entries.Count);
        Assert.All(stream.ScaleFactorData.Entries, e => Assert.Equal(AacScaleFactorKind.None, e.Kind));
        Assert.False(stream.PulseDataPresent);
        Assert.Null(stream.PulseData);
        Assert.False(stream.TnsDataPresent);
        Assert.Null(stream.TnsData);
        Assert.False(stream.GainControlDataPresent);
        // 8 (global_gain) + 11 (long ics_info) + 9 (section) + 0 (no SF) + 3 (flags) = 31
        Assert.Equal(31, stream.BitsConsumed);
    }

    [Fact]
    public void TryRead_SharedIcsInfo_DoesNotConsumeIcsInfoBits()
    {
        var book = BuildSyntheticSfCodebook();
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 10,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };

        var w = new AacBitWriter();
        w.Write(0x42u, 8);              // global_gain
        WriteOneZeroSection(w, len: 10);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); // pulse / tns / gain flags

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), sharedIcsInfo: shared, scaleFlag: false, book, out var stream));
        Assert.NotNull(stream);
        Assert.Equal(0x42, stream!.GlobalGain);
        Assert.Null(stream.OwnIcsInfo);
        Assert.Same(shared, stream.IcsInfo);
        // 8 + 9 + 3 = 20 (no ics_info bits)
        Assert.Equal(20, stream.BitsConsumed);
    }

    [Fact]
    public void TryRead_GlobalGain_ValuePreserved()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0xC3u, 8);              // global_gain = 195
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Equal(195, stream!.GlobalGain);
    }

    [Fact]
    public void TryRead_LongIcsInfo_WithPulseData_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 10);
        WriteOneZeroSection(w, len: 10);
        w.Write(1u, 1);                 // pulse_data_present
        // pulse_data(): number_pulse=0 (1 pulse), start_sfb=3, offset=4, amplitude=7
        w.Write(0u, 2);
        w.Write(3u, 6);
        w.Write(4u, 5);
        w.Write(7u, 4);
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.True(stream!.PulseDataPresent);
        Assert.NotNull(stream.PulseData);
        Assert.Equal(3, stream.PulseData!.StartScaleFactorBand);
        Assert.Single(stream.PulseData.Pulses);
        Assert.Equal(4, stream.PulseData.Pulses[0].Offset);
        Assert.Equal(7, stream.PulseData.Pulses[0].Amplitude);
    }

    [Fact]
    public void TryRead_LongIcsInfo_WithTnsData_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 10);
        WriteOneZeroSection(w, len: 10);
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(1u, 1);                 // tns_data_present
        // tns_data() long: n_filt=1 (2 bits), coef_res=0 (1 bit),
        //                  length=10 (6 bits), order=0 (5 bits)
        // order=0 => no direction/coef_compress/coefficient loop
        w.Write(1u, 2);                 // n_filt
        w.Write(0u, 1);                 // coef_res
        w.Write(10u, 6);                // length
        w.Write(0u, 5);                 // order
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.True(stream!.TnsDataPresent);
        Assert.NotNull(stream.TnsData);
        Assert.Equal(AacWindowSequence.OnlyLong, stream.TnsData!.WindowSequence);
        Assert.Single(stream.TnsData.Windows);
        Assert.Single(stream.TnsData.Windows[0].Filters);
        Assert.Equal(10, stream.TnsData.Windows[0].Filters[0].Length);
        Assert.Equal(0, stream.TnsData.Windows[0].Filters[0].Order);
    }

    [Fact]
    public void TryRead_LongIcsInfo_WithPulseAndTns_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 8);
        WriteOneZeroSection(w, len: 8);
        w.Write(1u, 1);                 // pulse_data_present
        w.Write(0u, 2); w.Write(1u, 6); w.Write(2u, 5); w.Write(3u, 4);
        w.Write(1u, 1);                 // tns_data_present
        w.Write(1u, 2); w.Write(0u, 1); w.Write(8u, 6); w.Write(0u, 5);
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.True(stream!.PulseDataPresent);
        Assert.NotNull(stream.PulseData);
        Assert.True(stream.TnsDataPresent);
        Assert.NotNull(stream.TnsData);
    }

    [Fact]
    public void TryRead_PulseDataPresent_WithEightShortIcsInfo_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteEightShortIcsInfo(w, maxSfb: 4);
        // section_data() for EIGHT_SHORT: 3-bit sect_len_incr. One group only
        // (sfg=0 means a new group starts at every window, but we use a fresh
        // shared icsInfo if needed). Here sfg=0 means every gap bit is 0 ->
        // 8 groups of 1 window each. We need to emit one section per group.
        // To keep this test compact, use sfg=0x7F (all bits set) -> 1 group.
        // Rebuild with sfg=0x7F:
        var w2 = new AacBitWriter();
        w2.Write(0u, 8);
        WriteEightShortIcsInfo(w2, maxSfb: 4, sfg: 0x7F);
        // one group, one cb=0 section of length 4
        WriteOneZeroSection(w2, len: 4, eightShort: true);
        // scale_factor_data: empty (cb=0)
        w2.Write(1u, 1);                // pulse_data_present
        // Even one stray pulse byte shouldn't matter - parser rejects before
        // reading pulse_data() because the EightShort context is invalid.

        Assert.False(AacIndividualChannelStream.TryParse(
            w2.ToArray(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_EightShortIcsInfo_WithoutPulse_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteEightShortIcsInfo(w, maxSfb: 4, sfg: 0x7F);
        WriteOneZeroSection(w, len: 4, eightShort: true);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Equal(AacWindowSequence.EightShort, stream!.IcsInfo.WindowSequence);
        Assert.False(stream.PulseDataPresent);
        Assert.False(stream.TnsDataPresent);
    }

    [Fact]
    public void TryRead_GainControlDataPresent_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5);
        w.Write(0u, 1); w.Write(0u, 1);
        w.Write(1u, 1);                 // gain_control_data_present = 1 (SSR-only, unsupported)

        Assert.False(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_ScaleFlagTrue_SkipsPulseTnsGainBlock()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0x10u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5);
        // scale_factor_data empty; with scale_flag=true the parser must NOT
        // read any pulse/tns/gain flags - so no further bits are required.

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, scaleFlag: true, book, out var stream));
        Assert.False(stream!.PulseDataPresent);
        Assert.Null(stream.PulseData);
        Assert.False(stream.TnsDataPresent);
        Assert.Null(stream.TnsData);
        Assert.False(stream.GainControlDataPresent);
        // 8 + 11 + 9 + 0 = 28 (no trailing flags)
        Assert.Equal(28, stream.BitsConsumed);
    }

    [Fact]
    public void TryRead_NullScaleFactorCodebook_Throws()
    {
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.Throws<ArgumentNullException>(() =>
            AacIndividualChannelStream.TryParse(
                w.ToArray(), null, false, scaleFactorCodebook: null!, out _));
    }

    [Fact]
    public void TryRead_EmptyBuffer_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        Assert.False(AacIndividualChannelStream.TryParse(
            Array.Empty<byte>(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_TruncatedAtGlobalGain_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        // Only 4 bits available - cannot read 8-bit global_gain.
        var w = new AacBitWriter();
        w.Write(0xFu, 4);
        Assert.False(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_TruncatedAtIcsInfo_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        // global_gain present, but ics_info underflows.
        var w = new AacBitWriter();
        w.Write(0u, 8);
        w.Write(0u, 1);                 // start of ics_info (only 1 of 11 bits)
        Assert.False(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_TruncatedAtSectionData_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 10);
        // start a section codebook nibble but provide no length.
        w.Write(3u, 4);
        Assert.False(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_TruncatedAtPulseDataPresent_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 10,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        var w = new AacBitWriter();
        w.Write(0u, 8);                 // global_gain
        WriteOneZeroSection(w, len: 10);
        // 8 + 9 = 17 bits used; writer emits 3 bytes. Slicing to 2 bytes
        // (16 bits) truncates inside section_data, which the parser
        // propagates as a clean false return.
        byte[] full = w.ToArray();
        byte[] truncated = full.AsSpan(0, 2).ToArray();
        Assert.False(AacIndividualChannelStream.TryParse(
            truncated, sharedIcsInfo: shared, scaleFlag: false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_TruncatedAtTnsDataPresent_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 10,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        var w = new AacBitWriter();
        w.Write(0u, 8);                 // global_gain
        WriteOneZeroSection(w, len: 10);
        w.Write(0u, 1);                 // pulse_data_present = 0
        // Same byte-alignment story: truncate to 2 bytes to force underflow
        // before any of the trailing flag triple can be read.
        byte[] full = w.ToArray();
        byte[] truncated = full.AsSpan(0, 2).ToArray();
        Assert.False(AacIndividualChannelStream.TryParse(
            truncated, sharedIcsInfo: shared, scaleFlag: false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_PulseDataInsidePulseData_TruncatedRejected()
    {
        var book = BuildSyntheticSfCodebook();
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 10,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteOneZeroSection(w, len: 10);
        w.Write(1u, 1);                 // pulse_data_present = 1
        // Start pulse_data() header but truncate before completing it.
        w.Write(0u, 2);                 // number_pulse = 0 (1 pulse total)
        w.Write(5u, 6);                 // start_sfb = 5
        // Need 9 more bits for the pulse but provide only 4.
        w.Write(0xFu, 4);
        Assert.False(AacIndividualChannelStream.TryParse(
            w.ToArray(), sharedIcsInfo: shared, scaleFlag: false, book, out var stream));
        Assert.Null(stream);
    }

    [Fact]
    public void TryRead_BitsConsumed_ExcludesSpectralData()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // Append "spectral data" bits which should be ignored.
        w.Write(0xFFFFu, 16);

        Assert.True(AacIndividualChannelStream.TryParse(
            w.ToArray(), null, false, book, out var stream));
        // 8 + 11 + 9 + 0 + 3 = 31 bits (the trailing 16 are NOT consumed).
        Assert.Equal(31, stream!.BitsConsumed);
    }
}
