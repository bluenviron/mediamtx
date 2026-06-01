using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelPairElementTests
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

    private static void WriteOneZeroSection(AacBitWriter w, int len)
    {
        w.Write(0u, 4);                 // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)len, 5);          // sect_len (long: 5-bit increment)
    }

    private static void WriteIcsBody(AacBitWriter w, byte globalGain, int maxSfb)
    {
        w.Write(globalGain, 8);         // global_gain
        // ics_info already written by caller when not common_window; CPE common_window
        // path has the shared ics_info OUTSIDE the body, and the body must NOT emit
        // its own. This helper covers the common_window=1 case.
        WriteOneZeroSection(w, maxSfb);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(0u, 1);                 // gain_control_data_present
    }

    private static void WriteIndependentIcsStream(AacBitWriter w, byte globalGain, int maxSfb)
    {
        // common_window=0 body: global_gain + own ics_info + section/sf/flags.
        w.Write(globalGain, 8);
        WriteLongIcsInfo(w, maxSfb);
        WriteOneZeroSection(w, maxSfb);
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);
    }

    private static byte[] BuildCommonWindowCpe(
        int tag,
        int maxSfb,
        AacMsMaskPresent msMask,
        bool[][] msUsed,
        byte gain1,
        byte gain2)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);          // element_instance_tag
        w.Write(1u, 1);                 // common_window = 1
        WriteLongIcsInfo(w, maxSfb);    // shared ics_info
        w.Write((uint)msMask, 2);       // ms_mask_present
        if (msMask == AacMsMaskPresent.PerBand)
        {
            // ms_used flat: one bit per (group, sfb). WindowGroupCount=1 for OnlyLong.
            foreach (var group in msUsed)
            {
                foreach (var bit in group)
                {
                    w.Write(bit ? 1u : 0u, 1);
                }
            }
        }
        WriteIcsBody(w, gain1, maxSfb); // first ics body (no own ics_info)
        WriteIcsBody(w, gain2, maxSfb); // second ics body (no own ics_info)
        return w.ToArray();
    }

    private static byte[] BuildIndependentCpe(int tag, int maxSfb, byte gain1, byte gain2)
    {
        var w = new AacBitWriter();
        w.Write((uint)tag, 4);          // element_instance_tag
        w.Write(0u, 1);                 // common_window = 0
        WriteIndependentIcsStream(w, gain1, maxSfb);
        WriteIndependentIcsStream(w, gain2, maxSfb);
        return w.ToArray();
    }

    /// <summary>
    /// Cross-fixture accessor: builds an independent-streams CPE byte body
    /// (no leading element-type bits, no trailing END) shaped exactly like
    /// <see cref="BuildIndependentCpe"/>. Used by sibling test fixtures that
    /// need a real CPE for end-to-end facade coverage without having to
    /// re-implement the bit layout.
    /// </summary>
    internal static byte[] BuildIndependentCpeShared(int tag, int maxSfb, byte gain1, byte gain2)
        => BuildIndependentCpe(tag, maxSfb, gain1, gain2);

    /// <summary>
    /// Cross-fixture accessor: same as <see cref="BuildCommonWindowCpe"/>
    /// (common_window=1 with optional MS mask). Used by sibling test
    /// fixtures that exercise the high-level facade with common-window CPE.
    /// </summary>
    internal static byte[] BuildCommonWindowCpeShared(
        int tag,
        int maxSfb,
        AacMsMaskPresent msMask,
        bool[][] msUsed,
        byte gain1,
        byte gain2)
        => BuildCommonWindowCpe(tag, maxSfb, msMask, msUsed, gain1, gain2);

    /// <summary>
    /// Cross-fixture accessor: writes an independent-streams CPE
    /// (common_window=0) body into a shared writer. Symmetric with
    /// <see cref="AacRawDataBlockTests.WriteEmptySceBodyShared"/> so
    /// callers can splice a CPE between an element-type prefix and an
    /// END sentinel inside one bit stream.
    /// </summary>
    internal static void WriteIndependentCpeBodyShared(
        AacBitWriter w,
        int tag,
        int maxSfb,
        byte gain1,
        byte gain2)
    {
        w.Write((uint)tag, 4);
        w.Write(0u, 1);                 // common_window = 0
        WriteIndependentIcsStream(w, gain1, maxSfb);
        WriteIndependentIcsStream(w, gain2, maxSfb);
    }

    /// <summary>
    /// Cross-fixture accessor: writes a common-window CPE
    /// (common_window=1, no MS) body into a shared writer.
    /// </summary>
    internal static void WriteCommonWindowCpeBodyShared(
        AacBitWriter w,
        int tag,
        int maxSfb,
        byte gain1,
        byte gain2)
    {
        w.Write((uint)tag, 4);
        w.Write(1u, 1);                 // common_window = 1
        WriteLongIcsInfo(w, maxSfb);
        w.Write((uint)AacMsMaskPresent.None, 2);
        WriteIcsBody(w, gain1, maxSfb);
        WriteIcsBody(w, gain2, maxSfb);
    }


    [Fact]
    public void TryParse_CommonWindow_NoMsMask_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCommonWindowCpe(
            tag: 0,
            maxSfb: 6,
            msMask: AacMsMaskPresent.None,
            msUsed: [],
            gain1: 0x40,
            gain2: 0x60);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.NotNull(cpe);
        Assert.Equal(0, cpe!.ElementInstanceTag);
        Assert.True(cpe.CommonWindow);
        Assert.NotNull(cpe.SharedIcsInfo);
        Assert.Equal(AacWindowSequence.OnlyLong, cpe.SharedIcsInfo!.WindowSequence);
        Assert.Equal(6, cpe.SharedIcsInfo.MaxSfb);
        Assert.Equal(AacMsMaskPresent.None, cpe.MsMaskPresent);
        Assert.Empty(cpe.MsUsed);
        Assert.Equal(0x40, cpe.FirstStream.GlobalGain);
        Assert.Equal(0x60, cpe.SecondStream.GlobalGain);
        // Both streams should reference the shared ics_info (their own is null).
        Assert.Null(cpe.FirstStream.OwnIcsInfo);
        Assert.Null(cpe.SecondStream.OwnIcsInfo);
    }

    [Fact]
    public void TryParse_CommonWindow_AllBandsMsMask_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCommonWindowCpe(
            tag: 1,
            maxSfb: 4,
            msMask: AacMsMaskPresent.AllBands,
            msUsed: [],
            gain1: 0x10,
            gain2: 0x20);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.NotNull(cpe);
        Assert.Equal(AacMsMaskPresent.AllBands, cpe!.MsMaskPresent);
        Assert.Empty(cpe.MsUsed);
    }

    [Fact]
    public void TryParse_CommonWindow_PerBandMsMask_RoundTripsFlags()
    {
        var book = BuildSyntheticSfCodebook();
        var pattern = new bool[] { true, false, true, true, false };
        var bytes = BuildCommonWindowCpe(
            tag: 7,
            maxSfb: 5,
            msMask: AacMsMaskPresent.PerBand,
            msUsed: [pattern],
            gain1: 0x80,
            gain2: 0x80);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.NotNull(cpe);
        Assert.Equal(AacMsMaskPresent.PerBand, cpe!.MsMaskPresent);
        Assert.Single(cpe.MsUsed);
        Assert.Equal(pattern, cpe.MsUsed[0]);
    }

    [Fact]
    public void TryParse_CommonWindow_ReservedMsMask_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(3u, 4);                 // element_instance_tag
        w.Write(1u, 1);                 // common_window = 1
        WriteLongIcsInfo(w, 4);
        w.Write(3u, 2);                 // ms_mask_present = 3 (reserved)
        // pad enough bytes
        for (int i = 0; i < 4; i++) w.Write(0u, 8);

        Assert.False(AacChannelPairElement.TryParse(w.ToArray(), book, out var cpe));
        Assert.Null(cpe);
    }

    [Fact]
    public void TryParse_IndependentChannels_NoCommonWindow_Succeeds()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildIndependentCpe(tag: 2, maxSfb: 4, gain1: 0x33, gain2: 0x44);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.NotNull(cpe);
        Assert.False(cpe!.CommonWindow);
        Assert.Null(cpe.SharedIcsInfo);
        Assert.Equal(AacMsMaskPresent.None, cpe.MsMaskPresent);
        Assert.Empty(cpe.MsUsed);
        Assert.Equal(0x33, cpe.FirstStream.GlobalGain);
        Assert.Equal(0x44, cpe.SecondStream.GlobalGain);
        // Each stream now owns its own ics_info.
        Assert.NotNull(cpe.FirstStream.OwnIcsInfo);
        Assert.NotNull(cpe.SecondStream.OwnIcsInfo);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(15)]
    public void TryParse_RoundTripsElementInstanceTag(int tag)
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCommonWindowCpe(
            tag,
            maxSfb: 4,
            msMask: AacMsMaskPresent.None,
            msUsed: [],
            gain1: 0x40,
            gain2: 0x50);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.Equal(tag, cpe!.ElementInstanceTag);
    }

    [Fact]
    public void TryParse_EmptyBuffer_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        Assert.False(AacChannelPairElement.TryParse(ReadOnlySpan<byte>.Empty, book, out var cpe));
        Assert.Null(cpe);
    }

    [Fact]
    public void TryParse_TruncatedAfterTagAndFlag_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        // 5 bits payload: element_instance_tag (4) + common_window (1). No body.
        var w = new AacBitWriter();
        w.Write(7u, 4);
        w.Write(1u, 1);
        Assert.False(AacChannelPairElement.TryParse(w.ToArray(), book, out var cpe));
        Assert.Null(cpe);
    }

    [Fact]
    public void TryParse_TruncatedMidFirstStream_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var full = BuildCommonWindowCpe(
            tag: 0,
            maxSfb: 6,
            msMask: AacMsMaskPresent.None,
            msUsed: [],
            gain1: 0x40,
            gain2: 0x60);
        // Slice to before second stream completes — find first-stream end empirically:
        // 4 (tag) + 1 (cw) + 11 (ics) + 2 (msmp) + 8 (gg1) + 9 (sect1) + 3 (flags1) = 38 bits = 4 full bytes + 6 bits.
        // Cut at byte index 4 so we're 6 bits into the first-stream body (mid scale-factor area).
        var truncated = full.AsSpan(0, 4).ToArray();
        Assert.False(AacChannelPairElement.TryParse(truncated, book, out var cpe));
        Assert.Null(cpe);
    }

    [Fact]
    public void TryParse_TruncatedMidSecondStream_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var full = BuildCommonWindowCpe(
            tag: 0,
            maxSfb: 6,
            msMask: AacMsMaskPresent.None,
            msUsed: [],
            gain1: 0x40,
            gain2: 0x60);
        // Drop only the final byte to cut into the second stream's trailing flag block.
        var truncated = full.AsSpan(0, full.Length - 1).ToArray();
        Assert.False(AacChannelPairElement.TryParse(truncated, book, out var cpe));
        Assert.Null(cpe);
    }

    [Fact]
    public void TryParse_GainControlDataInFirstStream_ParsesEmptyGainControlData()
    {
        // gain_control_data_present = 1 with an empty (max_band = 0) gcd body parses cleanly
        // in the first stream of a common_window CPE.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(3u, 4);                 // element_instance_tag
        w.Write(1u, 1);                 // common_window = 1
        WriteLongIcsInfo(w, maxSfb: 4);
        w.Write(0u, 2);                 // ms_mask_present = 0 (None)
        // First stream body with gain_control_data_present = 1
        w.Write(0x40u, 8);              // global_gain
        WriteOneZeroSection(w, len: 4);
        w.Write(0u, 1);                 // pulse_data_present
        w.Write(0u, 1);                 // tns_data_present
        w.Write(1u, 1);                 // gain_control_data_present = 1
        w.Write(0u, 2);                 // gain_control_data(): max_band = 0
        // Second stream body (no gcd)
        w.Write(0x60u, 8);              // global_gain
        WriteOneZeroSection(w, len: 4);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacChannelPairElement.TryParse(w.ToArray(), book, out var cpe));
        Assert.NotNull(cpe);
        Assert.True(cpe!.FirstStream.GainControlDataPresent);
        Assert.NotNull(cpe.FirstStream.GainControlData);
        Assert.Equal(0, cpe.FirstStream.GainControlData!.MaxBand);
        Assert.False(cpe.SecondStream.GainControlDataPresent);
        Assert.Null(cpe.SecondStream.GainControlData);
    }

    [Fact]
    public void TryParse_NullCodebook_Throws()
    {
        var bytes = new byte[] { 0x00 };
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelPairElement.TryParse(bytes, null!, out _));
    }

    [Fact]
    public void TryParse_BitsConsumedMath_CommonWindowNoMs()
    {
        // 4 (tag) + 1 (cw=1) + 11 (long ics) + 2 (msmp=0)
        //  + first body: 8 (gg) + 9 (sect cb=0 maxSfb=6) + 0 (sf cb=0) + 3 (flags) = 20
        //  + second body: 20
        // Total = 4 + 1 + 11 + 2 + 20 + 20 = 58 bits.
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCommonWindowCpe(
            tag: 0,
            maxSfb: 6,
            msMask: AacMsMaskPresent.None,
            msUsed: [],
            gain1: 0x40,
            gain2: 0x60);
        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.Equal(58, cpe!.BitsConsumed);
    }

    [Fact]
    public void MaxElementInstanceTag_IsFifteen()
    {
        Assert.Equal(15, AacChannelPairElement.MaxElementInstanceTag);
    }

    [Theory]
    [InlineData(AacMsMaskPresent.None, 0)]
    [InlineData(AacMsMaskPresent.PerBand, 1)]
    [InlineData(AacMsMaskPresent.AllBands, 2)]
    [InlineData(AacMsMaskPresent.Reserved, 3)]
    public void AacMsMaskPresent_HasSpecOrdinals(AacMsMaskPresent value, int expected)
    {
        Assert.Equal(expected, (int)value);
    }

    // CPE "full" overload tests

    private static AacHuffmanCodebook BuildFixed7BitCodebook(int symbolCount)
    {
        var lengths = new int[symbolCount];
        for (int i = 0; i < symbolCount; i++) lengths[i] = 7;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook?[] SpectralBooksWith(int slot, AacHuffmanCodebook book)
    {
        var arr = new AacHuffmanCodebook?[16];
        arr[slot] = book;
        return arr;
    }

    private static void WriteIcsBodyWithSection(AacBitWriter w, byte globalGain, int cb, int len)
    {
        // CPE common_window=1 body: NO ics_info, section_data + scale_factor_data + flags.
        w.Write(globalGain, 8);
        w.Write((uint)cb, 4);              // sect_cb
        w.Write((uint)len, 5);             // sect_len
        // scale_factor_data: if cb != 0 we need one SF symbol per band.
        if (cb != 0)
        {
            // 1-bit "0" code = SF diff symbol 60 (zero diff).
            for (int i = 0; i < len; i++) w.Write(0u, 1);
        }
        w.Write(0u, 1);                    // pulse_data_present
        w.Write(0u, 1);                    // tns_data_present
        w.Write(0u, 1);                    // gain_control_data_present
    }

    [Fact]
    public void CpeFull_CommonWindow_BothChannelsEmpty_PopulatesSpectralData()
    {
        var book = BuildSyntheticSfCodebook();
        var spectralBooks = new AacHuffmanCodebook?[16];

        var w = new AacBitWriter();
        w.Write(3u, 4);                    // element_instance_tag
        w.Write(1u, 1);                    // common_window = 1
        WriteLongIcsInfo(w, maxSfb: 10);
        w.Write((uint)AacMsMaskPresent.None, 2);
        WriteIcsBodyWithSection(w, 0x80, cb: 0, len: 10);
        WriteIcsBodyWithSection(w, 0x80, cb: 0, len: 10);

        Assert.True(AacChannelPairElement.TryParse(
            w.ToArray(), book, sampleRate: 48_000, spectralBooks, out var cpe));
        Assert.NotNull(cpe);
        Assert.True(cpe!.CommonWindow);
        Assert.NotNull(cpe.FirstSpectralData);
        Assert.NotNull(cpe.SecondSpectralData);
        Assert.All(cpe.FirstSpectralData!.Coefficients, c => Assert.Equal(0, c));
        Assert.All(cpe.SecondSpectralData!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, cpe.FirstSpectralData.BitsConsumed);
        Assert.Equal(0, cpe.SecondSpectralData.BitsConsumed);
    }

    [Fact]
    public void CpeFull_CommonWindow_BothChannelsCb1_DecodesSpectralCoefficients()
    {
        var book = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = SpectralBooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(5u, 4);
        w.Write(1u, 1);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write((uint)AacMsMaskPresent.None, 2);
        // First channel ICS + spectral_data (1 tuple, cb=1, dim 4).
        WriteIcsBodyWithSection(w, 0x80, cb: 1, len: 1);
        w.Write(80u, 7);                   // symbol 80 -> (1,1,1,1)
        // Second channel ICS + spectral_data.
        WriteIcsBodyWithSection(w, 0x80, cb: 1, len: 1);
        w.Write(40u, 7);                   // symbol 40 -> (0,0,0,0)

        Assert.True(AacChannelPairElement.TryParse(
            w.ToArray(), book, sampleRate: 48_000, spectralBooks, out var cpe));
        Assert.NotNull(cpe);
        Assert.NotNull(cpe!.FirstSpectralData);
        Assert.NotNull(cpe.SecondSpectralData);
        Assert.Equal(1, cpe.FirstSpectralData!.Coefficients[0]);
        Assert.Equal(1, cpe.FirstSpectralData.Coefficients[3]);
        Assert.Equal(0, cpe.SecondSpectralData!.Coefficients[0]);
        Assert.Equal(0, cpe.SecondSpectralData.Coefficients[3]);
        Assert.Equal(7, cpe.FirstSpectralData.BitsConsumed);
        Assert.Equal(7, cpe.SecondSpectralData.BitsConsumed);
    }

    [Fact]
    public void CpeFull_IndependentWindow_EachChannelOwnIcsInfo_ParsesBothSpectral()
    {
        var book = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = SpectralBooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0u, 1);                    // common_window = 0
        // First channel: full body with its own ics_info.
        w.Write(0x80, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);    // sect_cb=1, len=1
        w.Write(0u, 1);                    // sf symbol 60
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(80u, 7);                   // spectral
        // Second channel: same shape.
        w.Write(0x80, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(40u, 7);

        Assert.True(AacChannelPairElement.TryParse(
            w.ToArray(), book, sampleRate: 48_000, spectralBooks, out var cpe));
        Assert.NotNull(cpe);
        Assert.False(cpe!.CommonWindow);
        Assert.Null(cpe.SharedIcsInfo);
        Assert.NotNull(cpe.FirstSpectralData);
        Assert.NotNull(cpe.SecondSpectralData);
        Assert.Equal(1, cpe.FirstSpectralData!.Coefficients[0]);
        Assert.Equal(0, cpe.SecondSpectralData!.Coefficients[0]);
    }

    [Fact]
    public void CpeFull_BoundaryOverload_LeavesSpectralDataNull()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = BuildCommonWindowCpe(
            tag: 2,
            maxSfb: 10,
            msMask: AacMsMaskPresent.None,
            msUsed: Array.Empty<bool[]>(),
            gain1: 0x80,
            gain2: 0x40);

        Assert.True(AacChannelPairElement.TryParse(bytes, book, out var cpe));
        Assert.NotNull(cpe);
        Assert.Null(cpe!.FirstSpectralData);
        Assert.Null(cpe.SecondSpectralData);
    }

    [Fact]
    public void CpeFull_NullSpectralCodebooks_Throws()
    {
        var book = BuildSyntheticSfCodebook();
        var bytes = new byte[32];
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelPairElement.TryParse(
                bytes, book, sampleRate: 48_000, spectralCodebooks: null!, out _));
    }
}
