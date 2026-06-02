using System.Buffers.Binary;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;
using Mediar.Imaging;
using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Tests for <see cref="KtxReader"/> (KTX 1.x), covering Khronos identifier
/// validation, endianness handling, GL internal-format mapping to BCn,
/// uncompressed pixel decode, mip pyramid enumeration, and key-value
/// metadata pool parsing.
/// </summary>
public sealed class KtxReaderTests
{
    [Fact]
    public void Rejects_Truncated_File()
    {
        var tiny = new byte[10];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => KtxReader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_Identifier()
    {
        var bytes = new byte[128];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => KtxReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Endianness_Field()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058, // GL_RGBA8
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[4 * 4 * 4]);
        var bytes = b.Build();
        // Overwrite endianness field with garbage.
        bytes[12] = 0xDE; bytes[13] = 0xAD; bytes[14] = 0xBE; bytes[15] = 0xEF;
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => KtxReader.Open(ms));
    }

    [Fact]
    public void Parses_Uncompressed_RGBA8_Single_Mip()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058, // GL_RGBA8
            GlBaseInternalFormat = 0x1908, // GL_RGBA
            PixelWidth = 4,
            PixelHeight = 4,
        };
        var payload = new byte[4 * 4 * 4];
        for (int i = 0; i < payload.Length; i += 4)
        {
            payload[i + 0] = 0xFF; payload[i + 1] = 0x00;
            payload[i + 2] = 0x00; payload[i + 3] = 0xFF;
        }
        b.MipPayloads.Add(payload);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(4, reader.Info.Width);
        Assert.Equal(4, reader.Info.Height);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.Single(reader.Levels);
        Assert.Equal(BcnFormat.None, reader.Ktx.Bcn);
    }

    [Fact]
    public async Task ReadFrames_Uncompressed_RGBA8_Round_Trips_Pixels()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058,
            GlBaseInternalFormat = 0x1908,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        var payload = new byte[] {
            0xAA, 0xBB, 0xCC, 0xDD,
            0x11, 0x22, 0x33, 0x44,
            0x55, 0x66, 0x77, 0x88,
            0x99, 0xAA, 0xBB, 0xCC,
        };
        b.MipPayloads.Add(payload);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);

        int frames = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            frames++;
            Assert.Equal(2, frame.Width);
            Assert.Equal(2, frame.Height);
            Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
            Assert.Equal(0xAA, frame.Pixels.Span[0]);
            Assert.Equal(0xBB, frame.Pixels.Span[14]);
        }
        Assert.Equal(1, frames);
    }

    [Fact]
    public void Detects_Bc1_From_DXT1_GL_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x83F1, // GL_COMPRESSED_RGBA_S3TC_DXT1_EXT
            GlBaseInternalFormat = 0x1908,
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]); // 1 block of BC1
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(BcnFormat.Bc1, reader.Ktx.Bcn);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);
    }

    [Fact]
    public void Detects_Bc7_From_BPTC_GL_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8E8C, // GL_COMPRESSED_RGBA_BPTC_UNORM
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // 1 block of BC7
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(BcnFormat.Bc7, reader.Ktx.Bcn);
    }

    [Fact]
    public void Walks_Multiple_Mip_Levels()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058,
            GlBaseInternalFormat = 0x1908,
            PixelWidth = 4,
            PixelHeight = 4,
            MipLevels = 3,
        };
        b.MipPayloads.Add(new byte[4 * 4 * 4]); // 4x4
        b.MipPayloads.Add(new byte[2 * 2 * 4]); // 2x2
        b.MipPayloads.Add(new byte[1 * 1 * 4]); // 1x1
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(3, reader.Levels.Count);
        Assert.Equal((4, 4), (reader.Levels[0].Width, reader.Levels[0].Height));
        Assert.Equal((2, 2), (reader.Levels[1].Width, reader.Levels[1].Height));
        Assert.Equal((1, 1), (reader.Levels[2].Width, reader.Levels[2].Height));
    }

    [Fact]
    public void Key_Value_Pool_Is_Parsed()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058,
            GlBaseInternalFormat = 0x1908,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        b.KeyValues.Add(new("KTXorientation", "S=r,T=d"));
        b.KeyValues.Add(new("KTXwriter", "Mediar 0.1"));
        b.MipPayloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal("S=r,T=d", reader.Ktx.KeyValues["KTXorientation"]);
        Assert.Equal("Mediar 0.1", reader.Ktx.KeyValues["KTXwriter"]);
    }

    [Fact]
    public void Unsupported_Format_Is_Surfaced_As_Undecodable()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x93B0, // GL_COMPRESSED_RGBA_ASTC_4x4 (ASTC, undecodable)
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.False(reader.CanDecodePixels);
        Assert.Equal(BcnFormat.None, reader.Ktx.Bcn);
    }

    [Fact]
    public void Detector_Recognises_Ktx_Identifier()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058,
            PixelWidth = 2,
            PixelHeight = 2,
        };
        b.MipPayloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        Assert.Equal(ImageFormat.Ktx, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void Detects_Etc1_From_Gl_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8D64, // GL_ETC1_RGB8_OES
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]); // one 4x4 ETC1 block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.Etc1Rgb, reader.Ktx.Etc);
        Assert.Equal(BcnFormat.None, reader.Ktx.Bcn);
    }

    [Fact]
    public void Detects_Etc2_Rgba8_From_Gl_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x9278, // GL_COMPRESSED_RGBA8_ETC2_EAC
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // one 4x4 ETC2 RGBA8 block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.Etc2Rgba8, reader.Ktx.Etc);
    }

    [Fact]
    public async Task ReadFrames_Etc1_Yields_Decoded_Rgba32()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8D64, // GL_ETC1_RGB8_OES
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]); // all-zero ETC1 block
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(4, frame.Width);
            Assert.Equal(4, frame.Height);
            Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
            Assert.Equal(4 * 4 * 4, frame.Pixels.Length);
            // All-zero ETC1 block -> opaque (2,2,2,255) per pixel.
            for (int i = 0; i < 16; i++)
            {
                Assert.Equal(2, frame.Pixels.Span[i * 4 + 0]);
                Assert.Equal(255, frame.Pixels.Span[i * 4 + 3]);
            }
        }
    }

    [Fact]
    public async Task ReadFrames_EacRg11Unorm_Yields_Decoded_Rg32()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x9272, // GL_COMPRESSED_RG11_EAC
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]); // one 4x4 EAC RG11 block (16 bytes)
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.EacRg11Unorm, reader.Ktx.Etc);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.Equal(4, frame.Width);
            Assert.Equal(4, frame.Height);
            Assert.Equal(PixelFormat.Rg32, frame.PixelFormat);
            Assert.Equal(4 * 4 * 4, frame.Pixels.Length);
        }
    }

    [Fact]
    public async Task Decodes_GL_LUMINANCE16_To_Gray16()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8042, // GL_LUMINANCE16
            PixelWidth = 2,
            PixelHeight = 2,
        };
        var pixels = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Gray16, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_LUMINANCE8_ALPHA8_To_GrayAlpha16()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8045, // GL_LUMINANCE8_ALPHA8
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[] { 0x10, 0x20, 0x30, 0x40 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.GrayAlpha16, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_RGB16_To_Rgb48()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8054, // GL_RGB16
            PixelWidth = 2,
            PixelHeight = 1,
        };
        // 2 pixels * 6 bytes each
        var pixels = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgb48, reader.Info.PixelFormat);
        Assert.Equal(48, reader.Info.BitsPerPixel);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_RGBA16_To_Rgba64()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x805B, // GL_RGBA16
            PixelWidth = 1,
            PixelHeight = 2,
        };
        // 2 pixels * 8 bytes each
        var pixels = new byte[] { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
                                  0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18 };
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba64, reader.Info.PixelFormat);
        Assert.Equal(64, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_RGB32F_To_Rgb96Float()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8815, // GL_RGB32F
            PixelWidth = 2,
            PixelHeight = 1,
        };
        // 2 pixels * 12 bytes each = 24
        var pixels = new byte[2 * 12];
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(0, 4), 1.0f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(4, 4), 2.5f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(8, 4), -7.25f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(12, 4), 100.0f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(16, 4), 0.0f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(20, 4), -0.5f);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
        Assert.Equal(96, reader.Info.BitsPerPixel);
        Assert.Equal(3, reader.Info.ChannelCount);
        Assert.False(reader.Info.HasAlpha);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_RGBA32F_To_Rgba128Float()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8814, // GL_RGBA32F
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[16];
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(0, 4), 0.25f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(4, 4), 0.5f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(8, 4), 0.75f);
        BinaryPrimitives.WriteSingleLittleEndian(pixels.AsSpan(12, 4), 1.0f);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba128Float, reader.Info.PixelFormat);
        Assert.Equal(128, reader.Info.BitsPerPixel);
        Assert.Equal(4, reader.Info.ChannelCount);
        Assert.True(reader.Info.HasAlpha);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public async Task Decodes_GL_RGBA16F_To_Rgba64Float()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x881A, // GL_RGBA16F
            PixelWidth = 1,
            PixelHeight = 1,
        };
        var pixels = new byte[8];
        BinaryPrimitives.WriteHalfLittleEndian(pixels.AsSpan(0, 2), (Half)1.0f);
        BinaryPrimitives.WriteHalfLittleEndian(pixels.AsSpan(2, 2), (Half)0.5f);
        BinaryPrimitives.WriteHalfLittleEndian(pixels.AsSpan(4, 2), (Half)0.25f);
        BinaryPrimitives.WriteHalfLittleEndian(pixels.AsSpan(6, 2), (Half)1.0f);
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Rgba64Float, reader.Info.PixelFormat);
        Assert.Equal(64, reader.Info.BitsPerPixel);
        Assert.Equal(4, reader.Info.ChannelCount);
        Assert.True(reader.Info.HasAlpha);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            Assert.True(frame.Pixels.Span.SequenceEqual(pixels));
        }
    }

    [Fact]
    public void Decodes_GL_R16F_To_Gray16Float()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x822D, // GL_R16F
            PixelWidth = 2,
            PixelHeight = 1,
        };
        var pixels = new byte[4];
        b.MipPayloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal(PixelFormat.Gray16Float, reader.Info.PixelFormat);
        Assert.Equal(16, reader.Info.BitsPerPixel);
        Assert.Equal(1, reader.Info.ChannelCount);
    }

    [Fact]
    public void ColorSpace_Is_sRGB_For_GL_SRGB8_ALPHA8_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8C43, // GL_SRGB8_ALPHA8
            PixelWidth = 1,
            PixelHeight = 1,
        };
        b.MipPayloads.Add(new byte[] { 0xFF, 0x80, 0x40, 0xFF });
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void ColorSpace_Is_BCn_For_Linear_BCn_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x83F1, // GL_COMPRESSED_RGBA_S3TC_DXT1_EXT (linear BC1)
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal("BCn:Bc1", reader.Info.ColorSpace);
    }

    [Fact]
    public void ColorSpace_Is_sRGB_For_Compressed_SRgb_BPTC_Token()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8E8D, // GL_COMPRESSED_SRGB_ALPHA_BPTC_UNORM (BC7 sRGB)
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[16]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = KtxReader.Open(ms);
        Assert.Equal("sRGB", reader.Info.ColorSpace);
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => KtxReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => KtxReader.Open((string)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] bytes = MinimalKtx();
        var inner = new MemoryStream(bytes, writable: false);
        using (var r = KtxReader.Open(inner, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Ktx, r.Format);
        }
        Assert.False(inner.CanRead);
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = MinimalKtx();
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = KtxReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Ktx, r.Format);
        }
        Assert.True(ms.CanRead);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = MinimalKtx();
        var r = KtxReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Info_Format_Equals_Ktx()
    {
        byte[] bytes = MinimalKtx();
        using var r = KtxReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Ktx, r.Info.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = MinimalKtx();
        using var r = KtxReader.Open(new MemoryStream(bytes, writable: false));
        if (!r.CanDecodePixels) return;
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    private static byte[] MinimalKtx()
    {
        var b = new TestKtxBuilder
        {
            GlInternalFormat = 0x8058, // GL_RGBA8
            GlBaseInternalFormat = 0x1908, // GL_RGBA
            PixelWidth = 4,
            PixelHeight = 4,
        };
        b.MipPayloads.Add(new byte[4 * 4 * 4]);
        return b.Build();
    }
}
