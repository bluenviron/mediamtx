using Mediar.Imaging;
using Mediar.Imaging.Nrw;
using Mediar.Tests.Srw;
using Xunit;

namespace Mediar.Tests.Nrw;

public sealed class NrwReaderTests
{
    [Fact]
    public void Rejects_File_Without_Nikon_Make_Tag()
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
            Make = "Canon EOS R5",
            Model = "FakeModel",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => NrwReader.Open(ms));
        Assert.Contains("Nikon", ex.Message, StringComparison.Ordinal);
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
        Assert.Throws<ImageFormatException>(() => NrwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => NrwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => NrwReader.Open(ms));
    }

    [Theory]
    [InlineData("NIKON")]
    [InlineData("NIKON CORPORATION")]
    [InlineData("Nikon")]
    public void Accepts_Nikon_Make_Variants(string make)
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
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);
        Assert.Equal(ImageFormat.Nrw, reader.Format);
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
            Make = "NIKON",
            Model = "COOLPIX P7800",
            SubIfds = [sub],
        };
        byte[] bytes = TestSrwBuilder.Build(root);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);

        Assert.Equal(2, reader.SubImages.Count);
        Assert.Equal(8, reader.Info.Width);
        Assert.Equal(6, reader.Info.Height);
        Assert.Equal(1, reader.SubImages[1].SubIfdLevel);
    }

    [Fact]
    public void Parses_Nikon_Metadata_Tags()
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
            Make = "NIKON",
            Model = "COOLPIX P7800",
            Software = "Ver.1.10",
            DateTime = "2014:08:01 09:00:00",
            Artist = "Niko Photographer",
            Copyright = "(c) 2014 Nikon Test",
            MakerNote = new byte[256],
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);

        Assert.Equal("NIKON", reader.Raw.Make);
        Assert.Equal("COOLPIX P7800", reader.Raw.Model);
        Assert.Equal("Ver.1.10", reader.Raw.Software);
        Assert.Equal("2014:08:01 09:00:00", reader.Raw.DateTime);
        Assert.Equal("Niko Photographer", reader.Raw.Artist);
        Assert.Equal("(c) 2014 Nikon Test", reader.Raw.Copyright);
        Assert.Equal(256, reader.Raw.MakerNoteLength);
        Assert.Equal("NIKON", reader.Metadata.CameraMake);
        Assert.Equal("COOLPIX P7800", reader.Metadata.CameraModel);
        Assert.Equal("256", reader.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task Decodes_Uncompressed_Rgb_Through_Tiff_Reader()
    {
        // 4x2 RGB image with a recognizable pattern: red, green, blue, white,
        // and the second row inverted.
        byte[] payload =
        [
            // row 0
            0xFF, 0x00, 0x00,  0x00, 0xFF, 0x00,  0x00, 0x00, 0xFF,  0xFF, 0xFF, 0xFF,
            // row 1
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
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);
        Assert.True(reader.CanDecodePixels);

        int frameCount = 0;
        await foreach (var frame in reader.ReadFramesAsync())
        {
            frameCount++;
            Assert.Equal(4, frame.Width);
            Assert.Equal(2, frame.Height);
            // Check the first pixel is red
            var firstPixel = frame.Pixels.Span;
            Assert.Equal(0xFF, firstPixel[0]); // R
            Assert.Equal(0x00, firstPixel[1]); // G
            Assert.Equal(0x00, firstPixel[2]); // B
            frame.Dispose();
        }
        Assert.Equal(1, frameCount);
    }

    [Fact]
    public void Reports_Nikon_Compressed_As_CanDecodePixels_False()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 34713, // Nikon proprietary compression
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);

        Assert.False(reader.CanDecodePixels);
        Assert.Equal(34713, reader.SubImages[0].CompressionTag);
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
            Compression = 34713,
            Photometric = 32803,
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = NrwReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var frame in reader.ReadFramesAsync())
            {
                frame.Dispose();
            }
        });
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => NrwReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Nikon_Make_Is_Rejected()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "nikon",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            NrwReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = NrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, reader.Raw.MakerNoteLength);
    }

    [Fact]
    public void Software_Absent_Field_Is_Null()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = NrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(reader.Raw.Software);
    }

    [Fact]
    public void Multiple_SubIfds_All_Surfaced_As_SubImages()
    {
        var sub1 = new TestSrwBuilder.IfdSpec
        {
            Width = 8, Height = 6, BitsPerSample = 16, SamplesPerPixel = 1,
            Compression = 1, Photometric = 32803, NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var sub2 = new TestSrwBuilder.IfdSpec
        {
            Width = 4, Height = 3, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[4 * 3 * 3],
        };
        var root = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[12],
            Make = "NIKON",
            Model = "COOLPIX P7800",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestSrwBuilder.Build(root);
        using var reader = NrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, reader.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 29) & 0xFF);
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = NrwReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured) { Assert.Equal(payload, captured!.Pixels.ToArray()); }
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "NIKON",
            Model = "COOLPIX P7800",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var reader = NrwReader.Open(ms, ownsStream: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }
}
