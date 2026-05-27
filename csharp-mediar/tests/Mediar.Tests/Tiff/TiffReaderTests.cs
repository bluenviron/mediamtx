using Mediar.Imaging;
using Mediar.Imaging.Tiff;
using Xunit;

namespace Mediar.Tests.Tiff;

/// <summary>
/// Tests for the strip and tile decode paths in <see cref="TiffReader"/>,
/// including JPEG-in-TIFF (compression 7) for both strip and tile layouts.
/// </summary>
public sealed class TiffReaderTests
{
    // Tiny 16×16 solid-red baseline JPEG re-used from JpegBaselineDecoderTests.
    // It contains a complete self-contained baseline-DCT bitstream (SOF0).
    private const string RedJpegBase64 =
        "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAQCAwMDAgQDAwMEBAQEBQkGBQUFBQsICAYJDQsNDQ0LDAwOEBQRDg8TDwwMEhgSExUWFxcXDhEZGxkWGhQWFxb/" +
        "2wBDAQQEBAUFBQoGBgoWDwwPFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhb/wAARCAAQABADASIAAhEBAxEB/8QA" +
        "HwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2Jyggk" +
        "KFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMX" +
        "Gx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAEC" +
        "AxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOE" +
        "hYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxeiiivyk/" +
        "v4//2Q==";

    [Fact]
    public async Task UncompressedRgbStrip_Decodes_ExactPixels()
    {
        // 4×4 RGB image, one strip, uncompressed. Pixel value = (row*16+col, 0, 0).
        var payload = new byte[4 * 4 * 3];
        for (int y = 0; y < 4; y++)
        {
            for (int x = 0; x < 4; x++)
            {
                int o = (y * 4 + x) * 3;
                payload[o] = (byte)(y * 16 + x);
                payload[o + 1] = 0;
                payload[o + 2] = 0;
            }
        }
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, RowsPerStrip = 4,
            StripPayloads = [payload],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(ImageFormat.Tiff, r.Format);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(4, r.Info.Height);
        Assert.Equal(PixelFormat.Rgb24, r.Info.PixelFormat);
        Assert.True(r.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(payload, captured!.Pixels.ToArray());
        }
    }

