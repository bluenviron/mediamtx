using Mediar.Codecs.G711;
using Xunit;

namespace Mediar.Tests;

public sealed class G711Tests
{
    // -------------------- µ-law / A-law scalar API --------------------

    [Fact]
    public void MuLaw_Decode_Encode_Decode_Is_Idempotent_In_PCM_Domain()
    {
        for (int i = 0; i < 256; i++)
        {
            byte b = (byte)i;
            short pcm = G711.DecodeMuLaw(b);
            byte reencoded = G711.EncodeMuLaw(pcm);
            short pcm2 = G711.DecodeMuLaw(reencoded);
            Assert.Equal(pcm, pcm2);
        }
    }

    [Fact]
    public void ALaw_Decode_Encode_Decode_Is_Idempotent_In_PCM_Domain()
    {
        for (int i = 0; i < 256; i++)
        {
            byte b = (byte)i;
            short pcm = G711.DecodeALaw(b);
            byte reencoded = G711.EncodeALaw(pcm);
            short pcm2 = G711.DecodeALaw(reencoded);
            Assert.Equal(pcm, pcm2);
        }
    }

    [Fact]
    public void MuLaw_Zero_Decodes_To_Zero()
    {
        Assert.Equal(0, G711.DecodeMuLaw(0xFF));
    }

    [Fact]
    public void MuLaw_NegativeZero_Decodes_To_Zero()
    {
        Assert.Equal(0, G711.DecodeMuLaw(0x7F));
    }

    [Fact]
    public void ALaw_Silence_Bytes_Decode_To_Small_Magnitudes()
    {
        // A-law silence is 0xD5 (positive) and 0x55 (negative).
        Assert.InRange(G711.DecodeALaw(0xD5), -16, 16);
        Assert.InRange(G711.DecodeALaw(0x55), -16, 16);
    }

    [Fact]
    public void MuLaw_Encode_Clips_Above_Maximum()
    {
        byte clipped = G711.EncodeMuLaw(32767);
        byte atClip = G711.EncodeMuLaw(32635);
        Assert.Equal(atClip, clipped);
    }

    [Fact]
    public void ALaw_Encode_Clips_Above_Maximum()
    {
        byte clipped = G711.EncodeALaw(32767);
        byte atClip = G711.EncodeALaw(32635);
        Assert.Equal(atClip, clipped);
    }

    [Fact]
    public void MuLaw_Encode_Distinguishes_Sign()
    {
        byte pos = G711.EncodeMuLaw(8000);
        byte neg = G711.EncodeMuLaw(-8000);
        // µ-law inverts all bits at the end, so positive samples carry bit 7 set.
        Assert.NotEqual(pos, neg);
        Assert.Equal(0x80, pos & 0x80);
        Assert.Equal(0, neg & 0x80);
    }

    [Fact]
    public void ALaw_Encode_Distinguishes_Sign()
    {
        byte pos = G711.EncodeALaw(8000);
        byte neg = G711.EncodeALaw(-8000);
        Assert.NotEqual(pos, neg);
    }

    // -------------------- µ-law / A-law buffer helpers --------------------

    [Fact]
    public void MuLaw_Buffer_Decode_Produces_Normalized_Floats()
    {
        byte[] src = new byte[64];
        for (int i = 0; i < 64; i++) src[i] = (byte)i;
        float[] dst = new float[64];
        G711.DecodeMuLaw(src, dst);
        for (int i = 0; i < 64; i++)
        {
            Assert.InRange(dst[i], -1f, 1f);
            // Matches scalar API * 1/32768.
            Assert.Equal(G711.DecodeMuLaw(src[i]) / 32768f, dst[i], 6);
        }
    }

