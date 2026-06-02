using Mediar.Imaging;
using Mediar.Imaging.Fff;
using Mediar.Tests.Srw;
using Xunit;

namespace Mediar.Tests.Fff;

public sealed class FffReaderTests
{
    [Fact]
    public void Rejects_File_Without_Hasselblad_Make_Tag()
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
        var ex = Assert.Throws<ImageFormatException>(() => FffReader.Open(ms));
        Assert.Contains("Hasselblad", ex.Message, StringComparison.Ordinal);
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
        Assert.Throws<ImageFormatException>(() => FffReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => FffReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => FffReader.Open(ms));
    }

    [Theory]
    [InlineData("Hasselblad")]
    [InlineData("HASSELBLAD")]
    public void Accepts_Hasselblad_Make_Variants(string make)
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
            Model = "H4D-50",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);
        Assert.Equal(ImageFormat.Fff, reader.Format);
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
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 6],
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
            Make = "Hasselblad",
            Model = "H4D-50",
            SubIfds = [sub],
        };
        byte[] bytes = TestSrwBuilder.Build(root);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);

        Assert.Equal(2, reader.SubImages.Count);
        Assert.Equal(8, reader.Info.Width);
        Assert.Equal(6, reader.Info.Height);
        Assert.Equal(1, reader.SubImages[1].SubIfdLevel);
    }

    [Fact]
    public void Parses_Hasselblad_Metadata_Tags()
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
            Make = "Hasselblad",
            Model = "H4D-50",
            Software = "Phocus 3.0",
            DateTime = "2012:08:15 09:00:00",
            Artist = "Hassel Photographer",
            Copyright = "(c) 2012 Hasselblad Test",
            MakerNote = new byte[256],
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);

        Assert.Equal("Hasselblad", reader.Raw.Make);
        Assert.Equal("H4D-50", reader.Raw.Model);
        Assert.Equal("Phocus 3.0", reader.Raw.Software);
        Assert.Equal("2012:08:15 09:00:00", reader.Raw.DateTime);
        Assert.Equal("Hassel Photographer", reader.Raw.Artist);
        Assert.Equal("(c) 2012 Hasselblad Test", reader.Raw.Copyright);
        Assert.Equal(256, reader.Raw.MakerNoteLength);
        Assert.Equal("Hasselblad", reader.Metadata.CameraMake);
        Assert.Equal("H4D-50", reader.Metadata.CameraModel);
        Assert.Equal("256", reader.Metadata.Tags["Exif:MakerNoteLength"]);
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
            Make = "Hasselblad",
            Model = "H4D-50",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);
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
    public void Reports_Hasselblad_Compressed_Cfa_As_CanDecodePixels_False()
    {
        // FFF on H1D / H2D bodies uses CFA + compressed; the IsVendorUndecodable
        // rule (photometric=32803 + compression != 1) should flag this even if
        // the underlying TIFF compression code (e.g. 8 = Adobe Deflate) is one
        // the standard TIFF reader otherwise accepts.
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 8, // Adobe Deflate
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Hasselblad",
            Model = "H1D",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);

        Assert.False(reader.CanDecodePixels);
        Assert.Equal(8, reader.SubImages[0].CompressionTag);
        Assert.Equal(32803, reader.SubImages[0].Photometric);
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
            Compression = 32767,
            Photometric = 32803,
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "Hasselblad",
            Model = "H4D-50",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = FffReader.Open(ms);

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
        Assert.Throws<ImageFormatException>(() => FffReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Hasselblad_Make_Is_Rejected()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "hasselblad",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            FffReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "Hasselblad",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = FffReader.Open(new MemoryStream(bytes, writable: false));
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
            Make = "Hasselblad",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = FffReader.Open(new MemoryStream(bytes, writable: false));
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
            Make = "Hasselblad",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestSrwBuilder.Build(root);
        using var reader = FffReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, reader.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 17) & 0xFF);
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "Hasselblad",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var reader = FffReader.Open(new MemoryStream(bytes, writable: false));
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
            Make = "Hasselblad",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var reader = FffReader.Open(ms, ownsStream: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => FffReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalHasselbladSpec());
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = FffReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Fff, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalHasselbladSpec());
        using var fff = FffReader.Open(new MemoryStream(bytes, writable: false));
        if (!fff.CanDecodePixels) return;
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in fff.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Info_Format_Equals_Fff()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalHasselbladSpec());
        using var fff = FffReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Fff, fff.Info.Format);
    }

    [Fact]
    public void Info_HasAlpha_False_For_3Channel_Rgb_Strip()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalHasselbladSpec());
        using var fff = FffReader.Open(new MemoryStream(bytes, writable: false));
        Assert.False(fff.Info.HasAlpha);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalHasselbladSpec());
        var r = FffReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    private static TestSrwBuilder.IfdSpec MinimalHasselbladSpec() => new()
    {
        Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
        Compression = 1, Photometric = 2, NewSubFileType = 0,
        StripPayload = new byte[4 * 4 * 3],
        Make = "Hasselblad",
    };
}
