using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacFrameDecoderTests
{
    [Fact]
    public void Ctor_NullConfig_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacFrameDecoder(
            config: null!,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullScaleFactorCodebook_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacFrameDecoder(
            config: BuildAsc(channelConfig: 1),
            scaleFactorCodebook: null!,
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullSpectralCodebooks_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacFrameDecoder(
            config: BuildAsc(channelConfig: 1),
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: null!));
    }

    [Fact]
    public void Ctor_ChannelConfigZero_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(() => new AacFrameDecoder(
            config: BuildAsc(channelConfig: 0),
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
        Assert.Contains("AacPceRawDataBlockDecoder", ex.Message);
    }

    [Fact]
    public void Ctor_ChannelConfigEight_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(() => new AacFrameDecoder(
            config: BuildAsc(channelConfig: 8),
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
        Assert.Contains("channelConfiguration 1..7", ex.Message);
    }

    [Fact]
    public void Ctor_Mono_SpeakersIsSingleFrontCenter()
    {
        var dec = BuildDecoder(channelConfig: 1);
        Assert.Single(dec.Speakers);
        Assert.Equal(AacSpeaker.FrontCentre, dec.Speakers[0]);
    }

    [Fact]
    public void Ctor_Mono_ExposesSampleRateAndConfig()
    {
        var dec = BuildDecoder(channelConfig: 1, sampleRate: 44_100, sfIndex: 4);
        Assert.Equal(44_100, dec.SampleRate);
        Assert.Equal(1, dec.Config.ChannelConfiguration);
    }

    [Fact]
    public void DecodeFrame_Empty_Throws()
    {
        var dec = BuildDecoder(channelConfig: 1);
        Assert.Throws<ArgumentException>(() => dec.DecodeFrame(ReadOnlySpan<byte>.Empty));
    }

    [Fact]
    public void DecodeFrame_GarbageBytes_ThrowsInvalidData()
    {
        var dec = BuildDecoder(channelConfig: 1);
        // ID = 6 (FIL) with cnt = 15 + escape = 270 bytes; truncated.
        var garbage = new byte[] { 0xCF, 0xFF, 0x00 };
        Assert.Throws<InvalidDataException>(() => dec.DecodeFrame(garbage));
    }

    [Fact]
    public void DecodeFrame_NoEndSentinel_ThrowsInvalidData()
    {
        // A buffer that runs out of bits before an END id arrives.
        // 3 zero bits = SCE id (0). With no payload, parser will
        // either fail mid-SCE (InvalidData) or run out before END.
        var dec = BuildDecoder(channelConfig: 1);
        // Single byte of all zeros: 3 bits SCE id, then 5 bits of partial SCE body.
        var noEnd = new byte[] { 0x00 };
        Assert.Throws<InvalidDataException>(() => dec.DecodeFrame(noEnd));
    }

    [Fact]
    public void DecodeFrame_MonoSceThenEnd_ProducesFrontCenterPcm()
    {
        var dec = BuildDecoder(channelConfig: 1);
        var bytes = BuildMonoSceBlock(tag: 0, maxSfb: 10);

        var decoded = dec.DecodeFrame(bytes);

        Assert.Single(decoded.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, decoded.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[0].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_TwoFramesInSequence_BothShapeCorrect()
    {
        var dec = BuildDecoder(channelConfig: 1);
        var bytes = BuildMonoSceBlock(tag: 0, maxSfb: 10);

        var frame1 = dec.DecodeFrame(bytes);
        var frame2 = dec.DecodeFrame(bytes);

        Assert.Single(frame1.Channels);
        Assert.Single(frame2.Channels);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, frame1.Channels[0].Samples.Length);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, frame2.Channels[0].Samples.Length);
        // Filterbank-state carry-over across frames is verified in lower
        // dispatcher tests (AacRawDataBlockDecoderTests,
        // AacPceRawDataBlockDecoderTests) where non-trivial spectral
        // fixtures expose the difference; an all-zero spectrum here
        // produces zero PCM regardless of overlap state.
    }

    [Fact]
    public void ResetState_AllowsSubsequentDecodes()
    {
        var dec = BuildDecoder(channelConfig: 1);
        var bytes = BuildMonoSceBlock(tag: 0, maxSfb: 10);

        _ = dec.DecodeFrame(bytes);
        dec.ResetState();
        var frameAfterReset = dec.DecodeFrame(bytes);

        // Smoke test: ResetState + decode must not throw, and the
        // returned shape is still correct.
        Assert.Single(frameAfterReset.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, frameAfterReset.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, frameAfterReset.Channels[0].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_CustomPrngFactory_IsUsed()
    {
        int prngCalls = 0;
        AacPnsRandom Factory()
        {
            prngCalls++;
            return new AacPnsRandom(seed: 42u);
        }

        var dec = new AacFrameDecoder(
            BuildAsc(channelConfig: 1),
            GetSf(),
            new AacHuffmanCodebook?[16],
            prngFactory: Factory);

        var bytes = BuildMonoSceBlock(tag: 0, maxSfb: 10);
        _ = dec.DecodeFrame(bytes);

        // Mono frame: dispatcher requests one PRNG via the factory.
        Assert.Equal(1, prngCalls);
    }

    [Fact]
    public void DecodeFrame_StereoIndependentCpe_ProducesFrontLeftRightPcm()
    {
        var dec = BuildDecoder(channelConfig: 2);
        var bytes = BuildStereoIndependentCpeBlock(tag: 0, maxSfb: 10);

        var decoded = dec.DecodeFrame(bytes);

        Assert.Equal(2, decoded.Channels.Count);
        Assert.Equal(AacSpeaker.FrontLeft, decoded.Channels[0].Speaker);
        Assert.Equal(AacSpeaker.FrontRight, decoded.Channels[1].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[0].Samples.Length);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[1].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_StereoCommonWindowCpe_ProducesFrontLeftRightPcm()
    {
        var dec = BuildDecoder(channelConfig: 2);
        var bytes = BuildStereoCommonWindowCpeBlock(tag: 0, maxSfb: 10);

        var decoded = dec.DecodeFrame(bytes);

        Assert.Equal(2, decoded.Channels.Count);
        Assert.Equal(AacSpeaker.FrontLeft, decoded.Channels[0].Speaker);
        Assert.Equal(AacSpeaker.FrontRight, decoded.Channels[1].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[0].Samples.Length);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[1].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_StereoFacadeSpeakers_MatchMappingForConfig2()
    {
        var dec = BuildDecoder(channelConfig: 2);

        Assert.Equal(2, dec.Speakers.Count);
        Assert.Equal(AacSpeaker.FrontLeft, dec.Speakers[0]);
        Assert.Equal(AacSpeaker.FrontRight, dec.Speakers[1]);
    }

    [Fact]
    public void DecodeFrame_StereoTwoFramesInSequence_FilterbankStateCarriesPerChannel()
    {
        var dec = BuildDecoder(channelConfig: 2);
        var bytes = BuildStereoIndependentCpeBlock(tag: 0, maxSfb: 10);

        var frame1 = dec.DecodeFrame(bytes);
        var frame2 = dec.DecodeFrame(bytes);

        Assert.Equal(2, frame1.Channels.Count);
        Assert.Equal(2, frame2.Channels.Count);
        // Two separate filterbanks (one per stereo channel) carry overlap
        // state independently across frames. Empty spectra produce zero
        // PCM, so the only invariant checkable here is shape parity; the
        // numeric carry-over is exercised by the lower dispatcher tests.
        Assert.Equal(
            AacSynthesisFilterbank.LongFrameLength,
            frame1.Channels[0].Samples.Length);
        Assert.Equal(
            AacSynthesisFilterbank.LongFrameLength,
            frame2.Channels[1].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_StereoChannelConfig2_ReportsChannelCountTwo()
    {
        var dec = BuildDecoder(channelConfig: 2);

        Assert.Equal(2, dec.Config.ChannelConfiguration);
        Assert.Equal(2, dec.Config.ChannelCount);
    }

    // ----- EightShort window tests -----

    [Fact]
    public void DecodeFrame_MonoEightShort_ProducesFrontCenterPcmOfCorrectLength()
    {
        var dec = BuildDecoder(channelConfig: 1);
        var bytes = BuildShortWindowMonoSceBlock(tag: 0);

        var decoded = dec.DecodeFrame(bytes);

        Assert.Single(decoded.Channels);
        Assert.Equal(AacSpeaker.FrontCentre, decoded.Channels[0].Speaker);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, decoded.Channels[0].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_MonoEightShort_TwoConsecutiveFrames_BothHaveCorrectShape()
    {
        var dec = BuildDecoder(channelConfig: 1);
        var bytes = BuildShortWindowMonoSceBlock(tag: 0);

        var frame1 = dec.DecodeFrame(bytes);
        var frame2 = dec.DecodeFrame(bytes);

        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, frame1.Channels[0].Samples.Length);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, frame2.Channels[0].Samples.Length);
    }

    [Fact]
    public void DecodeFrame_MonoEightShort_ResetState_ProducesIdenticalOutputAsFreshDecoder()
    {
        var bytes = BuildShortWindowMonoSceBlock(tag: 0);

        var dec1 = BuildDecoder(channelConfig: 1);
        var frame1 = dec1.DecodeFrame(bytes);

        var dec2 = BuildDecoder(channelConfig: 1);
        // Burn one long-window frame to dirty the overlap buffer,
        // then reset so the next EightShort starts from a clean slate.
        _ = dec2.DecodeFrame(BuildMonoSceBlock(tag: 0, maxSfb: 4));
        dec2.ResetState();
        var frame2 = dec2.DecodeFrame(bytes);

        Assert.Equal(frame1.Channels[0].Samples, frame2.Channels[0].Samples);
    }

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AudioSpecificConfig BuildAsc(
        int aot = 2,
        int sfIndex = 3,            // 48000 Hz
        int sampleRate = 48_000,
        int channelConfig = 2)
    {
        return new AudioSpecificConfig
        {
            AudioObjectType = aot,
            SamplingFrequencyIndex = sfIndex,
            SamplingFrequency = sampleRate,
            ChannelConfiguration = channelConfig,
            ChannelCount = channelConfig is > 0 and < 8 ? channelConfig : 0,
            SbrPresent = false,
        };
    }

    private static AacFrameDecoder BuildDecoder(
        int channelConfig,
        int sampleRate = 48_000,
        int sfIndex = 3,
        int aot = 2)
    {
        return new AacFrameDecoder(
            BuildAsc(aot: aot, sfIndex: sfIndex, sampleRate: sampleRate, channelConfig: channelConfig),
            GetSf(),
            new AacHuffmanCodebook?[16]);
    }

    private static byte[] BuildMonoSceBlock(int tag, int maxSfb)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        AacRawDataBlockTests.WriteEmptySceBodyShared(w, tag, maxSfb);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildStereoIndependentCpeBlock(int tag, int maxSfb)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.ChannelPairElement, 3);
        AacChannelPairElementTests.WriteIndependentCpeBodyShared(
            w, tag, maxSfb, gain1: 0, gain2: 0);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    private static byte[] BuildStereoCommonWindowCpeBlock(int tag, int maxSfb)
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.ChannelPairElement, 3);
        AacChannelPairElementTests.WriteCommonWindowCpeBodyShared(
            w, tag, maxSfb, gain1: 0, gain2: 0);
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }

    /// <summary>
    /// 4-SFB EightShort SCE: all 8 windows in one group, ZERO_HCB
    /// sections, global_gain = 0. Produces all-zero PCM but exercises
    /// the short-window parse + filterbank dispatch path.
    /// </summary>
    private static byte[] BuildShortWindowMonoSceBlock(int tag)
    {
        const int maxSfb = 4;
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        // SCE body:
        w.Write((uint)tag, 4);                             // element_instance_tag
        w.Write(0u, 8);                                    // global_gain
        w.Write(0u, 1);                                    // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2);    // window_sequence
        w.Write(0u, 1);                                    // window_shape = Sine
        w.Write((uint)maxSfb, 4);                          // max_sfb (4 bits for short)
        w.Write(0x7Fu, 7);                                 // scale_factor_grouping: all 1s = 1 group of 8 windows
        // section_data: 1 group, ZERO_HCB covering all 4 SFBs
        w.Write(0u, 4);                                    // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)maxSfb, 3);                          // sect_len_incr = 4 (< escape 7)
        // No scale_factor_data (all ZERO_HCB)
        // No pulse_data for short window (flag still present)
        w.Write(0u, 1);                                    // pulse_data_present = 0
        w.Write(0u, 1);                                    // tns_data_present = 0
        w.Write(0u, 1);                                    // gain_control_data_present = 0
        // No spectral_data (all ZERO_HCB)
        w.Write((uint)AacSyntacticElementType.End, 3);
        return w.ToArray();
    }
}
