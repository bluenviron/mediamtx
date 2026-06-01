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
}
