using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Tests for KTX1 and KTX2 32-bit integer texture formats - <c>GL_R32UI</c>,
/// <c>GL_R32I</c>, <c>GL_RG32UI</c>, <c>GL_RG32I</c>, <c>GL_RGB32UI</c>,
/// <c>GL_RGB32I</c>, <c>GL_RGBA32UI</c>, <c>GL_RGBA32I</c> and their
/// Vulkan counterparts (codes 98/99/101/102/104/105/107/108).
/// </summary>
public sealed class KtxIntegerFormatsTests
{
    [Theory]
    [InlineData(0x8236u, PixelFormat.Gray32UInt, 32, 1)] // GL_R32UI
    [InlineData(0x8235u, PixelFormat.Gray32SInt, 32, 1)] // GL_R32I
    [InlineData(0x823Cu, PixelFormat.Rg64UInt, 64, 2)]   // GL_RG32UI
    [InlineData(0x823Bu, PixelFormat.Rg64SInt, 64, 2)]   // GL_RG32I
    [InlineData(0x8D71u, PixelFormat.Rgb96UInt, 96, 3)]  // GL_RGB32UI
    [InlineData(0x8D83u, PixelFormat.Rgb96SInt, 96, 3)]  // GL_RGB32I
    [InlineData(0x8D70u, PixelFormat.Rgba128UInt, 128, 4)] // GL_RGBA32UI
    [InlineData(0x8D82u, PixelFormat.Rgba128SInt, 128, 4)] // GL_RGBA32I
    public void Ktx1_Gl_Integer_Tokens_Map_To_Correct_PixelFormat(
        uint glFormat, PixelFormat expected, int bpp, int channels)
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = glFormat,
            PixelWidth = 1,
            PixelHeight = 1,
        };
        int bytesPerPixel = bpp / 8;
        b.MipPayloads.Add(new byte[bytesPerPixel]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
        Assert.Equal(channels, reader.Info.ChannelCount);
    }

    [Fact]
    public async Task Ktx1_Decodes_GL_RGBA32UI_Round_Trip()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8D70, // GL_RGBA32UI
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[16];
        BinaryPrimitives.WriteUInt32LittleEndian(pixels.AsSpan(0, 4), 0xCAFEBABE);
        BinaryPrimitives.WriteUInt32LittleEndian(pixels.AsSpan(4, 4), 0xDEADBEEF);
        BinaryPrimitives.WriteUInt32LittleEndian(pixels.AsSpan(8, 4), 0x12345678);
        BinaryPrimitives.WriteUInt32LittleEndian(pixels.AsSpan(12, 4), 0xFFFFFFFF);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba128UInt, reader.Info.PixelFormat);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Theory]
    [InlineData(98u, PixelFormat.Gray32UInt, 32, 1)]    // VK_FORMAT_R32_UINT
    [InlineData(99u, PixelFormat.Gray32SInt, 32, 1)]    // VK_FORMAT_R32_SINT
    [InlineData(101u, PixelFormat.Rg64UInt, 64, 2)]     // VK_FORMAT_R32G32_UINT
    [InlineData(102u, PixelFormat.Rg64SInt, 64, 2)]     // VK_FORMAT_R32G32_SINT
    [InlineData(104u, PixelFormat.Rgb96UInt, 96, 3)]    // VK_FORMAT_R32G32B32_UINT
    [InlineData(105u, PixelFormat.Rgb96SInt, 96, 3)]    // VK_FORMAT_R32G32B32_SINT
    [InlineData(107u, PixelFormat.Rgba128UInt, 128, 4)] // VK_FORMAT_R32G32B32A32_UINT
    [InlineData(108u, PixelFormat.Rgba128SInt, 128, 4)] // VK_FORMAT_R32G32B32A32_SINT
    public void Ktx2_Vk_Integer_Codes_Map_To_Correct_PixelFormat(
        uint vkFormat, PixelFormat expected, int bpp, int channels)
    {
        var b = new TestKtx2Builder
        {
            VkFormat = vkFormat,
            PixelWidth = 1,
            PixelHeight = 1,
        };
        int bytesPerPixel = bpp / 8;
        b.MipPayloads.Add(new byte[bytesPerPixel]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
        Assert.Equal(channels, reader.Info.ChannelCount);
    }

    [Fact]
    public async Task Ktx2_Decodes_VK_FORMAT_R32G32_SINT_Round_Trip()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 102, // VK_FORMAT_R32G32_SINT
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[8];
        BinaryPrimitives.WriteInt32LittleEndian(pixels.AsSpan(0, 4), int.MinValue);
        BinaryPrimitives.WriteInt32LittleEndian(pixels.AsSpan(4, 4), int.MaxValue);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rg64SInt, reader.Info.PixelFormat);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }
}
