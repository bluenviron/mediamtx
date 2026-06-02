using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacPceLayoutResolverTests
{
    [Fact]
    public void Resolve_NullBlock_Throws()
    {
        var pce = BuildPce(
            frontElements: new[] { Cpe(0) });
        Assert.Throws<ArgumentNullException>(
            () => AacPceLayoutResolver.Resolve(null!, pce));
    }

    [Fact]
    public void Resolve_NullPce_Throws()
    {
        var block = BuildBlock();
        Assert.Throws<ArgumentNullException>(
            () => AacPceLayoutResolver.Resolve(block, null!));
    }

    [Fact]
    public void Resolve_EmptyPce_ReturnsEmptyList()
    {
        var pce = BuildPce();
        var block = BuildBlock();
        var resolved = AacPceLayoutResolver.Resolve(block, pce);
        Assert.Empty(resolved);
    }

    [Fact]
    public void Resolve_SingleFrontCpe_ResolvesToBlockEntry()
    {
        var pce = BuildPce(frontElements: new[] { Cpe(3) });
        var cpeEntry = BuildCpeEntry(tag: 3);
        var block = BuildBlock(cpeEntry);

        var resolved = AacPceLayoutResolver.Resolve(block, pce);

        Assert.Single(resolved);
        Assert.Equal(AacPceChannelRegion.Front, resolved[0].Region);
        Assert.Equal(0, resolved[0].RegionIndex);
        Assert.Same(cpeEntry, resolved[0].RawEntry);
    }

    [Fact]
    public void Resolve_FrontSceAndCpe_ResolvedInOrder()
    {
        var pce = BuildPce(frontElements: new[] { Sce(0), Cpe(1) });
        var sceEntry = BuildSceEntry(tag: 0);
        var cpeEntry = BuildCpeEntry(tag: 1);
        var block = BuildBlock(cpeEntry, sceEntry); // block order != PCE order

        var resolved = AacPceLayoutResolver.Resolve(block, pce);

        Assert.Equal(2, resolved.Count);
        // PCE order wins; same region with sequential indices
        Assert.Same(sceEntry, resolved[0].RawEntry);
        Assert.Equal(0, resolved[0].RegionIndex);
        Assert.Same(cpeEntry, resolved[1].RawEntry);
        Assert.Equal(1, resolved[1].RegionIndex);
    }

    [Fact]
    public void Resolve_FrontSideBackLfeCouplingOrder()
    {
        var pce = BuildPce(
            frontElements: new[] { Sce(0) },
            sideElements: new[] { Cpe(1) },
            backElements: new[] { Cpe(2) },
            lfeElements: [0],
            couplingElements: new[] { Coupling(5) });

        var sceFront = BuildSceEntry(tag: 0);
        var cpeSide = BuildCpeEntry(tag: 1);
        var cpeBack = BuildCpeEntry(tag: 2);
        var lfeEntry = BuildLfeEntry(tag: 0);
        var cceEntry = BuildCceEntry(tag: 5);
        var block = BuildBlock(sceFront, cpeSide, cpeBack, lfeEntry, cceEntry);

        var resolved = AacPceLayoutResolver.Resolve(block, pce);

        Assert.Equal(5, resolved.Count);
        Assert.Equal(AacPceChannelRegion.Front, resolved[0].Region);
        Assert.Equal(AacPceChannelRegion.Side, resolved[1].Region);
        Assert.Equal(AacPceChannelRegion.Back, resolved[2].Region);
        Assert.Equal(AacPceChannelRegion.Lfe, resolved[3].Region);
        Assert.Equal(AacPceChannelRegion.Coupling, resolved[4].Region);
        Assert.Same(sceFront, resolved[0].RawEntry);
        Assert.Same(cpeSide, resolved[1].RawEntry);
        Assert.Same(cpeBack, resolved[2].RawEntry);
        Assert.Same(lfeEntry, resolved[3].RawEntry);
        Assert.Same(cceEntry, resolved[4].RawEntry);
    }

    [Fact]
    public void Resolve_MissingTag_Throws()
    {
        var pce = BuildPce(frontElements: new[] { Cpe(7) });
        var block = BuildBlock(BuildCpeEntry(tag: 0));

        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("element_instance_tag=7", ex.Message);
        Assert.Contains("Front slot #0", ex.Message);
    }

    [Fact]
    public void Resolve_KindMismatch_Throws()
    {
        // PCE expects SCE at tag 4 but block has CPE at tag 4
        var pce = BuildPce(frontElements: new[] { Sce(4) });
        var block = BuildBlock(BuildCpeEntry(tag: 4));

        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("SingleChannelElement", ex.Message);
    }

    [Fact]
    public void Resolve_MultipleEntriesWithSameTagButDifferentKinds_PicksMatchingKind()
    {
        // Tag 0 exists as both SCE and LFE; PCE wants LFE
        var pce = BuildPce(lfeElements: [0]);
        var sceEntry = BuildSceEntry(tag: 0);
        var lfeEntry = BuildLfeEntry(tag: 0);
        var block = BuildBlock(sceEntry, lfeEntry);

        var resolved = AacPceLayoutResolver.Resolve(block, pce);

        Assert.Single(resolved);
        Assert.Same(lfeEntry, resolved[0].RawEntry);
        Assert.Equal(AacPceChannelRegion.Lfe, resolved[0].Region);
    }

    [Fact]
    public void Resolve_MultipleLfes_ResolvedInPceOrder()
    {
        var pce = BuildPce(lfeElements: [1, 0]);
        var lfe0 = BuildLfeEntry(tag: 0);
        var lfe1 = BuildLfeEntry(tag: 1);
        var block = BuildBlock(lfe0, lfe1);

        var resolved = AacPceLayoutResolver.Resolve(block, pce);

        Assert.Equal(2, resolved.Count);
        Assert.Same(lfe1, resolved[0].RawEntry); // tag 1 first per PCE order
        Assert.Same(lfe0, resolved[1].RawEntry);
        Assert.Equal(0, resolved[0].RegionIndex);
        Assert.Equal(1, resolved[1].RegionIndex);
    }

    [Fact]
    public void Resolve_MissingTag_InSide_ThrowsWithSideMessage()
    {
        var pce = BuildPce(sideElements: new[] { Sce(7) });
        var block = BuildBlock(BuildSceEntry(tag: 0));
        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("Side slot #0", ex.Message);
        Assert.Contains("element_instance_tag=7", ex.Message);
    }

    [Fact]
    public void Resolve_MissingTag_InBack_ThrowsWithBackMessage()
    {
        var pce = BuildPce(backElements: new[] { Cpe(3) });
        var block = BuildBlock(BuildCpeEntry(tag: 0));
        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("Back slot #0", ex.Message);
        Assert.Contains("ChannelPairElement", ex.Message);
    }

    [Fact]
    public void Resolve_MissingLfeTag_Throws()
    {
        var pce = BuildPce(lfeElements: [5]);
        var block = BuildBlock(BuildLfeEntry(tag: 0));
        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("Lfe slot #0", ex.Message);
        Assert.Contains("LfeChannelElement", ex.Message);
    }

    [Fact]
    public void Resolve_MissingCouplingTag_Throws()
    {
        var pce = BuildPce(couplingElements: new[] { Coupling(9) });
        var block = BuildBlock(BuildCceEntry(tag: 0));
        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("Coupling slot #0", ex.Message);
        Assert.Contains("CouplingChannelElement", ex.Message);
    }

    [Fact]
    public void Resolve_KindMismatch_CpeExpectedButLfeProvidedAtSameTag_Throws()
    {
        var pce = BuildPce(frontElements: new[] { Cpe(2) });
        var block = BuildBlock(BuildLfeEntry(tag: 2));
        var ex = Assert.Throws<InvalidOperationException>(
            () => AacPceLayoutResolver.Resolve(block, pce));
        Assert.Contains("ChannelPairElement", ex.Message);
    }

    [Fact]
    public void Resolve_MultipleCouplings_ResolvedInPceOrder()
    {
        var pce = BuildPce(couplingElements: new[] { Coupling(3), Coupling(1), Coupling(2) });
        var cc1 = BuildCceEntry(tag: 1);
        var cc2 = BuildCceEntry(tag: 2);
        var cc3 = BuildCceEntry(tag: 3);
        var block = BuildBlock(cc1, cc2, cc3);

        var resolved = AacPceLayoutResolver.Resolve(block, pce);
        Assert.Equal(3, resolved.Count);
        Assert.Same(cc3, resolved[0].RawEntry);
        Assert.Same(cc1, resolved[1].RawEntry);
        Assert.Same(cc2, resolved[2].RawEntry);
        Assert.All(resolved, r => Assert.Equal(AacPceChannelRegion.Coupling, r.Region));
    }

    [Fact]
    public void Resolve_AssocDataElements_AreIgnored()
    {
        // PCE may carry assoc-data tags but the resolver should not surface
        // them as channel slots; an otherwise-empty PCE should yield an empty list.
        var pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = 0,
            ObjectType = 1,
            SamplingFrequencyIndex = 3,
            FrontElements = Array.Empty<AacPceChannelSlot>(),
            SideElements = Array.Empty<AacPceChannelSlot>(),
            BackElements = Array.Empty<AacPceChannelSlot>(),
            LfeElements = Array.Empty<int>(),
            AssocDataElements = new[] { 0, 1, 2 },
            CouplingElements = Array.Empty<AacPceCouplingSlot>(),
            CommentField = string.Empty,
        };
        var block = BuildBlock();
        var resolved = AacPceLayoutResolver.Resolve(block, pce);
        Assert.Empty(resolved);
    }

    [Fact]
    public void Resolve_RegionIndex_Restarts_PerRegion()
    {
        // Each region has its own index counter starting at 0.
        var pce = BuildPce(
            frontElements: new[] { Sce(0), Sce(1), Sce(2) },
            backElements: new[] { Cpe(3) });
        var block = BuildBlock(
            BuildSceEntry(0), BuildSceEntry(1), BuildSceEntry(2),
            BuildCpeEntry(3));

        var resolved = AacPceLayoutResolver.Resolve(block, pce);
        Assert.Equal(4, resolved.Count);
        Assert.Equal(0, resolved[0].RegionIndex);
        Assert.Equal(1, resolved[1].RegionIndex);
        Assert.Equal(2, resolved[2].RegionIndex);
        Assert.Equal(0, resolved[3].RegionIndex); // back restarts at 0
    }

    [Fact]
    public void AacPceResolvedEntry_Record_Equality_Across_Same_Fields()
    {
        var sce = BuildSceEntry(0);
        var a = new AacPceResolvedEntry { RawEntry = sce, Region = AacPceChannelRegion.Front, RegionIndex = 0 };
        var b = new AacPceResolvedEntry { RawEntry = sce, Region = AacPceChannelRegion.Front, RegionIndex = 0 };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
    }

    [Fact]
    public void AacPceResolvedEntry_With_Expression_Mutates_RegionIndex()
    {
        var sce = BuildSceEntry(0);
        var original = new AacPceResolvedEntry { RawEntry = sce, Region = AacPceChannelRegion.Front, RegionIndex = 0 };
        var mutated = original with { RegionIndex = 5 };
        Assert.Equal(5, mutated.RegionIndex);
        Assert.Same(sce, mutated.RawEntry);
        Assert.Equal(AacPceChannelRegion.Front, mutated.Region);
        Assert.Equal(0, original.RegionIndex);
    }

    // ----- helpers -----

    internal static AacPceChannelSlot Sce(int tag) => new() { IsCpe = false, TagSelect = tag };
    internal static AacPceChannelSlot Cpe(int tag) => new() { IsCpe = true, TagSelect = tag };
    internal static AacPceCouplingSlot Coupling(int tag) =>
        new() { IsIndependentlySwitched = false, TagSelect = tag };

    internal static AacProgramConfigurationElement BuildPce(
        IReadOnlyList<AacPceChannelSlot>? frontElements = null,
        IReadOnlyList<AacPceChannelSlot>? sideElements = null,
        IReadOnlyList<AacPceChannelSlot>? backElements = null,
        IReadOnlyList<int>? lfeElements = null,
        IReadOnlyList<AacPceCouplingSlot>? couplingElements = null)
    {
        return new AacProgramConfigurationElement
        {
            ElementInstanceTag = 0,
            ObjectType = 1,
            SamplingFrequencyIndex = 3,
            FrontElements = frontElements ?? Array.Empty<AacPceChannelSlot>(),
            SideElements = sideElements ?? Array.Empty<AacPceChannelSlot>(),
            BackElements = backElements ?? Array.Empty<AacPceChannelSlot>(),
            LfeElements = lfeElements ?? Array.Empty<int>(),
            AssocDataElements = Array.Empty<int>(),
            CouplingElements = couplingElements ?? Array.Empty<AacPceCouplingSlot>(),
            CommentField = string.Empty,
        };
    }

    internal static AacRawDataBlock BuildBlock(params AacRawDataBlockEntry[] entries)
    {
        return new AacRawDataBlock
        {
            Entries = entries,
            TerminatedByEnd = true,
            BitsConsumed = 0,
        };
    }

    internal static AacRawDataBlockEntry BuildSceEntry(int tag)
    {
        // Build a minimal SCE with a real frame so the typed payload is valid
        var frame = AacChannelDecoderTests.BuildFrameNoPns();
        var sce = new AacSingleChannelElement
        {
            ElementInstanceTag = tag,
            Stream = frame.Stream,
            SpectralData = frame.SpectralData,
            BitsConsumed = 0,
        };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.SingleChannelElement,
            BitOffset = 0,
            SingleChannel = sce,
        };
    }

    internal static AacRawDataBlockEntry BuildShortSceEntry(int tag)
    {
        // EightShort-windowed SCE entry; payload built from a real
        // AacChannelFrame so the SpectralData lengths line up.
        var frame = AacChannelDecoderTests.BuildShortFrameNoTnsShared();
        var sce = new AacSingleChannelElement
        {
            ElementInstanceTag = tag,
            Stream = frame.Stream,
            SpectralData = frame.SpectralData,
            BitsConsumed = 0,
        };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.SingleChannelElement,
            BitOffset = 0,
            SingleChannel = sce,
        };
    }

    internal static AacRawDataBlockEntry BuildCpeEntry(int tag)
    {
        var l = AacChannelDecoderTests.BuildFrameNoPns();
        var r = AacChannelDecoderTests.BuildFrameNoPns();
        var cpe = new AacChannelPairElement
        {
            ElementInstanceTag = tag,
            CommonWindow = false,
            SharedIcsInfo = null,
            MsMaskPresent = AacMsMaskPresent.None,
            MsUsed = Array.Empty<IReadOnlyList<bool>>(),
            FirstStream = l.Stream,
            SecondStream = r.Stream,
            FirstSpectralData = l.SpectralData,
            SecondSpectralData = r.SpectralData,
            BitsConsumed = 0,
        };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.ChannelPairElement,
            BitOffset = 0,
            ChannelPair = cpe,
        };
    }

    internal static AacRawDataBlockEntry BuildLfeEntry(int tag)
    {
        var frame = AacChannelDecoderTests.BuildFrameNoPns();
        var lfe = new AacLowFrequencyElement
        {
            ElementInstanceTag = tag,
            Stream = frame.Stream,
            SpectralData = frame.SpectralData,
            BitsConsumed = 0,
        };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.LfeChannelElement,
            BitOffset = 0,
            LowFrequency = lfe,
        };
    }

    internal static AacRawDataBlockEntry BuildCceEntry(int tag)
    {
        var cce = AacChannelDecoderTests.BuildCceCb1NoPns() with { ElementInstanceTag = tag };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.CouplingChannelElement,
            BitOffset = 0,
            CouplingChannel = cce,
        };
    }
}