    [Fact]
    public void ALaw_Buffer_Decode_Produces_Normalized_Floats()
    {
        byte[] src = new byte[64];
        for (int i = 0; i < 64; i++) src[i] = (byte)i;
        float[] dst = new float[64];
        G711.DecodeALaw(src, dst);
        for (int i = 0; i < 64; i++)
        {
            Assert.InRange(dst[i], -1f, 1f);
            Assert.Equal(G711.DecodeALaw(src[i]) / 32768f, dst[i], 6);
        }
    }

    [Fact]
    public void MuLaw_Decode_Buffer_Too_Small_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            G711.DecodeMuLaw(new byte[8], new float[4]));
    }

    [Fact]
    public void ALaw_Decode_Buffer_Too_Small_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            G711.DecodeALaw(new byte[8], new float[4]));
    }

    [Fact]
    public void MuLaw_Encode_Buffer_Clamps_Floats_Out_Of_Range()
    {
        float[] src = new float[] { 2f, 1f, 0f, -1f, -2f };
        byte[] dst = new byte[5];
        G711.EncodeMuLaw(src, dst);
        // Encoding +2 must match encoding +1 after clamp; same on negative side.
        Assert.Equal(dst[1], dst[0]);
        Assert.Equal(dst[3], dst[4]);
    }

    [Fact]
    public void ALaw_Encode_Buffer_Clamps_Floats_Out_Of_Range()
    {
        float[] src = new float[] { 2f, 1f, 0f, -1f, -2f };
        byte[] dst = new byte[5];
        G711.EncodeALaw(src, dst);
        Assert.Equal(dst[1], dst[0]);
        Assert.Equal(dst[3], dst[4]);
    }

    [Fact]
    public void MuLaw_Encode_Buffer_Too_Small_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            G711.EncodeMuLaw(new float[8], new byte[4]));
    }

    [Fact]
    public void ALaw_Encode_Buffer_Too_Small_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            G711.EncodeALaw(new float[8], new byte[4]));
    }

    [Fact]
    public void MuLaw_Float_RoundTrip_Reaches_Same_PCM_Symbol()
    {
        byte[] src = new byte[256];
        for (int i = 0; i < 256; i++) src[i] = (byte)i;

        float[] floats = new float[256];
        byte[] reencoded = new byte[256];
        G711.DecodeMuLaw(src, floats);
        G711.EncodeMuLaw(floats, reencoded);

        // The byte-level round trip must agree on the decoded PCM value.
        for (int i = 0; i < 256; i++)
        {
            Assert.Equal(G711.DecodeMuLaw(src[i]), G711.DecodeMuLaw(reencoded[i]));
        }
    }

    [Fact]
    public void ALaw_Float_RoundTrip_Reaches_Same_PCM_Symbol()
    {
        byte[] src = new byte[256];
        for (int i = 0; i < 256; i++) src[i] = (byte)i;

        float[] floats = new float[256];
        byte[] reencoded = new byte[256];
        G711.DecodeALaw(src, floats);
        G711.EncodeALaw(floats, reencoded);

        for (int i = 0; i < 256; i++)
        {
            Assert.Equal(G711.DecodeALaw(src[i]), G711.DecodeALaw(reencoded[i]));
        }
    }

    // -------------------- G711AudioDecoder --------------------

    [Fact]
    public void Decoder_Constructor_Null_Parameters_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new G711AudioDecoder(null!));
    }

    [Fact]
    public void Decoder_Constructor_Wrong_Codec_Throws()
    {
        var pars = new AudioCodecParameters
        {
            Codec = CodecId.Aac,
            SampleRate = 8000,
            Channels = 1,
        };
        Assert.Throws<ArgumentException>(() => new G711AudioDecoder(pars));
    }

    [Theory]
    [InlineData(CodecId.G711MuLaw)]
    [InlineData(CodecId.G711ALaw)]
    public void Decoder_Constructor_Accepts_Both_G711_Variants(CodecId id)
    {
        var pars = new AudioCodecParameters { Codec = id, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        Assert.Equal(id, dec.Codec);
        Assert.Same(pars, dec.Parameters);
    }

    [Fact]
    public void Decoder_Empty_Input_Returns_Default_Frame()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(ReadOnlySpan<byte>.Empty, 0);
        Assert.Equal(0, frame.SamplesPerChannel);
    }

    [Fact]
    public void Decoder_Too_Few_Bytes_For_Channels_Returns_Default()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 2 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(new byte[] { 0xFF }, 0);
        Assert.Equal(0, frame.SamplesPerChannel);
    }

    [Fact]
    public void Decoder_Stereo_Divides_Sample_Count_Correctly()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 2 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(new byte[10], 0);
        Assert.Equal(2, frame.Channels);
        Assert.Equal(5, frame.SamplesPerChannel);
    }

    [Fact]
    public void Decoder_Defaults_Channels_To_One_When_Unspecified()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 0, Channels = 0 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(new byte[16], 0);
        Assert.Equal(1, frame.Channels);
        Assert.Equal(16, frame.SamplesPerChannel);
    }

    [Fact]
    public void Decoder_Defaults_SampleRate_To_8000_When_Unspecified()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 0, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(new byte[4], 0);
        Assert.Equal(8000, frame.SampleRate);
    }

    [Fact]
    public void Decoder_Preserves_Pts()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        using var frame = dec.Decode(new byte[4], 12345L);
        Assert.Equal(12345L, frame.Pts);
    }

    [Fact]
    public void Decoder_ALaw_Produces_Floats_In_Range()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711ALaw, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        byte[] input = new byte[256];
        for (int i = 0; i < 256; i++) input[i] = (byte)i;
        using var frame = dec.Decode(input, 0);
        Assert.Equal(256, frame.SamplesPerChannel);
        var samples = frame.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.InRange(samples[i], -1f, 1f);
        }
    }

    [Fact]
    public void Decoder_MuLaw_Produces_Floats_In_Range()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        byte[] input = new byte[256];
        for (int i = 0; i < 256; i++) input[i] = (byte)i;
        using var frame = dec.Decode(input, 0);
        Assert.Equal(256, frame.SamplesPerChannel);
        var samples = frame.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.InRange(samples[i], -1f, 1f);
        }
    }

    [Fact]
    public void Decoder_Reset_Does_Not_Throw()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        using var dec = new G711AudioDecoder(pars);
        dec.Reset();
        dec.Reset(); // idempotent / stateless
    }

    [Fact]
    public void Decoder_Dispose_Is_Idempotent()
    {
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        var dec = new G711AudioDecoder(pars);
        dec.Dispose();
        dec.Dispose();
    }

    // -------------------- Factory --------------------

    [Theory]
    [InlineData(CodecId.G711MuLaw, true)]
    [InlineData(CodecId.G711ALaw, true)]
    [InlineData(CodecId.Aac, false)]
    [InlineData(CodecId.Mp3, false)]
    [InlineData(CodecId.Flac, false)]
    [InlineData(CodecId.Unknown, false)]
    public void Factory_Supports_Reflects_Codec_Id(CodecId id, bool expected)
    {
        var f = new G711AudioDecoderFactory();
        Assert.Equal(expected, f.Supports(id));
    }

    [Fact]
    public void Factory_Create_Returns_G711_Decoder()
    {
        var f = new G711AudioDecoderFactory();
        var pars = new AudioCodecParameters { Codec = CodecId.G711MuLaw, SampleRate = 8000, Channels = 1 };
        using var dec = f.Create(pars);
        Assert.IsType<G711AudioDecoder>(dec);
        Assert.Equal(CodecId.G711MuLaw, dec.Codec);
    }

    [Fact]
    public void Factory_Create_With_Unsupported_Codec_Throws_From_Constructor()
    {
        var f = new G711AudioDecoderFactory();
        var pars = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 8000, Channels = 1 };
        Assert.Throws<ArgumentException>(() => f.Create(pars));
    }
}
