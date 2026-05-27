using Mediar.Imaging;
using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Tests for KTX1 and KTX2 8/16-bit integer texture formats. These share
/// byte layout with their UNORM/SNORM siblings (Gray8/Gray16/Rg32/Rgb48/
/// Rgba64) so they decode through the same byte-copy path - the test
/// surface validates the format-id-to-PixelFormat mapping per spec.
/// </summary>
public sealed class KtxSmallIntegerFormatsTests
{
    [Theory]
    [InlineData(0x8232u, PixelFormat.Gray8, 8)]   // GL_R8UI
    [InlineData(0x8231u, PixelFormat.Gray8, 8)]   // GL_R8I
    [InlineData(0x8234u, PixelFormat.Gray16, 16)] // GL_R16UI
    [InlineData(0x8233u, PixelFormat.Gray16, 16)] // GL_R16I
    [InlineData(0x8238u, PixelFormat.GrayAlpha16, 16)] // GL_RG8UI
    [InlineData(0x8237u, PixelFormat.GrayAlpha16, 16)] // GL_RG8I
    [InlineData(0x823Au, PixelFormat.Rg32, 32)]   // GL_RG16UI
    [InlineData(0x8239u, PixelFormat.Rg32, 32)]   // GL_RG16I
    [InlineData(0x8D7Du, PixelFormat.Rgb24, 24)]  // GL_RGB8UI
    [InlineData(0x8D8Fu, PixelFormat.Rgb24, 24)]  // GL_RGB8I
    [InlineData(0x8D77u, PixelFormat.Rgb48, 48)]  // GL_RGB16UI
    [InlineData(0x8D89u, PixelFormat.Rgb48, 48)]  // GL_RGB16I
    [InlineData(0x8D7Cu, PixelFormat.Rgba32, 32)] // GL_RGBA8UI
    [InlineData(0x8D8Eu, PixelFormat.Rgba32, 32)] // GL_RGBA8I
    [InlineData(0x8D76u, PixelFormat.Rgba64, 64)] // GL_RGBA16UI
    [InlineData(0x8D88u, PixelFormat.Rgba64, 64)] // GL_RGBA16I
    public void Ktx1_Gl_Small_Integer_Tokens_Map_To_Correct_PixelFormat(
        uint glFormat, PixelFormat expected, int bpp)
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = glFormat,
            PixelWidth = 1,
            PixelHeight = 1,
        };
        b.MipPayloads.Add(new byte[bpp / 8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
    }

    [Theory]
    [InlineData(13u, PixelFormat.Gray8, 8)]   // VK_FORMAT_R8_UINT
    [InlineData(14u, PixelFormat.Gray8, 8)]   // VK_FORMAT_R8_SINT
    [InlineData(74u, PixelFormat.Gray16, 16)] // VK_FORMAT_R16_UINT
    [InlineData(75u, PixelFormat.Gray16, 16)] // VK_FORMAT_R16_SINT
    [InlineData(20u, PixelFormat.GrayAlpha16, 16)] // VK_FORMAT_R8G8_UINT
    [InlineData(21u, PixelFormat.GrayAlpha16, 16)] // VK_FORMAT_R8G8_SINT
    [InlineData(81u, PixelFormat.Rg32, 32)]   // VK_FORMAT_R16G16_UINT
    [InlineData(82u, PixelFormat.Rg32, 32)]   // VK_FORMAT_R16G16_SINT
    [InlineData(27u, PixelFormat.Rgb24, 24)]  // VK_FORMAT_R8G8B8_UINT
    [InlineData(28u, PixelFormat.Rgb24, 24)]  // VK_FORMAT_R8G8B8_SINT
    [InlineData(88u, PixelFormat.Rgb48, 48)]  // VK_FORMAT_R16G16B16_UINT
    [InlineData(89u, PixelFormat.Rgb48, 48)]  // VK_FORMAT_R16G16B16_SINT
    [InlineData(41u, PixelFormat.Rgba32, 32)] // VK_FORMAT_R8G8B8A8_UINT
    [InlineData(42u, PixelFormat.Rgba32, 32)] // VK_FORMAT_R8G8B8A8_SINT
    [InlineData(95u, PixelFormat.Rgba64, 64)] // VK_FORMAT_R16G16B16A16_UINT
    [InlineData(96u, PixelFormat.Rgba64, 64)] // VK_FORMAT_R16G16B16A16_SINT
    public void Ktx2_Vk_Small_Integer_Codes_Map_To_Correct_PixelFormat(
        uint vkFormat, PixelFormat expected, int bpp)
    {
        var b = new TestKtx2Builder
        {
            VkFormat = vkFormat,
            PixelWidth = 1,
            PixelHeight = 1,
        };
        b.MipPayloads.Add(new byte[bpp / 8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
    }
}
