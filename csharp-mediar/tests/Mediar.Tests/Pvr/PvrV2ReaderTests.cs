using Mediar.Imaging;
using Mediar.Imaging.Pvr;
using Xunit;

namespace Mediar.Tests.Pvr;

public class PvrV2ReaderTests
{
    [Fact]
    public void Rejects_Truncated_Header()
    {
        using var ms = new MemoryStream(new byte[8]);
        Assert.Throws<ImageFormatException>(() => PvrV2Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_Magic()
    {
        var b = new TestPvrV2Builder
        {
            FormatId = PvrV2FormatId.Argb8888,
            Magic = 0xDEADBEEFu,
        };
        b.Payloads.Add(new byte[4 * 4 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => PvrV2Reader.Open(ms));
        Assert.Contains("PVR v2", ex.Message);
    }

    [Fact]
    public void Rejects_Zero_Width_Or_Height()
    {
        var b = new TestPvrV2Builder { Width = 0 };
        b.Payloads.Add([]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PvrV2Reader.Open(ms));
    }

    [Fact]
    public void Parses_V2_Header_Fields()
    {
        var b = new TestPvrV2Builder
        {
            Width = 8,
            Height = 4,
            FormatId = PvrV2FormatId.Argb8888,
            Flags = PvrV2Flags.HasMipmaps | PvrV2Flags.PremultipliedAlpha,
            MipMapCount = 2,
            BitsPerPixel = 32,
            NumSurfaces = 1,
            DataLength = 8 * 4 * 4,
        };
        b.Payloads.Add(new byte[8 * 4 * 4]);
        b.Payloads.Add(new byte[4 * 2 * 4]);
        b.Payloads.Add(new byte[2 * 1 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.Equal(52u, reader.Pvr.HeaderSize);
        Assert.Equal(8u, reader.Pvr.Width);
        Assert.Equal(4u, reader.Pvr.Height);
        Assert.Equal(PvrV2FormatId.Argb8888, reader.Pvr.FormatId);
        Assert.True((reader.Pvr.Flags & PvrV2Flags.HasMipmaps) != 0);
        Assert.True((reader.Pvr.Flags & PvrV2Flags.PremultipliedAlpha) != 0);
        Assert.Equal(2u, reader.Pvr.MipMapCount);
        Assert.Equal(0x21525650u, reader.Pvr.Magic);
    }

    [Fact]
    public void Detects_Bc1_From_Dxt1_FormatId()
    {
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 4,
            FormatId = PvrV2FormatId.Dxt1,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PvrV2FormatId.Dxt1, reader.Pvr.FormatId);
    }

    [Fact]
    public void Detects_Bc3_From_Dxt5_FormatId()
    {
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 4,
            FormatId = PvrV2FormatId.Dxt5,
        };
        b.Payloads.Add(new byte[16]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public void Detects_Etc1_From_Gl_FormatId()
    {
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 4,
            FormatId = PvrV2FormatId.GlEtc1,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFrames_Uncompressed_RGBA8888_Round_Trips()
    {
        var pixels = new byte[4 * 2 * 4];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i & 0xFF);
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 2,
            FormatId = PvrV2FormatId.GlRgba8888,
        };
        b.Payloads.Add(pixels);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        Assert.Equal(4, frame!.Width);
        Assert.Equal(2, frame.Height);
        Assert.Equal(PixelFormat.Rgba32, frame.PixelFormat);
        Assert.Equal(pixels, frame.Pixels);
    }

    [Fact]
    public void Walks_MipMaps_When_Flag_Set()
    {
        var b = new TestPvrV2Builder
        {
            Width = 8,
            Height = 8,
            FormatId = PvrV2FormatId.GlRgba8888,
            Flags = PvrV2Flags.HasMipmaps,
            MipMapCount = 3, // 8x8, 4x4, 2x2, 1x1
        };
        b.Payloads.Add(new byte[8 * 8 * 4]);
        b.Payloads.Add(new byte[4 * 4 * 4]);
        b.Payloads.Add(new byte[2 * 2 * 4]);
        b.Payloads.Add(new byte[1 * 1 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.Equal(4, reader.Levels.Count);
        Assert.Equal(8, reader.Levels[0].Width);
        Assert.Equal(4, reader.Levels[1].Width);
        Assert.Equal(2, reader.Levels[2].Width);
        Assert.Equal(1, reader.Levels[3].Width);
    }

    [Fact]
    public void Cubemap_Flag_Produces_Six_Surfaces()
    {
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 4,
            FormatId = PvrV2FormatId.GlRgba8888,
            Flags = PvrV2Flags.Cubemap,
        };
        for (int i = 0; i < 6; i++) b.Payloads.Add(new byte[4 * 4 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.Equal(6, reader.Levels.Count);
        for (int i = 0; i < 6; i++)
        {
            Assert.Equal(i, reader.Levels[i].Surface);
            Assert.Equal(0, reader.Levels[i].Level);
        }
    }

    [Fact]
    public async Task Pvrtc_Surfaced_As_Undecodable()
    {
        var b = new TestPvrV2Builder
        {
            Width = 8,
            Height = 8,
            FormatId = PvrV2FormatId.Pvrtc4,
        };
        b.Payloads.Add(new byte[(8 / 4) * (8 / 4) * 8]); // 4 blocks, 8 bytes each
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        Assert.False(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync()) { }
        });
    }

    [Fact]
    public void Detector_Recognises_V2_Magic()
    {
        var b = new TestPvrV2Builder { FormatId = PvrV2FormatId.GlRgba8888 };
        b.Payloads.Add(new byte[4 * 4 * 4]);
        var bytes = b.Build();
        Assert.Equal(ImageFormat.Pvr, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void Rejects_Payload_Exceeding_File()
    {
        var b = new TestPvrV2Builder
        {
            Width = 64,
            Height = 64,
            FormatId = PvrV2FormatId.GlRgba8888,
        };
        b.Payloads.Add(new byte[8]); // way too small
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PvrV2Reader.Open(ms));
    }

    [Fact]
    public async Task ReadFrames_Dxt1_Decodes_Bcn_To_Bgra32()
    {
        var b = new TestPvrV2Builder
        {
            Width = 4,
            Height = 4,
            FormatId = PvrV2FormatId.Dxt1,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrV2Reader.Open(ms);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Bgra32, frame!.PixelFormat);
        Assert.Equal(4 * 4 * 4, frame.Pixels.Length);
    }
}
