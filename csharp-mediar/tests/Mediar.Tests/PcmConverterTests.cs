using Mediar.Codecs.Pcm;
using Xunit;

namespace Mediar.Tests;

public sealed class PcmConverterTests
{
    [Fact]
    public void S16Le_RoundTrip_Float()
    {
        short[] input = { 0, short.MaxValue, short.MinValue, 1000, -1000 };
        byte[] bytes = PackS16Le(input);
        Span<float> floats = stackalloc float[input.Length];
        PcmConverter.S16LeToFloat(bytes, floats);
        Assert.InRange(floats[0], -0.001f, 0.001f);
        Assert.InRange(floats[1], 0.999f, 1.001f);
        Assert.InRange(floats[2], -1.001f, -0.999f);

        byte[] back = new byte[bytes.Length];
        PcmConverter.FloatToS16Le(floats, back);
        for (int i = 0; i < input.Length; i++)
        {
            short v = (short)((back[i * 2 + 1] << 8) | back[i * 2]);
            Assert.InRange((int)v - input[i], -1, 1);
        }
    }

    [Fact]
    public void S16Be_To_Float_Reads_Big_Endian()
    {
        // Big-endian 0x7FFF, 0x8000, 0x0000.
        byte[] bytes = { 0x7F, 0xFF, 0x80, 0x00, 0x00, 0x00 };
        Span<float> floats = stackalloc float[3];
        PcmConverter.S16BeToFloat(bytes, floats);
        Assert.InRange(floats[0], 0.999f, 1.001f);
        Assert.InRange(floats[1], -1.001f, -0.999f);
        Assert.InRange(floats[2], -0.001f, 0.001f);
    }

    [Fact]
    public void S24Le_Roundtrip_Float()
    {
        byte[] bytes = { 0x00, 0x00, 0x00, 0xFF, 0xFF, 0x7F, 0x00, 0x00, 0x80 };
        Span<float> floats = stackalloc float[3];
        PcmConverter.S24LeToFloat(bytes, floats);
        Assert.InRange(floats[0], -0.0001f, 0.0001f);
        Assert.InRange(floats[1], 0.999f, 1.001f);
        Assert.InRange(floats[2], -1.001f, -0.999f);

        byte[] back = new byte[bytes.Length];
        PcmConverter.FloatToS24Le(floats, back);
        for (int i = 0; i < 3; i++)
        {
            int orig = SignExtend24(bytes, i);
            int got  = SignExtend24(back, i);
            Assert.InRange(got - orig, -2, 2);
        }
    }

    [Fact]
    public void S32Le_Roundtrip_Float()
    {
        int[] input = { 0, int.MaxValue, int.MinValue, 1 << 30, -(1 << 30) };
        byte[] bytes = PackS32Le(input);
        Span<float> floats = stackalloc float[input.Length];
        PcmConverter.S32LeToFloat(bytes, floats);
        Assert.InRange(floats[0], -1e-6f, 1e-6f);
        Assert.InRange(floats[1], 0.999f, 1.001f);
        Assert.InRange(floats[2], -1.001f, -0.999f);

        byte[] back = new byte[bytes.Length];
        PcmConverter.FloatToS32Le(floats, back);
        int[] decoded = UnpackS32Le(back);
        Assert.InRange(decoded[1] - input[1], -1024, 1024); // tiny rounding ok
    }

    [Fact]
    public void F32Le_Passthrough()
    {
        float[] input = { 0.0f, 0.5f, -0.5f, 1.0f, -1.0f };
        byte[] bytes = new byte[input.Length * 4];
        Buffer.BlockCopy(input, 0, bytes, 0, bytes.Length);
        Span<float> floats = stackalloc float[input.Length];
        PcmConverter.F32LeToFloat(bytes, floats);
        for (int i = 0; i < input.Length; i++) Assert.Equal(input[i], floats[i]);
    }

    [Fact]
    public void FromFloat_PcmF32Le_Roundtrip()
    {
        float[] input = { 0f, 0.25f, -0.75f };
        byte[] bytes = new byte[input.Length * 4];
        int written = PcmConverter.FromFloat(CodecId.PcmF32Le, input, bytes);
        Assert.Equal(input.Length * 4, written);
        float[] back = new float[input.Length];
        Buffer.BlockCopy(bytes, 0, back, 0, bytes.Length);
        Assert.Equal(input, back);
    }

