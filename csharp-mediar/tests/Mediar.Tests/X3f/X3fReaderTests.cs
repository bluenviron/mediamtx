using Mediar.Imaging;
using Mediar.Imaging.X3f;
using Xunit;

namespace Mediar.Tests.X3f;

/// <summary>
/// Tests for <see cref="X3fReader"/>, covering "FOVb" header validation,
/// trailing "SECd" directory walk, section classification, property pool
/// UTF-16 decode, and embedded JPEG preview decode delegation.
/// </summary>
public sealed class X3fReaderTests
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
        byte[] tiny = [0x46, 0x4F, 0x56];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => X3fReader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_FOVb_Magic()
    {
        var bytes = new byte[64];
        bytes[0] = (byte)'X'; bytes[1] = (byte)'X'; bytes[2] = (byte)'X'; bytes[3] = (byte)'X';
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => X3fReader.Open(ms));
    }

    [Fact]
    public void Rejects_Directory_Offset_Out_Of_Bounds()
    {
        var b = new TestX3fBuilder
        {
            VersionMajor = 2,
            VersionMinor = 0,
            DirectoryOffsetOverride = 0xFFFFFF00u,
        }
        .AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => X3fReader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_SECd_Magic_At_Directory_Offset()
    {
        var b = new TestX3fBuilder { VersionMajor = 2, VersionMinor = 0 }
            .AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        // Replace "SECd" magic at the directory offset with garbage.
        uint dirOff = BitConverter.ToUInt32(bytes, bytes.Length - 4);
        bytes[dirOff + 0] = (byte)'X';
        bytes[dirOff + 1] = (byte)'X';
        bytes[dirOff + 2] = (byte)'X';
        bytes[dirOff + 3] = (byte)'X';
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => X3fReader.Open(ms));
    }

    [Fact]
    public void Parses_Header_Version_FileId_And_Mark()
    {
        var fileId = new byte[16];
        for (int i = 0; i < 16; i++) fileId[i] = (byte)(0xA0 + i);
        var b = new TestX3fBuilder
        {
            VersionMajor = 2,
            VersionMinor = 3,
            FileId = fileId,
            FileMark = 0xDEADBEEF,
            Rotation = 90,
            WhiteBalanceLabel = "Daylight",
        }
        .AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.Equal(2, reader.X3f.VersionMajor);
        Assert.Equal(3, reader.X3f.VersionMinor);
        Assert.Equal("a0a1a2a3a4a5a6a7a8a9aaabacadaeaf", reader.X3f.FileIdHex);
        Assert.Equal(0xDEADBEEFu, reader.X3f.FileMark);
        Assert.Equal(90u, reader.X3f.Rotation);
        Assert.Equal("Daylight", reader.X3f.WhiteBalanceLabel);
    }

    [Fact]
    public void Walks_Directory_And_Exposes_All_Sections()
    {
        var props = new List<KeyValuePair<string, string>>
        {
            new("CAMMANUF", "SIGMA"),
            new("CAMMODEL", "SIGMA dp2 Quattro"),
        };
        var b = new TestX3fBuilder()
            .AddJpegPreview(LoadRedJpeg(), 16, 16)
            .AddProperties(props)
            .AddCameraMetadata(new byte[] { 1, 2, 3, 4, 5, 6, 7, 8 });
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.Equal(3, reader.SubImages.Count);
        Assert.Equal(X3fSubImageKind.JpegPreview, reader.SubImages[0].Kind);
        Assert.Equal("IMA2", reader.SubImages[0].SectionId);
        Assert.Equal(X3fSubImageKind.Properties, reader.SubImages[1].Kind);
        Assert.Equal("PROP", reader.SubImages[1].SectionId);
        Assert.Equal(X3fSubImageKind.CameraMetadata, reader.SubImages[2].Kind);
        Assert.Equal("CAMF", reader.SubImages[2].SectionId);
    }

    [Fact]
    public void Properties_Section_Decodes_Sigma_Make_And_Model()
    {
        var props = new List<KeyValuePair<string, string>>
        {
            new("CAMMANUF", "SIGMA"),
            new("CAMMODEL", "SIGMA dp2 Quattro"),
            new("FIRMVERS", "1.04"),
            new("TIME", "2017:06:15 12:00:00"),
        };
        var b = new TestX3fBuilder()
            .AddJpegPreview(LoadRedJpeg(), 16, 16)
            .AddProperties(props);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.Equal("SIGMA", reader.X3f.Make);
        Assert.Equal("SIGMA dp2 Quattro", reader.X3f.Model);
        Assert.Equal("1.04", reader.X3f.Software);
        Assert.Equal("2017:06:15 12:00:00", reader.X3f.DateTime);
        Assert.Equal("SIGMA", reader.Metadata.CameraMake);
        Assert.Equal("SIGMA dp2 Quattro", reader.Metadata.CameraModel);
    }

    [Fact]
    public void Raw_Mosaic_Section_Is_Surfaced_As_Undecodable()
    {
        var raw = new byte[1024];
        var b = new TestX3fBuilder()
            .AddRawMosaic(raw, width: 32, height: 32, imageType: 2, dataFormat: 11, rowStride: 32);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.Single(reader.SubImages);
        var sub = reader.SubImages[0];
        Assert.Equal(X3fSubImageKind.RawMosaic, sub.Kind);
        Assert.False(sub.CanDecodePixels);
        Assert.Equal(2u, sub.ImageType);
        Assert.Equal(11u, sub.DataFormat);
        Assert.Equal(32, sub.Width);
        Assert.Equal(32, sub.Height);
        Assert.False(reader.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Embedded_JPEG_Preview()
    {
        var b = new TestX3fBuilder()
            .AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(16, reader.Info.Width);
        Assert.Equal(16, reader.Info.Height);

        int frames = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            frames++;
            Assert.Equal(16, frame.Width);
            Assert.Equal(16, frame.Height);
            // Centre pixel should be roughly red.
            int idx = ((frame.Height / 2) * frame.Stride) + (frame.Width / 2) * 3;
            Assert.True(frame.Pixels.Span[idx] > 200, $"R={frame.Pixels.Span[idx]}");
        }
        Assert.Equal(1, frames);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_No_Decodable_Section()
    {
        var b = new TestX3fBuilder()
            .AddRawMosaic(new byte[64], width: 8, height: 8);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.False(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync()) { }
        });
    }

    [Fact]
    public void Detector_Recognises_FOVb_Magic()
    {
        var b = new TestX3fBuilder().AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        Assert.Equal(ImageFormat.X3f, ImageFormatDetector.Detect(bytes));
    }

    [Theory]
    [InlineData((ushort)2, (ushort)0)]
    [InlineData((ushort)2, (ushort)1)]
    [InlineData((ushort)2, (ushort)3)]
    public void Accepts_Common_Header_Versions(ushort major, ushort minor)
    {
        var b = new TestX3fBuilder
        {
            VersionMajor = major,
            VersionMinor = minor,
            Rotation = minor >= 1 ? 0u : null,
            WhiteBalanceLabel = minor >= 1 ? "Auto" : null,
        }
        .AddJpegPreview(LoadRedJpeg(), 16, 16);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = X3fReader.Open(ms);
        Assert.Equal(major, reader.X3f.VersionMajor);
        Assert.Equal(minor, reader.X3f.VersionMinor);
    }
}
