using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelLayoutResolverTests
{
    private static AacRawDataBlockEntry Entry(AacSyntacticElementType t) => new()
    {
        Type = t,
        BitOffset = 0,
    };

    private static AacRawDataBlock Block(params AacRawDataBlockEntry[] entries) => new()
    {
        Entries = entries,
        TerminatedByEnd = true,
        BitsConsumed = entries.Length * 3,
    };

    [Fact]
    public void Resolve_NullBlock_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelLayoutResolver.Resolve(null!, 2));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(8)]
    [InlineData(15)]
    public void Resolve_OutOfRangeChannelConfig_Throws(int config)
    {
        var block = Block(Entry(AacSyntacticElementType.End));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelLayoutResolver.Resolve(block, config));
    }

    [Fact]
    public void Resolve_Config1_MatchesSce()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 1);
        Assert.Single(resolved);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, resolved[0].RawEntry.Type);
        Assert.NotNull(resolved[0].Mapping);
        Assert.Equal(AacSpeaker.FrontCentre, resolved[0].Mapping!.FirstSpeaker);
    }

    [Fact]
    public void Resolve_Config2_MatchesCpe()
    {
        var block = Block(
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 2);
        Assert.Single(resolved);
        Assert.Equal(AacSpeaker.FrontLeft, resolved[0].Mapping!.FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, resolved[0].Mapping!.SecondSpeaker);
    }

    [Fact]
    public void Resolve_Config6_5_1_SceCpeCpeLfe()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.LfeChannelElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 6);
        Assert.Equal(4, resolved.Count);
        Assert.Equal(AacSpeaker.FrontCentre, resolved[0].Mapping!.FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontLeft, resolved[1].Mapping!.FirstSpeaker);
        Assert.Equal(AacSpeaker.SurroundLeft, resolved[2].Mapping!.FirstSpeaker);
        Assert.Equal(AacSpeaker.Lfe, resolved[3].Mapping!.FirstSpeaker);
    }

    [Fact]
    public void Resolve_TooFewAudioElements_Throws()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.End));   // config 3 needs SCE + CPE

        Assert.Throws<InvalidOperationException>(() =>
            AacChannelLayoutResolver.Resolve(block, 3));
    }

    [Fact]
    public void Resolve_TooManyAudioElements_Throws()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.SingleChannelElement),   // config 1 only expects 1 SCE
            Entry(AacSyntacticElementType.End));

        Assert.Throws<InvalidOperationException>(() =>
            AacChannelLayoutResolver.Resolve(block, 1));
    }

    [Fact]
    public void Resolve_WrongElementKind_Throws()
    {
        // Config 2 expects CPE but we send a SCE.
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.End));

        Assert.Throws<InvalidOperationException>(() =>
            AacChannelLayoutResolver.Resolve(block, 2));
    }

    [Fact]
    public void Resolve_DseFilPcePresent_AreSkipped()
    {
        var block = Block(
            Entry(AacSyntacticElementType.FillElement),
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.DataStreamElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.ProgramConfigElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 3);
        Assert.Equal(2, resolved.Count);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, resolved[0].RawEntry.Type);
        Assert.Equal(AacSyntacticElementType.ChannelPairElement, resolved[1].RawEntry.Type);
    }

    [Fact]
    public void Resolve_CceInterleaved_SurfacedAsAuxiliary()
    {
        // CCE between SCE and CPE -> auxiliary; not counted against
        // the speaker mapping.
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.CouplingChannelElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 3);
        Assert.Equal(3, resolved.Count);

        // Speaker-bound entries are at indices 0 and 2.
        Assert.NotNull(resolved[0].Mapping);
        Assert.Equal(AacSpeaker.FrontCentre, resolved[0].Mapping!.FirstSpeaker);

        Assert.Null(resolved[1].Mapping);
        Assert.Equal(AacSyntacticElementType.CouplingChannelElement, resolved[1].RawEntry.Type);

        Assert.NotNull(resolved[2].Mapping);
        Assert.Equal(AacSpeaker.FrontLeft, resolved[2].Mapping!.FirstSpeaker);
    }

    [Fact]
    public void FilterSpeakerEntries_OnlyMappedReturned()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.CouplingChannelElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 3);
        var speakers = AacChannelLayoutResolver.FilterSpeakerEntries(resolved);

        Assert.Equal(2, speakers.Count);
        Assert.All(speakers, s => Assert.NotNull(s.Mapping));
    }

    [Fact]
    public void FilterCouplingEntries_OnlyCceReturned()
    {
        var block = Block(
            Entry(AacSyntacticElementType.SingleChannelElement),
            Entry(AacSyntacticElementType.CouplingChannelElement),
            Entry(AacSyntacticElementType.CouplingChannelElement),
            Entry(AacSyntacticElementType.ChannelPairElement),
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 3);
        var cces = AacChannelLayoutResolver.FilterCouplingEntries(resolved);

        Assert.Equal(2, cces.Count);
        Assert.All(cces, c => Assert.Equal(AacSyntacticElementType.CouplingChannelElement, c.RawEntry.Type));
    }

    [Fact]
    public void FilterSpeakerEntries_NullList_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelLayoutResolver.FilterSpeakerEntries(null!));
    }

    [Fact]
    public void FilterCouplingEntries_NullList_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelLayoutResolver.FilterCouplingEntries(null!));
    }

    [Fact]
    public void Resolve_EmptyBlock_FailsForNonZeroConfig()
    {
        var block = Block();
        Assert.Throws<InvalidOperationException>(() =>
            AacChannelLayoutResolver.Resolve(block, 1));
    }

    [Fact]
    public void Resolve_PreservesRawEntryOrder()
    {
        // Even with non-audio elements interleaved, audio-element
        // ordering in the resolved list must match the original
        // raw_data_block order.
        var sce = Entry(AacSyntacticElementType.SingleChannelElement);
        var cpe1 = Entry(AacSyntacticElementType.ChannelPairElement);
        var cpe2 = Entry(AacSyntacticElementType.ChannelPairElement);

        var block = Block(
            sce,
            Entry(AacSyntacticElementType.FillElement),
            cpe1,
            Entry(AacSyntacticElementType.DataStreamElement),
            cpe2,
            Entry(AacSyntacticElementType.End));

        var resolved = AacChannelLayoutResolver.Resolve(block, 5);
        Assert.Equal(3, resolved.Count);
        Assert.Same(sce, resolved[0].RawEntry);
        Assert.Same(cpe1, resolved[1].RawEntry);
        Assert.Same(cpe2, resolved[2].RawEntry);
    }
}
