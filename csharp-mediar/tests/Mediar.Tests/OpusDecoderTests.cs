using Mediar.Codecs.Opus.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for the Phase 1 <see cref="OpusDecoder"/> skeleton — verifies the
/// IAudioDecoder shape (codec id, parameters, output rate, channel count,
/// silence frames, PTS pass-through, sample bookkeeping, ExtraData / OpusHead
/// handling, factory behaviour). Real PCM verification arrives with Phase 2.
/// </summary>
public sealed class OpusDecoderTests
{
    private static AudioCodecParameters MakeParams(int channels = 2, ReadOnlyMemory<byte> extraData = default)
        => new()
        {
            Codec = CodecId.Opus,
            SampleRate = 48_000,
            Channels = channels,
            BitsPerSample = 16,
            ExtraData = extraData,
        };

    private static byte TocByte(int config, bool stereo, int code)
        => (byte)((config << 3) | (stereo ? 0x4 : 0) | (code & 0x3));

    private static byte[] BuildOpusHeadOgg(byte channels, byte family = 0)
    {
        var head = new OpusHead
        {
            ChannelCount = channels,
            PreSkip = 312,
            InputSampleRate = 48_000,
            OutputGain = 0,
            ChannelMappingFamily = family,
        };
        byte[] buf = new byte[head.OggByteCount];
        OpusHead.WriteOgg(in head, buf);
        return buf;
    }

    // ---------- constructor ----------

    [Fact]
    public void Constructor_Rejects_Null_Parameters()
    {
        Assert.Throws<ArgumentNullException>(() => new OpusDecoder(null!));
    }

    [Fact]
    public void Constructor_Rejects_NonOpus_Codec()
    {
        var p = new AudioCodecParameters { Codec = CodecId.Vorbis, SampleRate = 48_000, Channels = 2 };
        Assert.Throws<ArgumentException>(() => new OpusDecoder(p));
    }

    [Fact]
    public void Constructor_Accepts_Empty_ExtraData()
    {
        using var dec = new OpusDecoder(MakeParams());
        Assert.Equal(CodecId.Opus, dec.Codec);
        Assert.Equal(48_000, dec.OutputSampleRate);
        Assert.False(dec.HasHead);
    }

    [Fact]
    public void Constructor_Accepts_Valid_OpusHead_ExtraData()
    {
        byte[] head = BuildOpusHeadOgg(channels: 2);
        using var dec = new OpusDecoder(MakeParams(extraData: head));
        Assert.True(dec.HasHead);
        Assert.Equal(2, dec.Head.ChannelCount);
        Assert.Equal(312, dec.Head.PreSkip);
    }

    [Fact]
    public void Constructor_Rejects_Malformed_OpusHead_ExtraData()
    {
        // Wrong magic.
        byte[] bad = new byte[19];
        bad[0] = (byte)'G';
        Assert.Throws<ArgumentException>(() => new OpusDecoder(MakeParams(extraData: bad)));
    }

    // ---------- Decode ----------

    [Fact]
    public void Decode_Empty_Packet_Returns_Default()
    {
        using var dec = new OpusDecoder(MakeParams());
        var frame = dec.Decode(ReadOnlySpan<byte>.Empty, pts: 0);
        Assert.Equal(0, frame.SamplesPerChannel);
        Assert.Equal(0, frame.Channels);
        Assert.Null(frame.Owner);
    }

    [Fact]
    public void Decode_Code0_Stereo_20ms_Returns_Correctly_Shaped_Frame()
    {
        // config 20 = CELT-only Wideband 2.5ms — let's pick config 19 instead
        // = CELT-only Narrowband 20ms. Stereo bit = 1, code = 0.
        byte[] pkt = new byte[1 + 50];
        pkt[0] = TocByte(19, stereo: true, code: 0);
        for (int i = 1; i < pkt.Length; i++) pkt[i] = (byte)(i * 7);

        using var dec = new OpusDecoder(MakeParams(channels: 2));
        using var frame = dec.Decode(pkt, pts: 100);

        Assert.Equal(2, frame.Channels);
        Assert.Equal(48_000, frame.SampleRate);
        Assert.Equal(960, frame.SamplesPerChannel); // 20ms @ 48k
        Assert.Equal(100, frame.Pts);
        Assert.Equal(960 * 2, frame.Samples.Length);
        Assert.True(IsSilent(frame.Samples), "Phase 1 emits silence.");
    }

    [Fact]
    public void Decode_Code1_TwoFrames_Yields_Double_SamplesPerChannel()
    {
        // config 16 = CELT-only Narrowband 2.5ms (120 samples), code 1 (2 equal frames).
        // Mono. Total payload must be even.
        byte[] pkt = new byte[1 + 40];
        pkt[0] = TocByte(16, stereo: false, code: 1);

        using var dec = new OpusDecoder(MakeParams(channels: 1));
        using var frame = dec.Decode(pkt, pts: 0);

        Assert.Equal(1, frame.Channels);
        Assert.Equal(120 * 2, frame.SamplesPerChannel);
        Assert.Equal(120 * 2, frame.Samples.Length);
    }

