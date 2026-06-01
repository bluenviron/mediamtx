using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacPceRawDataBlockDecoderTests
{
    private const int SampleRate = 48000;

    [Fact]
    public void GetExpectedChannelCount_NullPce_Throws()
    {
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.GetExpectedChannelCount(null!));
    }

    [Fact]
    public void GetExpectedChannelCount_EmptyPce_ReturnsZero()
    {
        var pce = AacPceLayoutResolverTests.BuildPce();
        Assert.Equal(0, AacPceRawDataBlockDecoder.GetExpectedChannelCount(pce));
    }

    [Fact]
    public void GetExpectedChannelCount_OneFrontSceTwoFrontCpe_ReturnsFive()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements:
            [
                AacPceLayoutResolverTests.Sce(0),
                AacPceLayoutResolverTests.Cpe(1),
                AacPceLayoutResolverTests.Cpe(2),
            ]);
        // SCE=1 + CPE=2 + CPE=2 = 5
        Assert.Equal(5, AacPceRawDataBlockDecoder.GetExpectedChannelCount(pce));
    }

    [Fact]
    public void GetExpectedChannelCount_FrontPlusLfe_CountsLfeAsOne()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)],
            lfeElements: [0]);
        Assert.Equal(3, AacPceRawDataBlockDecoder.GetExpectedChannelCount(pce));
    }

    [Fact]
    public void GetExpectedChannelCount_CouplingDoesNotCount()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)],
            couplingElements: [AacPceLayoutResolverTests.Coupling(0)]);
        Assert.Equal(1, AacPceRawDataBlockDecoder.GetExpectedChannelCount(pce));
    }

    [Fact]
    public void CreateFilterbanks_NullPce_Throws()
    {
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.CreateFilterbanks(null!));
    }

    [Fact]
    public void CreateFilterbanks_StereoPce_ReturnsTwoNonNullInstances()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)]);
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);
        Assert.Equal(2, fbs.Length);
        Assert.NotNull(fbs[0]);
        Assert.NotNull(fbs[1]);
        Assert.NotSame(fbs[0], fbs[1]);
    }

    [Fact]
    public void DecodeToSamples_NullBlock_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                null!, pce, SampleRate, NewPrng, fbs));
    }

    [Fact]
    public void DecodeToSamples_NullPce_Throws()
    {
        var block = AacPceLayoutResolverTests.BuildBlock();
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, null!, SampleRate, NewPrng,
                Array.Empty<AacSynthesisFilterbank>()));
    }

    [Fact]
    public void DecodeToSamples_NullPrngFactory_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, pce, SampleRate, null!, fbs));
    }

    [Fact]
    public void DecodeToSamples_NullFilterbanks_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        Assert.Throws<ArgumentNullException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, pce, SampleRate, NewPrng, null!));
    }

    [Fact]
    public void DecodeToSamples_FilterbankCountMismatch_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildCpeEntry(0));
        var fbs = new[] { new AacSynthesisFilterbank() }; // 1 != 2 expected
        var ex = Assert.Throws<ArgumentException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, pce, SampleRate, NewPrng, fbs));
        Assert.Contains("does not match", ex.Message);
    }

    [Fact]
    public void DecodeToSamples_FilterbankEntryNull_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var fbs = new AacSynthesisFilterbank[] { null! };
        var ex = Assert.Throws<ArgumentException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, pce, SampleRate, NewPrng, fbs));
        Assert.Contains("filterbanks[0]", ex.Message);
    }

    [Fact]
    public void DecodeToSamples_SingleFrontSce_ProducesOneChannelOutput()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        Assert.Single(decoded.Channels);
        var ch = decoded.Channels[0];
        Assert.Equal(AacPceChannelRegion.Front, ch.Region);
        Assert.Equal(0, ch.RegionIndex);
        Assert.Null(ch.PairIndex);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, ch.Samples.Length);
    }

    [Fact]
    public void DecodeToSamples_FrontCpe_ProducesTwoOutputsWithPairIndices()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildCpeEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        Assert.Equal(2, decoded.Channels.Count);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[0].Region);
        Assert.Equal(0, decoded.Channels[0].RegionIndex);
        Assert.Equal(0, decoded.Channels[0].PairIndex);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[1].Region);
        Assert.Equal(0, decoded.Channels[1].RegionIndex);
        Assert.Equal(1, decoded.Channels[1].PairIndex);
    }

    [Fact]
    public void DecodeToSamples_FrontPlusLfe_ProducesOutputsInPceOrder()
    {
        // Front Cpe(0), Back Sce(1), Lfe(2)
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)],
            backElements: [AacPceLayoutResolverTests.Sce(1)],
            lfeElements: [2]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildCpeEntry(0),
            AacPceLayoutResolverTests.BuildSceEntry(1),
            AacPceLayoutResolverTests.BuildLfeEntry(2));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        // 2 from CPE + 1 from SCE + 1 from LFE = 4
        Assert.Equal(4, decoded.Channels.Count);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[0].Region);
        Assert.Equal(0, decoded.Channels[0].PairIndex);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[1].Region);
        Assert.Equal(1, decoded.Channels[1].PairIndex);
        Assert.Equal(AacPceChannelRegion.Back, decoded.Channels[2].Region);
        Assert.Null(decoded.Channels[2].PairIndex);
        Assert.Equal(AacPceChannelRegion.Lfe, decoded.Channels[3].Region);
        Assert.Null(decoded.Channels[3].PairIndex);
    }

    [Fact]
    public void DecodeToSamples_CouplingPresent_Skipped()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)],
            couplingElements: [AacPceLayoutResolverTests.Coupling(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0),
            AacPceLayoutResolverTests.BuildCceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        // CCE traversed but not in outputs
        Assert.Single(decoded.Channels);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[0].Region);
    }

    [Fact]
    public void DecodeToSamples_PrngFactoryReturnsNull_Throws()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        Func<AacPnsRandom> badFactory = () => null!;
        Assert.Throws<InvalidOperationException>(
            () => AacPceRawDataBlockDecoder.DecodeToSamples(
                block, pce, SampleRate, badFactory, fbs));
    }

    [Fact]
    public void DecodeToSamples_FilterbankStatePersistsAcrossFrames()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block1 = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var block2 = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var frame1 = AacPceRawDataBlockDecoder.DecodeToSamples(
            block1, pce, SampleRate, NewPrng, fbs);
        var frame2 = AacPceRawDataBlockDecoder.DecodeToSamples(
            block2, pce, SampleRate, NewPrng, fbs);

        // Second frame uses overlap-add state from the first; if the
        // dispatcher silently discarded the dictionary, frame2 would
        // ramp from zero again and equal frame1 in their leading half.
        var s1 = frame1.Channels[0].Samples;
        var s2 = frame2.Channels[0].Samples;
        bool anyDiff = false;
        for (int i = 0; i < 128; i++)
        {
            if (s1[i] != s2[i]) { anyDiff = true; break; }
        }
        Assert.True(anyDiff, "Second frame should differ from first due to overlap-add carry-over.");
    }

    [Fact]
    public void DecodeToSamples_AotOverload_ProducesSameChannelCountAsBaseOverload()
    {
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Cpe(0)],
            lfeElements: [1]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildCpeEntry(0),
            AacPceLayoutResolverTests.BuildLfeEntry(1));

        var baseFbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);
        var aotFbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var baseDecoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, baseFbs);
        var aotDecoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, AacAudioObjectType.AacLc, aotFbs);

        Assert.Equal(baseDecoded.Channels.Count, aotDecoded.Channels.Count);
        for (int i = 0; i < baseDecoded.Channels.Count; i++)
        {
            Assert.Equal(baseDecoded.Channels[i].Region, aotDecoded.Channels[i].Region);
            Assert.Equal(baseDecoded.Channels[i].RegionIndex, aotDecoded.Channels[i].RegionIndex);
            Assert.Equal(baseDecoded.Channels[i].PairIndex, aotDecoded.Channels[i].PairIndex);
        }
    }

    [Fact]
    public void DecodeToSamples_FilterbanksConsumedInExpansionOrder()
    {
        // Two front CPEs: should consume filterbanks 0,1,2,3 in order.
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements:
            [
                AacPceLayoutResolverTests.Cpe(0),
                AacPceLayoutResolverTests.Cpe(1),
            ]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildCpeEntry(0),
            AacPceLayoutResolverTests.BuildCpeEntry(1));

        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);
        Assert.Equal(4, fbs.Length);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        Assert.Equal(4, decoded.Channels.Count);
        // Verify ordering: (front,0,0) (front,0,1) (front,1,0) (front,1,1)
        Assert.Equal(0, decoded.Channels[0].RegionIndex);
        Assert.Equal(0, decoded.Channels[0].PairIndex);
        Assert.Equal(0, decoded.Channels[1].RegionIndex);
        Assert.Equal(1, decoded.Channels[1].PairIndex);
        Assert.Equal(1, decoded.Channels[2].RegionIndex);
        Assert.Equal(0, decoded.Channels[2].PairIndex);
        Assert.Equal(1, decoded.Channels[3].RegionIndex);
        Assert.Equal(1, decoded.Channels[3].PairIndex);
    }

    // ----- EightShort coverage -----

    [Fact]
    public void DecodeToSamples_SingleFrontShortSce_ProducesOneChannelWithFullFrame()
    {
        // PCE-driven dispatch must accept an EightShort SCE and route it
        // through the short-window filterbank path.
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildShortSceEntry(0));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        Assert.Single(decoded.Channels);
        var ch = decoded.Channels[0];
        Assert.Equal(AacPceChannelRegion.Front, ch.Region);
        Assert.Equal(0, ch.RegionIndex);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, ch.Samples.Length);
        Assert.Contains(ch.Samples, s => s != 0f);
    }

    [Fact]
    public void DecodeToSamples_MixedLongAndShortAcrossPceRegions_ProducesAllChannels()
    {
        // Front SCE long + Back SCE short — the PCE walker must dispatch
        // each entry to the correct filterbank for its own window sequence.
        var pce = AacPceLayoutResolverTests.BuildPce(
            frontElements: [AacPceLayoutResolverTests.Sce(0)],
            backElements: [AacPceLayoutResolverTests.Sce(1)]);
        var block = AacPceLayoutResolverTests.BuildBlock(
            AacPceLayoutResolverTests.BuildSceEntry(0),
            AacPceLayoutResolverTests.BuildShortSceEntry(1));
        var fbs = AacPceRawDataBlockDecoder.CreateFilterbanks(pce);

        var decoded = AacPceRawDataBlockDecoder.DecodeToSamples(
            block, pce, SampleRate, NewPrng, fbs);

        Assert.Equal(2, decoded.Channels.Count);
        Assert.Equal(AacPceChannelRegion.Front, decoded.Channels[0].Region);
        Assert.Equal(AacPceChannelRegion.Back, decoded.Channels[1].Region);
        Assert.All(decoded.Channels, ch =>
            Assert.Equal(AacSynthesisFilterbank.LongFrameLength, ch.Samples.Length));
    }

    private static AacPnsRandom NewPrng() => new(seed: 1u);
}
