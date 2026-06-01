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
}
