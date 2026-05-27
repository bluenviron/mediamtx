using Mediar.Imaging;
using Mediar.Imaging.Iiq;
using Mediar.Tests.Srw;
using Xunit;

namespace Mediar.Tests.Iiq;

public sealed class IiqReaderTests
{
    [Fact]
    public void Rejects_File_Without_Phase_One_Make_Tag()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "NIKON CORPORATION",
            Model = "FakeModel",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => IiqReader.Open(ms));
        Assert.Contains("Phase One", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2,
            Height = 2,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[2 * 2 * 3],
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => IiqReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => IiqReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => IiqReader.Open(ms));
    }

    [Theory]
    [InlineData("Phase One")]
    [InlineData("PhaseOne")]
    [InlineData("PHASE ONE")]
    public void Accepts_Phase_One_Make_Variants(string make)
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = make,
            Model = "IQ180",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);
        Assert.Equal(ImageFormat.Iiq, reader.Format);
        Assert.Equal(make, reader.Raw.Make);
    }

    [Fact]
    public void Discovers_Sub_Ifd_And_Picks_It_As_Primary()
    {
        var sub = new TestSrwBuilder.IfdSpec
        {
            Width = 8,
            Height = 6,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 1,
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var root = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 3,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1, // preview
            StripPayload = new byte[4 * 3 * 3],
            Make = "Phase One",
            Model = "IQ180",
            SubIfds = [sub],
        };
        byte[] bytes = TestSrwBuilder.Build(root);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);

        Assert.Equal(2, reader.SubImages.Count);
        Assert.Equal(8, reader.Info.Width);
        Assert.Equal(6, reader.Info.Height);
        Assert.Equal(1, reader.SubImages[1].SubIfdLevel);
    }

    [Fact]
    public void Parses_Phase_One_Metadata_Tags()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "Phase One",
            Model = "IQ180",
            Software = "Capture One 12.1",
            DateTime = "2018:05:01 14:30:00",
            Artist = "Phase Photographer",
            Copyright = "(c) 2018 Phase One Test",
            MakerNote = new byte[512],
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);

        Assert.Equal("Phase One", reader.Raw.Make);
        Assert.Equal("IQ180", reader.Raw.Model);
        Assert.Equal("Capture One 12.1", reader.Raw.Software);
        Assert.Equal("2018:05:01 14:30:00", reader.Raw.DateTime);
        Assert.Equal("Phase Photographer", reader.Raw.Artist);
        Assert.Equal("(c) 2018 Phase One Test", reader.Raw.Copyright);
        Assert.Equal(512, reader.Raw.MakerNoteLength);
        Assert.Equal("Phase One", reader.Metadata.CameraMake);
        Assert.Equal("IQ180", reader.Metadata.CameraModel);
        Assert.Equal("512", reader.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task Decodes_Uncompressed_Rgb_Through_Tiff_Reader()
    {
        byte[] payload =
        [
            0xFF, 0x00, 0x00,  0x00, 0xFF, 0x00,  0x00, 0x00, 0xFF,  0xFF, 0xFF, 0xFF,
            0x00, 0xFF, 0xFF,  0xFF, 0x00, 0xFF,  0xFF, 0xFF, 0x00,  0x00, 0x00, 0x00,
        ];
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 2,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = payload,
            Make = "Phase One",
            Model = "IQ180",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);
        Assert.True(reader.CanDecodePixels);

        int frameCount = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            frameCount++;
            Assert.Equal(4, frame.Width);
            Assert.Equal(2, frame.Height);
            var firstPixel = frame.Pixels.Span;
            Assert.Equal(0xFF, firstPixel[0]);
            Assert.Equal(0x00, firstPixel[1]);
            Assert.Equal(0x00, firstPixel[2]);
            frame.Dispose();
        }
        Assert.Equal(1, frameCount);
    }

    [Fact]
    public void Reports_Phase_One_Compressed_As_CanDecodePixels_False()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 34892, // Phase One IIQ-L
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Phase One",
            Model = "IQ180",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);

        Assert.False(reader.CanDecodePixels);
        Assert.Equal(34892, reader.SubImages[0].CompressionTag);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 34892,
            Photometric = 32803,
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Phase One",
            Model = "IQ180",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = IiqReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var frame in reader.ReadFramesAsync())
            {
                frame.Dispose();
            }
        });
    }
}
