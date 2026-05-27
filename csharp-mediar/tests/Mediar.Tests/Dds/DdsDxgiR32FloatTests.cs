using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI <c>R32_FLOAT</c> (code 41) decoded via <c>Gray32Float</c>.
/// </summary>
public sealed class DdsDxgiR32FloatTests
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
    public void R32_FLOAT_Maps_To_Gray32Float_With_Linear_ColorSpace()
    {
        var file = Concat(BuildDx10Dds(2, 2, 41), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Gray32Float, reader.Info.PixelFormat);
        Assert.False(reader.Info.HasAlpha);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.Equal(32, reader.Info.BitsPerPixel);
    }

    [Fact]
    public async Task R32_FLOAT_Round_Trips_Pixel_Bytes()
    {
        const int w = 2, h = 1;
        var payload = new byte[w * h * 4];
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(0, 4), 1.5f);
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(4, 4), -3.25f);

        var file = Concat(BuildDx10Dds(w, h, 41), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));

        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Gray32Float, frame!.PixelFormat);
        Assert.Equal(w * 4, frame.Stride);

        float v0 = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(0, 4));
        float v1 = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        Assert.Equal(1.5f, v0);
        Assert.Equal(-3.25f, v1);
    }

    [Fact]
    public void Gray32Float_Format_Reports_32_Bits_And_1_Channel()
    {
        Assert.Equal(32, PixelFormat.Gray32Float.BitsPerPixel());
        Assert.Equal(1, PixelFormat.Gray32Float.ChannelCount());
        Assert.False(PixelFormat.Gray32Float.HasAlpha());
    }
}
