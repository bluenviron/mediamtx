using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the Phase 2a <c>CeltDecoder</c> skeleton. The decoder is
/// internal, so we exercise it through <c>InternalsVisibleTo</c>. Real
/// audio output is verified once Phase 2d ships.
/// </summary>
public sealed class CeltDecoderTests
{
    [Fact]
    public void Constructor_Rejects_Invalid_Channel_Count()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        Assert.Throws<ArgumentOutOfRangeException>(() => new CeltDecoder(mode, 0));
        Assert.Throws<ArgumentOutOfRangeException>(() => new CeltDecoder(mode, 3));
    }

    [Fact]
    public void Constructor_Rejects_Uninitialised_Mode()
    {
        Assert.Throws<ArgumentException>(() => new CeltDecoder(default, 1));
    }

    [Fact]
    public void Newly_Constructed_Decoder_IsFirstFrame()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 20_000);
        var dec = new CeltDecoder(mode, 1);
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
        Assert.Equal(1, dec.Channels);
        Assert.Equal(mode, dec.Mode);
    }

    [Fact]
    public void DecodeFrame_Emits_Silent_Block_Of_Correct_Size()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        for (int i = 0; i < output.Length; i++) output[i] = 0.5f; // dirty buffer

        byte[] dummyPayload = new byte[16];
        dummyPayload[0] = 0x80;
        var rd = new OpusRangeDecoder(dummyPayload);

        int produced = dec.DecodeFrame(ref rd, output);
        Assert.Equal(mode.SamplesPerFrame, produced);
        for (int i = 0; i < output.Length; i++)
            Assert.Equal(0.0f, output[i]); // silence
        Assert.False(dec.IsFirstFrame);
        Assert.Equal(mode.SamplesPerFrame, dec.SamplesProduced);
    }

    [Fact]
    public void DecodeFrame_Rejects_Buffer_Too_Small()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> tooSmall = new float[10];
        Assert.Throws<ArgumentException>(() =>
        {
            var local = new CeltDecoder(mode, 2);
            Span<float> small = new float[10];
            byte[] buf = { 0x80 };
            var rd = new OpusRangeDecoder(buf);
            local.DecodeFrame(ref rd, small);
        });
    }

    [Fact]
    public void Reset_Restores_FirstFrame_And_Clears_Counter()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 10_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> buf = new float[mode.SamplesPerFrame];
        byte[] dummyPayload = { 0x80 };
        var rd = new OpusRangeDecoder(dummyPayload);
        dec.DecodeFrame(ref rd, buf);
        Assert.False(dec.IsFirstFrame);
        Assert.True(dec.SamplesProduced > 0);

        dec.Reset();
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
    }

    [Fact]
    public void OpusDecoder_Routes_CeltOnly_Packets_Through_Celt_Path()
    {
        // Build a CELT-only packet (config 28 = CELT FB 2.5 ms) — through
        // OpusDecoder this exercises the new CELT routing path. Phase 2a
        // still emits silence, so we verify the *shape* — Phase 2b+
        // upgrades the content.
        byte toc = (byte)((28 << 3) | (1 << 2) | 0); // config=28, stereo=1, code=0
        byte[] pkt = new byte[1 + 20];
        pkt[0] = toc;
        var p = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48_000, Channels = 2, BitsPerSample = 16 };
        using var dec = new OpusDecoder(p);
        using var frame = dec.Decode(pkt, pts: 42);

        Assert.Equal(2, frame.Channels);
        Assert.Equal(48_000, frame.SampleRate);
        Assert.Equal(120, frame.SamplesPerChannel); // 2.5 ms @ 48k
        Assert.Equal(42, frame.Pts);
        Assert.Equal(120 * 2, frame.Samples.Length);
        // Phase 2a still silent.
        foreach (var s in frame.Samples.Span) Assert.Equal(0.0f, s);
    }
}
