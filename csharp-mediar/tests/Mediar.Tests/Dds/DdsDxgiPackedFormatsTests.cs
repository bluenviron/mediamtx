using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI packed bit-field uncompressed formats: R10G10B10A2_UNORM
/// (DXGI 24), R11G11B10_FLOAT (DXGI 26), R9G9B9E5_SHAREDEXP (DXGI 67).
/// These three formats pack 3 or 4 channels into a single 32-bit word and
/// therefore require runtime unpacking via <see cref="DdsPackedUnpacker"/>
/// instead of a flat byte copy.
/// </summary>
public sealed class DdsDxgiPackedFormatsTests
{
    private const uint DDPF_FOURCC = 0x4;

    private static byte[] BuildDx10Dds(int width, int height, uint dxgiFormat)
    {
        var hdr = new byte[128];
        hdr[0] = (byte)'D'; hdr[1] = (byte)'D'; hdr[2] = (byte)'S'; hdr[3] = (byte)' ';
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4), 124);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(8), 0x1007);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(12), (uint)height);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(16), (uint)width);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(76), 32);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(80), DDPF_FOURCC);
        var dx10 = Encoding.ASCII.GetBytes("DX10");
        hdr[84] = dx10[0]; hdr[85] = dx10[1]; hdr[86] = dx10[2]; hdr[87] = dx10[3];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(108), 0x1000);

        var tail = new byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(0, 4), dxgiFormat);
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(4, 4), 3);
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(12, 4), 1);

        var combined = new byte[hdr.Length + tail.Length];
        Buffer.BlockCopy(hdr, 0, combined, 0, hdr.Length);
        Buffer.BlockCopy(tail, 0, combined, hdr.Length, tail.Length);
        return combined;
    }

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    [Fact]
    public void R10G10B10A2_UNORM_Maps_To_Rgba32_With_Alpha()
    {
        var file = Concat(BuildDx10Dds(2, 2, 24), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public void R11G11B10_FLOAT_Maps_To_Rgb96Float_Linear()
    {
        var file = Concat(BuildDx10Dds(2, 2, 26), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.False(reader.Info.HasAlpha);
    }

    [Fact]
    public void R9G9B9E5_SHAREDEXP_Maps_To_Rgb96Float_Linear()
    {
        var file = Concat(BuildDx10Dds(2, 2, 67), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.False(reader.Info.HasAlpha);
    }

    [Fact]
    public async Task R10G10B10A2_UNORM_Unpacks_To_Rgba32_Pixels()
    {
        // 1x1 pixel: R10=1023 (full red), G10=0, B10=0, A2=3 (full alpha).
        // Packed word = (3 << 30) | (0 << 20) | (0 << 10) | 1023
        //             = 0xC00003FF
        uint w0 = (3u << 30) | (0u << 20) | (0u << 10) | 1023u;
        uint w1 = (0u << 30) | (1023u << 20) | (0u << 10) | 0u; // full blue, zero alpha
        uint w2 = (3u << 30) | (0u << 20) | (1023u << 10) | 0u; // full green, full alpha
        uint w3 = 0u;                                            // black, zero alpha

        var data = new byte[16];
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(0, 4), w0);
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(4, 4), w1);
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(8, 4), w2);
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(12, 4), w3);

        var file = Concat(BuildDx10Dds(2, 2, 24), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgba32, frame!.PixelFormat);
        Assert.Equal(2, frame.Width);
        Assert.Equal(2, frame.Height);

        // (0,0): R=255, G=0, B=0, A=255
        Assert.Equal(255, frame.Pixels.Span[0]);
        Assert.Equal(0, frame.Pixels.Span[1]);
        Assert.Equal(0, frame.Pixels.Span[2]);
        Assert.Equal(255, frame.Pixels.Span[3]);

        // (1,0): R=0, G=0, B=255, A=0
        Assert.Equal(0, frame.Pixels.Span[4]);
        Assert.Equal(0, frame.Pixels.Span[5]);
        Assert.Equal(255, frame.Pixels.Span[6]);
        Assert.Equal(0, frame.Pixels.Span[7]);

        // (0,1): R=0, G=255, B=0, A=255
        Assert.Equal(0, frame.Pixels.Span[8]);
        Assert.Equal(255, frame.Pixels.Span[9]);
        Assert.Equal(0, frame.Pixels.Span[10]);
        Assert.Equal(255, frame.Pixels.Span[11]);

        // (1,1): all zero
        Assert.Equal(0, frame.Pixels.Span[12]);
        Assert.Equal(0, frame.Pixels.Span[13]);
        Assert.Equal(0, frame.Pixels.Span[14]);
        Assert.Equal(0, frame.Pixels.Span[15]);
    }

    [Fact]
    public async Task R11G11B10_FLOAT_Unpacks_To_Rgb96Float_Pixels()
    {
        // Each pixel: choose an exponent + mantissa for each channel.
        // For 5e6 floats (R/G), the value 0x3C0 (exp=15, mant=0) is 1.0.
        // For 5e5 float (B), the value 0x1E0 (exp=15, mant=0) is 1.0.
        // Pack 1x1: R=1.0, G=1.0, B=1.0
        ushort rEnc = 0x3C0;
        ushort gEnc = 0x3C0;
        ushort bEnc = 0x1E0;
        uint packed = (uint)rEnc | ((uint)gEnc << 11) | ((uint)bEnc << 22);

        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);

        var file = Concat(BuildDx10Dds(1, 1, 26), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgb96Float, frame!.PixelFormat);

        float r = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(0, 4));
        float g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        float b = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(8, 4));
        Assert.Equal(1.0f, r, 5);
        Assert.Equal(1.0f, g, 5);
        Assert.Equal(1.0f, b, 5);
    }

    [Fact]
    public async Task R11G11B10_FLOAT_Zero_Word_Yields_Zero_Pixels()
    {
        var data = new byte[4];
        var file = Concat(BuildDx10Dds(1, 1, 26), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        float r = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(0, 4));
        float g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        float b = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(8, 4));
        Assert.Equal(0.0f, r);
        Assert.Equal(0.0f, g);
        Assert.Equal(0.0f, b);
    }

    [Fact]
    public async Task R9G9B9E5_SHAREDEXP_Unpacks_To_Rgb96Float_Pixels()
    {
        // R/G/B mantissas = 256, shared exp = 24. Expected:
        //   scale = 2^(24 - 15 - 9) = 2^0 = 1.0
        //   r = 256 * 1.0 = 256.0
        // So all three channels = 256.0.
        uint rm = 256u;
        uint gm = 256u;
        uint bm = 256u;
        uint exp = 24u;
        uint packed = rm | (gm << 9) | (bm << 18) | (exp << 27);

        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);

        var file = Concat(BuildDx10Dds(1, 1, 67), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgb96Float, frame!.PixelFormat);

        float r = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(0, 4));
        float g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        float b = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(8, 4));
        Assert.Equal(256.0f, r, 3);
        Assert.Equal(256.0f, g, 3);
        Assert.Equal(256.0f, b, 3);
    }

    [Fact]
    public async Task R9G9B9E5_SHAREDEXP_Zero_Word_Yields_Zero_Pixels()
    {
        var data = new byte[4];
        var file = Concat(BuildDx10Dds(1, 1, 67), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        float r = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(0, 4));
        float g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        float b = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(8, 4));
        Assert.Equal(0.0f, r);
        Assert.Equal(0.0f, g);
        Assert.Equal(0.0f, b);
    }

    [Theory]
    [InlineData(0u, 0)]
    [InlineData(1u, 85)]
    [InlineData(2u, 170)]
    [InlineData(3u, 255)]
    public async Task R10G10B10A2_UNORM_Alpha_Quantization_Matches_Spec(uint a2, int expectedByte)
    {
        uint packed = a2 << 30;
        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);

        var file = Concat(BuildDx10Dds(1, 1, 24), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        Assert.Equal(expectedByte, frame!.Pixels.Span[3]);
    }

    [Fact]
    public async Task R10G10B10A2_UNORM_Zero_Word_Yields_Zero_Pixels()
    {
        var data = new byte[4];
        var file = Concat(BuildDx10Dds(1, 1, 24), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        Assert.Equal(0, frame!.Pixels.Span[0]);
        Assert.Equal(0, frame.Pixels.Span[1]);
        Assert.Equal(0, frame.Pixels.Span[2]);
        Assert.Equal(0, frame.Pixels.Span[3]);
    }

    [Fact]
    public async Task R10G10B10A2_UNORM_Channel_Order_Is_R_G_B_A()
    {
        // R=1023 (red full), G=0, B=0, A=0 -> first byte is full red.
        uint packed = 1023u;
        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);
        var file = Concat(BuildDx10Dds(1, 1, 24), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        Assert.Equal(255, frame!.Pixels.Span[0]);
        Assert.Equal(0, frame.Pixels.Span[1]);
        Assert.Equal(0, frame.Pixels.Span[2]);
        Assert.Equal(0, frame.Pixels.Span[3]);
    }

    [Fact]
    public async Task R11G11B10_FLOAT_R_Channel_Infinity_When_Exp_Saturated_Mantissa_Zero()
    {
        // F11 R: exp=0x1F (5 high bits), mant=0 -> +infinity.
        ushort rEnc = (ushort)(0x1Fu << 6);
        uint packed = rEnc;
        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);
        var file = Concat(BuildDx10Dds(1, 1, 26), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        float r = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(0, 4));
        Assert.True(float.IsPositiveInfinity(r));
    }

    [Fact]
    public async Task R11G11B10_FLOAT_G_Channel_NaN_When_Exp_Saturated_Mantissa_NonZero()
    {
        // F11 G: shift up by 11. exp=0x1F mant=1 -> NaN.
        ushort gEnc = (ushort)((0x1Fu << 6) | 1u);
        uint packed = (uint)gEnc << 11;
        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);
        var file = Concat(BuildDx10Dds(1, 1, 26), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        float g = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(4, 4));
        Assert.True(float.IsNaN(g));
    }

    [Fact]
    public async Task R11G11B10_FLOAT_B_Channel_Infinity_When_Exp_Saturated_Mantissa_Zero()
    {
        // F10 B: exp=0x1F (5 high bits), mant=0 -> +infinity. Shifted by 22.
        ushort bEnc = (ushort)(0x1Fu << 5);
        uint packed = (uint)bEnc << 22;
        var data = new byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(data, packed);
        var file = Concat(BuildDx10Dds(1, 1, 26), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        float b = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(8, 4));
        Assert.True(float.IsPositiveInfinity(b));
    }

    [Fact]
    public async Task R9G9B9E5_SHAREDEXP_Two_Pixels_Decoded_Independently()
    {
        // Pixel 0: rm=gm=bm=256 with exp=24 -> 256.0.
        // Pixel 1: rm=gm=bm=128 with exp=24 -> 128.0.
        uint p0 = 256u | (256u << 9) | (256u << 18) | (24u << 27);
        uint p1 = 128u | (128u << 9) | (128u << 18) | (24u << 27);
        var data = new byte[8];
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(0, 4), p0);
        BinaryPrimitives.WriteUInt32LittleEndian(data.AsSpan(4, 4), p1);

        var file = Concat(BuildDx10Dds(2, 1, 67), data);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) frame = f;
        Assert.NotNull(frame);
        // Each pixel is 12 bytes (3 floats).
        float r0 = BinaryPrimitives.ReadSingleLittleEndian(frame!.Pixels.Span.Slice(0, 4));
        float r1 = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(12, 4));
        Assert.Equal(256.0f, r0, 3);
        Assert.Equal(128.0f, r1, 3);
    }
}
