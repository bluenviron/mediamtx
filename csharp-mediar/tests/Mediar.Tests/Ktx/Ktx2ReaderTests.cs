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

    [Fact]
    public async Task Decodes_VK_FORMAT_R16G16B16_UNORM_To_Rgb48()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 84, // VK_FORMAT_R16G16B16_UNORM
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rgb48, reader.Info.PixelFormat);
        Assert.Equal(48, reader.Info.BitsPerPixel);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R16G16B16A16_UNORM_To_Rgba64()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 91, // VK_FORMAT_R16G16B16A16_UNORM
            PixelWidth = 1,
            PixelHeight = 2,
        };
        var pixels = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
                                  0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rgba64, reader.Info.PixelFormat);
        Assert.Equal(64, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R32G32B32_SFLOAT_To_Rgb96Float()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 106, // VK_FORMAT_R32G32B32_SFLOAT
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[12];
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(0, 4), 1.0f);
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(4, 4), -2.5f);
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(8, 4), 1024.0f);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
        Assert.Equal(96, reader.Info.BitsPerPixel);
        Assert.Equal(3, reader.Info.ChannelCount);
        Assert.False(reader.Info.HasAlpha);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R32G32B32A32_SFLOAT_To_Rgba128Float()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 109, // VK_FORMAT_R32G32B32A32_SFLOAT
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[16];
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(0, 4), 0.1f);
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(4, 4), 0.2f);
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(8, 4), 0.3f);
        System.Buffers.Binary.BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(12, 4), 0.9f);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.Rgba128Float, reader.Info.PixelFormat);
        Assert.Equal(128, reader.Info.BitsPerPixel);
        Assert.Equal(4, reader.Info.ChannelCount);
        Assert.True(reader.Info.HasAlpha);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public async Task Decodes_VK_FORMAT_R8G8_UNORM_To_GrayAlpha16()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 16, // VK_FORMAT_R8G8_UNORM
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[] { 0x10, 0x20, 0x30, 0x40 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Equal(PixelFormat.GrayAlpha16, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        Assert.True(frame!.Pixels.Span.SequenceEqual(pixels));
    }

    [Fact]
    public void Dfd_Is_Null_When_Container_Omits_The_Section()
    {
        var b = new TestKtx2Builder
        {
            VkFormat = 37, // VK_FORMAT_R8G8B8A8_UNORM
            PixelWidth = 1,
            PixelHeight = 1,
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);
        Assert.Null(reader.Ktx2.Dfd);
    }

    [Fact]
    public void Dfd_Parsed_From_Container_Exposes_Colour_Model_And_Transfer_Function()
    {
        var dfd = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
            Flags = KhrDfdFlags.None,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        dfd.AddSample(0, 8, 0, sampleUpper: 255);
        dfd.AddSample(8, 8, 1, sampleUpper: 255);
        dfd.AddSample(16, 8, 2, sampleUpper: 255);
        // Alpha sample is forced linear (high bit 0x10).
        dfd.AddSample(24, 8, 15 | 0x10, sampleUpper: 255);

        var b = new TestKtx2Builder
        {
            VkFormat = 43, // VK_FORMAT_R8G8B8A8_SRGB
            PixelWidth = 1,
            PixelHeight = 1,
            DfdBytes = dfd.Build(),
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.NotNull(reader.Ktx2.Dfd);
        var basic = reader.Ktx2.Dfd!.Basic;
        Assert.NotNull(basic);
        Assert.Equal(KhrColorModel.Rgbsda, basic!.ColorModel);
        Assert.Equal(KhrColorPrimaries.Bt709, basic.ColorPrimaries);
        Assert.Equal(KhrTransferFunction.SRgb, basic.TransferFunction);
        Assert.Equal(4, basic.Samples.Count);
        Assert.Equal(4, basic.BytesPlanes[0]);

        // Surface DFD info to ImageMetadata.Tags so consumers using only
        // the IImageReader interface can see colour-management info.
        Assert.Equal("Rgbsda", reader.Metadata.Tags["KTX2:DFD:ColorModel"]);
        Assert.Equal("SRgb", reader.Metadata.Tags["KTX2:DFD:TransferFunction"]);
        Assert.Equal("Bt709", reader.Metadata.Tags["KTX2:DFD:ColorPrimaries"]);
        Assert.Equal("None", reader.Metadata.Tags["KTX2:DFD:Flags"]);
        Assert.Equal("4", reader.Metadata.Tags["KTX2:DFD:SampleCount"]);
        Assert.Equal("4", reader.Metadata.Tags["KTX2:DFD:BytesPerTexelBlock"]);
    }

    [Fact]
    public void Dfd_With_AlphaPremultiplied_Flag_Surfaces_In_Metadata()
    {
        var dfd = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.Linear,
            Flags = KhrDfdFlags.AlphaPremultiplied,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        dfd.AddSample(0, 8, 0); dfd.AddSample(8, 8, 1);
        dfd.AddSample(16, 8, 2); dfd.AddSample(24, 8, 15);

        var b = new TestKtx2Builder
        {
            VkFormat = 37, // VK_FORMAT_R8G8B8A8_UNORM
            PixelWidth = 1,
            PixelHeight = 1,
            DfdBytes = dfd.Build(),
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.Equal(KhrDfdFlags.AlphaPremultiplied, reader.Ktx2.Dfd!.Basic!.Flags);
        Assert.Equal("AlphaPremultiplied", reader.Metadata.Tags["KTX2:DFD:Flags"]);
    }

    [Fact]
    public void Malformed_Dfd_Does_Not_Prevent_Open()
    {
        // Section length > actual section bytes -> DfdParser returns null,
        // but the surrounding container parse must still succeed.
        var bogus = new byte[8];
        // dfdTotalSize claims 99 (way more than 8 actual bytes).
        bogus[0] = 99;

        var b = new TestKtx2Builder
        {
            VkFormat = 37,
            PixelWidth = 1,
            PixelHeight = 1,
            DfdBytes = bogus,
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.Null(reader.Ktx2.Dfd);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public void ColorSpace_Is_sRGB_When_Dfd_Marks_Bt709_SRgb()
    {
        var dfd = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
        };
        dfd.AddSample(0, 8, 0); dfd.AddSample(8, 8, 1);
        dfd.AddSample(16, 8, 2); dfd.AddSample(24, 8, 15 | 0x10);

        var b = new TestKtx2Builder
        {
            VkFormat = 43, // R8G8B8A8_SRGB
            PixelWidth = 1,
            PixelHeight = 1,
            DfdBytes = dfd.Build(),
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void ColorSpace_Falls_Back_To_SRgb_From_VkFormat_When_Dfd_Absent()
    {
        // VK_FORMAT_R8G8B8A8_SRGB without an accompanying DFD; the reader
        // must still report sRGB because the VkFormat token itself encodes it.
        var b = new TestKtx2Builder
        {
            VkFormat = 43,
            PixelWidth = 1,
            PixelHeight = 1,
        };
        b.MipPayloads.Add(new byte[4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.Null(reader.Ktx2.Dfd);
        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void ColorSpace_Is_BT2020_PQ_For_HDR_Dfd()
    {
        var dfd = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt2020,
            TransferFunction = KhrTransferFunction.PqEotf,
        };
        dfd.AddSample(0, 10, 0);

        var b = new TestKtx2Builder
        {
            VkFormat = 91, // VK_FORMAT_R16G16B16A16_UNORM (linear binary, DFD overrides)
            PixelWidth = 1,
            PixelHeight = 1,
            DfdBytes = dfd.Build(),
        };
        b.MipPayloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = Ktx2Reader.Open(ms);

        Assert.Equal("BT.2020 PQ", reader.Info.ColorSpace);
    }
}
