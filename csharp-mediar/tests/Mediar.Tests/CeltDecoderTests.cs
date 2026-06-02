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
        // OpusDecoder this exercises the new CELT routing path. Phase 2b
        // parses the front-of-packet flag set and the coarse-energy
        // spectrum, but still emits silence for the audio output until
        // Phase 2c/2d ship.
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
        // Phase 2b still emits silence (PCM lands in Phase 2d).
        foreach (var s in frame.Samples.Span) Assert.Equal(0.0f, s);
    }

    [Fact]
    public void DecodeFrame_Populates_State_For_NonSilent_Packet()
    {
        // A payload of 16 all-zero bytes is large enough (128 bits) that
        // all Phase 2b flags + the coarse-energy loop get exercised. The
        // silence flag in particular comes out false because after init
        // the range coder sits at the top of the window — so we get a
        // full pass through post-filter / transient / intra / coarse
        // energy decoding. State must update accordingly.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        int tellBefore = rd.Tell();
        int produced = dec.DecodeFrame(ref rd, output);
        int tellAfter = rd.Tell();

        Assert.Equal(mode.SamplesPerFrame, produced);
        Assert.False(dec.LastFrameWasSilent, "All-zero payload trips the silent=0 branch.");
        Assert.True(tellAfter > tellBefore + 17,
            "Coarse energy decode should consume well past the silence-flag budget.");
        // Output stays zeroed until Phase 2d.
        foreach (var s in output) Assert.Equal(0f, s);
        Assert.False(dec.IsFirstFrame);
    }

    [Fact]
    public void DecodeFrame_Silent_Path_Clamps_Energy_State()
    {
        // A 4-byte (32-bit) payload is large enough to trigger the
        // silence-flag branch but small enough that — once silence
        // resolves true — we skip post-filter / transient / intra /
        // coarse-energy. With our specific init pattern the silence
        // flag *can* resolve either way; regardless of which path was
        // taken, the recorded state must be internally consistent.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 10_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> output = new float[mode.SamplesPerFrame];

        byte[] payload = new byte[] { 0xFF, 0xFF, 0xFF, 0xFF };
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        if (dec.LastFrameWasSilent)
        {
            Assert.False(dec.LastFrameWasTransient);
            Assert.False(dec.LastFrameUsedIntra);
            Assert.False(dec.LastPostFilter.Enabled);
            for (int i = 0; i < dec.OldLogE.Length; i++)
            {
                Assert.Equal(-28.0f * 1024.0f, dec.OldLogE[i]);
            }
        }
    }

    [Fact]
    public void Reset_Clears_Energy_And_Flags()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
        Assert.False(dec.LastFrameWasSilent);
        Assert.False(dec.LastFrameWasTransient);
        Assert.False(dec.LastFrameUsedIntra);
        Assert.False(dec.LastPostFilter.Enabled);
        for (int i = 0; i < dec.OldLogE.Length; i++)
            Assert.Equal(0f, dec.OldLogE[i]);
    }
}
