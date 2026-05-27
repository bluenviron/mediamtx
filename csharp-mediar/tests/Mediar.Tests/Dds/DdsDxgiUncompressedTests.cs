using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI extended uncompressed pixel formats in the DX10 DDS header
/// path. Covers the common 8-bit / 16-bit / float SRGB-suffixed variants the
/// modern Direct3D 12 / Vulkan asset pipelines emit.
/// </summary>
public sealed class DdsDxgiUncompressedTests
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
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(4, 4), 3); // resourceDimension = TEXTURE2D
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(12, 4), 1); // arraySize

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
    public void Dxgi_R8G8B8A8_UNORM_Maps_To_Rgba32_NoSrgb()
    {
        var file = Concat(BuildDx10Dds(2, 2, 28), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
        Assert.Null(reader.Info.ColorSpace);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public void Dxgi_R8G8B8A8_UNORM_SRGB_Sets_ColorSpace_To_sRGB()
    {
        var file = Concat(BuildDx10Dds(2, 2, 29), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void Dxgi_B8G8R8A8_UNORM_SRGB_Maps_To_Bgra32_sRGB()
    {
        var file = Concat(BuildDx10Dds(2, 2, 91), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void Dxgi_B8G8R8X8_UNORM_Maps_To_Bgra32_NoAlpha()
    {
        var file = Concat(BuildDx10Dds(2, 2, 88), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);
        Assert.False(reader.Info.HasAlpha);
        Assert.Null(reader.Info.ColorSpace);
    }

    [Fact]
    public void Dxgi_R16G16_UNORM_Maps_To_Rg32()
    {
        var file = Concat(BuildDx10Dds(2, 2, 35), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rg32, reader.Info.PixelFormat);
        Assert.Equal(32, reader.Info.BitsPerPixel);
        Assert.False(reader.Info.HasAlpha);
    }

    [Fact]
    public void Dxgi_R16G16B16A16_UNORM_Maps_To_Rgba64()
    {
        var file = Concat(BuildDx10Dds(2, 2, 11), new byte[2 * 2 * 8]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba64, reader.Info.PixelFormat);
        Assert.Equal(64, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
    }

    [Fact]
    public void Dxgi_R16G16B16A16_FLOAT_Maps_To_Rgba64Float_With_Linear_ColorSpace()
    {
        var file = Concat(BuildDx10Dds(2, 2, 10), new byte[2 * 2 * 8]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba64Float, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
        Assert.Equal("Linear", reader.Info.ColorSpace);
    }

    [Fact]
    public void Dxgi_R32G32B32A32_FLOAT_Maps_To_Rgba128Float()
    {
        var file = Concat(BuildDx10Dds(2, 2, 2), new byte[2 * 2 * 16]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba128Float, reader.Info.PixelFormat);
        Assert.Equal(128, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
        Assert.Equal("Linear", reader.Info.ColorSpace);
    }

    [Fact]
    public void Dxgi_R8_UNORM_Maps_To_Gray8()
    {
        var file = Concat(BuildDx10Dds(2, 2, 61), new byte[2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
        Assert.Equal(8, reader.Info.BitsPerPixel);
    }

    [Fact]
    public void Dxgi_A8_UNORM_Maps_To_Gray8()
    {
        var file = Concat(BuildDx10Dds(2, 2, 65), new byte[2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
    }

    [Fact]
    public void Dxgi_R8G8_UNORM_Maps_To_GrayAlpha16()
    {
        var file = Concat(BuildDx10Dds(2, 2, 49), new byte[2 * 2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.GrayAlpha16, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
    }

    [Fact]
    public void Dxgi_R16_UNORM_Maps_To_Gray16()
    {
        var file = Concat(BuildDx10Dds(2, 2, 56), new byte[2 * 2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Gray16, reader.Info.PixelFormat);
    }

    [Fact]
    public void Dxgi_B5G6R5_UNORM_Maps_To_Rgb565()
    {
        var file = Concat(BuildDx10Dds(2, 2, 85), new byte[2 * 2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgb565, reader.Info.PixelFormat);
    }

    [Fact]
    public void Dxgi_B5G5R5A1_UNORM_Maps_To_Rgba5551()
    {
        var file = Concat(BuildDx10Dds(2, 2, 86), new byte[2 * 2 * 2]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba5551, reader.Info.PixelFormat);
        Assert.True(reader.Info.HasAlpha);
    }

    [Fact]
    public void Dxgi_Unknown_Format_Surfaces_As_Undecodable()
    {
        // DXGI value 999 is not a real DXGI_FORMAT - falls through to "unknown".
        // The Open() path raises ImageFormatException or marks as undecodable.
        var file = Concat(BuildDx10Dds(2, 2, 999), new byte[2 * 2 * 4]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.False(reader.CanDecodePixels);
    }

    [Fact]
    public async Task Dxgi_R8G8B8A8_UNORM_Round_Trips_Pixels()
    {
        var pixels = new byte[]
        {
            0xFF, 0x00, 0x00, 0xFF,
            0x00, 0xFF, 0x00, 0xFF,
            0x00, 0x00, 0xFF, 0xFF,
            0x80, 0x80, 0x80, 0x80,
        };
        var file = Concat(BuildDx10Dds(2, 2, 28), pixels);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
            frame.Dispose();
        }
    }

    [Fact]
    public async Task Dxgi_R16G16B16A16_UNORM_Round_Trips_Pixels()
    {
        var pixels = new byte[2 * 2 * 8];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 7);
        var file = Concat(BuildDx10Dds(2, 2, 11), pixels);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
            frame.Dispose();
        }
    }
}