    [Fact]
    public void FloatToS16Le_Clamps_Out_Of_Range()
    {
        float[] input = { 2.5f, -2.5f };
        byte[] bytes = new byte[input.Length * 2];
        PcmConverter.FloatToS16Le(input, bytes);
        short v0 = (short)((bytes[1] << 8) | bytes[0]);
        short v1 = (short)((bytes[3] << 8) | bytes[2]);
        Assert.Equal(32767, v0);
        Assert.Equal(-32767, v1);
    }

    [Fact]
    public void FloatToS24Le_Clamps_Out_Of_Range()
    {
        float[] input = { 2.5f, -2.5f };
        byte[] bytes = new byte[input.Length * 3];
        PcmConverter.FloatToS24Le(input, bytes);
        int v0 = SignExtend24(bytes, 0);
        int v1 = SignExtend24(bytes, 1);
        Assert.Equal(8388607, v0);
        Assert.Equal(-8388607, v1);
    }

    [Fact]
    public void FloatToS32Le_Clamps_Out_Of_Range()
    {
        float[] input = { 2.5f, -2.5f };
        byte[] bytes = new byte[input.Length * 4];
        PcmConverter.FloatToS32Le(input, bytes);
        int v0 = BitConverter.ToInt32(bytes, 0);
        int v1 = BitConverter.ToInt32(bytes, 4);
        Assert.Equal(int.MaxValue, v0);
        Assert.Equal(-int.MaxValue, v1);
    }

    [Fact]
    public void ToFloat_FromFloat_Dispatch_Works()
    {
        short[] input = { 0, 1000, -1000, short.MaxValue, short.MinValue };
        byte[] bytes = PackS16Le(input);
        Span<float> floats = stackalloc float[input.Length];
        Assert.Equal(input.Length, PcmConverter.ToFloat(CodecId.PcmS16Le, bytes, floats));

        byte[] back = new byte[bytes.Length];
        Assert.Equal(bytes.Length, PcmConverter.FromFloat(CodecId.PcmS16Le, floats, back));
    }

    [Theory]
    [InlineData(CodecId.PcmS16Le, 2)]
    [InlineData(CodecId.PcmS16Be, 2)]
    [InlineData(CodecId.PcmS24Le, 3)]
    [InlineData(CodecId.PcmS32Le, 4)]
    [InlineData(CodecId.PcmF32Le, 4)]
    public void BytesPerSample_Known_Codecs(CodecId codec, int expected)
    {
        Assert.Equal(expected, PcmConverter.BytesPerSample(codec));
    }

    [Fact]
    public void BytesPerSample_Unknown_Throws()
    {
        Assert.Throws<ArgumentException>(() => PcmConverter.BytesPerSample(CodecId.Aac));
    }

    [Fact]
    public void ToFloat_Unsupported_Codec_Throws()
    {
        byte[] src = new byte[8];
        Assert.Throws<ArgumentException>(() => PcmConverter.ToFloat(CodecId.Aac, src, new float[4]));
    }

    [Fact]
    public void FromFloat_Unsupported_Codec_Throws()
    {
        float[] src = new float[4];
        Assert.Throws<ArgumentException>(() => PcmConverter.FromFloat(CodecId.Aac, src, new byte[8]));
    }

    [Fact]
    public void FromFloat_S16Be_Unsupported_Throws()
    {
        // S16Be is read-only — the FromFloat side has no entry for it.
        float[] src = { 0.5f };
        Assert.Throws<ArgumentException>(() => PcmConverter.FromFloat(CodecId.PcmS16Be, src, new byte[2]));
    }

    [Fact]
    public void S16LeToFloat_Destination_Too_Small_Throws()
    {
        byte[] src = new byte[8];
        Assert.Throws<ArgumentException>(() =>
        {
            float[] dst = new float[1];
            PcmConverter.S16LeToFloat(src, dst);
        });
    }

