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

    private static readonly int[] SymbolsNoise60 = new[] { 60 };
    private static readonly int[] SymbolsIntensity75 = new[] { 75 };
    private static readonly int[] SymbolsIntensity45 = new[] { 45 };
    private static readonly int[] SymbolsMixed3Sections = new[] { 60, 70, 50 };

    [Fact]
    public void TryRead_NoiseCodebook13_ReadsAsNoiseEnergy()
    {
        var book = BuildSyntheticSfCodebook();
        var sections = MakeSections((0, 13, 0, 1));
        var bytes = EncodeSymbols(SymbolsNoise60);
        var reader = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, book, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Entries);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[0].Kind);
        Assert.Equal(0, data.Entries[0].Differential);
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
        // 3 sections: spectral cb=1, noise cb=13, intensity cb=14, one band each.
        var sections = MakeSections((0, 1, 0, 1), (0, 13, 1, 2), (0, 14, 2, 3));
        var bytes = EncodeSymbols(SymbolsMixed3Sections);
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
}
