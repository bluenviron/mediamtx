using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the baseline-DCT JPEG decoder. Uses a tiny 16×16 solid-red JPEG
/// (encoded with quality ≈ 90) embedded as base64 to avoid depending on
/// platform-specific image libraries inside the test assembly.
/// </summary>
public sealed class JpegBaselineDecoderTests
{
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
    public async Task SolidRed_BaselineJpeg_DecodesAndIsRedDominant()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);

        Assert.Equal(16, reader.Info.Width);
        Assert.Equal(16, reader.Info.Height);
        Assert.Equal(PixelFormat.Rgb24, reader.Info.PixelFormat);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);

        using (captured)
        {
            var pixels = captured!.Pixels.Span;
            Assert.Equal(16 * 16 * 3, pixels.Length);

            long rSum = 0, gSum = 0, bSum = 0;
            for (int i = 0; i + 2 < pixels.Length; i += 3)
            {
                rSum += pixels[i];
                gSum += pixels[i + 1];
                bSum += pixels[i + 2];
            }
            int n = pixels.Length / 3;
            int rAvg = (int)(rSum / n);
            int gAvg = (int)(gSum / n);
            int bAvg = (int)(bSum / n);

            Assert.True(rAvg > 180, $"expected red-dominant pixel, got R={rAvg} G={gAvg} B={bAvg}");
            Assert.True(gAvg < 40, $"expected low green, got R={rAvg} G={gAvg} B={bAvg}");
            Assert.True(bAvg < 40, $"expected low blue, got R={rAvg} G={gAvg} B={bAvg}");
        }
    }

    [Fact]
    public async Task SolidRed_BaselineJpeg_DecodesViaImagingFacade()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        var tmp = Path.Combine(Path.GetTempPath(), $"mediar-test-{Guid.NewGuid():N}.jpg");
        try
        {
            await File.WriteAllBytesAsync(tmp, bytes);
            using var reader = MediarImage.Open(tmp);

            Assert.Equal(ImageFormat.Jpeg, reader.Format);
            Assert.Equal(16, reader.Info.Width);
            Assert.Equal(16, reader.Info.Height);
            Assert.True(reader.CanDecodePixels);

            int frames = 0;
            await foreach (var frame in reader.ReadFramesAsync())
            {
                using (frame)
                {
                    Assert.True(frame.Pixels.Length > 0);
                    frames++;
                }
            }
            Assert.Equal(1, frames);
        }
        finally
        {
            if (File.Exists(tmp)) File.Delete(tmp);
        }
    }

    [Fact]
    public async Task SolidRed_Format_Defaults_To_Jpeg_From_Stream()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.Equal(ImageFormat.Jpeg, reader.Format);
    }

    [Theory]
    [InlineData(".jpg", ImageFormat.Jpeg)]
    [InlineData(".jpeg", ImageFormat.Jpeg)]
    [InlineData(".thm", ImageFormat.Thm)]
    [InlineData(".mpo", ImageFormat.Mpo)]
    [InlineData(".jfif", ImageFormat.Jfif)]
    [InlineData(".jpg_large", ImageFormat.JpgLarge)]
    public async Task Open_Path_Selects_Format_From_Extension(string ext, ImageFormat expected)
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        var tmp = Path.Combine(Path.GetTempPath(), $"mediar-jpeg-{Guid.NewGuid():N}{ext}");
        try
        {
            await File.WriteAllBytesAsync(tmp, bytes);
            using var reader = JpegReader.Open(tmp);
            Assert.Equal(expected, reader.Format);
        }
        finally
        {
            if (File.Exists(tmp)) File.Delete(tmp);
        }
    }

    [Fact]
    public void Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => JpegReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Missing_Path_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-jpeg-missing-{Guid.NewGuid():N}.jpg");
        Assert.Throws<FileNotFoundException>(() => JpegReader.Open(path));
    }

    [Fact]
    public void Open_Wrong_Magic_Throws()
    {
        var bytes = new byte[16];
        bytes[0] = 0xFF; bytes[1] = 0xC8; // wrong SOI
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => JpegReader.Open(ms));
    }

    [Fact]
    public void Open_Truncated_Magic_Throws()
    {
        using var ms = new MemoryStream(new byte[1]);
        Assert.Throws<EndOfStreamException>(() => JpegReader.Open(ms));
    }

    [Fact]
    public async Task ExifTags_Reflect_Metadata()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.NotNull(reader.ExifTags);
        Assert.NotNull(reader.Metadata);
    }

    [Fact]
    public async Task Dispose_Idempotent()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        var reader = JpegReader.Open(ms);
        reader.Dispose();
        reader.Dispose();
    }

    [Fact]
    public async Task OwnsStream_False_Leaves_Stream_Open_After_Dispose()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        var ms = new MemoryStream(bytes);
        var reader = JpegReader.Open(ms, ownsStream: false);
        reader.Dispose();
        _ = ms.Length;
        ms.Dispose();
    }

    [Fact]
    public async Task OwnsStream_True_Closes_Stream_On_Dispose()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        var ms = new MemoryStream(bytes);
        var reader = JpegReader.Open(ms, ownsStream: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public async Task Info_Exposes_Frame_Count_1()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.Equal(1, reader.Info.FrameCount);
    }
}