    [Fact]
    public void S16BeToFloat_Destination_Too_Small_Throws()
    {
        byte[] src = new byte[8];
        Assert.Throws<ArgumentException>(() => PcmConverter.S16BeToFloat(src, new float[1]));
    }

    [Fact]
    public void S24LeToFloat_Destination_Too_Small_Throws()
    {
        byte[] src = new byte[9];
        Assert.Throws<ArgumentException>(() => PcmConverter.S24LeToFloat(src, new float[1]));
    }

    [Fact]
    public void S32LeToFloat_Destination_Too_Small_Throws()
    {
        byte[] src = new byte[8];
        Assert.Throws<ArgumentException>(() => PcmConverter.S32LeToFloat(src, new float[1]));
    }

    [Fact]
    public void F32LeToFloat_Destination_Too_Small_Throws()
    {
        byte[] src = new byte[8];
        Assert.Throws<ArgumentException>(() => PcmConverter.F32LeToFloat(src, new float[1]));
    }

    [Fact]
    public void FloatToS16Le_Destination_Too_Small_Throws()
    {
        float[] src = new float[4];
        Assert.Throws<ArgumentException>(() => PcmConverter.FloatToS16Le(src, new byte[2]));
    }

    [Fact]
    public void FloatToS24Le_Destination_Too_Small_Throws()
    {
        float[] src = new float[4];
        Assert.Throws<ArgumentException>(() => PcmConverter.FloatToS24Le(src, new byte[2]));
    }

    [Fact]
    public void FloatToS32Le_Destination_Too_Small_Throws()
    {
        float[] src = new float[4];
        Assert.Throws<ArgumentException>(() => PcmConverter.FloatToS32Le(src, new byte[2]));
    }

    [Fact]
    public void Empty_S16Le_Roundtrip_Is_NoOp()
    {
        PcmConverter.S16LeToFloat(ReadOnlySpan<byte>.Empty, Span<float>.Empty);
        PcmConverter.FloatToS16Le(ReadOnlySpan<float>.Empty, Span<byte>.Empty);
    }

    [Fact]
    public void ToFloat_Empty_Returns_Zero()
    {
        Assert.Equal(0, PcmConverter.ToFloat(CodecId.PcmS16Le, ReadOnlySpan<byte>.Empty, Span<float>.Empty));
        Assert.Equal(0, PcmConverter.ToFloat(CodecId.PcmS24Le, ReadOnlySpan<byte>.Empty, Span<float>.Empty));
        Assert.Equal(0, PcmConverter.ToFloat(CodecId.PcmS32Le, ReadOnlySpan<byte>.Empty, Span<float>.Empty));
        Assert.Equal(0, PcmConverter.ToFloat(CodecId.PcmF32Le, ReadOnlySpan<byte>.Empty, Span<float>.Empty));
        Assert.Equal(0, PcmConverter.ToFloat(CodecId.PcmS16Be, ReadOnlySpan<byte>.Empty, Span<float>.Empty));
    }

    [Fact]
    public void FromFloat_Empty_Returns_Zero()
    {
        Assert.Equal(0, PcmConverter.FromFloat(CodecId.PcmS16Le, ReadOnlySpan<float>.Empty, Span<byte>.Empty));
        Assert.Equal(0, PcmConverter.FromFloat(CodecId.PcmS24Le, ReadOnlySpan<float>.Empty, Span<byte>.Empty));
        Assert.Equal(0, PcmConverter.FromFloat(CodecId.PcmS32Le, ReadOnlySpan<float>.Empty, Span<byte>.Empty));
        Assert.Equal(0, PcmConverter.FromFloat(CodecId.PcmF32Le, ReadOnlySpan<float>.Empty, Span<byte>.Empty));
    }

