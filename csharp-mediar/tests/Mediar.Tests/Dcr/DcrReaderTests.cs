using Mediar.Imaging;
using Mediar.Imaging.Dcr;
using Xunit;

namespace Mediar.Tests.Dcr;

public sealed class DcrReaderTests
{
    [Fact]
    public void Rejects_File_Without_Kodak_Make_Tag()
    {
        var spec = new TestDcrBuilder.IfdSpec
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
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => DcrReader.Open(ms));
        Assert.Contains("Kodak", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestDcrBuilder.IfdSpec
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
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => DcrReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => DcrReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => DcrReader.Open(ms));
    }

    [Theory]
    [InlineData("EASTMAN KODAK COMPANY")]
    [InlineData("KODAK")]
    [InlineData("Kodak")]
    public void Accepts_Kodak_Make_Variants(string make)
    {
        var spec = new TestDcrBuilder.IfdSpec
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
            Model = "DCS Pro 14n",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);

        Assert.Equal(make, dcr.Raw.Make);
        Assert.Equal(make, dcr.Metadata.CameraMake);
    }

    [Fact]
    public void Discovers_SubIfd_And_Picks_It_As_Primary()
    {
        var raw = new TestDcrBuilder.IfdSpec
        {
            Width = 16,
            Height = 16,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 1,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16 * 16 * 2],
        };
        var thumb = new TestDcrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            Make = "EASTMAN KODAK COMPANY",
            Model = "DCS Pro 14n",
            SubIfds = [raw],
        };
        byte[] bytes = TestDcrBuilder.Build(thumb);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);

        Assert.Equal(2, dcr.SubImages.Count);
        Assert.Equal(4, dcr.SubImages[0].Width);
        Assert.Equal(0, dcr.SubImages[0].SubIfdLevel);
        Assert.Equal(1, dcr.SubImages[0].NewSubFileType);

        Assert.Equal(16, dcr.SubImages[1].Width);
        Assert.Equal(1, dcr.SubImages[1].SubIfdLevel);
        Assert.Equal(0, dcr.SubImages[1].NewSubFileType);

        Assert.Equal(16, dcr.Info.Width);
        Assert.Equal(16, dcr.Info.Height);
        Assert.Equal(ImageFormat.Dcr, dcr.Format);
    }

    [Fact]
    public void Parses_Kodak_Metadata_Fields()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "EASTMAN KODAK COMPANY",
            Model = "DCS Pro 14n",
            Software = "Kodak DCS Photo Desk 4.2",
            DateTime = "2024:08:11 18:22:09",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            MakerNote = new byte[256],
        };
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);

        Assert.Equal("EASTMAN KODAK COMPANY", dcr.Raw.Make);
        Assert.Equal("DCS Pro 14n", dcr.Raw.Model);
        Assert.Equal("Kodak DCS Photo Desk 4.2", dcr.Raw.Software);
        Assert.Equal("2024:08:11 18:22:09", dcr.Raw.DateTime);
        Assert.Equal("Test Photographer", dcr.Raw.Artist);
        Assert.Equal("(c) 2024 Test", dcr.Raw.Copyright);
        Assert.Equal(256, dcr.Raw.MakerNoteLength);

        Assert.Equal("EASTMAN KODAK COMPANY", dcr.Metadata.CameraMake);
        Assert.Equal("DCS Pro 14n", dcr.Metadata.CameraModel);
        Assert.Equal("Kodak DCS Photo Desk 4.2", dcr.Metadata.Software);
        Assert.Equal("2024:08:11 18:22:09", dcr.Metadata.CapturedAtRaw);
        Assert.Equal("Test Photographer", dcr.Metadata.Author);
        Assert.Equal("(c) 2024 Test", dcr.Metadata.Copyright);
        Assert.True(dcr.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
        Assert.Equal("256", dcr.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Uncompressed_Rgb_Through_Tiff()
    {
        const int W = 8, H = 4;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 17) & 0xFF);

        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "EASTMAN KODAK COMPANY",
            Model = "DCS Pro 14n",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);
        Assert.True(dcr.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in dcr.ReadFramesAsync())
        {
            frame = f;
            break;
        }

        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(W, frame.Width);
            Assert.Equal(H, frame.Height);
            Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);
            Assert.Equal(strip, frame.Pixels.ToArray());
        }
    }

    [Fact]
    public void Reports_Kodak_Compressed_As_CanDecodePixels_False()
    {
        // Compression 65000 = "Kodak DCR compressed" - proprietary, not yet supported.
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
            Compression = 65000,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);

        Assert.False(dcr.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
            Compression = 65000,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dcr = DcrReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in dcr.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => DcrReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Kodak_Make_Is_Rejected()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "kodak",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            DcrReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);
        using var dcr = DcrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, dcr.Raw.MakerNoteLength);
    }

    [Fact]
    public void Software_Absent_Field_Is_Null()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);
        using var dcr = DcrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(dcr.Raw.Software);
    }

    [Fact]
    public void Multiple_SubIfds_All_Surfaced_As_SubImages()
    {
        var sub1 = new TestDcrBuilder.IfdSpec
        {
            Width = 8, Height = 6, BitsPerSample = 16, SamplesPerPixel = 1,
            Compression = 1, Photometric = 32803, NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var sub2 = new TestDcrBuilder.IfdSpec
        {
            Width = 4, Height = 3, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[4 * 3 * 3],
        };
        var root = new TestDcrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[12],
            Make = "EASTMAN KODAK COMPANY",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestDcrBuilder.Build(root);
        using var dcr = DcrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, dcr.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 31) & 0xFF);
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);
        using var dcr = DcrReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? captured = null;
        await foreach (var f in dcr.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured) { Assert.Equal(payload, captured!.Pixels.ToArray()); }
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestDcrBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "EASTMAN KODAK COMPANY",
        };
        byte[] bytes = TestDcrBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var dcr = DcrReader.Open(ms, ownsStream: true);
        dcr.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }
}
