using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacDequantizedSpectrumTests
{
    private const int Sr48k = 48_000;

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
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.OnlyLong, 2);
        w.Write(0u, 1);
        w.Write((uint)maxSfb, 6);
        w.Write(0u, 1);
    }

    /// <summary>Encode the SF symbol whose differential equals (sym - 60).</summary>
    private static (uint code, int len) EncodeSfSymbol(int sym)
        => sym == 60 ? (0u, 1) : ((uint)(0x80 + (sym < 60 ? sym : sym - 1)), 8);

    /// <summary>Encode the SF symbol whose differential equals <paramref name="diff"/>.</summary>
    private static (uint code, int len) EncodeSfDiff(int diff)
        => EncodeSfSymbol(60 + diff);

    /// <summary>
    /// Build a 1-SFB long-window frame with a single cb=1 section (4 coefs of
    /// value 1) and the given SF diff against global_gain.
    /// </summary>
    private static AacChannelFrame BuildSimpleFrame(int globalGain, int sfDiff)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4);             // sect_cb = 1
        w.Write(1u, 5);             // sect_len_incr = 1
        var (sfCode, sfLen) = EncodeSfDiff(sfDiff);
        w.Write(sfCode, sfLen);     // SF diff
        w.Write(0u, 1);             // pulse_data_present
        w.Write(0u, 1);             // tns_data_present
        w.Write(0u, 1);             // gain_control_data_present
        w.Write(80u, 7);            // spectral symbol 80 -> (1, 1, 1, 1)

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    [Fact]
    public void FromFrame_NullFrame_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacDequantizedSpectrum.FromFrame(null!, Sr48k));
    }

    [Fact]
    public void FromFrame_BadSampleRate_Throws()
    {
        var frame = BuildSimpleFrame(globalGain: 100, sfDiff: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacDequantizedSpectrum.FromFrame(frame, sampleRate: 192_000));
        Assert.Contains("SWB", ex.Message);
    }

    [Fact]
    public void FromFrame_EmptyMaxSfb_ReturnsAllZeros()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 0);
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));
        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        Assert.Equal(1024, dq.Coefficients.Length);
        Assert.All(dq.Coefficients, c => Assert.Equal(0f, c));
    }

    [Fact]
    public void FromFrame_SingleBandUnityGain_ProducesInverseQuantizedValues()
    {
        // global_gain=100, sf_diff=0 → sf=100 → gain=1.0
        // Coefficients: 4 x 1 → inverse-quant: 4 x 1.0 → gained: 4 x 1.0
        var frame = BuildSimpleFrame(globalGain: 100, sfDiff: 0);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        Assert.Equal(1f, dq.Coefficients[0], precision: 5);
        Assert.Equal(1f, dq.Coefficients[1], precision: 5);
        Assert.Equal(1f, dq.Coefficients[2], precision: 5);
        Assert.Equal(1f, dq.Coefficients[3], precision: 5);
        for (int i = 4; i < 1024; i++) Assert.Equal(0f, dq.Coefficients[i]);
    }

    [Fact]
    public void FromFrame_SingleBandDoubleGain_AppliesPerBandGain()
    {
        // global_gain=104, sf_diff=0 → sf=104 → gain=2.0
        var frame = BuildSimpleFrame(globalGain: 104, sfDiff: 0);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        Assert.Equal(2f, dq.Coefficients[0], precision: 5);
        Assert.Equal(2f, dq.Coefficients[3], precision: 5);
    }

    [Fact]
    public void FromFrame_SingleBandHalfGain_AppliesPerBandGain()
    {
        // global_gain=96, sf_diff=0 → sf=96 → gain=0.5
        var frame = BuildSimpleFrame(globalGain: 96, sfDiff: 0);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        Assert.Equal(0.5f, dq.Coefficients[0], precision: 5);
        Assert.Equal(0.5f, dq.Coefficients[3], precision: 5);
    }

    [Fact]
    public void FromFrame_SfDiffShiftsGain()
    {
        // global_gain=100, sf_diff=+4 → sf=104 → gain=2.0
        var frame = BuildSimpleFrame(globalGain: 100, sfDiff: +4);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        Assert.Equal(2f, dq.Coefficients[0], precision: 5);
    }

    [Fact]
    public void FromFrame_PnsSection_LeavesCoefficientsZero()
    {
        // Build a single-section cb=13 (NoiseEnergy) frame.
        // No spectral_data for PNS bands, coefficients stay at zero.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);                // global_gain
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(13u, 4);                  // sect_cb = 13 (PNS)
        w.Write(1u, 5);                   // sect_len_incr
        // PNS first band: 9-bit PCM = raw - 256. raw=256 → diff=0.
        w.Write(256u, 9);
        w.Write(0u, 1);                   // pulse_data_present
        w.Write(0u, 1);                   // tns_data_present
        w.Write(0u, 1);                   // gain_control_data_present
        // No spectral_data for the PNS band.

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));
        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        // PNS band coefficients are zero — PNS noise generation is a later stage.
        Assert.All(dq.Coefficients, c => Assert.Equal(0f, c));
    }

    [Fact]
    public void FromFrame_TwoSectionsDifferentGains_AppliesIndependently()
    {
        // Two cb=1 sections, each 1 sfb. SF diffs: 0 (sf=100, gain=1), +4 (sf=104, gain=2).
        // First band: 4 coefs of value 1 → 1.0 each.
        // Second band: 4 coefs of value 1 → 2.0 each.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(100u, 8);                 // global_gain = 100
        WriteLongIcsInfo(w, maxSfb: 2);
        // Two sections cb=1 of length 1 each.
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(1u, 4); w.Write(1u, 5);
        // SF diffs:
        var (c0, l0) = EncodeSfDiff(0);   w.Write(c0, l0);
        var (c1, l1) = EncodeSfDiff(+4);  w.Write(c1, l1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // Two spectral tuples of 4 ones each.
        w.Write(80u, 7);
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));
        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        // SWB 0 on 48 kHz long covers samples [0, 4), SWB 1 covers [4, 8).
        for (int i = 0; i < 4; i++) Assert.Equal(1f, dq.Coefficients[i], precision: 5);
        for (int i = 4; i < 8; i++) Assert.Equal(2f, dq.Coefficients[i], precision: 5);
        for (int i = 8; i < 1024; i++) Assert.Equal(0f, dq.Coefficients[i]);
    }

    [Fact]
    public void FromFrame_NegativeQuantizedValue_SignPreservedAfterDequantization()
    {
        // cb=1 (4D signed). Symbol 0 → (-1, -1, -1, -1). global_gain=100 → gain=1.0.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(100u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        var (c, l) = EncodeSfDiff(0); w.Write(c, l);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(0u, 7);                   // symbol 0 → (-1, -1, -1, -1)

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));
        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        Assert.Equal(-1f, dq.Coefficients[0], precision: 5);
        Assert.Equal(-1f, dq.Coefficients[3], precision: 5);
    }

    [Fact]
    public void FromFrame_TransformLengthConstant_Is1024()
    {
        Assert.Equal(1024, AacDequantizedSpectrum.TransformLength);
    }

    [Fact]
    public void FromFrame_AllZeroSection_AllOutputsZero()
    {
        // cb=0 (ZERO_HCB) section, no SF read, no spectral. Output all zero.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(0u, 4); w.Write(1u, 5);  // cb=0, len=1
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));
        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        Assert.All(dq.Coefficients, c => Assert.Equal(0f, c));
    }
}
