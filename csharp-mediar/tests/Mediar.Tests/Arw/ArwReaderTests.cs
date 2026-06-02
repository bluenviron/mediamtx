using Mediar.Imaging;
using Mediar.Imaging.Arw;
using Xunit;

namespace Mediar.Tests.Arw;

public sealed class ArwReaderTests
{
    [Fact]
    public void Rejects_File_Without_Sony_Make_Tag()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "NIKON CORPORATION",  // not SONY
            Model = "D850",
        };
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => ArwReader.Open(ms));
        Assert.Contains("Sony", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestArwBuilder.IfdSpec
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
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => ArwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => ArwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => ArwReader.Open(ms));
    }

    [Fact]
    public void Discovers_SubIfd_And_Picks_It_As_Primary()
    {
        var raw = new TestArwBuilder.IfdSpec
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
        var thumb = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            Make = "SONY",
            Model = "ILCE-7M4",
            SubIfds = [raw],
        };
        byte[] bytes = TestArwBuilder.Build(thumb);

        using var ms = new MemoryStream(bytes, writable: false);
        using var arw = ArwReader.Open(ms);

        Assert.Equal(2, arw.SubImages.Count);
        Assert.Equal(4, arw.SubImages[0].Width);
        Assert.Equal(0, arw.SubImages[0].SubIfdLevel);
        Assert.Equal(1, arw.SubImages[0].NewSubFileType);

        Assert.Equal(16, arw.SubImages[1].Width);
        Assert.Equal(1, arw.SubImages[1].SubIfdLevel);
        Assert.Equal(0, arw.SubImages[1].NewSubFileType);

        Assert.Equal(16, arw.Info.Width);
        Assert.Equal(16, arw.Info.Height);
        Assert.Equal(ImageFormat.Arw, arw.Format);
    }

    [Fact]
    public void Parses_Sony_Metadata_Fields()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "SONY",
            Model = "ILCE-7M4",
            Software = "ILCE-7M4 v1.10",
            DateTime = "2024:03:15 12:34:56",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            MakerNote = new byte[256],
        };
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var arw = ArwReader.Open(ms);

        Assert.Equal("SONY", arw.Raw.Make);
        Assert.Equal("ILCE-7M4", arw.Raw.Model);
        Assert.Equal("ILCE-7M4 v1.10", arw.Raw.Software);
        Assert.Equal("2024:03:15 12:34:56", arw.Raw.DateTime);
        Assert.Equal("Test Photographer", arw.Raw.Artist);
        Assert.Equal("(c) 2024 Test", arw.Raw.Copyright);
        Assert.Equal(256, arw.Raw.MakerNoteLength);

        Assert.Equal("SONY", arw.Metadata.CameraMake);
        Assert.Equal("ILCE-7M4", arw.Metadata.CameraModel);
        Assert.Equal("ILCE-7M4 v1.10", arw.Metadata.Software);
        Assert.Equal("2024:03:15 12:34:56", arw.Metadata.CapturedAtRaw);
        Assert.Equal("Test Photographer", arw.Metadata.Author);
        Assert.Equal("(c) 2024 Test", arw.Metadata.Copyright);
        Assert.True(arw.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
        Assert.Equal("256", arw.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Uncompressed_Rgb_Through_Tiff()
    {
        const int W = 8, H = 4;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 13) & 0xFF);

        var spec = new TestArwBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "SONY",
            Model = "ILCE-7M4",
        };
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var arw = ArwReader.Open(ms);
        Assert.True(arw.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in arw.ReadFramesAsync())
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
    public void Reports_Unsupported_Sony_Compression_As_CanDecodePixels_False()
    {
        // Compression 32767 = "Sony ARW v1 8-bit packed" — proprietary, not yet supported.
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
            Compression = 32767,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "SONY",
        };
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var arw = ArwReader.Open(ms);

        Assert.False(arw.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        // Compression 32769 = "Sony ARW v2 lossless" — proprietary, not yet supported.
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 14,
            SamplesPerPixel = 1,
            Compression = 32769,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "SONY",
        };
        byte[] bytes = TestArwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var arw = ArwReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in arw.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Mixed_Case_Sony_Make_Is_Accepted()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "Sony Corporation",
        };
        byte[] bytes = TestArwBuilder.Build(spec);
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal("Sony Corporation", arw.Raw.Make);
    }

    [Fact]
    public void Lowercase_Make_Is_Rejected()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "sony",
        };
        byte[] bytes = TestArwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => ArwReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => ArwReader.Open(ms));
    }

    [Fact]
    public void Without_MakerNote_MakerNoteLength_Is_Zero()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "SONY",
        };
        byte[] bytes = TestArwBuilder.Build(spec);
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, arw.Raw.MakerNoteLength);
        Assert.False(arw.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
    }

    [Fact]
    public void Without_Software_Field_Metadata_Software_Is_Null()
    {
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "SONY",
        };
        byte[] bytes = TestArwBuilder.Build(spec);
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(arw.Raw.Software);
        Assert.Null(arw.Metadata.Software);
    }

    [Fact]
    public async Task ReadFramesAsync_Multi_Row_RGB_Preserves_All_Pixels()
    {
        const int W = 5, H = 6;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 19) ^ 0xA5);
        var spec = new TestArwBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "SONY",
        };
        byte[] bytes = TestArwBuilder.Build(spec);
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in arw.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(strip, frame.Pixels.ToArray());
        }
    }

    [Fact]
    public void Multiple_SubIfds_All_Discovered_As_SubImages()
    {
        var raw1 = new TestArwBuilder.IfdSpec
        {
            Width = 12,
            Height = 12,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 1,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[12 * 12 * 2],
        };
        var raw2 = new TestArwBuilder.IfdSpec
        {
            Width = 6,
            Height = 6,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[6 * 6 * 3],
        };
        var ifd0 = new TestArwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            Make = "SONY",
            SubIfds = [raw1, raw2],
        };
        byte[] bytes = TestArwBuilder.Build(ifd0);
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, arw.SubImages.Count);
        // Primary should still be the largest with NewSubFileType=0 -> raw1 (12x12).
        Assert.Equal(12, arw.Info.Width);
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => ArwReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        var ms = new MemoryStream(bytes, writable: false);
        using (var r = ArwReader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Arw, r.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = ArwReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Arw, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.True(arw.CanDecodePixels);
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in arw.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Format_Is_Arw_And_Info_Format_Matches()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Arw, arw.Format);
        Assert.Equal(ImageFormat.Arw, arw.Info.Format);
    }

    [Fact]
    public void Info_HasAlpha_False_For_3Channel_Rgb_Strip()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        using var arw = ArwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.False(arw.Info.HasAlpha);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestArwBuilder.Build(MinimalSonySpec());
        var r = ArwReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    private static TestArwBuilder.IfdSpec MinimalSonySpec() => new()
    {
        Width = 4,
        Height = 4,
        BitsPerSample = 8,
        SamplesPerPixel = 3,
        Compression = 1,
        Photometric = 2,
        NewSubFileType = 0,
        StripPayload = new byte[4 * 4 * 3],
        Make = "SONY",
    };
}
