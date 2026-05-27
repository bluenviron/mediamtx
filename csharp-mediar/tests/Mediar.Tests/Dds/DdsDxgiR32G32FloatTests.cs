using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI <c>R32G32_FLOAT</c> (code 16) decoded via <c>Rg64Float</c>.
/// </summary>
public sealed class DdsDxgiR32G32FloatTests
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
    public void R32G32_FLOAT_Maps_To_Rg64Float_With_Linear_ColorSpace()
    {
        var file = Concat(BuildDx10Dds(2, 2, 16), new byte[2 * 2 * 8]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rg64Float, reader.Info.PixelFormat);
        Assert.False(reader.Info.HasAlpha);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal("Linear", reader.Info.ColorSpace);
        Assert.Equal(64, reader.Info.BitsPerPixel);
        Assert.Equal(2, reader.Info.ChannelCount);
    }

    [Fact]
    public async Task R32G32_FLOAT_Round_Trips_Pixel_Bytes()
    {
        const int w = 2, h = 1;
        var payload = new byte[w * h * 8];
        // pixel 0: (1.5, 0.25)
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(0, 4), 1.5f);
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(4, 4), 0.25f);
        // pixel 1: (-3.5, 100.0)
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(8, 4), -3.5f);
        BinaryPrimitives.WriteSingleLittleEndian(payload.AsSpan(12, 4), 100.0f);

        var file = Concat(BuildDx10Dds(w, h, 16), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));

        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rg64Float, frame!.PixelFormat);
        Assert.Equal(w * 8, frame.Stride);

        float p0r = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(0, 4));
        float p0g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(4, 4));
        float p1r = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(8, 4));
        float p1g = BinaryPrimitives.ReadSingleLittleEndian(frame.Pixels.Span.Slice(12, 4));
        Assert.Equal(1.5f, p0r);
        Assert.Equal(0.25f, p0g);
        Assert.Equal(-3.5f, p1r);
        Assert.Equal(100.0f, p1g);
    }

    [Fact]
    public void Rg64Float_Format_Reports_64_Bits_And_2_Channels()
    {
        Assert.Equal(64, PixelFormat.Rg64Float.BitsPerPixel());
        Assert.Equal(2, PixelFormat.Rg64Float.ChannelCount());
        Assert.False(PixelFormat.Rg64Float.HasAlpha());
    }
}
