using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI half-float (FP16) formats - <c>R16_FLOAT</c> (54),
/// <c>R16G16_FLOAT</c> (34), <c>R16G16B16A16_FLOAT</c> (10) decoded
/// via the <c>Gray16Float</c> / <c>Rg32Float</c> / <c>Rgba64Float</c>
/// PixelFormat family.
/// </summary>
public sealed class DdsDxgiHalfFloatTests
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
    public void R16_FLOAT_Maps_To_Gray16Float()
    {
        var file = Concat(BuildDx10Dds(2, 1, 54), new byte[2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Gray16Float, reader.Info.PixelFormat);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        Assert.False(reader.Info.HasAlpha);
    }

    [Fact]
    public void R16G16_FLOAT_Maps_To_Rg32Float()
    {
        var file = Concat(BuildDx10Dds(2, 1, 34), new byte[2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rg32Float, reader.Info.PixelFormat);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.Equal(32, reader.Info.BitsPerPixel);
        Assert.False(reader.Info.HasAlpha);
    }

    [Fact]
    public async Task R16G16B16A16_FLOAT_Round_Trips_Half_Bytes()
    {
        const int w = 1, h = 1;
        var payload = new byte[w * h * 8];
        BinaryPrimitives.WriteHalfLittleEndian(payload.AsSpan(0, 2), (Half)1.0f);
        BinaryPrimitives.WriteHalfLittleEndian(payload.AsSpan(2, 2), (Half)0.5f);
        BinaryPrimitives.WriteHalfLittleEndian(payload.AsSpan(4, 2), (Half)0.25f);
        BinaryPrimitives.WriteHalfLittleEndian(payload.AsSpan(6, 2), (Half)1.0f);

        var file = Concat(BuildDx10Dds(w, h, 10), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));

        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgba64Float, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public void HalfFloat_Formats_Report_Correct_BitsPerPixel()
    {
        Assert.Equal(16, PixelFormat.Gray16Float.BitsPerPixel());
        Assert.Equal(32, PixelFormat.Rg32Float.BitsPerPixel());
        Assert.Equal(48, PixelFormat.Rgb48Float.BitsPerPixel());
        Assert.Equal(64, PixelFormat.Rgba64Float.BitsPerPixel());

        Assert.Equal(1, PixelFormat.Gray16Float.ChannelCount());
        Assert.Equal(2, PixelFormat.Rg32Float.ChannelCount());
        Assert.Equal(3, PixelFormat.Rgb48Float.ChannelCount());
        Assert.Equal(4, PixelFormat.Rgba64Float.ChannelCount());

        Assert.False(PixelFormat.Gray16Float.HasAlpha());
        Assert.False(PixelFormat.Rg32Float.HasAlpha());
        Assert.False(PixelFormat.Rgb48Float.HasAlpha());
        Assert.True(PixelFormat.Rgba64Float.HasAlpha());
    }
}
