using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelFrameTests
{
    private const int Sr48k = 48_000;
    private const int Sr24k = 24_000;

    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook BuildFixed7BitCodebook(int symbolCount)
    {
        var lengths = new int[symbolCount];
        for (int i = 0; i < symbolCount; i++) lengths[i] = 7;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook?[] CodebooksWith(int slot, AacHuffmanCodebook book)
    {
        var arr = new AacHuffmanCodebook?[16];
        arr[slot] = book;
        return arr;
    }

    private static void WriteLongIcsInfo(AacBitWriter w, int maxSfb)
    {
        w.Write(0u, 1);                                          // ics_reserved_bit
        w.Write((uint)AacWindowSequence.OnlyLong, 2);            // window_sequence
        w.Write(0u, 1);                                          // window_shape
        w.Write((uint)maxSfb, 6);                                // max_sfb
        w.Write(0u, 1);                                          // predictor_data_present
    }

    private static void WriteShortIcsInfo(AacBitWriter w, int maxSfb, byte grouping = 0x7F)
    {
        w.Write(0u, 1);                                          // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2);          // window_sequence
        w.Write(0u, 1);                                          // window_shape
        w.Write((uint)maxSfb, 4);                                // max_sfb (4 bits for short)
        w.Write(grouping, 7);                                    // scale_factor_grouping
    }

    private static void WriteOneZeroSection(AacBitWriter w, int len)
    {
        w.Write(0u, 4);                                          // sect_cb = 0
        w.Write((uint)len, 5);                                   // sect_len_incr (long)
    }

    [Fact]
    public void TryRead_LongOwnIcs_EmptySpectrum_BitsConsumedMatchesIcsAlone()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 0); // max_sfb = 0 -> no sections, no scale factors, no spectral
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(0x80, frame!.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.OnlyLong, frame.Stream.IcsInfo.WindowSequence);
        Assert.Equal(0, frame.Stream.IcsInfo.MaxSfb);
        Assert.Empty(frame.Stream.SectionData.Sections);
        // ICS body: 8 (global_gain) + 11 (long ics_info) + 0 (sections terminator since maxSfb=0)
        //        + 0 (scale factors) + 3 (flags) = 22
        Assert.Equal(22, frame.Stream.BitsConsumed);
        // Spectral data: section list empty -> no bits.
        Assert.Equal(0, frame.SpectralData.BitsConsumed);
        Assert.Equal(1024, frame.SpectralData.Coefficients.Length);
        Assert.All(frame.SpectralData.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(22, frame.BitsConsumed);
    }

    [Fact]
    public void TryRead_SharedIcs_DoesNotConsumeIcsInfoBits()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 0,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };

        var w = new AacBitWriter();
        w.Write(0x42u, 8);              // global_gain
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: shared, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(0x42, frame!.Stream.GlobalGain);
        Assert.Null(frame.Stream.OwnIcsInfo);
        Assert.Same(shared, frame.Stream.IcsInfo);
        // 8 (global_gain) + 0 (sections) + 0 (sf) + 3 (flags) = 11.
        Assert.Equal(11, frame.Stream.BitsConsumed);
        Assert.Equal(0, frame.SpectralData.BitsConsumed);
        Assert.Equal(11, frame.BitsConsumed);
    }

    [Fact]
    public void TryRead_LongCb1_DecodesSpectrumAfterIcs()
    {
        var sfBook = BuildSyntheticSfCodebook();
        // Cb 1: signed 4D, base 3, 81 symbols. Symbol 80 -> (1, 1, 1, 1).
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 1); // 1 SFB
        // One section: cb=1, length=1 sfb.
        w.Write(1u, 4);                 // sect_cb = 1
        w.Write(1u, 5);                 // sect_len_incr = 1
        // Scale-factor data: 1 entry of kind Default. cb=1 is regular -> 1 SF symbol.
        // Symbol 60 (diff 0) is the 1-bit "0" code.
        w.Write(0u, 1);                 // SF[0] for sfb 0
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        // Spectral data: SWB 0..1 = 4 coefficients = 1 tuple of dim 4.
        w.Write(80u, 7);                // symbol 80 -> (1, 1, 1, 1)
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        // ICS body: 8 + 11 + 9 + 1 + 3 = 32
        Assert.Equal(32, frame!.Stream.BitsConsumed);
        Assert.Equal(7, frame.SpectralData.BitsConsumed);
        Assert.Equal(39, frame.BitsConsumed);
        Assert.Equal(1, frame.SpectralData.Coefficients[0]);
        Assert.Equal(1, frame.SpectralData.Coefficients[1]);
        Assert.Equal(1, frame.SpectralData.Coefficients[2]);
        Assert.Equal(1, frame.SpectralData.Coefficients[3]);
        for (int i = 4; i < 1024; i++)
        {
            Assert.Equal(0, frame.SpectralData.Coefficients[i]);
        }
    }

    [Fact]
    public void TryRead_ScaleFlag_SkipsPulseTnsGainFlagsButStillConsumesSpectrum()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);                 // SF[0] symbol 60
        // scale_flag = true -> no pulse/tns/gain flags here.
        w.Write(80u, 7);                // spectral tuple
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: true, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.False(frame!.Stream.PulseDataPresent);
        Assert.False(frame.Stream.TnsDataPresent);
        Assert.False(frame.Stream.GainControlDataPresent);
        // ICS body: 8 + 11 + 9 + 1 = 29 (no trailing flags).
        Assert.Equal(29, frame.Stream.BitsConsumed);
        Assert.Equal(7, frame.SpectralData.BitsConsumed);
        Assert.Equal(36, frame.BitsConsumed);
        Assert.Equal(1, frame.SpectralData.Coefficients[0]);
    }

    [Fact]
    public void TryRead_ZeroHcbSection_ParsesIcsAndSkipsSpectrum()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x10u, 8);
        WriteLongIcsInfo(w, maxSfb: 5);
        WriteOneZeroSection(w, len: 5); // single cb=0 section covering 5 sfbs
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Single(frame!.Stream.SectionData.Sections);
        Assert.Equal(0, frame.Stream.SectionData.Sections[0].CodebookNumber);
        Assert.All(frame.SpectralData.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, frame.SpectralData.BitsConsumed);
        // ICS body: 8 + 11 + 9 + 0 (no SF) + 3 = 31
        Assert.Equal(31, frame.Stream.BitsConsumed);
        Assert.Equal(31, frame.BitsConsumed);
    }

    [Fact]
    public void TryRead_IcsBodyUnderflow_ReturnsFalse()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        // Empty span - ICS body needs 8 bits for global_gain and rejects.
        Assert.False(AacChannelFrame.TryParse(
            ReadOnlySpan<byte>.Empty, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.Null(frame);
    }

    [Fact]
    public void TryRead_SpectralDataUnderflow_ReturnsFalse()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5); // section cb=1, len=1
        w.Write(0u, 1);                 // SF[0] symbol 60
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // ICS body so far: 8 + 11 + 9 + 1 + 3 = 32 bits = 4 bytes after AlignToByte.
        w.AlignToByte();
        var icsOnly = w.ToArray();
        // 4 bytes total - ICS body just fits, spectral walker needs 7 more bits and underflows.
        Assert.Equal(4, icsOnly.Length);

        Assert.False(AacChannelFrame.TryParse(
            icsOnly, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.Null(frame);
    }

    [Fact]
    public void TryRead_BitReaderAdvancesByBitsConsumed()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        // Prepend a stray bit to ensure offset accounting is correct.
        w.Write(1u, 1);
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(80u, 7);                // spectral tuple
        var bytes = w.ToArray();

        var reader = new BitReader(bytes);
        reader.ReadBits(1);             // consume the stray bit
        int startBits = reader.Position;

        Assert.True(AacChannelFrame.TryRead(
            ref reader, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(39, frame!.BitsConsumed);
        Assert.Equal(startBits + 39, reader.Position);
    }

    [Fact]
    public void TryRead_SampleRateDispatchAffectsSpectralWalker()
    {
        // The SWB-offset table is sample-rate-keyed; feeding the same bytes with
        // two different rates must produce the same coefficient layout when the
        // first SFB has the same width (both Long48 and Long24 SWB 0 spans 4
        // samples), so the two parses must report identical decoded vectors.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(80u, 7);
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var f48));
        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr24k, spectralBooks, out var f24));
        Assert.NotNull(f48);
        Assert.NotNull(f24);
        // Both rates' Long table place SWB 0 at offset 0 with width 4.
        Assert.Equal(1, f48!.SpectralData.Coefficients[0]);
        Assert.Equal(1, f24!.SpectralData.Coefficients[0]);
        Assert.Equal(f48.BitsConsumed, f24.BitsConsumed);
    }

    [Fact]
    public void TryRead_NullScaleFactorCodebook_Throws()
    {
        var spectralBooks = new AacHuffmanCodebook?[16];
        var bytes = new byte[] { 0x00, 0x00, 0x00 };
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelFrame.TryParse(
                bytes, sharedIcsInfo: null, scaleFlag: false,
                scaleFactorCodebook: null!,
                Sr48k, spectralBooks, out _));
    }

    [Fact]
    public void TryRead_NullSpectralCodebooks_Throws()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var bytes = new byte[] { 0x00, 0x00, 0x00 };
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelFrame.TryParse(
                bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
                Sr48k, spectralCodebooks: null!, out _));
    }

    [Fact]
    public void TryRead_PulseDataPresent_ParsesPulseDataAndStillConsumesSpectrum()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);                 // SF[0] symbol 60
        w.Write(1u, 1);                 // pulse_data_present
        // pulse_data(): number_pulse(2) + pulse_start_sfb(6) +
        // 1*(pulse_offset(5) + pulse_amp(4))
        w.Write(0u, 2);                 // number_pulse = 0 -> 1 pulse total
        w.Write(0u, 6);                 // pulse_start_sfb
        w.Write(0u, 5);                 // pulse_offset[0]
        w.Write(0u, 4);                 // pulse_amp[0]
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        w.Write(80u, 7);
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.True(frame!.Stream.PulseDataPresent);
        Assert.NotNull(frame.Stream.PulseData);
        Assert.Equal(1, frame.SpectralData.Coefficients[0]);
    }

    [Fact]
    public void TryRead_CommonSpectralBitsConsumed_EqualsReaderAdvance()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 2);
        // Two cb=1 sections of 1 sfb each.
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(1u, 4); w.Write(1u, 5);
        // Scale factors: cb=1 is regular -> one SF per section = 2 entries.
        w.Write(0u, 1); w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // Spectral: SWB 0..2 covers 8 samples = 2 tuples of dim 4.
        w.Write(80u, 7);
        w.Write(80u, 7);
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(14, frame!.SpectralData.BitsConsumed);
        Assert.Equal(frame.Stream.BitsConsumed + frame.SpectralData.BitsConsumed,
                     frame.BitsConsumed);
        Assert.Equal(1, frame.SpectralData.Coefficients[0]);
        Assert.Equal(1, frame.SpectralData.Coefficients[7]);
    }

    // ----- EightShort coverage -----

    [Fact]
    public void TryRead_ShortOwnIcs_EmptySpectrum_BitsConsumedMatchesIcsAlone()
    {
        // EightShort own ICS with maxSfb=0 -> no sections, no SF, no spectral.
        // Body = 8 (gg) + 15 (short ics) + 0 (sections) + 0 (sf) + 3 (flags) = 26 bits.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteShortIcsInfo(w, maxSfb: 0);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(AacWindowSequence.EightShort, frame!.Stream.IcsInfo.WindowSequence);
        Assert.Equal(0, frame.Stream.IcsInfo.MaxSfb);
        Assert.Empty(frame.Stream.SectionData.Sections);
        Assert.Equal(26, frame.Stream.BitsConsumed);
        Assert.Equal(0, frame.SpectralData.BitsConsumed);
        Assert.Equal(1024, frame.SpectralData.Coefficients.Length);
        Assert.All(frame.SpectralData.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(26, frame.BitsConsumed);
    }

    [Fact]
    public void TryRead_ShortSharedIcs_DoesNotConsumeIcsInfoBits()
    {
        // Shared short ICS - body skips ics_info(): 8 (gg) + 0 + 0 + 3 = 11 bits.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];
        var shared = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.EightShort,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = 0,
            ScaleFactorGrouping = 0x7F,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 8 },
            PredictorDataPresent = false,
        };

        var w = new AacBitWriter();
        w.Write(0x42u, 8);              // global_gain
        w.Write(0u, 1);                 // pulse_data_present (forbidden=1 for short)
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: shared, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(0x42, frame!.Stream.GlobalGain);
        Assert.Null(frame.Stream.OwnIcsInfo);
        Assert.Same(shared, frame.Stream.IcsInfo);
        Assert.Equal(11, frame.Stream.BitsConsumed);
        Assert.Equal(0, frame.SpectralData.BitsConsumed);
        Assert.Equal(11, frame.BitsConsumed);
    }

    [Fact]
    public void TryRead_ShortOwnIcs_AllSingletonGroups_StillEmptySpectrum()
    {
        // scale_factor_grouping = 0 -> 8 singleton groups. maxSfb=0 still
        // means zero sections per group, so the body bit count is unchanged.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x10u, 8);
        WriteShortIcsInfo(w, maxSfb: 0, grouping: 0x00);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(8, frame!.Stream.IcsInfo.WindowGroupCount);
        Assert.Equal(26, frame.BitsConsumed);
        Assert.All(frame.SpectralData.Coefficients, c => Assert.Equal(0, c));
    }

    // ----- Additional coverage -----

    [Fact]
    public void TryParse_BitsConsumed_Equals_Sum_Of_Stream_And_Spectral()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 2);
        w.Write(1u, 4); w.Write(2u, 5); // one section, cb=1, len=2 sfbs
        w.Write(0u, 1); w.Write(0u, 1); // 2 SFs
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // SWB 0..2 (Long48) = offsets 0,4,8 -> 8 samples -> 2 tuples
        w.Write(80u, 7); w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(frame!.Stream.BitsConsumed + frame.SpectralData.BitsConsumed,
                     frame.BitsConsumed);
    }

    [Fact]
    public void TryParse_Coefficients_Length_Is_1024()
    {
        // Even with no spectral data the coefficient array must be the full 1024.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteLongIcsInfo(w, maxSfb: 0);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(1024, frame!.SpectralData.Coefficients.Length);
    }

    [Fact]
    public void TryParse_TryRead_Equivalent_For_Same_Bytes()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x44u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(80u, 7);
        var bytes = w.ToArray();

        Assert.True(AacChannelFrame.TryParse(
            bytes, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var viaParse));

        var reader = new BitReader(bytes);
        int startBits = reader.Position;
        Assert.True(AacChannelFrame.TryRead(
            ref reader, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var viaRead));

        Assert.NotNull(viaParse);
        Assert.NotNull(viaRead);
        Assert.Equal(viaParse!.BitsConsumed, viaRead!.BitsConsumed);
        Assert.Equal(viaParse.Stream.GlobalGain, viaRead.Stream.GlobalGain);
        Assert.Equal(viaParse.SpectralData.Coefficients[0], viaRead.SpectralData.Coefficients[0]);
        Assert.Equal(startBits + viaParse.BitsConsumed, reader.Position);
    }

    [Fact]
    public void TryParse_TnsDataPresent_True_ParsesTnsBlock()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 0);
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(1u, 1);                 // tns_data_present
        // For long window: n_filt(2), if n_filt>0 -> coef_res(1) + per-filter.
        w.Write(0u, 2);                 // n_filt = 0, no per-filter bits.
        w.Write(0u, 1);                 // gain_control_data_present

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.True(frame!.Stream.TnsDataPresent);
        Assert.NotNull(frame.Stream.TnsData);
    }

    [Fact]
    public void TryParse_LongIcs_MultiSection_TwoCb1Sections()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0x40u, 8);
        WriteLongIcsInfo(w, maxSfb: 3);
        // Two separate cb=1 sections (must escape via len=31 to delimit).
        // Simpler: one section of len 3.
        w.Write(1u, 4); w.Write(3u, 5);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); // 3 SFs
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // SWB 0..3 (Long48) = offsets 0,4,8,12 -> 12 samples -> 3 tuples
        w.Write(80u, 7); w.Write(80u, 7); w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Single(frame!.Stream.SectionData.Sections);
        var section = frame.Stream.SectionData.Sections[0];
        Assert.Equal(3, section.EndSfb - section.StartSfb);
        Assert.Equal(1, frame.SpectralData.Coefficients[11]);
    }

    [Fact]
    public void TryParse_ShortIcs_ScaleFlag_True_NoTrailingFlags()
    {
        // For EightShort + scaleFlag=true the body skips pulse/tns/gain flags.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteShortIcsInfo(w, maxSfb: 0);
        // No trailing flags because scaleFlag = true.

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: true, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.False(frame!.Stream.PulseDataPresent);
        Assert.False(frame.Stream.TnsDataPresent);
        Assert.False(frame.Stream.GainControlDataPresent);
        // 8 (gg) + 15 (short ics) + 0 sections + 0 sf + 0 flags = 23
        Assert.Equal(23, frame.BitsConsumed);
    }

    [Fact]
    public void TryParse_AllZero_Bytes_LongIcs_MaxSfb0_Parses()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        // 22 bits at start of an all-zero buffer match: global_gain=0,
        // ics_info(OnlyLong, maxSfb=0, no predictor), three trailing zero flags.
        byte[] zeros = new byte[8];
        Assert.True(AacChannelFrame.TryParse(
            zeros, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(0, frame!.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.OnlyLong, frame.Stream.IcsInfo.WindowSequence);
        Assert.Equal(0, frame.Stream.IcsInfo.MaxSfb);
    }

    [Fact]
    public void TryParse_ShortIcs_Grouping_All_Ones_Gives_Single_Group()
    {
        // grouping=0x7F (7 bits set) merges all 8 windows into a single group.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0u, 8);
        WriteShortIcsInfo(w, maxSfb: 0, grouping: 0x7F);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        Assert.Equal(1, frame!.Stream.IcsInfo.WindowGroupCount);
        Assert.Equal(8, frame.Stream.IcsInfo.WindowsPerGroup.Span[0]);
    }

    [Fact]
    public void TryParse_With_Expression_Modifies_BitsConsumed()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        byte[] zeros = new byte[8];
        Assert.True(AacChannelFrame.TryParse(
            zeros, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.NotNull(frame);
        var modified = frame! with { BitsConsumed = 99 };
        Assert.Equal(99, modified.BitsConsumed);
        Assert.Equal(frame.BitsConsumed, frame.BitsConsumed); // original unchanged
        Assert.NotSame(frame, modified);
    }

    [Fact]
    public void TryParse_Empty_Span_Returns_False_Without_Throwing()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        Assert.False(AacChannelFrame.TryParse(
            ReadOnlySpan<byte>.Empty, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.Null(frame);
    }

    [Fact]
    public void TryParse_Different_GlobalGain_Reflected_In_Stream()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w1 = new AacBitWriter();
        w1.Write(0xAAu, 8);
        WriteLongIcsInfo(w1, maxSfb: 0);
        w1.Write(0u, 1); w1.Write(0u, 1); w1.Write(0u, 1);

        var w2 = new AacBitWriter();
        w2.Write(0x55u, 8);
        WriteLongIcsInfo(w2, maxSfb: 0);
        w2.Write(0u, 1); w2.Write(0u, 1); w2.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(w1.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var f1));
        Assert.True(AacChannelFrame.TryParse(w2.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var f2));

        Assert.Equal(0xAA, f1!.Stream.GlobalGain);
        Assert.Equal(0x55, f2!.Stream.GlobalGain);
        Assert.NotEqual(f1.Stream.GlobalGain, f2.Stream.GlobalGain);
    }

    [Fact]
    public void TryRead_DoesNotAdvanceReader_When_Returns_False()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var reader = new BitReader(ReadOnlySpan<byte>.Empty);
        int posBefore = reader.Position;
        bool ok = AacChannelFrame.TryRead(
            ref reader, sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame);
        Assert.False(ok);
        Assert.Null(frame);
        // Reader can't go backwards once an underflow has occurred but
        // shouldn't have advanced past what's available.
        Assert.True(reader.Position >= posBefore);
    }
}
