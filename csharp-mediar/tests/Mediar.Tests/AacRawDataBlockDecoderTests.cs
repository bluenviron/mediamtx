using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacRawDataBlockDecoderTests
{
    private const int Sr48k = 48_000;

    // ----- GetExpectedSpeakers -----

    [Fact]
    public void GetExpectedSpeakers_Config0_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacRawDataBlockDecoder.GetExpectedSpeakers(0));
    }

    [Fact]
    public void GetExpectedSpeakers_Config8_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacRawDataBlockDecoder.GetExpectedSpeakers(8));
    }

    [Fact]
    public void GetExpectedSpeakers_Mono_ReturnsFrontCentre()
    {
        var speakers = AacRawDataBlockDecoder.GetExpectedSpeakers(1);
        Assert.Equal(new[] { AacSpeaker.FrontCentre }, speakers.ToArray());
    }

    [Fact]
    public void GetExpectedSpeakers_Stereo_ReturnsLeftRight()
    {
        var speakers = AacRawDataBlockDecoder.GetExpectedSpeakers(2);
        Assert.Equal(new[] { AacSpeaker.FrontLeft, AacSpeaker.FrontRight }, speakers.ToArray());
    }

    [Fact]
    public void GetExpectedSpeakers_FivePointOne_ReturnsSixSpeakers()
    {
        var speakers = AacRawDataBlockDecoder.GetExpectedSpeakers(6);
        Assert.Equal(6, speakers.Count);
        Assert.Contains(AacSpeaker.FrontCentre, speakers);
        Assert.Contains(AacSpeaker.FrontLeft, speakers);
        Assert.Contains(AacSpeaker.FrontRight, speakers);
        Assert.Contains(AacSpeaker.SurroundLeft, speakers);
        Assert.Contains(AacSpeaker.SurroundRight, speakers);
        Assert.Contains(AacSpeaker.Lfe, speakers);
    }

    [Fact]
    public void GetExpectedSpeakers_SevenPointOne_ReturnsEightSpeakers()
    {
        var speakers = AacRawDataBlockDecoder.GetExpectedSpeakers(7);
        Assert.Equal(8, speakers.Count);
        Assert.Contains(AacSpeaker.Lfe, speakers);
    }

    // ----- CreateFilterbanks -----

    [Fact]
    public void CreateFilterbanks_Mono_HasOneSpeaker()
    {
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Single(fbs);
        Assert.True(fbs.ContainsKey(AacSpeaker.FrontCentre));
    }

    [Fact]
    public void CreateFilterbanks_FivePointOne_HasSixSpeakers()
    {
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(6);
        Assert.Equal(6, fbs.Count);
        foreach (var s in AacRawDataBlockDecoder.GetExpectedSpeakers(6))
        {
            Assert.True(fbs.ContainsKey(s), $"missing filterbank for {s}");
        }
    }

    [Fact]
    public void CreateFilterbanks_FilterbanksAreDistinctInstances()
    {
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(2);
        var left = fbs[AacSpeaker.FrontLeft];
        var right = fbs[AacSpeaker.FrontRight];
        Assert.NotSame(left, right);
    }

    // ----- DecodeToSamples: argument validation -----

    [Fact]
    public void DecodeToSamples_NullBlock_Throws()
    {
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                null!, 1, Sr48k, () => new AacPnsRandom(), fbs));
    }

    [Fact]
    public void DecodeToSamples_NullPrngFactory_Throws()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                block, 1, Sr48k, null!, fbs));
    }

    [Fact]
    public void DecodeToSamples_NullFilterbanks_Throws()
    {
        var block = BuildMonoBlock();
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                block, 1, Sr48k, () => new AacPnsRandom(), null!));
    }

    [Fact]
    public void DecodeToSamples_MissingSpeakerFilterbank_Throws()
    {
        var block = BuildMonoBlock();
        var fbs = new Dictionary<AacSpeaker, AacSynthesisFilterbank>();
        var ex = Assert.Throws<ArgumentException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                block, 1, Sr48k, () => new AacPnsRandom(), fbs));
        Assert.Contains("FrontCentre", ex.Message);
    }

    // ----- DecodeToSamples: behaviour -----

    [Fact]
    public void DecodeToSamples_Mono_ProducesOneChannel()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 1u), fbs);

        Assert.Single(result.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, result.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, result.Channels[0].Samples.Length);
    }

    [Fact]
    public void DecodeToSamples_Mono_ParityWithDirectMonoCall()
    {
        var frame = AacChannelDecoderTests.BuildFrameNoPns();
        var block = BuildBlockFromEntries(new[]
        {
            BuildSceEntry(frame),
        });

        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 4u), fbs);

        var directFb = new AacSynthesisFilterbank();
        var directOut = new float[AacSynthesisFilterbank.LongFrameLength];
        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(seed: 4u), directFb, directOut);

        Assert.Equal(directOut, result.Channels[0].Samples);
    }

    [Fact]
    public void DecodeToSamples_Stereo_ProducesTwoChannels()
    {
        var block = BuildStereoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(2);
        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 2, Sr48k, () => new AacPnsRandom(seed: 8u), fbs);

        Assert.Equal(2, result.Channels.Count);
        Assert.Equal(AacSpeaker.FrontLeft, result.Channels[0].Speaker);
        Assert.Equal(AacSpeaker.FrontRight, result.Channels[1].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, result.Channels[0].Samples.Length);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, result.Channels[1].Samples.Length);
    }

    [Fact]
    public void DecodeToSamples_FilterbankStatePersistsAcrossFrames()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);

        var r1 = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 1u), fbs);
        var r2 = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 1u), fbs);

        // Frame 1 has half a frame of silent ramp-up; frame 2 has full overlap-add
        // state from the previous frame, so the outputs MUST differ in some sample.
        bool anyDiff = false;
        for (int i = 0; i < r1.Channels[0].Samples.Length; i++)
        {
            if (r1.Channels[0].Samples[i] != r2.Channels[0].Samples[i])
            {
                anyDiff = true;
                break;
            }
        }
        Assert.True(anyDiff, "Filterbank state did not persist between frames");
    }

    [Fact]
    public void DecodeToSamples_FiveOneWithLfe_ProducesSixChannelsIncludingLfe()
    {
        var block = BuildFiveOneBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(6);
        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 6, Sr48k, () => new AacPnsRandom(seed: 100u), fbs);

        Assert.Equal(6, result.Channels.Count);
        var lfeChan = result.Channels.FirstOrDefault(c => c.Speaker == AacSpeaker.Lfe);
        Assert.NotNull(lfeChan);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, lfeChan!.Samples.Length);
    }

    [Fact]
    public void DecodeToSamples_CceTraversedButProducesNoChannel()
    {
        var sce = BuildSceEntry(AacChannelDecoderTests.BuildFrameNoPns());
        var cce = BuildCceEntry();
        var block = BuildBlockFromEntries(new[] { sce, cce });
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);

        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 5u), fbs);

        // CCE is auxiliary; only the SCE produces a channel
        Assert.Single(result.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, result.Channels[0].Speaker);
    }

    [Fact]
    public void DecodeToSamples_Aot_Mono_ProducesOneChannel()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        var result = AacRawDataBlockDecoder.DecodeToSamples(
            block, 1, Sr48k, () => new AacPnsRandom(seed: 7u),
            AacAudioObjectType.AacLc, fbs);

        Assert.Single(result.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, result.Channels[0].Speaker);
    }

    [Fact]
    public void DecodeToSamples_Aot_NullBlock_Throws()
    {
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                null!, 1, Sr48k, () => new AacPnsRandom(),
                AacAudioObjectType.AacLc, fbs));
    }

    [Fact]
    public void DecodeToSamples_Aot_NullPrngFactory_Throws()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                block, 1, Sr48k, null!, AacAudioObjectType.AacLc, fbs));
    }

    [Fact]
    public void DecodeToSamples_PrngFactoryReturnsNull_Throws()
    {
        var block = BuildMonoBlock();
        var fbs = AacRawDataBlockDecoder.CreateFilterbanks(1);
        Assert.Throws<InvalidOperationException>(() =>
            AacRawDataBlockDecoder.DecodeToSamples(
                block, 1, Sr48k, () => null!, fbs));
    }

    // ----- helpers -----

    private static AacRawDataBlockEntry BuildSceEntry(AacChannelFrame frame, int tag = 0)
    {
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

    private static AacRawDataBlockEntry BuildCpeEntry(
        AacChannelFrame leftFrame,
        AacChannelFrame rightFrame,
        int tag = 0)
    {
        var cpe = new AacChannelPairElement
        {
            ElementInstanceTag = tag,
            CommonWindow = false,
            SharedIcsInfo = null,
            MsMaskPresent = AacMsMaskPresent.None,
            MsUsed = Array.Empty<IReadOnlyList<bool>>(),
            FirstStream = leftFrame.Stream,
            SecondStream = rightFrame.Stream,
            FirstSpectralData = leftFrame.SpectralData,
            SecondSpectralData = rightFrame.SpectralData,
            BitsConsumed = 0,
        };
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.ChannelPairElement,
            BitOffset = 0,
            ChannelPair = cpe,
        };
    }

    private static AacRawDataBlockEntry BuildLfeEntry(AacChannelFrame frame, int tag = 0)
    {
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

    private static AacRawDataBlockEntry BuildCceEntry()
    {
        var cce = AacChannelDecoderTests.BuildCceCb1NoPns();
        return new AacRawDataBlockEntry
        {
            Type = AacSyntacticElementType.CouplingChannelElement,
            BitOffset = 0,
            CouplingChannel = cce,
        };
    }

    private static AacRawDataBlock BuildBlockFromEntries(IReadOnlyList<AacRawDataBlockEntry> entries)
    {
        return new AacRawDataBlock
        {
            Entries = entries,
            TerminatedByEnd = true,
            BitsConsumed = 0,
        };
    }

    private static AacRawDataBlock BuildMonoBlock()
    {
        var frame = AacChannelDecoderTests.BuildFrameNoPns();
        return BuildBlockFromEntries(new[] { BuildSceEntry(frame) });
    }

    private static AacRawDataBlock BuildStereoBlock()
    {
        var l = AacChannelDecoderTests.BuildFrameNoPns();
        var r = AacChannelDecoderTests.BuildFrameNoPns();
        return BuildBlockFromEntries(new[] { BuildCpeEntry(l, r) });
    }

    private static AacRawDataBlock BuildFiveOneBlock()
    {
        // Config 6 (5.1) = SCE C, CPE L R, CPE Ls Rs, LFE
        var c = AacChannelDecoderTests.BuildFrameNoPns();
        var l = AacChannelDecoderTests.BuildFrameNoPns();
        var r = AacChannelDecoderTests.BuildFrameNoPns();
        var ls = AacChannelDecoderTests.BuildFrameNoPns();
        var rs = AacChannelDecoderTests.BuildFrameNoPns();
        var lfe = AacChannelDecoderTests.BuildFrameNoPns();
        return BuildBlockFromEntries(new[]
        {
            BuildSceEntry(c, tag: 0),
            BuildCpeEntry(l, r, tag: 0),
            BuildCpeEntry(ls, rs, tag: 1),
            BuildLfeEntry(lfe, tag: 0),
        });
    }
}
