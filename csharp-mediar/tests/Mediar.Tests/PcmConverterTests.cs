using Mediar.Codecs.Pcm;
using Xunit;

namespace Mediar.Tests;

public sealed class PcmConverterTests
{
    [Fact]
    public void S16Le_RoundTrip_Float()
    {
        short[] input = { 0, short.MaxValue, short.MinValue, 1000, -1000 };
        byte[] bytes = new byte[input.Length * 2];
        for (int i = 0; i < input.Length; i++)
        {
            bytes[i * 2] = (byte)(input[i] & 0xFF);
            bytes[i * 2 + 1] = (byte)((input[i] >> 8) & 0xFF);
        }
        Span<float> floats = stackalloc float[input.Length];
        PcmConverter.S16LeToFloat(bytes, floats);
        Assert.InRange(floats[0], -0.001f, 0.001f);
        Assert.InRange(floats[1], 0.999f, 1.001f);
        Assert.InRange(floats[2], -1.001f, -0.999f);

        byte[] back = new byte[bytes.Length];
        PcmConverter.FloatToS16Le(floats, back);
        // Round-trip preserves values to within 1 LSB.
        for (int i = 0; i < input.Length; i++)
        {
            short v = (short)((back[i * 2 + 1] << 8) | back[i * 2]);
            Assert.InRange((int)v - input[i], -1, 1);
        }
    }

    [Fact]
    public void S24Le_Roundtrip_Float()
    {
        // 24-bit signed: 0x000000, 0x7FFFFF, 0x800000 (most negative)
        byte[] bytes = { 0x00, 0x00, 0x00, 0xFF, 0xFF, 0x7F, 0x00, 0x00, 0x80 };
        Span<float> floats = stackalloc float[3];
        PcmConverter.S24LeToFloat(bytes, floats);
        Assert.InRange(floats[0], -0.0001f, 0.0001f);
        Assert.InRange(floats[1], 0.999f, 1.001f);
        Assert.InRange(floats[2], -1.001f, -0.999f);

        byte[] back = new byte[bytes.Length];
        PcmConverter.FloatToS24Le(floats, back);
        // Compare 24-bit signed values
        for (int i = 0; i < 3; i++)
        {
            int orig = bytes[i * 3] | (bytes[i * 3 + 1] << 8) | (bytes[i * 3 + 2] << 16);
            if ((orig & 0x800000) != 0) orig |= unchecked((int)0xFF000000);
            int got = back[i * 3] | (back[i * 3 + 1] << 8) | (back[i * 3 + 2] << 16);
            if ((got & 0x800000) != 0) got |= unchecked((int)0xFF000000);
            Assert.InRange(got - orig, -2, 2);
        }
    }

    [Fact]
    public void F32Le_Passthrough()
    {
        float[] input = { 0.0f, 0.5f, -0.5f, 1.0f, -1.0f };
        byte[] bytes = new byte[input.Length * 4];
        Buffer.BlockCopy(input, 0, bytes, 0, bytes.Length);
        Span<float> floats = stackalloc float[input.Length];
        PcmConverter.F32LeToFloat(bytes, floats);
        for (int i = 0; i < input.Length; i++)
        {
            Assert.Equal(input[i], floats[i]);
        }
    }

    [Fact]
    public void ToFloat_FromFloat_Dispatch_Works()
    {
        short[] input = { 0, 1000, -1000, short.MaxValue, short.MinValue };
        byte[] bytes = new byte[input.Length * 2];
        for (int i = 0; i < input.Length; i++)
        {
            bytes[i * 2] = (byte)(input[i] & 0xFF);
            bytes[i * 2 + 1] = (byte)((input[i] >> 8) & 0xFF);
        }
        Span<float> floats = stackalloc float[input.Length];
        Assert.Equal(input.Length, PcmConverter.ToFloat(CodecId.PcmS16Le, bytes, floats));

        byte[] back = new byte[bytes.Length];
        Assert.Equal(bytes.Length, PcmConverter.FromFloat(CodecId.PcmS16Le, floats, back));
    }
}
