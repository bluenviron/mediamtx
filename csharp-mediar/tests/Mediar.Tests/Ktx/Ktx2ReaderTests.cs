using System.IO.Compression;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;
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

    [Fact]
    public void Detects_Etc2_Rgba8_From_Vk_Format()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 151, // VK_FORMAT_ETC2_R8G8B8A8_UNORM_BLOCK
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // one 4x4 ETC2 RGBA8 block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.Etc2Rgba8, reader.Ktx2.Etc);
        Assert.Equal(BcnFormat.None, reader.Ktx2.Bcn);
    }

    [Fact]
    public async Task ReadFrames_Etc2_Rgb_Yields_Decoded_Rgba32()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 147, // VK_FORMAT_ETC2_R8G8B8_UNORM_BLOCK
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]); // all-zero ETC2 RGB block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
            Assert.Equal(4 * 4 * 4, frame.Pixels.Length);
            // All-zero ETC2 RGB block (diff=0) -> (2,2,2,255) per pixel.
            for (int i = 0; i < 16; i++)
            {
                Assert.Equal(2, frame.Pixels.Span[i * 4 + 0]);
                Assert.Equal(255, frame.Pixels.Span[i * 4 + 3]);
            }
        }
    }

    [Fact]
    public async Task Zlib_Supercompression_Decodes_Successfully()
    {
        // Build the original uncompressed RGBA8 payload (2x2 = 16 bytes).
        var original = new byte[2 * 2 * 4];
        for (int i = 0; i < original.Length; i++) original[i] = (byte)(i * 7 + 11);

        // ZLIB-compress it.
        byte[] compressed;
        using (var outMs = new MemoryStream())
        {
            using (var zls = new ZLibStream(outMs, CompressionLevel.Optimal, leaveOpen: true))
            {
                zls.Write(original, 0, original.Length);
            }
            compressed = outMs.ToArray();
        }

        var b = new TestKtx2Builder
        {
            VkFormat = 37, // VK_FORMAT_R8G8B8A8_UNORM
            PixelWidth = 2,
            PixelHeight = 2,
            SupercompressionScheme = 3, // ZLIB
            UncompressedSizes = new List<ulong> { (ulong)original.Length },
        };
        b.MipPayloads.Add(compressed);

        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal((uint)3, reader.Ktx2.SupercompressionScheme);

        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadFramesAsync()) frames.Add(f);
        Assert.Single(frames);
        var frame = frames[0];
        Assert.Equal(2, frame.Width);
        Assert.Equal(2, frame.Height);
        Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(original));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_B8G8R8_UNORM()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 30, // VK_FORMAT_B8G8R8_UNORM
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[] { 0x11, 0x22, 0x33, 0x44, 0x55, 0x66 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Bgr24, reader.Info.PixelFormat);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R16_UNORM_To_Gray16()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 70, // VK_FORMAT_R16_UNORM
            PixelWidth = 2,
            PixelHeight = 2,
        };
        var pixels = new byte[] { 0x00, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Gray16, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R16G16_UNORM_To_Rg32()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 77, // VK_FORMAT_R16G16_UNORM
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[] { 0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rg32, reader.Info.PixelFormat);
        Assert.Equal(32, reader.Info.BitsPerPixel);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_EAC_RG11_UNORM_Through_EtcDecoder_To_Rg32()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 155, // VK_FORMAT_EAC_R11G11_UNORM_BLOCK
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // one 4x4 EAC RG11 block (16 bytes)
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.EacRg11Unorm, reader.Ktx2.Etc);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(4, frame.Width);
            Assert.Equal(4, frame.Height);
            Assert.Equal(PixelFormat.Rg32, frame.PixelFormat);
            Assert.Equal(4 * 4 * 4, frame.Pixels.Length);
        }
    }
}