    [Fact]
    public void ToFloat_Returns_Sample_Count_For_Each_Codec()
    {
        // Each codec returns source.Length / bytesPerSample for that codec.
        Assert.Equal(4, PcmConverter.ToFloat(CodecId.PcmS16Le, new byte[8], new float[8]));
        Assert.Equal(4, PcmConverter.ToFloat(CodecId.PcmS16Be, new byte[8], new float[8]));
        Assert.Equal(3, PcmConverter.ToFloat(CodecId.PcmS24Le, new byte[9], new float[8]));
        Assert.Equal(2, PcmConverter.ToFloat(CodecId.PcmS32Le, new byte[8], new float[8]));
        Assert.Equal(2, PcmConverter.ToFloat(CodecId.PcmF32Le, new byte[8], new float[8]));
    }

    [Fact]
    public void FromFloat_Returns_Byte_Count_For_Each_Codec()
    {
        float[] src = new float[4];
        Assert.Equal(8,  PcmConverter.FromFloat(CodecId.PcmS16Le, src, new byte[8]));
        Assert.Equal(12, PcmConverter.FromFloat(CodecId.PcmS24Le, src, new byte[12]));
        Assert.Equal(16, PcmConverter.FromFloat(CodecId.PcmS32Le, src, new byte[16]));
        Assert.Equal(16, PcmConverter.FromFloat(CodecId.PcmF32Le, src, new byte[16]));
    }

    [Fact]
    public void S16LeToFloat_Odd_Source_Length_Drops_Trailing_Byte()
    {
        // source.Length = 5 -> 2 samples (the trailing odd byte is ignored).
        byte[] src = { 0x00, 0x00, 0xFF, 0x7F, 0x99 };
        float[] dst = new float[2];
        PcmConverter.S16LeToFloat(src, dst);
        Assert.InRange(dst[0], -1e-3f, 1e-3f);
        Assert.InRange(dst[1], 0.999f, 1.001f);
    }

    [Fact]
    public void S24Le_Sign_Extends_Across_All_Negative_Values()
    {
        // -1 in 24-bit signed is 0xFFFFFF; expected float is -1/8388608 ≈ -1.19e-7.
        byte[] bytes = { 0xFF, 0xFF, 0xFF };
        Span<float> dst = stackalloc float[1];
        PcmConverter.S24LeToFloat(bytes, dst);
        Assert.True(dst[0] < 0f);
        Assert.InRange(dst[0], -1e-6f, 0f);
    }

    [Fact]
    public void S32Le_Min_Value_Maps_To_NegativeOne()
    {
        byte[] bytes = { 0x00, 0x00, 0x00, 0x80 }; // int.MinValue
        Span<float> dst = stackalloc float[1];
        PcmConverter.S32LeToFloat(bytes, dst);
        Assert.Equal(-1.0f, dst[0], 6);
    }

    // ---------- helpers ----------

    private static byte[] PackS16Le(short[] input)
    {
        byte[] bytes = new byte[input.Length * 2];
        for (int i = 0; i < input.Length; i++)
        {
            bytes[i * 2] = (byte)(input[i] & 0xFF);
            bytes[i * 2 + 1] = (byte)((input[i] >> 8) & 0xFF);
        }
        return bytes;
    }

    private static byte[] PackS32Le(int[] input)
    {
        byte[] bytes = new byte[input.Length * 4];
        for (int i = 0; i < input.Length; i++)
        {
            int v = input[i];
            bytes[i * 4]     = (byte)(v & 0xFF);
            bytes[i * 4 + 1] = (byte)((v >> 8) & 0xFF);
            bytes[i * 4 + 2] = (byte)((v >> 16) & 0xFF);
            bytes[i * 4 + 3] = (byte)((v >> 24) & 0xFF);
        }
        return bytes;
    }

    private static int[] UnpackS32Le(byte[] bytes)
    {
        int[] outArr = new int[bytes.Length / 4];
        for (int i = 0; i < outArr.Length; i++)
        {
            outArr[i] = bytes[i * 4] | (bytes[i * 4 + 1] << 8) | (bytes[i * 4 + 2] << 16) | (bytes[i * 4 + 3] << 24);
        }
        return outArr;
    }

    private static int SignExtend24(byte[] bytes, int sample)
    {
        int o = sample * 3;
        int v = bytes[o] | (bytes[o + 1] << 8) | (bytes[o + 2] << 16);
        if ((v & 0x800000) != 0) v |= unchecked((int)0xFF000000);
        return v;
    }
}
