using Mediar.Imaging;
using Mediar.Imaging.Cr3;
using Xunit;

namespace Mediar.Tests.Cr3;

/// <summary>
/// Tests for <see cref="Cr3Reader"/>, covering ftyp brand validation,
/// Canon UUID box discovery, CMT1 TIFF IFD parsing, THMB / PRVW JPEG
/// sub-image extraction and end-to-end JPEG decode delegation.
/// </summary>
public sealed class Cr3ReaderTests
{
    // Tiny 16x16 solid-red baseline JPEG fixture (same as other RAW tests).
    private const string RedJpegBase64 =
        "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAQCAwMDAgQDAwMEBAQEBQkGBQUFBQsICAYJDQsNDQ0LDAwOEBQRDg8TDwwMEhgSExUWFxcXDhEZGxkWGhQWFxb/" +
        "2wBDAQQEBAUFBQoGBgoWDwwPFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhb/wAARCAAQABADASIAAhEBAxEB/8QA" +
        "HwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2Jyggk" +
        "KFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMX" +
        "Gx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAEC" +
        "AxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOE" +
        "hYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxeiiivyk/" +
        "v4//2Q==";

    private static byte[] LoadRedJpeg() => Convert.FromBase64String(RedJpegBase64);

    [Fact]
    public void Rejects_Truncated_File()
    {
        byte[] tiny = [0x00, 0x00, 0x00];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_Ftyp()
    {
        // A box that isn't 'ftyp'.
        byte[] bytes =
        [
            0x00, 0x00, 0x00, 0x10, // size = 16
            (byte)'m', (byte)'d', (byte)'a', (byte)'t',
            0, 0, 0, 0, 0, 0, 0, 0,
        ];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Wrong_Brand()
    {
        var spec = new TestCr3Builder.Cr3Spec { Brand = "heic" };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Parses_Ftyp_Brand_And_Compatible_Brands()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Brand = "crx ",
            CompatibleBrands = ["crx ", "isom", "mp41"],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Equal(ImageFormat.Cr3, cr3.Format);
        Assert.Equal("crx ", cr3.Cr3.MajorBrand);
        Assert.Equal(3, cr3.Cr3.CompatibleBrands.Count);
        Assert.Contains("isom", cr3.Cr3.CompatibleBrands);
        Assert.Contains("mp41", cr3.Cr3.CompatibleBrands);
    }

    [Fact]
    public void Parses_Canon_Cmt1_Tiff_Tags()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Model = "Canon EOS R5",
            Software = "EOS R5 Firmware 2.0.1",
            DateTime = "2024:08:11 18:22:09",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCanonUuid);
        Assert.True(cr3.Cr3.HasCmt1);
        Assert.Equal("Canon", cr3.Cr3.Make);
        Assert.Equal("Canon EOS R5", cr3.Cr3.Model);
        Assert.Equal("EOS R5 Firmware 2.0.1", cr3.Cr3.Software);
        Assert.Equal("2024:08:11 18:22:09", cr3.Cr3.DateTime);
        Assert.Equal("Test Photographer", cr3.Cr3.Artist);
        Assert.Equal("(c) 2024 Test", cr3.Cr3.Copyright);

        Assert.Equal("Canon", cr3.Metadata.CameraMake);
        Assert.Equal("Canon EOS R5", cr3.Metadata.CameraModel);
    }

    [Fact]
    public void Discovers_Thumbnail_SubImage()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        var thumb = cr3.SubImages.Single(s => s.Kind == Cr3SubImageKind.Thumbnail);
        Assert.Equal(16, thumb.Width);
        Assert.Equal(16, thumb.Height);
        Assert.True(thumb.CanDecodePixels);
        Assert.True(cr3.CanDecodePixels);
    }

    [Fact]
    public void Discovers_Preview_And_Picks_It_As_Primary_Over_Thumbnail()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
            PrvwJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Contains(cr3.SubImages, s => s.Kind == Cr3SubImageKind.Thumbnail);
        Assert.Contains(cr3.SubImages, s => s.Kind == Cr3SubImageKind.Preview);
        Assert.True(cr3.CanDecodePixels);
        // Both JPEGs are 16x16 in this synthetic test, so primary tie-break picks one;
        // the important property is that Info reflects the largest sub-image.
        Assert.Equal(16, cr3.Info.Width);
        Assert.Equal(16, cr3.Info.Height);
    }

    [Fact]
    public void Mdat_SurfacedAs_RawMosaic_SubImage_Undecodable()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
            MdatPayload = new byte[1024],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        var raw = cr3.SubImages.Single(s => s.Kind == Cr3SubImageKind.RawMosaic);
        Assert.Equal(1024, raw.Length);
        Assert.False(raw.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Thumbnail_When_Only_Thumbnail_Present()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        ImageFrame? frame = null;
        await foreach (var f in cr3.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(16, frame.Width);
            Assert.Equal(16, frame.Height);
            Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);
        }
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_No_Decodable_SubImage_Present()
    {
        // CR3 with only an mdat raw payload — no JPEG sub-images at all.
        var spec = new TestCr3Builder.Cr3Spec
        {
            MdatPayload = new byte[64],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));
        Assert.False(cr3.CanDecodePixels);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in cr3.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }
}
