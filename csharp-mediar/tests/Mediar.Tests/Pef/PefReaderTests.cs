using Mediar.Imaging;
using Mediar.Imaging.Pef;
using Xunit;

namespace Mediar.Tests.Pef;

public sealed class PefReaderTests
{
    [Fact]
    public void Rejects_File_Without_Pentax_Or_Ricoh_Make_Tag()
    {
        var spec = new TestPefBuilder.IfdSpec
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
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => PefReader.Open(ms));
        Assert.Contains("Pentax", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestPefBuilder.IfdSpec
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
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PefReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => PefReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PefReader.Open(ms));
    }

    [Theory]
    [InlineData("PENTAX")]
    [InlineData("PENTAX Corporation")]
    [InlineData("RICOH IMAGING COMPANY, LTD.")]
    [InlineData("RICOH")]
    public void Accepts_Pentax_And_Ricoh_Imaging_Make_Variants(string make)
    {
        var spec = new TestPefBuilder.IfdSpec
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
            Model = "K-3 Mark III",
        };
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);

        Assert.Equal(make, pef.Raw.Make);
        Assert.Equal(make, pef.Metadata.CameraMake);
    }

    [Fact]
    public void Discovers_SubIfd_And_Picks_It_As_Primary()
    {
        var raw = new TestPefBuilder.IfdSpec
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
        var thumb = new TestPefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            Make = "PENTAX",
            Model = "K-3 Mark III",
            SubIfds = [raw],
        };
        byte[] bytes = TestPefBuilder.Build(thumb);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);

        Assert.Equal(2, pef.SubImages.Count);
        Assert.Equal(4, pef.SubImages[0].Width);
        Assert.Equal(0, pef.SubImages[0].SubIfdLevel);
        Assert.Equal(1, pef.SubImages[0].NewSubFileType);

        Assert.Equal(16, pef.SubImages[1].Width);
        Assert.Equal(1, pef.SubImages[1].SubIfdLevel);
        Assert.Equal(0, pef.SubImages[1].NewSubFileType);

        Assert.Equal(16, pef.Info.Width);
        Assert.Equal(16, pef.Info.Height);
        Assert.Equal(ImageFormat.Pef, pef.Format);
    }

    [Fact]
    public void Parses_Pentax_Metadata_Fields()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "RICOH IMAGING COMPANY, LTD.",
            Model = "PENTAX K-3 Mark III",
            Software = "K-3 Mark III Ver 1.50",
            DateTime = "2024:08:11 18:22:09",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            MakerNote = new byte[200],
        };
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);

        Assert.Equal("RICOH IMAGING COMPANY, LTD.", pef.Raw.Make);
        Assert.Equal("PENTAX K-3 Mark III", pef.Raw.Model);
        Assert.Equal("K-3 Mark III Ver 1.50", pef.Raw.Software);
        Assert.Equal("2024:08:11 18:22:09", pef.Raw.DateTime);
        Assert.Equal("Test Photographer", pef.Raw.Artist);
        Assert.Equal("(c) 2024 Test", pef.Raw.Copyright);
        Assert.Equal(200, pef.Raw.MakerNoteLength);

        Assert.Equal("RICOH IMAGING COMPANY, LTD.", pef.Metadata.CameraMake);
        Assert.Equal("PENTAX K-3 Mark III", pef.Metadata.CameraModel);
        Assert.Equal("K-3 Mark III Ver 1.50", pef.Metadata.Software);
        Assert.Equal("2024:08:11 18:22:09", pef.Metadata.CapturedAtRaw);
        Assert.Equal("Test Photographer", pef.Metadata.Author);
        Assert.Equal("(c) 2024 Test", pef.Metadata.Copyright);
        Assert.True(pef.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
        Assert.Equal("200", pef.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Uncompressed_Rgb_Through_Tiff()
    {
        const int W = 8, H = 4;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 17) & 0xFF);

        var spec = new TestPefBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "PENTAX",
            Model = "K-3 Mark III",
        };
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);
        Assert.True(pef.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in pef.ReadFramesAsync())
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
    public void Reports_Pentax_Compressed_As_CanDecodePixels_False()
    {
        // Compression 65535 = "Pentax PEF compressed" — proprietary, not yet supported.
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
            Compression = 65535,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);

        Assert.False(pef.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
            Compression = 65535,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var pef = PefReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in pef.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => PefReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Pentax_Make_Is_Rejected()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "pentax",
        };
        byte[] bytes = TestPefBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            PefReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, pef.Raw.MakerNoteLength);
    }

    [Fact]
    public void Software_Absent_Field_Is_Null()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(pef.Raw.Software);
    }

    [Fact]
    public void Multiple_SubIfds_All_Surfaced_As_SubImages()
    {
        var sub1 = new TestPefBuilder.IfdSpec
        {
            Width = 8, Height = 6, BitsPerSample = 16, SamplesPerPixel = 1,
            Compression = 1, Photometric = 32803, NewSubFileType = 0,
            StripPayload = new byte[8 * 6 * 2],
        };
        var sub2 = new TestPefBuilder.IfdSpec
        {
            Width = 4, Height = 3, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[4 * 3 * 3],
        };
        var root = new TestPefBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 1,
            StripPayload = new byte[12],
            Make = "PENTAX",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestPefBuilder.Build(root);
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, pef.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 23) & 0xFF);
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? captured = null;
        await foreach (var f in pef.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured) { Assert.Equal(payload, captured!.Pixels.ToArray()); }
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestPefBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "PENTAX",
        };
        byte[] bytes = TestPefBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var pef = PefReader.Open(ms, ownsStream: true);
        pef.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => PefReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestPefBuilder.Build(MinimalPentaxSpec());
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = PefReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Pef, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestPefBuilder.Build(MinimalPentaxSpec());
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        if (!pef.CanDecodePixels) return;
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in pef.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Format_Is_Pef_And_Info_Format_Matches()
    {
        byte[] bytes = TestPefBuilder.Build(MinimalPentaxSpec());
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Pef, pef.Format);
        Assert.Equal(ImageFormat.Pef, pef.Info.Format);
    }

    [Fact]
    public void Info_HasAlpha_False_For_3Channel_Rgb_Strip()
    {
        byte[] bytes = TestPefBuilder.Build(MinimalPentaxSpec());
        using var pef = PefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.False(pef.Info.HasAlpha);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestPefBuilder.Build(MinimalPentaxSpec());
        var r = PefReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    private static TestPefBuilder.IfdSpec MinimalPentaxSpec() => new()
    {
        Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
        Compression = 1, Photometric = 2, NewSubFileType = 0,
        StripPayload = new byte[4 * 4 * 3],
        Make = "PENTAX",
    };
}
