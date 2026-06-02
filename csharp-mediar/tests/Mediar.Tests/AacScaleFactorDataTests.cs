using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacScaleFactorDataTests
{
    private static readonly int[] SymbolsDiffZero = new[] { 60 };
    private static readonly int[] SymbolsExtreme = new[] { 0, 60, 120 };
    private static readonly int[] SymbolsMixed = new[] { 75, 45 };
    private static readonly int[] SymbolsMultiGroup = new[] { 60, 70, 50 };
    private static readonly int[] SymbolsSingleZero = new[] { 60 };

    // Build a synthetic 121-entry scale-factor codebook:
    //   symbol 60 (diff 0) -> "0" (1 bit)
    //   symbols other than 60 -> 8-bit fixed-length codes "1xxxxxxx"
    // Canonical sum: 2^-1 + 120 * 2^-8 = 0.96875 (incomplete tree, Kraft valid).
    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static (uint bits, int length) EncodeSymbol(int symbol)
    {
        if (symbol == 60) return (0u, 1);
        // After symbol 60's "0", canonical next code at length 8 = (0 + 1) << 7 = 0x80.
        int position = symbol < 60 ? symbol : symbol - 1;
        return ((uint)(0x80 + position), 8);
    }

    private static byte[] EncodeSymbols(int[] symbols)
    {
        var w = new AacBitWriter();
        foreach (var s in symbols)
        {
            var (bits, len) = EncodeSymbol(s);
            w.Write(bits, len);
        }
        return w.ToArray();
    }

    private static AacSectionData MakeSections(params (int group, int cb, int startSfb, int endSfb)[] sections)
    {
        var list = new List<AacSection>();
        foreach (var s in sections)
        {
            list.Add(new AacSection { Group = s.group, CodebookNumber = s.cb, StartSfb = s.startSfb, EndSfb = s.endSfb });
        }
        return new AacSectionData { Sections = list };
    }

    [Fact]
    public void TryRead_EmptySectionList_ReturnsEmpty()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections();
        var reader = new BitReader(new byte[] { 0 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Empty(data!.Entries);
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void TryRead_ZeroCodebook_EmitsNoneEntriesWithoutReadingBits()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 0, 0, 3));
        var reader = new BitReader(new byte[] { 0xFF });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.None, e.Kind);
            Assert.Equal(0, e.Differential);
        }
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void TryRead_ReservedCodebook12_EmitsNoneEntries()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 12, 0, 2));
        var reader = new BitReader(new byte[] { 0 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(2, data!.Entries.Count);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.None, e.Kind);
        }
        Assert.Equal(0, data.BitsConsumed);
    }

    private static readonly int[] SymbolsIntensity75 = new[] { 75 };
    private static readonly int[] SymbolsIntensity45 = new[] { 45 };

    [Fact]
    public void TryRead_NoiseCodebook13_FirstBandReadsAs9BitPcm()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        // First PNS band uses 9-bit unsigned PCM (raw - 256). Encode raw=256 -> diff=0.
        var w = new AacBitWriter();
        w.Write(256u, 9);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[0].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
        Assert.Equal(9, data.BitsConsumed);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_FirstBand9BitNegative()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        // raw=128 -> diff=-128 (in the [-256, +255] range that 9-bit PCM allows).
        var w = new AacBitWriter();
        w.Write(128u, 9);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[0].Kind);
        Assert.Equal(-128, data.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_MultiBand_FirstIsPcm_RestAreHuffman()
    {
        var book = BuildSyntheticSfCodebook();
        // Two PNS bands in one section.
        var sections = MakeSections((0, 13, 0, 2));
        var w = new AacBitWriter();
        w.Write(256u, 9);                 // first band: raw=256 -> diff=0
        var (b, l) = EncodeSymbol(70);    // second band: huffman symbol 70 -> diff=10
        w.Write(b, l);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(2, data!.Entries.Count);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[0].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[1].Kind);
        Assert.Equal(10, data.Entries[1].Differential);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_MultiSection_OnlyFirstPnsBandPcm()
    {
        var book = BuildSyntheticSfCodebook();
        // 3 sections: spectral(cb=1, 1 band) + noise(cb=13, 1 band) + noise(cb=13, 1 band)
        var sections = MakeSections((0, 1, 0, 1), (0, 13, 1, 2), (0, 13, 2, 3));
        var w = new AacBitWriter();
        var (b1, l1) = EncodeSymbol(60); w.Write(b1, l1);   // spectral diff 0
        w.Write(256u, 9);                                    // first PNS band (PCM): diff 0
        var (b2, l2) = EncodeSymbol(50); w.Write(b2, l2);   // second PNS band (huffman): diff -10
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[0].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[1].Kind);
        Assert.Equal(0, data.Entries[1].Differential);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[2].Kind);
        Assert.Equal(-10, data.Entries[2].Differential);
    }

    [Fact]
    public void TryRead_IntensityCodebook14_ReadsAsIntensityPosition()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 14, 0, 1));
        var bytes = EncodeSymbols(SymbolsIntensity75);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.IntensityPosition, data.Entries[0].Kind);
        Assert.Equal(15, data.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_IntensityCodebook15_ReadsAsIntensityPosition()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 15, 0, 1));
        var bytes = EncodeSymbols(SymbolsIntensity45);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.IntensityPosition, data.Entries[0].Kind);
        Assert.Equal(-15, data.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_MixedSections_KindsTagCorrectly()
    {
        var book = BuildSyntheticSfCodebook();
        // 3 sections: spectral cb=1, noise cb=13 (first PNS band uses 9-bit PCM), intensity cb=14.
        var sections = MakeSections((0, 1, 0, 1), (0, 13, 1, 2), (0, 14, 2, 3));
        var w = new AacBitWriter();
        var (b1, l1) = EncodeSymbol(60); w.Write(b1, l1);   // spectral diff 0
        w.Write(266u, 9);                                    // first PNS band PCM: diff +10
        var (b3, l3) = EncodeSymbol(50); w.Write(b3, l3);   // intensity diff -10
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[0].Kind);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[1].Kind);
        Assert.Equal(AacScaleFactorKind.IntensityPosition, data.Entries[2].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
        Assert.Equal(10, data.Entries[1].Differential);
        Assert.Equal(-10, data.Entries[2].Differential);
    }

    [Fact]
    public void TryRead_SingleBandDiffZero_RoundTrip()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 1, 0, 1));
        var bytes = EncodeSymbols(SymbolsDiffZero);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        var e = data.Entries[0];
        Assert.Equal(0, e.Group);
        Assert.Equal(0, e.Sfb);
        Assert.Equal(AacScaleFactorKind.SpectralGain, e.Kind);
        Assert.Equal(0, e.Differential);
        Assert.Equal(1, data.BitsConsumed);
    }

    [Fact]
    public void TryRead_ExtremeDifferentials_PreservesSign()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 3, 0, 3));
        var bytes = EncodeSymbols(SymbolsExtreme);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Collection(data!.Entries,
            e => Assert.Equal(-60, e.Differential),
            e => Assert.Equal(0, e.Differential),
            e => Assert.Equal(60, e.Differential));
    }

    [Fact]
    public void TryRead_MultipleSections_MixedKinds_OnlySpectralReadsBits()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections(
            (0, 0, 0, 2),
            (0, 5, 2, 4),
            (0, 12, 4, 5));
        var bytes = EncodeSymbols(SymbolsMixed);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(5, data!.Entries.Count);
        Assert.Equal(AacScaleFactorKind.None, data.Entries[0].Kind);
        Assert.Equal(AacScaleFactorKind.None, data.Entries[1].Kind);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[2].Kind);
        Assert.Equal(15, data.Entries[2].Differential);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[3].Kind);
        Assert.Equal(-15, data.Entries[3].Differential);
        Assert.Equal(AacScaleFactorKind.None, data.Entries[4].Kind);
        Assert.Equal(16, data.BitsConsumed);
    }

    [Fact]
    public void TryRead_MultipleGroups_PreservesGroupIndex()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections(
            (0, 1, 0, 2),
            (1, 1, 0, 1));
        var bytes = EncodeSymbols(SymbolsMultiGroup);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        Assert.Equal(0, data.Entries[0].Group);
        Assert.Equal(0, data.Entries[1].Group);
        Assert.Equal(1, data.Entries[2].Group);
        Assert.Equal(0, data.Entries[0].Sfb);
        Assert.Equal(1, data.Entries[1].Sfb);
        Assert.Equal(0, data.Entries[2].Sfb);
    }

    [Fact]
    public void TryRead_StreamUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 1, 0, 2));
        // First band: 1-bit "0" (diff=0). Second band starts with "1" (enters 8-bit branch),
        // but only 6 trailing zero pad bits remain - decoder cannot consume the 8th bit.
        var w = new AacBitWriter();
        w.Write(0u, 1); // band 0 -> diff=0 code "0"
        w.Write(1u, 1); // band 1 partial: "1" prefix only, 6 pad zeros follow, then EOF.
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.False(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryRead_WrongCodebookSize_Rejected()
    {
        var lengths = new int[120];
        for (int i = 0; i < 120; i++) lengths[i] = 7;
        var badBook = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        var sections = MakeSections((0, 1, 0, 1));
        var reader = new BitReader(new byte[] { 0 });
        Assert.False(AacScaleFactorData.TryRead(ref reader, sections, badBook, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryRead_NullSectionData_Throws()
    {
        var book = BuildSyntheticSfCodebook();
        var reader = new BitReader(new byte[] { 0 });
        AacScaleFactorData? data;
        try
        {
            AacScaleFactorData.TryRead(ref reader, null!, book, out data);
            Assert.Fail("Expected ArgumentNullException");
        }
        catch (ArgumentNullException) { /* expected */ }
    }

    [Fact]
    public void TryRead_NullCodebook_Throws()
    {
        var sections = MakeSections();
        var reader = new BitReader(new byte[] { 0 });
        AacScaleFactorData? data;
        try
        {
            AacScaleFactorData.TryRead(ref reader, sections, null!, out data);
            Assert.Fail("Expected ArgumentNullException");
        }
        catch (ArgumentNullException) { /* expected */ }
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    [InlineData(6)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(10)]
    [InlineData(11)]
    public void TryRead_SpectralCodebooks_1To11_Are_SpectralGain(int cb)
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, cb, 0, 1));
        var bytes = EncodeSymbols(SymbolsSingleZero);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[0].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
    }

    [Theory]
    [InlineData(16)]
    [InlineData(31)]
    [InlineData(99)]
    public void TryRead_OutOfRangeCodebook_Treated_As_None(int cb)
    {
        // The classifier has a catch-all `_ => None` arm for codebooks
        // outside [0, 15]. Such sections must emit None entries without
        // consuming any bits.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, cb, 0, 2));
        var reader = new BitReader(new byte[] { 0xFF });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Equal(2, data!.Entries.Count);
        Assert.All(data.Entries, e => Assert.Equal(AacScaleFactorKind.None, e.Kind));
        Assert.All(data.Entries, e => Assert.Equal(0, e.Differential));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Record_AacScaleFactorEntry_Equality_By_Value()
    {
        var a = new AacScaleFactorEntry
        {
            Group = 0,
            Sfb = 1,
            Kind = AacScaleFactorKind.SpectralGain,
            Differential = -3,
        };
        var b = new AacScaleFactorEntry
        {
            Group = 0,
            Sfb = 1,
            Kind = AacScaleFactorKind.SpectralGain,
            Differential = -3,
        };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
    }

    [Fact]
    public void Record_AacScaleFactorEntry_With_Expression_Returns_Modified_Copy()
    {
        var a = new AacScaleFactorEntry
        {
            Group = 0,
            Sfb = 0,
            Kind = AacScaleFactorKind.SpectralGain,
            Differential = 0,
        };
        var b = a with { Differential = 42 };
        Assert.Equal(42, b.Differential);
        Assert.Equal(0, a.Differential);
        Assert.NotSame(a, b);
    }

    [Fact]
    public void Record_AacScaleFactorData_With_Expression_Modifies_BitsConsumed()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 1, 0, 1));
        var bytes = EncodeSymbols(SymbolsSingleZero);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        var modified = data! with { BitsConsumed = 999 };
        Assert.Equal(999, modified.BitsConsumed);
        Assert.Equal(1, data.BitsConsumed);
        Assert.NotSame(data, modified);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_FirstBand_RawZero_GivesDiff_MinusTwoFiftySix()
    {
        // Boundary case: raw 9-bit value 0 maps to differential -256.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        var w = new AacBitWriter();
        w.Write(0u, 9);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Single(data!.Entries);
        Assert.Equal(-256, data.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_FirstBand_RawMax_GivesDiff_PlusTwoFiftyFive()
    {
        // Boundary case: raw 9-bit value 511 maps to differential 255.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        var w = new AacBitWriter();
        w.Write(511u, 9);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Single(data!.Entries);
        Assert.Equal(255, data.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_NoiseCodebook13_PcmFlag_Resets_Across_Calls()
    {
        // The first PNS band of *each* scale_factor_data() call uses
        // 9-bit PCM. Two separate TryRead calls must each see the first
        // PNS band as 9-bit PCM.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));

        var w1 = new AacBitWriter();
        w1.Write(256u, 9); // first call's PCM band
        var reader1 = new BitReader(w1.ToArray());
        Assert.True(AacScaleFactorData.TryRead(ref reader1, sections, book, out var data1));
        Assert.Equal(9, data1!.BitsConsumed);

        var w2 = new AacBitWriter();
        w2.Write(266u, 9); // second call's PCM band
        var reader2 = new BitReader(w2.ToArray());
        Assert.True(AacScaleFactorData.TryRead(ref reader2, sections, book, out var data2));
        Assert.Equal(9, data2!.BitsConsumed);
        Assert.Equal(10, data2.Entries[0].Differential);
    }

    [Fact]
    public void TryRead_NoiseEnergy_PnsAfterSpectral_FirstNoiseBand_Uses_NinBit_Pcm()
    {
        // PNS comes after a spectral section. The first PNS band is
        // still the first PNS band overall and uses 9-bit PCM.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 1, 0, 1), (0, 13, 1, 2));
        var w = new AacBitWriter();
        var (b1, l1) = EncodeSymbol(60); w.Write(b1, l1);   // spectral: 1 bit
        w.Write(266u, 9);                                    // first PNS band: 9 bits, diff +10
        var reader = new BitReader(w.ToArray());
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Equal(2, data!.Entries.Count);
        Assert.Equal(AacScaleFactorKind.SpectralGain, data.Entries[0].Kind);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[1].Kind);
        Assert.Equal(10, data.Entries[1].Differential);
        Assert.Equal(10, data.BitsConsumed); // 1 + 9
    }

    [Fact]
    public void TryRead_BitsConsumed_Equals_Sum_Of_Each_Section_Bits()
    {
        // Mix spectral (1+8 bits), PNS first band (9 bits), intensity (8 bits)
        // -> expected total = 1 + 8 + 9 + 8 = 26.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 1, 0, 2), (0, 13, 2, 3), (0, 14, 3, 4));
        var w = new AacBitWriter();
        var (b1, l1) = EncodeSymbol(60); w.Write(b1, l1); // 1 bit
        var (b2, l2) = EncodeSymbol(70); w.Write(b2, l2); // 8 bits
        w.Write(256u, 9);                                  // 9 bits PCM
        var (b3, l3) = EncodeSymbol(50); w.Write(b3, l3); // 8 bits
        var reader = new BitReader(w.ToArray());
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Equal(26, data!.BitsConsumed);
    }

    [Fact]
    public void TryRead_PnsPcm_StreamUnderflow_Returns_False()
    {
        // 13 has the 9-bit PCM first-band path, but only 4 bits remain
        // in the buffer. The decoder must return false rather than
        // overrun.
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        // 1-byte buffer; advance reader by 4 bits so only 4 are left.
        var reader = new BitReader(new byte[] { 0x00 });
        reader.ReadBits(4);
        Assert.False(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.Null(data);
    }
}
