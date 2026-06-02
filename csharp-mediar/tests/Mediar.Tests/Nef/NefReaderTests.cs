using Mediar.Imaging;
using Mediar.Imaging.Nef;
using Xunit;

namespace Mediar.Tests.Nef;

public sealed class NefReaderTests
{
    [Fact]
    public void Rejects_File_Without_Nikon_Make_Tag()
    {
        // A perfectly valid TIFF whose Make is not Nikon should be rejected.
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "CANON",  // not NIKON
            Model = "EOS-1D",
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => NefReader.Open(ms));
        Assert.Contains("Nikon", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_File_With_No_Make_Tag()
    {
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 2,
            Height = 2,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[2 * 2 * 3],
            // No Make tag.
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => NefReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => NefReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => NefReader.Open(ms));
    }

    [Fact]
    public void Discovers_SubIfd_And_Picks_It_As_Primary()
    {
        // IFD0 = 4x4 RGB thumbnail (NewSubFileType=1).
        // SubIFD = 16x16 Gray16 raw mosaic (NewSubFileType=0).
        var raw = new TestNefBuilder.IfdSpec
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
        var thumb = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            Make = "NIKON CORPORATION",
            Model = "NIKON D850",
            SubIfds = [raw],
        };
        byte[] bytes = TestNefBuilder.Build(thumb);

        using var ms = new MemoryStream(bytes, writable: false);
        using var nef = NefReader.Open(ms);

        Assert.Equal(2, nef.SubImages.Count);
        Assert.Equal(4, nef.SubImages[0].Width);
        Assert.Equal(0, nef.SubImages[0].SubIfdLevel);
        Assert.Equal(1, nef.SubImages[0].NewSubFileType);

        Assert.Equal(16, nef.SubImages[1].Width);
        Assert.Equal(1, nef.SubImages[1].SubIfdLevel);
        Assert.Equal(0, nef.SubImages[1].NewSubFileType);

