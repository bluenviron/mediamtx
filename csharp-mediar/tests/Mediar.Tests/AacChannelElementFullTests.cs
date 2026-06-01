using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelElementFullTests
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

    private static void WriteShortIcsInfo(AacBitWriter w, int maxSfb, byte grouping = 0x7F)
    {
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.EightShort, 2);
        w.Write(0u, 1);
        w.Write((uint)maxSfb, 4);
        w.Write(grouping, 7);
    }

    private static byte[] BuildEmptyElementBytes(int tag, byte globalGain)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);
        w.Write(globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 0);
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        // No spectral bits (maxSfb=0).
        return w.ToArray();
    }

    private static byte[] BuildEmptyShortElementBytes(int tag, byte globalGain, byte grouping = 0x7F)
    {
        // Short-windowed analogue of BuildEmptyElementBytes: same shape but
        // EightShort window_sequence and 4-bit max_sfb + 7-bit grouping.
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);
        w.Write(globalGain, 8);
        WriteShortIcsInfo(w, maxSfb: 0, grouping: grouping);
        w.Write(0u, 1);                 // pulse_data_present (must be 0 for short)
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
        return w.ToArray();
    }

    private static byte[] BuildCb1ElementBytes(int tag)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);
        w.Write(0x80u, 8);              // global_gain
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4);                 // sect_cb = 1
        w.Write(1u, 5);                 // sect_len_incr = 1
        w.Write(0u, 1);                 // SF[0] symbol 60 (diff 0)
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); // pulse/tns/gain flags
        w.Write(80u, 7);                // spectral symbol -> (1,1,1,1)
        return w.ToArray();
    }

    // SCE Full

    [Fact]
    public void SceFull_EmptySpectrum_Succeeds()
    {
        var sf = BuildSyntheticSfCodebook();
        var spectral = new AacHuffmanCodebook?[16];

        Assert.True(AacSingleChannelElement.TryParse(
            BuildEmptyElementBytes(tag: 3, globalGain: 0x40),
            sf, Sr48k, spectral, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(3, sce!.ElementInstanceTag);
        Assert.Equal(0x40, sce.Stream.GlobalGain);
        Assert.NotNull(sce.SpectralData);
        Assert.All(sce.SpectralData!.Coefficients, c => Assert.Equal(0, c));
        // 4 (tag) + 22 (ICS with maxSfb=0 + flags) + 0 (spectral) = 26
        Assert.Equal(26, sce.BitsConsumed);
    }

    [Fact]
    public void SceFull_Cb1Section_DecodesSpectralCoefficients()
    {
        var sf = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);

        Assert.True(AacSingleChannelElement.TryParse(
            BuildCb1ElementBytes(tag: 5),
            sf, Sr48k, CodebooksWith(1, spectralBook), out var sce));
        Assert.NotNull(sce);
        Assert.Equal(5, sce!.ElementInstanceTag);
        Assert.NotNull(sce.SpectralData);
        Assert.Equal(1, sce.SpectralData!.Coefficients[0]);
        Assert.Equal(1, sce.SpectralData.Coefficients[1]);
        Assert.Equal(1, sce.SpectralData.Coefficients[2]);
        Assert.Equal(1, sce.SpectralData.Coefficients[3]);
        // tag(4) + ICS body(32) + spectral(7) = 43
        Assert.Equal(43, sce.BitsConsumed);
    }

    [Fact]
    public void SceFull_SpectralUnderflow_ReturnsFalse()
    {
        var sf = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        // Construct an SCE bytes that needs spectral_data but truncates before it.
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // No spectral bits written. Total = 4+32 = 36 bits; padded to 5 bytes.
        w.AlignToByte();
        var bytes = w.ToArray();

        Assert.False(AacSingleChannelElement.TryParse(
            bytes, sf, Sr48k, CodebooksWith(1, spectralBook), out var sce));
        Assert.Null(sce);
    }

    [Fact]
    public void SceFull_BoundaryOverload_LeavesSpectralDataNull()
    {
        var sf = BuildSyntheticSfCodebook();
        var bytes = BuildEmptyElementBytes(tag: 1, globalGain: 0x10);

        Assert.True(AacSingleChannelElement.TryParse(bytes, sf, out var sce));
        Assert.NotNull(sce);
        Assert.Null(sce!.SpectralData);
        // Boundary overload doesn't include spectral bits in count.
        Assert.Equal(26, sce.BitsConsumed);
    }

    [Fact]
    public void SceFull_NullSpectralCodebooks_Throws()
    {
        var sf = BuildSyntheticSfCodebook();
        var bytes = BuildEmptyElementBytes(tag: 0, globalGain: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacSingleChannelElement.TryParse(
                bytes, sf, Sr48k, spectralCodebooks: null!, out _));
    }

    [Fact]
    public void SceFull_NullScaleFactorCodebook_Throws()
    {
        var spectral = new AacHuffmanCodebook?[16];
        var bytes = BuildEmptyElementBytes(tag: 0, globalGain: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacSingleChannelElement.TryParse(
                bytes, scaleFactorCodebook: null!, Sr48k, spectral, out _));
    }

    // LFE Full (byte-identical bitstream to SCE)

    [Fact]
    public void LfeFull_EmptySpectrum_Succeeds()
    {
        var sf = BuildSyntheticSfCodebook();
        var spectral = new AacHuffmanCodebook?[16];

        Assert.True(AacLowFrequencyElement.TryParse(
            BuildEmptyElementBytes(tag: 7, globalGain: 0x20),
            sf, Sr48k, spectral, out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(7, lfe!.ElementInstanceTag);
        Assert.Equal(0x20, lfe.Stream.GlobalGain);
        Assert.NotNull(lfe.SpectralData);
        Assert.All(lfe.SpectralData!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(26, lfe.BitsConsumed);
    }

    [Fact]
    public void LfeFull_Cb1Section_DecodesSpectralCoefficients()
    {
        var sf = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);

        Assert.True(AacLowFrequencyElement.TryParse(
            BuildCb1ElementBytes(tag: 2),
            sf, Sr48k, CodebooksWith(1, spectralBook), out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(2, lfe!.ElementInstanceTag);
        Assert.NotNull(lfe.SpectralData);
        Assert.Equal(1, lfe.SpectralData!.Coefficients[0]);
        Assert.Equal(43, lfe.BitsConsumed);
    }

    [Fact]
    public void LfeFull_BoundaryOverload_LeavesSpectralDataNull()
    {
        var sf = BuildSyntheticSfCodebook();
        var bytes = BuildEmptyElementBytes(tag: 0, globalGain: 0);

        Assert.True(AacLowFrequencyElement.TryParse(bytes, sf, out var lfe));
        Assert.NotNull(lfe);
        Assert.Null(lfe!.SpectralData);
        Assert.Equal(26, lfe.BitsConsumed);
    }

    [Fact]
    public void LfeFull_NullSpectralCodebooks_Throws()
    {
        var sf = BuildSyntheticSfCodebook();
        var bytes = BuildEmptyElementBytes(tag: 0, globalGain: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacLowFrequencyElement.TryParse(
                bytes, sf, Sr48k, spectralCodebooks: null!, out _));
    }

    // ----- EightShort full-parse coverage -----

    [Fact]
    public void SceFull_EmptyShortSpectrum_Succeeds()
    {
        // EightShort SCE with maxSfb=0 has no sections, no scale factors,
        // and no spectral coefficients. Verifies the "Full" overload (which
        // walks spectral_data) handles a short empty body.
        var sf = BuildSyntheticSfCodebook();
        var spectral = new AacHuffmanCodebook?[16];

        Assert.True(AacSingleChannelElement.TryParse(
            BuildEmptyShortElementBytes(tag: 4, globalGain: 0x55),
            sf, Sr48k, spectral, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(4, sce!.ElementInstanceTag);
        Assert.Equal(0x55, sce.Stream.GlobalGain);
        Assert.Equal(AacWindowSequence.EightShort, sce.Stream.IcsInfo!.WindowSequence);
        Assert.NotNull(sce.SpectralData);
        Assert.All(sce.SpectralData!.Coefficients, c => Assert.Equal(0, c));
        // tag(4) + global_gain(8) + ics(15) + flags(3) + spectral(0) = 30 bits.
        Assert.Equal(30, sce.BitsConsumed);
    }

    [Fact]
    public void SceFull_EmptyShortSpectrum_AllSeparateGroups_Succeeds()
    {
        // scale_factor_grouping=0 -> 8 singleton groups but maxSfb=0 still
        // means zero sections per group, so the body shape is identical.
        var sf = BuildSyntheticSfCodebook();
        var spectral = new AacHuffmanCodebook?[16];

        Assert.True(AacSingleChannelElement.TryParse(
            BuildEmptyShortElementBytes(tag: 1, globalGain: 0x10, grouping: 0x00),
            sf, Sr48k, spectral, out var sce));
        Assert.NotNull(sce);
        Assert.Equal(8, sce!.Stream.IcsInfo!.WindowGroupCount);
        Assert.NotNull(sce.SpectralData);
        Assert.Equal(30, sce.BitsConsumed);
    }

    [Fact]
    public void LfeFull_EmptyShortSpectrum_Succeeds()
    {
        // LFE shares the SCE bitstream shape; the full-parser must accept
        // an EightShort body even though no encoder emits short-windowed LFE.
        var sf = BuildSyntheticSfCodebook();
        var spectral = new AacHuffmanCodebook?[16];

        Assert.True(AacLowFrequencyElement.TryParse(
            BuildEmptyShortElementBytes(tag: 6, globalGain: 0x33),
            sf, Sr48k, spectral, out var lfe));
        Assert.NotNull(lfe);
        Assert.Equal(6, lfe!.ElementInstanceTag);
        Assert.Equal(AacWindowSequence.EightShort, lfe.Stream.IcsInfo!.WindowSequence);
        Assert.NotNull(lfe.SpectralData);
        Assert.All(lfe.SpectralData!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(30, lfe.BitsConsumed);
    }
}
