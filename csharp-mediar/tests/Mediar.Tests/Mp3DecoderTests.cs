using Mediar.Codecs.Mp3.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp3DecoderTests
{
    [Fact]
    public void Ctor_NullParameters_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new Mp3Decoder(null!));
    }

    [Fact]
    public void Ctor_WrongCodec_Throws()
    {
        var pars = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = 44100,
            Channels = 2,
        };
        Assert.Throws<ArgumentException>(() => new Mp3Decoder(pars));
    }

    [Fact]
    public void Empty_Input_Returns_Default()
    {
        var pars = NewParams();
        using var dec = new Mp3Decoder(pars);
        var frame = dec.Decode(ReadOnlySpan<byte>.Empty, pts: 0);
        Assert.Equal(0, frame.Channels);
        Assert.Null(frame.Owner);
    }

    [Fact]
    public void Truncated_Input_Throws()
    {
        var pars = NewParams();
        using var dec = new Mp3Decoder(pars);
        byte[] tooShort = { 0xFF, 0xFB };
        Assert.Throws<InvalidDataException>(() => dec.Decode(tooShort, 0));
    }

    [Fact]
    public void Bad_Sync_Throws()
    {
        var pars = NewParams();
        using var dec = new Mp3Decoder(pars);
        byte[] notMp3 = { 0x00, 0x00, 0x00, 0x00 };
        Assert.Throws<InvalidDataException>(() => dec.Decode(notMp3, 0));
    }

    [Fact]
    public void Layer_1_Throws_NotSupported()
    {
        var pars = NewParams();
        using var dec = new Mp3Decoder(pars);
        // Build minimal MPEG-1 Layer I header (bits: ver=11=MPEG-1, layer=11=Layer I)
        // byte 1 = 1111 1111 = 0xFF (sync continues, ver=11, layer=11, no_protection=1)
        // byte 2 = 32 kbps M1L1 → bitrateIx=1, sampleRate=44.1→idx=0, no padding
        //   bits: 0001 0000 = 0x10
        // byte 3 = mono → 11 in top bits → 0xC0
        byte[] header = { 0xFF, 0xFF, 0x10, 0xC0 };
        // Frame size for L1: (12 * 32000 / 44100 + 0) * 4 = 32 bytes
        // We provide just the header — should fail with NotSupportedException
        // before reaching truncation check IF the frame_size > 4. Test that
        // either way an exception comes back; not specifically NotSupportedException
        // since a too-short frame triggers InvalidDataException first.
        byte[] frame = new byte[32];
        Array.Copy(header, frame, 4);
        Assert.Throws<NotSupportedException>(() => dec.Decode(frame, 0));
    }

    [Fact]
    public void Silence_Frame_Mono_Decodes_To_Silence()
    {
        var pars = NewParams(channels: 1);
        using var dec = new Mp3Decoder(pars);
        byte[] frame = BuildSilentMpeg1Frame(channels: 1);

        using var decoded = dec.Decode(frame, pts: 0);
        Assert.Equal(1, decoded.Channels);
        Assert.Equal(44100, decoded.SampleRate);
        Assert.Equal(1152, decoded.SamplesPerChannel);
        var samples = decoded.Samples.Span;
        Assert.Equal(1152, samples.Length);
        for (int i = 0; i < samples.Length; i++)
            Assert.Equal(0f, samples[i]);
    }

    [Fact]
    public void Silence_Frame_Stereo_Decodes_To_Silence()
    {
        var pars = NewParams(channels: 2);
        using var dec = new Mp3Decoder(pars);
        byte[] frame = BuildSilentMpeg1Frame(channels: 2);

        using var decoded = dec.Decode(frame, pts: 0);
        Assert.Equal(2, decoded.Channels);
        Assert.Equal(44100, decoded.SampleRate);
        Assert.Equal(1152, decoded.SamplesPerChannel);
        var samples = decoded.Samples.Span;
        Assert.Equal(2304, samples.Length);
        for (int i = 0; i < samples.Length; i++)
            Assert.Equal(0f, samples[i]);
    }

    [Fact]
    public void Reset_Clears_Reservoir()
    {
        var pars = NewParams(channels: 1);
        using var dec = new Mp3Decoder(pars);
        byte[] frame = BuildSilentMpeg1Frame(channels: 1);

        using (var _ = dec.Decode(frame, 0)) { }
        dec.Reset();
        using var decoded = dec.Decode(frame, 1);
        Assert.Equal(1152, decoded.SamplesPerChannel);
    }

    [Fact]
    public void Dispose_Idempotent()
    {
        var pars = NewParams();
        var dec = new Mp3Decoder(pars);
        dec.Dispose();
        dec.Dispose(); // must not throw
    }

    [Fact]
    public void Factory_Supports_Only_Mp3()
    {
        var f = new Mp3DecoderFactory();
        Assert.True(f.Supports(CodecId.Mp3));
        Assert.False(f.Supports(CodecId.Flac));
        Assert.False(f.Supports(CodecId.Aac));
    }

    [Fact]
    public void Factory_Create_Returns_Mp3Decoder()
    {
        var pars = NewParams();
        var f = new Mp3DecoderFactory();
        using var dec = f.Create(pars);
        Assert.IsType<Mp3Decoder>(dec);
        Assert.Equal(CodecId.Mp3, dec.Codec);
    }

    private static AudioCodecParameters NewParams(int channels = 2) => new()
    {
        Codec = CodecId.Mp3,
        SampleRate = 44100,
        Channels = channels,
    };

    /// <summary>
    /// Build a syntactically-valid MPEG-1 Layer III 128 kbps 44.1 kHz frame
    /// whose side info specifies "no data" (all granules empty). Decoded
    /// output should be all-zero PCM regardless of any approximations in the
    /// downstream synthesis filterbank.
    /// </summary>
    private static byte[] BuildSilentMpeg1Frame(int channels)
    {
        // MPEG-1 Layer III: frameSize = 144 * bitrate / sampleRate + padding
        // = 144 * 128000 / 44100 = 417 bytes (integer div).
        int frameSize = 144 * 128000 / 44100;
        var frame = new byte[frameSize];

        // Header: 0xFF, 0xFB, 0x90, 0x00 (stereo) or 0xC0 (mono).
        frame[0] = 0xFF;
        frame[1] = 0xFB; // MPEG-1, Layer III, no_protection=1
        frame[2] = 0x90; // 128kbps (idx 9), 44.1kHz (idx 0), no padding
        frame[3] = (byte)(channels == 1 ? 0xC0 : 0x00); // mono or stereo

        // Side info + main data: all zeros = empty granules with no Huffman data.
        // (Already zero-initialized.)
        return frame;
    }
}
