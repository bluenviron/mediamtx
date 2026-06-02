using Mediar.Imaging;
using Mediar.Imaging.ThreeFr;
using Xunit;

namespace Mediar.Tests.ThreeFr;

public sealed class ThreeFrReaderTests
{
    [Fact]
    public void Rejects_File_Without_Hasselblad_Make_Tag()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
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
            Model = "D850",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => ThreeFrReader.Open(ms));
        Assert.Contains("Hasselblad", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
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
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => ThreeFrReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => ThreeFrReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => ThreeFrReader.Open(ms));
    }

    [Theory]
    [InlineData("Hasselblad")]
    [InlineData("HASSELBLAD")]
    public void Accepts_Hasselblad_Make_Variants(string make)
    {
        var spec = new TestThreeFrBuilder.IfdSpec
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
            Model = "H6D-100c",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);
        Assert.Equal(ImageFormat.ThreeFr, tfr.Format);
        Assert.Equal(make, tfr.Raw.Make);
    }

    [Fact]
    public void Discovers_Sub_Ifd_And_Picks_It_As_Primary()
    {
        var sub = new TestThreeFrBuilder.IfdSpec
        {
            Width = 8,
            Height = 6,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 1, // uncompressed CFA so it's decodable
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var root = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4,
            Height = 3,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1, // preview
            StripPayload = new byte[4 * 3 * 3],
            Make = "Hasselblad",
            Model = "X1D II 50C",
            SubIfds = [sub],
        };
        byte[] bytes = TestThreeFrBuilder.Build(root);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);

        Assert.Equal(2, tfr.SubImages.Count);
        Assert.Equal(8, tfr.Info.Width);
        Assert.Equal(6, tfr.Info.Height);
        Assert.Equal(1, tfr.SubImages[1].SubIfdLevel);
    }

    [Fact]
    public void Parses_Hasselblad_Metadata_Tags()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "Hasselblad",
            Model = "H6D-100c",
            Software = "Phocus 3.6",
            DateTime = "2019:08:15 14:22:31",
            Artist = "Henri Hasselblad",
            Copyright = "(c) 2019",
            MakerNote = new byte[512],
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);

        Assert.Equal("Hasselblad", tfr.Raw.Make);
        Assert.Equal("H6D-100c", tfr.Raw.Model);
        Assert.Equal("Phocus 3.6", tfr.Raw.Software);
        Assert.Equal("2019:08:15 14:22:31", tfr.Raw.DateTime);
        Assert.Equal("Henri Hasselblad", tfr.Raw.Artist);
        Assert.Equal("(c) 2019", tfr.Raw.Copyright);
        Assert.Equal(512, tfr.Raw.MakerNoteLength);
        Assert.Equal("Hasselblad", tfr.Metadata.CameraMake);
        Assert.Equal("H6D-100c", tfr.Metadata.CameraModel);
        Assert.Equal("512", tfr.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task Decodes_Uncompressed_Rgb_Through_Tiff_Reader()
    {
        byte[] payload =
        [
            // row 0
            0xFF, 0x00, 0x00,  0x00, 0xFF, 0x00,  0x00, 0x00, 0xFF,  0xFF, 0xFF, 0xFF,
            // row 1
            0x00, 0xFF, 0xFF,  0xFF, 0x00, 0xFF,  0xFF, 0xFF, 0x00,  0x00, 0x00, 0x00,
        ];
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4,
            Height = 2,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = payload,
            Make = "Hasselblad",
            Model = "X1D II 50C",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);
        Assert.True(tfr.CanDecodePixels);

        int frameCount = 0;
        await foreach (var frame in tfr.ReadFramesAsync())
        {
            frameCount++;
            Assert.Equal(4, frame.Width);
            Assert.Equal(2, frame.Height);
            var firstPixel = frame.Pixels.Span;
            Assert.Equal(0xFF, firstPixel[0]); // R
            Assert.Equal(0x00, firstPixel[1]); // G
            Assert.Equal(0x00, firstPixel[2]); // B
            frame.Dispose();
        }
        Assert.Equal(1, frameCount);
    }

    [Fact]
    public void Reports_Hasselblad_Compressed_Bayer_As_CanDecodePixels_False()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 8,      // deflate-like with predictor (used by Hasselblad)
            Photometric = 32803,  // CFA
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Hasselblad",
            Model = "H6D-100c",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);

        Assert.False(tfr.CanDecodePixels);
        Assert.Equal(8, tfr.SubImages[0].CompressionTag);
        Assert.Equal(32803, tfr.SubImages[0].Photometric);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 8,
            Photometric = 32803,
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Hasselblad",
            Model = "H6D-100c",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var tfr = ThreeFrReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var frame in tfr.ReadFramesAsync())
            {
                frame.Dispose();
            }
        });
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => ThreeFrReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Hasselblad_Make_Is_Rejected()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "hasselblad",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            ThreeFrReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "Hasselblad",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);
        using var tfr = ThreeFrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, tfr.Raw.MakerNoteLength);
    }

    [Fact]
    public void Software_Absent_Field_Is_Null()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "Hasselblad",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);
        using var tfr = ThreeFrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(tfr.Raw.Software);
    }

    [Fact]
    public void Multiple_SubIfds_All_Surfaced_As_SubImages()
    {
        var sub1 = new TestThreeFrBuilder.IfdSpec
        {
            Width = 8, Height = 6, BitsPerSample = 16, SamplesPerPixel = 1,
            Compression = 1, Photometric = 32803, NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var sub2 = new TestThreeFrBuilder.IfdSpec
        {
            Width = 4, Height = 3, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[4 * 3 * 3],
        };
        var root = new TestThreeFrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[12],
            Make = "Hasselblad",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestThreeFrBuilder.Build(root);
        using var tfr = ThreeFrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, tfr.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 29) & 0xFF);
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "Hasselblad",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);
        using var tfr = ThreeFrReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? captured = null;
        await foreach (var f in tfr.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured) { Assert.Equal(payload, captured!.Pixels.ToArray()); }
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestThreeFrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "Hasselblad",
        };
        byte[] bytes = TestThreeFrBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var tfr = ThreeFrReader.Open(ms, ownsStream: true);
        tfr.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }
}