    [Fact]
    public void Decode_Code3_Three_Equal_Frames_Yields_Triple_SamplesPerChannel()
    {
        // config 0 = SILK NB 10ms (480 samples). Code 3 CBR, m=3.
        int m = 3, per = 20;
        byte[] pkt = new byte[1 + 1 + m * per];
        pkt[0] = TocByte(0, stereo: false, code: 3);
        pkt[1] = (byte)m; // vbr=0, padding=0
        using var dec = new OpusDecoder(MakeParams(channels: 1));
        using var frame = dec.Decode(pkt, pts: 1_000);

        Assert.Equal(480 * 3, frame.SamplesPerChannel);
        Assert.Equal(1_000, frame.Pts);
        Assert.Equal(480 * 3, dec.SamplesProduced);
    }

    [Fact]
    public void Decode_Pts_Passes_Through_Unchanged()
    {
        byte[] pkt = new byte[1 + 30];
        pkt[0] = TocByte(0, stereo: false, code: 0);
        using var dec = new OpusDecoder(MakeParams(channels: 1));
        long[] timestamps = { 0, 480, 960, 1440 };
        foreach (long pts in timestamps)
        {
            using var frame = dec.Decode(pkt, pts);
            Assert.Equal(pts, frame.Pts);
        }
    }

    [Fact]
    public void Decode_SamplesProduced_Accumulates()
    {
        byte[] pkt = new byte[1 + 20];
        pkt[0] = TocByte(0, stereo: false, code: 0);
        using var dec = new OpusDecoder(MakeParams(channels: 1));
        long expected = 0;
        for (int i = 0; i < 5; i++)
        {
            using var frame = dec.Decode(pkt, i * 480);
            expected += frame.SamplesPerChannel;
        }
        Assert.Equal(expected, dec.SamplesProduced);
        Assert.Equal(480 * 5, dec.SamplesProduced);
    }

    [Fact]
    public void Decode_Channels_Follow_Toc_Stereo_Bit_For_Family0()
    {
        using var dec = new OpusDecoder(MakeParams(channels: 2));
        // Mono TOC even though Params.Channels = 2.
        byte[] monoPkt = new byte[1 + 20];
        monoPkt[0] = TocByte(0, stereo: false, code: 0);
        using var monoFrame = dec.Decode(monoPkt, 0);
        Assert.Equal(1, monoFrame.Channels);

        // Stereo TOC.
        byte[] stereoPkt = new byte[1 + 20];
        stereoPkt[0] = TocByte(0, stereo: true, code: 0);
        using var stereoFrame = dec.Decode(stereoPkt, 0);
        Assert.Equal(2, stereoFrame.Channels);
    }

    [Fact]
    public void Reset_Clears_SamplesProduced()
    {
        byte[] pkt = new byte[1 + 20];
        pkt[0] = TocByte(0, stereo: false, code: 0);
        using var dec = new OpusDecoder(MakeParams(channels: 1));
        using (var _ = dec.Decode(pkt, 0)) { }
        Assert.True(dec.SamplesProduced > 0);
        dec.Reset();
        Assert.Equal(0, dec.SamplesProduced);
    }

    // ---------- factory ----------

    [Fact]
    public void Factory_Supports_Only_Opus()
    {
        var f = new OpusDecoderFactory();
        Assert.True(f.Supports(CodecId.Opus));
        Assert.False(f.Supports(CodecId.Vorbis));
        Assert.False(f.Supports(CodecId.Mp3));
        Assert.False(f.Supports(CodecId.Alac));
        Assert.False(f.Supports(CodecId.Aac));
    }

    [Fact]
    public void Factory_Creates_Decoder()
    {
        var f = new OpusDecoderFactory();
        using var dec = f.Create(MakeParams());
        Assert.IsType<OpusDecoder>(dec);
        Assert.Equal(CodecId.Opus, dec.Codec);
    }

    [Fact]
    public void Factory_Rejects_Null_Parameters()
    {
        var f = new OpusDecoderFactory();
        Assert.Throws<ArgumentNullException>(() => f.Create(null!));
    }

    [Fact]
    public void Factory_Propagates_NonOpus_Codec_Rejection()
    {
        var f = new OpusDecoderFactory();
        var p = new AudioCodecParameters { Codec = CodecId.Vorbis, SampleRate = 48_000, Channels = 2 };
        Assert.Throws<ArgumentException>(() => f.Create(p));
    }

    // ---------- helpers ----------

    private static bool IsSilent(ReadOnlyMemory<float> samples)
    {
        var span = samples.Span;
        for (int i = 0; i < span.Length; i++)
        {
            if (span[i] != 0.0f) return false;
        }
        return true;
    }
}