        // Primary should be the SubIFD (NewSubFileType == 0).
        Assert.Equal(16, nef.Info.Width);
        Assert.Equal(16, nef.Info.Height);
        Assert.Equal(ImageFormat.Nef, nef.Format);
    }

    [Fact]
    public void Parses_Nikon_Metadata_Fields()
    {
        var spec = new TestNefBuilder.IfdSpec
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
            Model = "NIKON D850",
            Software = "Ver.1.20",
            DateTime = "2024:03:15 12:34:56",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            MakerNote = new byte[128],
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var nef = NefReader.Open(ms);

        Assert.Equal("NIKON CORPORATION", nef.Raw.Make);
        Assert.Equal("NIKON D850", nef.Raw.Model);
        Assert.Equal("Ver.1.20", nef.Raw.Software);
        Assert.Equal("2024:03:15 12:34:56", nef.Raw.DateTime);
        Assert.Equal("Test Photographer", nef.Raw.Artist);
        Assert.Equal("(c) 2024 Test", nef.Raw.Copyright);
        Assert.Equal(128, nef.Raw.MakerNoteLength);

        // ImageMetadata projection.
        Assert.Equal("NIKON CORPORATION", nef.Metadata.CameraMake);
        Assert.Equal("NIKON D850", nef.Metadata.CameraModel);
        Assert.Equal("Ver.1.20", nef.Metadata.Software);
        Assert.Equal("2024:03:15 12:34:56", nef.Metadata.CapturedAtRaw);
        Assert.Equal("Test Photographer", nef.Metadata.Author);
        Assert.Equal("(c) 2024 Test", nef.Metadata.Copyright);
        Assert.True(nef.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
        Assert.Equal("128", nef.Metadata.Tags["Exif:MakerNoteLength"]);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Uncompressed_Rgb_Through_Tiff()
    {
        const int W = 8, H = 4;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 11) & 0xFF);

        var spec = new TestNefBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "NIKON CORPORATION",
            Model = "NIKON D850",
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var nef = NefReader.Open(ms);
        Assert.True(nef.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in nef.ReadFramesAsync())
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
    public void Reports_Unsupported_Compression_As_CanDecodePixels_False()
    {
        // Compression 34713 = "Nikon NEF compressed" — proprietary, not yet supported.
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 14,
            SamplesPerPixel = 1,
            Compression = 34713,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "NIKON CORPORATION",
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var nef = NefReader.Open(ms);

        Assert.False(nef.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 14,
            SamplesPerPixel = 1,
            Compression = 34713,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            Make = "NIKON CORPORATION",
        };
        byte[] bytes = TestNefBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var nef = NefReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in nef.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Mixed_Case_Nikon_Make_Is_Accepted()
    {
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "Nikon Imaging",
        };
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal("Nikon Imaging", nef.Raw.Make);
    }

    [Fact]
    public void Lowercase_Make_Is_Rejected()
    {
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            Make = "nikon",
        };
        byte[] bytes = TestNefBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => NefReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => NefReader.Open(ms));
    }

    [Fact]
    public void Without_MakerNote_MakerNoteLength_Is_Zero()
    {
        var spec = new TestNefBuilder.IfdSpec
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
        };
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, nef.Raw.MakerNoteLength);
        Assert.False(nef.Metadata.Tags.ContainsKey("Exif:MakerNoteLength"));
    }

    [Fact]
    public void Without_Software_Field_Metadata_Software_Is_Null()
    {
        var spec = new TestNefBuilder.IfdSpec
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
        };
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(nef.Raw.Software);
        Assert.Null(nef.Metadata.Software);
    }

    [Fact]
    public async Task ReadFramesAsync_Multi_Row_RGB_Preserves_All_Pixels()
    {
        const int W = 6, H = 5;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 17) ^ 0x5A);
        var spec = new TestNefBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            Make = "NIKON CORPORATION",
        };
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in nef.ReadFramesAsync()) { frame = f; break; }
        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(strip, frame.Pixels.ToArray());
        }
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => NefReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        var ms = new MemoryStream(bytes, writable: false);
        using (var r = NefReader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Nef, r.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = NefReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Nef, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token_For_Decodable_File()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.True(nef.CanDecodePixels);
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in nef.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Format_Is_Nef_And_Info_Format_Matches()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Nef, nef.Format);
        Assert.Equal(ImageFormat.Nef, nef.Info.Format);
    }

    [Fact]
    public void Info_HasAlpha_Is_False_For_Rgb_Strip()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.False(nef.Info.HasAlpha);
    }

    [Fact]
    public void Open_Then_Dispose_Twice_Does_Not_Throw()
    {
        var spec = MinimalNikonSpec();
        byte[] bytes = TestNefBuilder.Build(spec);
        var r = NefReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    private static TestNefBuilder.IfdSpec MinimalNikonSpec() => new()
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
    };

    [Fact]
    public void NewSubFileType_Two_Treated_As_Non_Primary()
    {
        // NewSubFileType bit 1 = reduced-resolution preview, bit 0 = thumbnail.
        // Value 2 means "single page of multi-page" - should not be picked
        // as primary when a NewSubFileType=0 SubIFD is present.
        var raw = new TestNefBuilder.IfdSpec
        {
            Width = 20,
            Height = 20,
            BitsPerSample = 8,
            SamplesPerPixel = 1,
            Compression = 1,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[20 * 20],
        };
        var ifd0 = new TestNefBuilder.IfdSpec
        {
            Width = 8,
            Height = 8,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 2,
            StripPayload = new byte[8 * 8 * 3],
            Make = "NIKON CORPORATION",
            SubIfds = [raw],
        };
        byte[] bytes = TestNefBuilder.Build(ifd0);
        using var nef = NefReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(20, nef.Info.Width);
        Assert.Equal(20, nef.Info.Height);
    }
}
