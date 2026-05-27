using Mediar.Imaging;
using Mediar.Imaging.Bmp;
using Mediar.Imaging.Png;
using Xunit;

namespace Mediar.Tests;

public sealed class ImagingBmpPngTests
{
    [Fact]
    public async Task Bmp_24bpp_RoundTrips()
    {
        const int W = 16, H = 8;
        // Build an RGB24 (well, we'll feed BGR24 since that's BMP-native).
        var pixels = new byte[W * H * 3];
        for (int y = 0; y < H; y++)
            for (int x = 0; x < W; x++)
            {
                int o = (y * W + x) * 3;
                pixels[o + 0] = (byte)(x * 16);     // B
                pixels[o + 1] = (byte)(y * 32);     // G
                pixels[o + 2] = (byte)(x * y);      // R
            }
        var frameIn = new ImageFrame(W, H, PixelFormat.Bgr24, W * 3, pixels);

        await using var ms = new MemoryStream();
        await using (var w = new BmpWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frameIn);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = BmpReader.Open(ms, ownsStream: false);
        Assert.Equal(W, reader.Info.Width);
        Assert.Equal(H, reader.Info.Height);
        Assert.Equal(24, reader.Info.BitsPerPixel);

        await foreach (var frame in reader.ReadFramesAsync())
        {
            using (frame)
            {
                Assert.Equal(W, frame.Width);
                Assert.Equal(H, frame.Height);
                Assert.Equal(PixelFormat.Bgr24, frame.PixelFormat);
                // Verify a single pixel survives the round-trip.
                int o = (3 * W + 5) * 3;
                var data = frame.Pixels.Span;
                Assert.Equal(pixels[o + 0], data[o + 0]);
                Assert.Equal(pixels[o + 1], data[o + 1]);
                Assert.Equal(pixels[o + 2], data[o + 2]);
            }
        }
    }

    [Fact]
    public async Task Png_Rgba32_RoundTrips()
    {
        const int W = 8, H = 4;
        var pixels = new byte[W * H * 4];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i & 0xFF);
        var frameIn = new ImageFrame(W, H, PixelFormat.Rgba32, W * 4, pixels);

        await using var ms = new MemoryStream();
        await using (var w = new PngWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frameIn);
            await w.FinishAsync();
        }

        // PNG magic
        var sigBytes = ms.ToArray();
        Assert.Equal(0x89, sigBytes[0]);
        Assert.Equal((byte)'P', sigBytes[1]);
        Assert.Equal((byte)'N', sigBytes[2]);
        Assert.Equal((byte)'G', sigBytes[3]);

        ms.Position = 0;
        using var reader = PngReader.Open(ms, ownsStream: false);
        Assert.Equal(W, reader.Info.Width);
        Assert.Equal(H, reader.Info.Height);
        Assert.Equal(ImageFormat.Png, reader.Info.Format);
        Assert.True(reader.Info.HasAlpha);

        await foreach (var f in reader.ReadFramesAsync())
        {
            using (f)
            {
                var data = f.Pixels.Span;
                for (int i = 0; i < pixels.Length; i++)
                {
                    Assert.Equal(pixels[i], data[i]);
                }
            }
        }
    }
}
