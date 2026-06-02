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

    [Fact]
    public async Task Info_BitsPerPixel_Is_24_For_Rgb24()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.Equal(PixelFormat.Rgb24, reader.Info.PixelFormat);
        Assert.Equal(24, reader.Info.BitsPerPixel);
    }

    [Fact]
    public async Task ReadFramesAsync_Honours_Cancellation()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var frame in reader.ReadFramesAsync(cts.Token))
            {
                frame.Dispose();
            }
        });
    }

    [Fact]
    public async Task All_Pixels_Are_Solid_Red_Within_Tolerance()
    {
        // Tighter than the average test: every individual pixel should be
        // red-dominant, not just the global average.
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        using (frame)
        {
            var px = frame!.Pixels.Span;
            for (int i = 0; i + 2 < px.Length; i += 3)
            {
                Assert.True(px[i] >= px[i + 1], $"pixel {i/3}: R<G ({px[i]}<{px[i+1]})");
                Assert.True(px[i] >= px[i + 2], $"pixel {i/3}: R<B ({px[i]}<{px[i+2]})");
            }
        }
    }

    [Fact]
    public async Task ExifTags_Is_Empty_For_Plain_Jpeg()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.NotNull(reader.ExifTags);
        // A no-EXIF JPEG should produce zero EXIF entries.
        Assert.Empty(reader.ExifTags);
    }

    [Fact]
    public void Open_Empty_Stream_Throws()
    {
        using var ms = new MemoryStream();
        Assert.ThrowsAny<Exception>(() => JpegReader.Open(ms));
    }

    [Fact]
    public void Open_AsciiOnly_Throws()
    {
        // Plain ASCII text should be rejected as non-JPEG.
        using var ms = new MemoryStream(System.Text.Encoding.ASCII.GetBytes("HELLO WORLD"));
        Assert.ThrowsAny<Exception>(() => JpegReader.Open(ms));
    }

    [Fact]
    public async Task Reading_All_Frames_Yields_Exactly_One()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        int count = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            count++;
            frame.Dispose();
        }
        Assert.Equal(1, count);
    }

    [Fact]
    public async Task Pixels_Length_Matches_Width_Times_Height_Times_3()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        await foreach (var frame in reader.ReadFramesAsync())
        {
            using (frame)
            {
                Assert.Equal(reader.Info.Width * reader.Info.Height * 3, frame.Pixels.Length);
            }
        }
    }

    [Fact]
    public async Task Format_Selected_Via_Extension_Is_Reflected_On_Reader()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        var tmp = Path.Combine(Path.GetTempPath(), $"mediar-jpeg-fmt-{Guid.NewGuid():N}.thm");
        try
        {
            await File.WriteAllBytesAsync(tmp, bytes);
            using var reader = JpegReader.Open(tmp);
            Assert.Equal(ImageFormat.Thm, reader.Format);
            Assert.Equal(16, reader.Info.Width);
        }
        finally
        {
            if (File.Exists(tmp)) File.Delete(tmp);
        }
    }

    [Fact]
    public async Task Metadata_Is_Always_Present()
    {
        var bytes = Convert.FromBase64String(RedJpegBase64);
        await using var ms = new MemoryStream(bytes);
        using var reader = JpegReader.Open(ms);
        Assert.NotNull(reader.Metadata);
    }
}
