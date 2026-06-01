using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacScaleFactorDataStandardCodebookTests
{
    private static AacSectionData MakeSections(params (int group, int cb, int startSfb, int endSfb)[] sections)
    {
        var list = new List<AacSection>();
        foreach (var s in sections)
        {
            list.Add(new AacSection
            {
                Group = s.group,
                CodebookNumber = s.cb,
                StartSfb = s.startSfb,
                EndSfb = s.endSfb,
            });
        }
        return new AacSectionData { Sections = list };
    }

    [Fact]
    public void Overload_EmptySectionList_ReturnsEmpty()
    {
        var sections = MakeSections();
        var reader = new BitReader(new byte[] { 0 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Empty(data!.Entries);
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_ZeroCodebook_EmitsNoneEntries()
    {
        var sections = MakeSections((0, 0, 0, 4));
        var reader = new BitReader(new byte[] { 0xFF });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Equal(4, data!.Entries.Count);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.None, e.Kind);
            Assert.Equal(0, e.Differential);
        }
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_ReservedCodebook12_EmitsNoneEntries()
    {
        var sections = MakeSections((0, 12, 0, 3));
        var reader = new BitReader(new byte[] { 0 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.None, e.Kind);
        }
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_SpectralGain_AllZeroDelta_DecodesSingleBitPerBand()
    {
        // The canonical SF codebook encodes delta=0 (symbol 60) as the
        // single-bit "0". Four bands of cb 1 with deltas 0,0,0,0 should
        // consume exactly 4 bits and produce SpectralGain entries with
        // Differential==0.
        var sections = MakeSections((0, 1, 0, 4));
        var reader = new BitReader(new byte[] { 0b0000_0000 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Equal(4, data!.Entries.Count);
        Assert.Equal(4, data.BitsConsumed);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.SpectralGain, e.Kind);
            Assert.Equal(0, e.Differential);
        }
    }

    [Fact]
    public void Overload_NoiseEnergy_FirstBandUsesNinebitPcm()
    {
        // PNS codebook 13: first band uses 9-bit dpcm_noise_nrg with
        // 256 offset; second uses Huffman.
        // 9 bits of 0_0001_0000 (=16) → diff = 16 - 256 = -240.
        // Then single-bit "0" for symbol 60 (delta 0) on second band.
        // Bitstream: 000010000 0 padding → 10 bits.
        // Packed MSB-first: 00001_0000 | 0xxx_xxxx
        // = 0b0000_1000 0b0000_0000 = 0x08, 0x00.
        var sections = MakeSections((0, 13, 0, 2));
        var reader = new BitReader(new byte[] { 0x08, 0x00 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Equal(2, data!.Entries.Count);

        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[0].Kind);
        Assert.Equal(-240, data.Entries[0].Differential);

        Assert.Equal(AacScaleFactorKind.NoiseEnergy, data.Entries[1].Kind);
        Assert.Equal(0, data.Entries[1].Differential);

        Assert.Equal(10, data.BitsConsumed);
    }

    [Fact]
    public void Overload_IntensityPosition_ReadsHuffmanPerBand()
    {
        // Intensity cb 14: each band reads a Huffman codeword from the
        // SF codebook (delta = symbol - 60). With three zero-delta
        // bands we should see 3 SpectralGain-like reads of "0" each.
        var sections = MakeSections((0, 14, 0, 3));
        var reader = new BitReader(new byte[] { 0b0000_0000 });
        Assert.True(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Entries.Count);
        foreach (var e in data.Entries)
        {
            Assert.Equal(AacScaleFactorKind.IntensityPosition, e.Kind);
            Assert.Equal(0, e.Differential);
        }
        Assert.Equal(3, data.BitsConsumed);
    }

    [Fact]
    public void Overload_ProducesIdenticalEntries_To_ExplicitCodebookOverload()
    {
        // The two overloads must produce identical results when the
        // explicit overload is given the standard codebook.
        var sections = MakeSections((0, 1, 0, 2), (0, 0, 2, 3), (0, 14, 3, 4));
        byte[] bytes = { 0b0000_0000 };

        var readerA = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(ref readerA, sections, out var dataA));

        var readerB = new BitReader(bytes);
        Assert.True(AacScaleFactorData.TryRead(
            ref readerB, sections, AacStandardScaleFactorCodebook.Book, out var dataB));

        Assert.NotNull(dataA);
        Assert.NotNull(dataB);
        Assert.Equal(dataA!.BitsConsumed, dataB!.BitsConsumed);
        Assert.Equal(dataA.Entries.Count, dataB.Entries.Count);
        for (int i = 0; i < dataA.Entries.Count; i++)
        {
            Assert.Equal(dataA.Entries[i].Group, dataB.Entries[i].Group);
            Assert.Equal(dataA.Entries[i].Sfb, dataB.Entries[i].Sfb);
            Assert.Equal(dataA.Entries[i].Kind, dataB.Entries[i].Kind);
            Assert.Equal(dataA.Entries[i].Differential, dataB.Entries[i].Differential);
        }
    }

    [Fact]
    public void Overload_UnderflowOnEmptyBuffer_ForNonzeroCb_ReturnsFalse()
    {
        // A spectral-gain section with no input bits cannot decode and
        // must signal failure cleanly.
        var sections = MakeSections((0, 1, 0, 1));
        var reader = new BitReader(ReadOnlySpan<byte>.Empty);
        Assert.False(AacScaleFactorData.TryRead(ref reader, sections, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void Overload_NullSectionData_Throws()
    {
        var reader = new BitReader(new byte[] { 0 });
        BitReader r = reader;
        // ArgumentNullException from the underlying overload should
        // surface through the convenience entry point too.
        try
        {
            AacScaleFactorData.TryRead(ref r, null!, out _);
            Assert.Fail("Expected ArgumentNullException.");
        }
        catch (ArgumentNullException)
        {
            // expected
        }
    }
}
