using Mediar.Imaging;
using Mediar.Imaging.Bmp;
using Mediar.Imaging.Png;
using Xunit;

namespace Mediar.Tests;

public sealed class ImagingBmpPngTests
{
    // ---------- BMP round-trips ----------

    [Fact]
    public async Task Bmp_24bpp_RoundTrips()
    {
        const int W = 16, H = 8;
        var pixels = new byte[W * H * 3];
        for (int y = 0; y < H; y++)
            for (int x = 0; x < W; x++)
            {
                int o = (y * W + x) * 3;
                pixels[o + 0] = (byte)(x * 16);
                pixels[o + 1] = (byte)(y * 32);
                pixels[o + 2] = (byte)(x * y);
            }
        var frameIn = new ImageFrame(W, H, PixelFormat.Bgr24, W * 3, pixels);

        byte[] bytes = await BmpEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = BmpReader.Open(ms, ownsStream: false);
        Assert.Equal(W, reader.Info.Width);
        Assert.Equal(H, reader.Info.Height);
        Assert.Equal(24, reader.Info.BitsPerPixel);
        Assert.False(reader.Info.HasAlpha);
        Assert.Equal(ImageFormat.Bmp, reader.Format);
        Assert.Equal(1, reader.Info.FrameCount);
        Assert.True(reader.CanDecodePixels);

        await foreach (var frame in reader.ReadFramesAsync())
        {
            using (frame)
            {
                Assert.Equal(W, frame.Width);
                Assert.Equal(H, frame.Height);
                Assert.Equal(PixelFormat.Bgr24, frame.PixelFormat);
                int o = (3 * W + 5) * 3;
                var data = frame.Pixels.Span;
                Assert.Equal(pixels[o + 0], data[o + 0]);
                Assert.Equal(pixels[o + 1], data[o + 1]);
                Assert.Equal(pixels[o + 2], data[o + 2]);
            }
        }
    }

    [Fact]
    public async Task Bmp_32bpp_RoundTrips_With_Alpha()
    {
        const int W = 4, H = 3;
        var pixels = new byte[W * H * 4];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 7);
        var frameIn = new ImageFrame(W, H, PixelFormat.Bgra32, W * 4, pixels);

