using Mediar.Codecs.Bcn;
using Mediar.Imaging;
using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Tests for <see cref="Ktx2Reader"/>, covering Khronos identifier
/// validation, VkFormat-to-BCn mapping, uncompressed pixel decode, mip
/// level index walk, key-value pool parsing, and supercompression
/// fallback behaviour.
/// </summary>
public sealed class Ktx2ReaderTests
{
    [Fact]
    public void Rejects_Truncated_File()
    {
        using var ms = new MemoryStream(new byte[20], writable: false);
        Assert.Throws<ImageFormatException>(() => Ktx2Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_Identifier()
    {
        var bytes = new byte[128];
        // First byte already 0x00 -> mismatch.
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Ktx2Reader.Open(ms));
    }

    [Fact]
    public void Parses_Uncompressed_R8G8B8A8_UNORM()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37, // VK_FORMAT_R8G8B8A8_UNORM
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[4 * 4 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.Equal(4, reader.Info.Width);
        Assert.Equal(4, reader.Info.Height);
        Assert.Single(reader.Levels);
        Assert.Equal(BcnFormat.None, reader.Ktx2.Bcn);
    }

    [Fact]
    public void Detects_Bc1_From_VK_FORMAT_BC1_RGBA_UNORM()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 133, // VK_FORMAT_BC1_RGBA_UNORM_BLOCK
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]); // 1 BC1 block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(BcnFormat.Bc1, reader.Ktx2.Bcn);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public void Detects_Bc6h_From_VK_FORMAT_BC6H_UFLOAT_BLOCK()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 143, // VK_FORMAT_BC6H_UFLOAT_BLOCK
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // 1 BC6H block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(BcnFormat.Bc6hUf16, reader.Ktx2.Bcn);
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
    }

    [Fact]
    public void Walks_Multiple_Mip_Levels()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37,
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[4 * 4 * 4]);
        b.MipPayloads.Add(new byte[2 * 2 * 4]);
        b.MipPayloads.Add(new byte[1 * 1 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(3, reader.Levels.Count);
        Assert.Equal((4, 4), (reader.Levels[0].Width, reader.Levels[0].Height));
        Assert.Equal((2, 2), (reader.Levels[1].Width, reader.Levels[1].Height));
        Assert.Equal((1, 1), (reader.Levels[2].Width, reader.Levels[2].Height));
    }

    [Fact]
    public async Task ReadFrames_Uncompressed_Round_Trips_Pixels()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        var payload = new byte[] {
            0x11, 0x22, 0x33, 0x44,
            0x55, 0x66, 0x77, 0x88,
            0x99, 0xAA, 0xBB, 0xCC,
            0xDD, 0xEE, 0xFF, 0x00,
        };
        b.MipPayloads.Add(payload);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        int frames = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            frames++;
            Assert.Equal(2, frame.Width);
            Assert.Equal(2, frame.Height);
            Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
            Assert.Equal(0x11, frame.Pixels.Span[0]);
            Assert.Equal(0xFF, frame.Pixels.Span[14]);
        }
        Assert.Equal(1, frames);
    }

    [Fact]
    public void Key_Value_Pool_Is_Parsed()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        b.KeyValues.Add(new("KTXorientation", "rd"));
        b.KeyValues.Add(new("KTXwriter", "Mediar 0.1"));
        b.MipPayloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal("rd", reader.Ktx2.KeyValues["KTXorientation"]);
        Assert.Equal("Mediar 0.1", reader.Ktx2.KeyValues["KTXwriter"]);
    }

    [Fact]
    public async Task Supercompressed_Surface_Throws_From_ReadFramesAsync()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 0, // Basis Universal supercompressed
            SupercompressionScheme = 1, // BasisLZ
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[32]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.False(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync()) { }
        });
    }

    [Fact]
    public void Detector_Recognises_Ktx2_Identifier()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        b.MipPayloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        Assert.Equal(ImageFormat.Ktx2, ImageFormatDetector.Detect(bytes));
    }

    [Theory]
    [InlineData(37u, BcnFormat.None)]    // R8G8B8A8_UNORM
    [InlineData(131u, BcnFormat.Bc1)]    // BC1_RGB_UNORM
    [InlineData(135u, BcnFormat.Bc2)]    // BC2
    [InlineData(137u, BcnFormat.Bc3)]    // BC3
    [InlineData(139u, BcnFormat.Bc4)]    // BC4 UNORM
    [InlineData(141u, BcnFormat.Bc5)]    // BC5 UNORM
    [InlineData(143u, BcnFormat.Bc6hUf16)]
    [InlineData(144u, BcnFormat.Bc6hSf16)]
    [InlineData(145u, BcnFormat.Bc7)]
    public void Vk_Format_Maps_To_Bcn(uint vk, BcnFormat expected)
    {
        Assert.Equal(expected, KtxFormat.MapVkFormat(vk));
    }
}
