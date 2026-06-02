using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DDS DXGI 8/16-bit integer texture formats. These share
/// byte layout with their UNORM/SNORM counterparts (Gray8/Gray16/Rg32/
/// Rgba32/Rgba64/GrayAlpha16) so they decode through the same byte-copy
/// path - the test surface validates the DXGI code-to-PixelFormat
/// mapping per Microsoft's dxgiformat.h.
/// </summary>
public sealed class DdsDxgiSmallIntegerFormatsTests
{
    [Theory]
    // 8-bit single-channel
    [InlineData(62u, PixelFormat.Gray8, 8, false)]   // R8_UINT
    [InlineData(64u, PixelFormat.Gray8, 8, false)]   // R8_SINT
    // 16-bit single-channel
    [InlineData(57u, PixelFormat.Gray16, 16, false)] // R16_UINT
    [InlineData(59u, PixelFormat.Gray16, 16, false)] // R16_SINT
    // 8-bit two-channel
    [InlineData(50u, PixelFormat.GrayAlpha16, 16, false)] // R8G8_UINT
    [InlineData(52u, PixelFormat.GrayAlpha16, 16, false)] // R8G8_SINT
    // 16-bit two-channel
    [InlineData(36u, PixelFormat.Rg32, 32, false)]   // R16G16_UINT
    [InlineData(38u, PixelFormat.Rg32, 32, false)]   // R16G16_SINT
    // 8-bit four-channel
    [InlineData(30u, PixelFormat.Rgba32, 32, true)]  // R8G8B8A8_UINT
    [InlineData(32u, PixelFormat.Rgba32, 32, true)]  // R8G8B8A8_SINT
    // 16-bit four-channel
    [InlineData(12u, PixelFormat.Rgba64, 64, true)]  // R16G16B16A16_UINT
    [InlineData(14u, PixelFormat.Rgba64, 64, true)]  // R16G16B16A16_SINT
    public void DxgiSmallIntegerFormats_Map_To_Correct_PixelFormat(
        uint dxgi, PixelFormat expected, int bpp, bool hasAlpha)
    {
        var dds = BuildDx10Dds(1, 1, dxgi);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
        Assert.Equal(hasAlpha, reader.Info.HasAlpha);
        Assert.Equal(1, reader.Info.Width);
        Assert.Equal(1, reader.Info.Height);
    }

    [Fact]
    public async Task DxgiR8G8B8A8_UINT_Round_Trips_Pixels_Byte_For_Byte()
    {
        const uint dxgi = 30u; // R8G8B8A8_UINT
        var pixels = new byte[] { 10, 20, 30, 40, 50, 60, 70, 80 };
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);

        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR16_UINT_Round_Trips_Pixels_Byte_For_Byte()
    {
        const uint dxgi = 57u; // R16_UINT
        // Two pixels of unsigned 16-bit data: 0x1234 and 0xABCD
        var pixels = new byte[] { 0x34, 0x12, 0xCD, 0xAB };
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Gray16, reader.Info.PixelFormat);

        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR8_UINT_Round_Trips_Pixels_Byte_For_Byte()
    {
        const uint dxgi = 62u; // R8_UINT
        var pixels = new byte[] { 0x00, 0x7F, 0x80, 0xFF };
        var dds = BuildDx10Dds(4, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR8G8_SINT_Round_Trips_Signed_Bytes()
    {
        const uint dxgi = 52u; // R8G8_SINT
        var pixels = new byte[] { 0x80, 0x7F, 0xFF, 0x01 }; // -128, 127, -1, 1
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.GrayAlpha16, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR16G16_UINT_Round_Trips_Pixels_Byte_For_Byte()
    {
        const uint dxgi = 36u; // R16G16_UINT
        var pixels = new byte[] { 0x00, 0x00, 0xFF, 0xFF, 0x34, 0x12, 0xCD, 0xAB };
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Rg32, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR16G16B16A16_UINT_Round_Trips_Pixels_Byte_For_Byte()
    {
        const uint dxgi = 12u; // R16G16B16A16_UINT
        var pixels = new byte[16];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 17);
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba64, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR8G8B8A8_SINT_Round_Trips_Bytes_Verbatim()
    {
        const uint dxgi = 32u; // R8G8B8A8_SINT
        var pixels = new byte[] { 0x80, 0x7F, 0xFF, 0x01, 0x00, 0x00, 0xFF, 0xFF };
        var dds = BuildDx10Dds(2, 1, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    [Fact]
    public async Task DxgiR16G16_UINT_MultiRow_Stride_Is_Correct()
    {
        const int w = 2, h = 3;
        const uint dxgi = 36u; // R16G16_UINT, 4 bpp
        var pixels = new byte[w * h * 4];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)((i * 7) & 0xFF);
        var dds = BuildDx10Dds(w, h, dxgi, pixels);
        using var ms = new MemoryStream(dds, writable: false);
        using var reader = DdsReader.Open(ms);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(w * 4, frame.Stride);
            Assert.Equal(h, frame.Height);
            Assert.Equal(pixels, frame.Pixels.ToArray());
            return;
        }
        Assert.Fail("expected one frame");
    }

    private static byte[] BuildDx10Dds(int width, int height, uint dxgi, byte[]? payload = null)
    {
        var bpp = dxgi switch
        {
            62u or 64u or 65u => 1,
            57u or 59u or 50u or 52u => 2,
            30u or 32u or 36u or 38u => 4,
            12u or 14u => 8,
            _ => throw new ArgumentException("unhandled dxgi for test builder"),
        };
        payload ??= new byte[width * height * bpp];

        var dds = new byte[148 + payload.Length];
        // "DDS " magic
        dds[0] = (byte)'D'; dds[1] = (byte)'D'; dds[2] = (byte)'S'; dds[3] = (byte)' ';
        WriteU32(dds, 4, 124);  // header size
        WriteU32(dds, 8, 0x1007); // flags: caps | height | width | pixelformat
        WriteU32(dds, 12, (uint)height);
        WriteU32(dds, 16, (uint)width);
        WriteU32(dds, 20, (uint)(width * bpp)); // pitch
        // pixelformat block at offset 76, size = 32 bytes
        WriteU32(dds, 76, 32);
        WriteU32(dds, 80, 0x4); // DDPF_FOURCC
        // FourCC = "DX10"
        dds[84] = (byte)'D'; dds[85] = (byte)'X'; dds[86] = (byte)'1'; dds[87] = (byte)'0';
        WriteU32(dds, 108, 0x1000); // caps: texture
        // DX10 header at offset 128: dxgiFormat / resourceDimension / miscFlag / arraySize / miscFlags2
        WriteU32(dds, 128, dxgi);
        WriteU32(dds, 132, 3); // TEXTURE2D
        WriteU32(dds, 136, 0);
        WriteU32(dds, 140, 1); // arraySize
        WriteU32(dds, 144, 0);
        Array.Copy(payload, 0, dds, 148, payload.Length);
        return dds;
    }

    private static void WriteU32(byte[] buf, int off, uint val)
    {
        buf[off] = (byte)(val & 0xFF);
        buf[off + 1] = (byte)((val >> 8) & 0xFF);
        buf[off + 2] = (byte)((val >> 16) & 0xFF);
        buf[off + 3] = (byte)((val >> 24) & 0xFF);
    }
}