    [Fact]
    public async Task TiledUncompressedRgb_4x4_TwoByTwoTiles_Decodes()
    {
        // 4×4 image with 2×2 tiles → 2 tiles across × 2 tiles down = 4 tiles total.
        // Each tile is 2×2×3 = 12 bytes. Fill each tile with a distinct solid color.
        byte[] tile00 = BuildSolidRgbTile(2, 2, 255, 0, 0);   // red   – top-left
        byte[] tile01 = BuildSolidRgbTile(2, 2, 0, 255, 0);   // green – top-right
        byte[] tile10 = BuildSolidRgbTile(2, 2, 0, 0, 255);   // blue  – bottom-left
        byte[] tile11 = BuildSolidRgbTile(2, 2, 255, 255, 0); // yellow – bottom-right
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2,
            TileWidth = 2, TileHeight = 2,
            TilePayloads = [tile00, tile01, tile10, tile11],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(4, r.Info.Height);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            var px = captured!.Pixels.Span;
            int stride = 4 * 3;
            // Top-left pixel = red, top-right pixel = green, bottom-left = blue, bottom-right = yellow.
            AssertRgb(px, 0, 0, stride, 255, 0, 0);
            AssertRgb(px, 3, 0, stride, 0, 255, 0);
            AssertRgb(px, 0, 3, stride, 0, 0, 255);
            AssertRgb(px, 3, 3, stride, 255, 255, 0);
            // Spot-check an interior pixel of each tile.
            AssertRgb(px, 1, 1, stride, 255, 0, 0);
            AssertRgb(px, 2, 1, stride, 0, 255, 0);
            AssertRgb(px, 1, 2, stride, 0, 0, 255);
            AssertRgb(px, 2, 2, stride, 255, 255, 0);
        }
    }

    [Fact]
    public async Task TiledUncompressed_WithEdgeClipping_DoesNotOverrun()
    {
        // 6×6 image with 4×4 tiles → 2×2 = 4 tiles. Right + bottom edge tiles
        // are partial (only 2 valid columns/rows out of the 4 tile dimension).
        byte[] tile00 = BuildSolidRgbTile(4, 4, 10, 20, 30);
        byte[] tile01 = BuildSolidRgbTile(4, 4, 40, 50, 60);
        byte[] tile10 = BuildSolidRgbTile(4, 4, 70, 80, 90);
        byte[] tile11 = BuildSolidRgbTile(4, 4, 100, 110, 120);
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 6, Height = 6, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2,
            TileWidth = 4, TileHeight = 4,
            TilePayloads = [tile00, tile01, tile10, tile11],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            var px = captured!.Pixels.Span;
            int stride = 6 * 3;
            Assert.Equal(6 * stride, px.Length);
            // Tile (0,0) covers (0,0)..(3,3).
            AssertRgb(px, 0, 0, stride, 10, 20, 30);
            AssertRgb(px, 3, 3, stride, 10, 20, 30);
            // Tile (0,1) covers cols 4..5 in rows 0..3 (only 2 visible cols).
            AssertRgb(px, 4, 0, stride, 40, 50, 60);
            AssertRgb(px, 5, 3, stride, 40, 50, 60);
            // Tile (1,0) covers rows 4..5 in cols 0..3 (only 2 visible rows).
            AssertRgb(px, 0, 4, stride, 70, 80, 90);
            AssertRgb(px, 3, 5, stride, 70, 80, 90);
            // Tile (1,1) covers the 2×2 corner.
            AssertRgb(px, 4, 4, stride, 100, 110, 120);
            AssertRgb(px, 5, 5, stride, 100, 110, 120);
        }
    }

    [Fact]
    public async Task JpegInTiff_SingleStrip_RedDominantDecode()
    {
        byte[] jpeg = Convert.FromBase64String(RedJpegBase64);
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 16, Height = 16, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 7, Photometric = 2, RowsPerStrip = 16,
            StripPayloads = [jpeg],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);
        Assert.Equal(16, r.Info.Width);
        Assert.Equal(16, r.Info.Height);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            AssertRedDominantRgb24(captured!.Pixels.Span);
        }
    }

    [Fact]
    public async Task JpegInTiff_SingleTile_RedDominantDecode()
    {
        byte[] jpeg = Convert.FromBase64String(RedJpegBase64);
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 16, Height = 16, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 7, Photometric = 2,
            TileWidth = 16, TileHeight = 16,
            TilePayloads = [jpeg],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            AssertRedDominantRgb24(captured!.Pixels.Span);
        }
    }

    [Fact]
    public async Task JpegInTiff_FourTiles_MosaicedDecode()
    {
        // 32×32 image with four 16×16 JPEG tiles. Each tile is the same red
        // JPEG; the resulting 32×32 image should still be red-dominant
        // everywhere, and crucially exercise the multi-tile JPEG dispatch.
        byte[] jpeg = Convert.FromBase64String(RedJpegBase64);
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 32, Height = 32, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 7, Photometric = 2,
            TileWidth = 16, TileHeight = 16,
            TilePayloads = [jpeg, jpeg, jpeg, jpeg],
        });

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);
        Assert.Equal(32, r.Info.Width);
        Assert.Equal(32, r.Info.Height);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            var px = captured!.Pixels.Span;
            Assert.Equal(32 * 32 * 3, px.Length);
            AssertRedDominantRgb24(px);
        }
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        Assert.Throws<ImageFormatException>(() =>
            TiffReader.Open(new MemoryStream(new byte[4]), ownsStream: true));
    }

    [Fact]
    public async Task MultiPageTiff_TwoPages_YieldsBothFramesAndPagesList()
    {
        // Page 0: 2x2 RGB red. Page 1: 4x4 RGB green. Both uncompressed.
        var page0 = new TestTiffBuilder.TiffSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, RowsPerStrip = 2,
            StripPayloads = [BuildSolidRgbTile(2, 2, 255, 0, 0)],
        };
        var page1 = new TestTiffBuilder.TiffSpec
        {
            Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, RowsPerStrip = 4,
            StripPayloads = [BuildSolidRgbTile(4, 4, 0, 255, 0)],
        };
        var bytes = TestTiffBuilder.Build([page0, page1]);

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(2, r.Pages.Count);
        Assert.Equal(2, r.Info.FrameCount);
        Assert.Equal(2, r.Pages[0].Width); Assert.Equal(2, r.Pages[0].Height);
        Assert.Equal(4, r.Pages[1].Width); Assert.Equal(4, r.Pages[1].Height);
        Assert.True(r.Pages[0].CanDecodePixels);
        Assert.True(r.Pages[1].CanDecodePixels);

        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync()) frames.Add(f);
        Assert.Equal(2, frames.Count);
        try
        {
            Assert.Equal(2, frames[0].Width); Assert.Equal(2, frames[0].Height);
            AssertRgb(frames[0].Pixels.Span, 0, 0, 2 * 3, 255, 0, 0);
            Assert.Equal(4, frames[1].Width); Assert.Equal(4, frames[1].Height);
            AssertRgb(frames[1].Pixels.Span, 0, 0, 4 * 3, 0, 255, 0);
            AssertRgb(frames[1].Pixels.Span, 3, 3, 4 * 3, 0, 255, 0);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public async Task MultiPageTiff_MixesStripAndTilePages()
    {
        // Page 0: stripped 4x4 red. Page 1: tiled 4x4 (2x2 tiles) blue.
        var stripped = new TestTiffBuilder.TiffSpec
        {
            Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, RowsPerStrip = 4,
            StripPayloads = [BuildSolidRgbTile(4, 4, 255, 0, 0)],
        };
        var tiled = new TestTiffBuilder.TiffSpec
        {
            Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2,
            TileWidth = 2, TileHeight = 2,
            TilePayloads = [
                BuildSolidRgbTile(2, 2, 0, 0, 255),
                BuildSolidRgbTile(2, 2, 0, 0, 255),
                BuildSolidRgbTile(2, 2, 0, 0, 255),
                BuildSolidRgbTile(2, 2, 0, 0, 255),
            ],
        };
        var bytes = TestTiffBuilder.Build([stripped, tiled]);

        using var r = TiffReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(2, r.Pages.Count);
        Assert.False(r.Pages[0].IsTiled);
        Assert.True(r.Pages[1].IsTiled);

        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync()) frames.Add(f);
        try
        {
            Assert.Equal(2, frames.Count);
            AssertRgb(frames[0].Pixels.Span, 0, 0, 12, 255, 0, 0);
            AssertRgb(frames[1].Pixels.Span, 0, 0, 12, 0, 0, 255);
            AssertRgb(frames[1].Pixels.Span, 3, 3, 12, 0, 0, 255);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    private static byte[] BuildSolidRgbTile(int w, int h, byte r, byte g, byte b)
    {
        var buf = new byte[w * h * 3];
        for (int i = 0; i < buf.Length; i += 3)
        {
            buf[i] = r; buf[i + 1] = g; buf[i + 2] = b;
        }
        return buf;
    }

    private static void AssertRgb(ReadOnlySpan<byte> px, int x, int y, int stride,
                                  byte r, byte g, byte b)
    {
        int o = y * stride + x * 3;
        Assert.Equal(r, px[o]);
        Assert.Equal(g, px[o + 1]);
        Assert.Equal(b, px[o + 2]);
    }

    private static void AssertRedDominantRgb24(ReadOnlySpan<byte> px)
    {
        long rSum = 0, gSum = 0, bSum = 0;
        for (int i = 0; i + 2 < px.Length; i += 3)
        {
            rSum += px[i]; gSum += px[i + 1]; bSum += px[i + 2];
        }
        int n = px.Length / 3;
        int rAvg = (int)(rSum / n);
        int gAvg = (int)(gSum / n);
        int bAvg = (int)(bSum / n);
        Assert.True(rAvg > 180, $"expected red-dominant pixel, got R={rAvg} G={gAvg} B={bAvg}");
        Assert.True(gAvg < 40, $"expected low green, got R={rAvg} G={gAvg} B={bAvg}");
        Assert.True(bAvg < 40, $"expected low blue, got R={rAvg} G={gAvg} B={bAvg}");
    }
}