        byte[] bytes = await BmpEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = BmpReader.Open(ms);
        Assert.Equal(32, reader.Info.BitsPerPixel);
        Assert.True(reader.Info.HasAlpha);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                Assert.Equal(PixelFormat.Bgra32, f.PixelFormat);
                var data = f.Pixels.Span;
                for (int i = 0; i < pixels.Length; i++) Assert.Equal(pixels[i], data[i]);
            }
        }
    }

    [Fact]
    public async Task Bmp_RoundTrip_Preserves_Bytes_Rgb24_Mirror_To_Bgr24()
    {
        const int W = 4, H = 2;
        var pixels = new byte[W * H * 3];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)i;
        // Rgb24 is mirrored to Bgr24 on write — we should round trip as Bgr24 with R/B swapped.
        var frameIn = new ImageFrame(W, H, PixelFormat.Rgb24, W * 3, pixels);

        byte[] bytes = await BmpEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = BmpReader.Open(ms);
        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                Assert.Equal(PixelFormat.Bgr24, f.PixelFormat);
                var data = f.Pixels.Span;
                // Each pixel: original RGB -> stored BGR -> we expect data[o+0]=pixels[o+2] etc.
                for (int p = 0; p < W * H; p++)
                {
                    int o = p * 3;
                    Assert.Equal(pixels[o + 2], data[o + 0]);
                    Assert.Equal(pixels[o + 1], data[o + 1]);
                    Assert.Equal(pixels[o + 0], data[o + 2]);
                }
            }
        }
    }

    [Fact]
    public async Task Bmp_Indexed8_RoundTrips_With_Palette()
    {
        const int W = 4, H = 2;
        var pixels = new byte[] { 0, 1, 2, 3, 4, 5, 6, 7 };
        var palette = new uint[256];
        for (uint i = 0; i < 256; i++) palette[i] = (0xFFu << 24) | (i << 16) | (i << 8) | i;
        var frameIn = new ImageFrame(W, H, PixelFormat.Indexed8, W, pixels, palette: palette);

        byte[] bytes = await BmpEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = BmpReader.Open(ms);
        Assert.Equal(8, reader.Info.BitsPerPixel);
        Assert.Equal(PixelFormat.Indexed8, reader.Info.PixelFormat);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                var data = f.Pixels.Span;
                Assert.Equal(8, data.Length);
                for (int i = 0; i < 8; i++) Assert.Equal(pixels[i], data[i]);
            }
        }
    }

    // ---------- BMP guards ----------

    [Fact]
    public void Bmp_Writer_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new BmpWriter(null!));
    }

    [Fact]
    public void Bmp_Writer_NonWritable_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new BmpWriter(ms));
    }

    [Fact]
    public void Bmp_Writer_Format_Is_Bmp()
    {
        using var ms = new MemoryStream();
        var w = new BmpWriter(ms, ownsStream: false);
        Assert.Equal(ImageFormat.Bmp, w.Format);
    }

    [Fact]
    public async Task Bmp_Writer_Null_Frame_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new BmpWriter(ms, ownsStream: false);
        await Assert.ThrowsAsync<ArgumentNullException>(async () => await w.WriteFrameAsync(null!));
    }

    [Fact]
    public async Task Bmp_Writer_Unsupported_PixelFormat_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new BmpWriter(ms, ownsStream: false);
        var frame = new ImageFrame(2, 2, PixelFormat.Rgb48, 2 * 6, new byte[4 * 6]);
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Bmp_Writer_Second_Frame_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new BmpWriter(ms, ownsStream: false);
        var frame = new ImageFrame(2, 2, PixelFormat.Bgr24, 2 * 3, new byte[2 * 2 * 3]);
        await w.WriteFrameAsync(frame);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Bmp_Writer_FinishAsync_NoOp()
    {
        await using var ms = new MemoryStream();
        await using var w = new BmpWriter(ms, ownsStream: false);
        await w.FinishAsync();
    }

    [Fact]
    public async Task Bmp_Writer_OwnsStream_True_Disposes_Stream()
    {
        var ms = new MemoryStream();
        await using (var w = new BmpWriter(ms, ownsStream: true))
        {
            // nothing
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public async Task Bmp_Writer_OwnsStream_False_Leaves_Stream()
    {
        var ms = new MemoryStream();
        await using (var w = new BmpWriter(ms, ownsStream: false))
        {
            // nothing
        }
        _ = ms.Length;
        ms.Dispose();
    }

    [Fact]
    public void Bmp_Reader_Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => BmpReader.Open((Stream)null!));
    }

    [Fact]
    public void Bmp_Reader_Open_Missing_Magic_Throws()
    {
        var bytes = new byte[40];
        bytes[0] = (byte)'X'; bytes[1] = (byte)'X';
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => BmpReader.Open(ms));
    }

    [Fact]
    public void Bmp_Reader_Open_Missing_Path_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-bmp-{Guid.NewGuid():N}.bmp");
        Assert.Throws<FileNotFoundException>(() => BmpReader.Open(path));
    }

    [Fact]
    public async Task Bmp_Reader_Format_Is_Dib_When_Flag_Set()
    {
        const int W = 4, H = 2;
        var pixels = new byte[W * H * 3];
        var frame = new ImageFrame(W, H, PixelFormat.Bgr24, W * 3, pixels);
        byte[] bytes = await BmpEncodeAsync(frame);
        // Strip the 14-byte file header to make it a DIB.
        byte[] dib = bytes[14..];
        await using var ms = new MemoryStream(dib);
        using var reader = BmpReader.Open(ms, isDib: true);
        Assert.Equal(ImageFormat.Dib, reader.Format);
    }

    // ---------- PNG round-trips ----------

    [Fact]
    public async Task Png_Rgba32_RoundTrips()
    {
        const int W = 8, H = 4;
        var pixels = new byte[W * H * 4];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i & 0xFF);
        var frameIn = new ImageFrame(W, H, PixelFormat.Rgba32, W * 4, pixels);

        byte[] bytes = await PngEncodeAsync(frameIn);

        Assert.Equal(0x89, bytes[0]);
        Assert.Equal((byte)'P', bytes[1]);
        Assert.Equal((byte)'N', bytes[2]);
        Assert.Equal((byte)'G', bytes[3]);

        await using var ms = new MemoryStream(bytes);
        using var reader = PngReader.Open(ms, ownsStream: false);
        Assert.Equal(W, reader.Info.Width);
        Assert.Equal(H, reader.Info.Height);
        Assert.Equal(ImageFormat.Png, reader.Info.Format);
        Assert.True(reader.Info.HasAlpha);
        Assert.True(reader.CanDecodePixels);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                var data = f.Pixels.Span;
                for (int i = 0; i < pixels.Length; i++) Assert.Equal(pixels[i], data[i]);
            }
        }
    }

    [Fact]
    public async Task Png_Rgb24_RoundTrips_Without_Alpha()
    {
        const int W = 4, H = 4;
        var pixels = new byte[W * H * 3];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 3);
        var frameIn = new ImageFrame(W, H, PixelFormat.Rgb24, W * 3, pixels);

        byte[] bytes = await PngEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = PngReader.Open(ms);
        Assert.False(reader.Info.HasAlpha);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                var data = f.Pixels.Span;
                for (int i = 0; i < pixels.Length; i++) Assert.Equal(pixels[i], data[i]);
            }
        }
    }

    [Fact]
    public async Task Png_Gray8_RoundTrips()
    {
        const int W = 4, H = 4;
        var pixels = new byte[W * H];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 11);
        var frameIn = new ImageFrame(W, H, PixelFormat.Gray8, W, pixels);

        byte[] bytes = await PngEncodeAsync(frameIn);
        await using var ms = new MemoryStream(bytes);
        using var reader = PngReader.Open(ms);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                var data = f.Pixels.Span;
                Assert.Equal(W * H, data.Length);
                for (int i = 0; i < pixels.Length; i++) Assert.Equal(pixels[i], data[i]);
            }
        }
    }

    // ---------- PNG guards ----------

    [Fact]
    public void Png_Writer_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new PngWriter(null!));
    }

    [Fact]
    public void Png_Writer_NonWritable_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new PngWriter(ms));
    }

    [Fact]
    public void Png_Writer_Format_Is_Png()
    {
        using var ms = new MemoryStream();
        var w = new PngWriter(ms, ownsStream: false);
        Assert.Equal(ImageFormat.Png, w.Format);
    }

    [Fact]
    public async Task Png_Writer_Null_Frame_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new PngWriter(ms, ownsStream: false);
        await Assert.ThrowsAsync<ArgumentNullException>(async () => await w.WriteFrameAsync(null!));
    }

    [Fact]
    public async Task Png_Writer_Second_Frame_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new PngWriter(ms, ownsStream: false);
        var frame = new ImageFrame(2, 2, PixelFormat.Gray8, 2, new byte[4]);
        await w.WriteFrameAsync(frame);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Png_Writer_Unsupported_PixelFormat_Throws()
    {
        await using var ms = new MemoryStream();
        await using var w = new PngWriter(ms, ownsStream: false);
        var frame = new ImageFrame(2, 2, PixelFormat.Bgra32, 2 * 4, new byte[4 * 4]);
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Png_Writer_OwnsStream_True_Disposes_Stream()
    {
        var ms = new MemoryStream();
        await using (var w = new PngWriter(ms, ownsStream: true))
        {
            // nothing
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public void Png_Reader_Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => PngReader.Open((Stream)null!));
    }

    [Fact]
    public void Png_Reader_Open_Wrong_Signature_Throws()
    {
        var bytes = new byte[16];
        bytes[0] = 0x88; bytes[1] = (byte)'X';
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => PngReader.Open(ms));
    }

    [Fact]
    public void Png_Reader_Open_Missing_Path_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-png-{Guid.NewGuid():N}.png");
        Assert.Throws<FileNotFoundException>(() => PngReader.Open(path));
    }

    // ---------- helpers ----------

    private static async Task<byte[]> BmpEncodeAsync(ImageFrame frame)
    {
        await using var ms = new MemoryStream();
        await using (var w = new BmpWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        return ms.ToArray();
    }

    private static async Task<byte[]> PngEncodeAsync(ImageFrame frame)
    {
        await using var ms = new MemoryStream();
        await using (var w = new PngWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        return ms.ToArray();
    }
}
