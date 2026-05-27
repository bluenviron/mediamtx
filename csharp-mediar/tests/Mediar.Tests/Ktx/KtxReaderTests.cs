using Mediar.Codecs.Bcn;
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
            GlInternalFormat = 0x9270, // GL_COMPRESSED_R11_EAC (ETC2)
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
}
