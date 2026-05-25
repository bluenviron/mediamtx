using Mediar.Codecs.G711;
using Xunit;

namespace Mediar.Tests;

public sealed class G711Tests
{
    [Fact]
    public void MuLaw_Decode_Encode_Decode_Is_Idempotent_In_PCM_Domain()
    {
        // G.711 µ-law has two encodings for zero (0x7F = +0, 0xFF = -0) that
        // both decode to 0, so byte-level round-trip is not the right property.
        // The correct invariant is: re-encoding a decoded sample produces a
        // byte that decodes to the SAME PCM value.
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
        // µ-law silence is 0xFF (which becomes 0x00 after the inversion in the decoder).
        Assert.Equal(0, G711.DecodeMuLaw(0xFF));
    }

    [Fact]
    public void Decoder_Produces_Floats_In_Range()
    {
        var pars = new AudioCodecParameters
        {
            Codec = CodecId.G711MuLaw,
            SampleRate = 8000,
            Channels = 1,
            BitsPerSample = 0,
        };
        using var dec = new G711AudioDecoder(pars);
        byte[] input = new byte[256];
        for (int i = 0; i < 256; i++) input[i] = (byte)i;
        using var frame = dec.Decode(input, 0);
        Assert.Equal(256, frame.SamplesPerChannel);
        Assert.Equal(1, frame.Channels);
        var samples = frame.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.InRange(samples[i], -1f, 1f);
        }
    }
}
