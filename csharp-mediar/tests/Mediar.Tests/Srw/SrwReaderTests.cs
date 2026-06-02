using Mediar.Imaging;
using Mediar.Imaging.Srw;
using Xunit;

namespace Mediar.Tests.Srw;

public sealed class SrwReaderTests
{
    [Fact]
    public void Rejects_File_Without_Samsung_Make_Tag()
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
            Model = "D850",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => SrwReader.Open(ms));
        Assert.Contains("Samsung", ex.Message, StringComparison.Ordinal);
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
        Assert.Throws<ImageFormatException>(() => SrwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => SrwReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => SrwReader.Open(ms));
    }

    [Theory]
    [InlineData("SAMSUNG TECHWIN")]
    [InlineData("SAMSUNG ELECTRONICS CO.,LTD.")]
    [InlineData("SAMSUNG")]
    [InlineData("Samsung")]
    public void Accepts_Samsung_Make_Variants(string make)
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
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);
        Assert.Equal(ImageFormat.Srw, srw.Format);
        Assert.Equal(make, srw.Raw.Make);
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
            Make = "SAMSUNG ELECTRONICS CO.,LTD.",
            Model = "NX1",
            SubIfds = [sub],
        };
        byte[] bytes = TestSrwBuilder.Build(root);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);

        Assert.Equal(2, srw.SubImages.Count);
        Assert.Equal(8, srw.Info.Width);
        Assert.Equal(6, srw.Info.Height);
        Assert.Equal(1, srw.SubImages[1].SubIfdLevel);
    }

    [Fact]
    public void Parses_Samsung_Metadata_Tags()
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
            Make = "SAMSUNG",
            Model = "NX300",
            Software = "Ver 1.20",
            DateTime = "2014:05:20 11:30:00",
            Artist = "Sam Samsung",
            Copyright = "(c) 2014",
            MakerNote = new byte[256],
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);

        Assert.Equal("SAMSUNG", srw.Raw.Make);
        Assert.Equal("NX300", srw.Raw.Model);
        Assert.Equal("Ver 1.20", srw.Raw.Software);
        Assert.Equal("2014:05:20 11:30:00", srw.Raw.DateTime);
        Assert.Equal("Sam Samsung", srw.Raw.Artist);
        Assert.Equal("(c) 2014", srw.Raw.Copyright);
        Assert.Equal(256, srw.Raw.MakerNoteLength);
        Assert.Equal("SAMSUNG", srw.Metadata.CameraMake);
        Assert.Equal("NX300", srw.Metadata.CameraModel);
        Assert.Equal("256", srw.Metadata.Tags["Exif:MakerNoteLength"]);
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
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);
        Assert.True(srw.CanDecodePixels);

        int frameCount = 0;
        await foreach (var frame in srw.ReadFramesAsync())
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
    public void Reports_Samsung_Compressed_As_CanDecodePixels_False()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 16,
            SamplesPerPixel = 1,
            Compression = 32770, // Samsung proprietary compression
            Photometric = 32803, // CFA
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);

        Assert.False(srw.CanDecodePixels);
        Assert.Equal(32770, srw.SubImages[0].CompressionTag);
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
            Compression = 32770,
            Photometric = 32803,
            NewSubFileType = 0,
            StripPayload = new byte[64],
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var srw = SrwReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var frame in srw.ReadFramesAsync())
            {
                frame.Dispose();
            }
        });
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => SrwReader.Open(ms));
    }

    [Fact]
    public void Lowercase_Samsung_Make_Is_Rejected()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "samsung",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() =>
            SrwReader.Open(new MemoryStream(bytes, writable: false)));
    }

    [Fact]
    public void MakerNote_Absent_Length_Is_Zero()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(0, srw.Raw.MakerNoteLength);
    }

    [Fact]
    public void Software_Absent_Field_Is_Null()
    {
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = 2, Height = 2, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = new byte[12],
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Null(srw.Raw.Software);
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
            Make = "SAMSUNG",
            Model = "NX1",
            SubIfds = [sub1, sub2],
        };
        byte[] bytes = TestSrwBuilder.Build(root);
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(3, srw.SubImages.Count);
    }

    [Fact]
    public async Task Multi_Row_Rgb_Strip_Preserved_In_Output()
    {
        int w = 3, h = 3;
        byte[] payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 23) & 0xFF);
        var spec = new TestSrwBuilder.IfdSpec
        {
            Width = w, Height = h, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 1, Photometric = 2, NewSubFileType = 0,
            StripPayload = payload,
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        ImageFrame? captured = null;
        await foreach (var f in srw.ReadFramesAsync()) { captured = f; break; }
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
            Make = "SAMSUNG",
            Model = "NX1",
        };
        byte[] bytes = TestSrwBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var srw = SrwReader.Open(ms, ownsStream: true);
        srw.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => SrwReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalSamsungSpec());
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = SrwReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Srw, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalSamsungSpec());
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        if (!srw.CanDecodePixels) return;
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in srw.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Format_Is_Srw_And_Info_Format_Matches()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalSamsungSpec());
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Srw, srw.Format);
        Assert.Equal(ImageFormat.Srw, srw.Info.Format);
    }

    [Fact]
    public void Info_HasAlpha_False_For_3Channel_Rgb_Strip()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalSamsungSpec());
        using var srw = SrwReader.Open(new MemoryStream(bytes, writable: false));
        Assert.False(srw.Info.HasAlpha);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestSrwBuilder.Build(MinimalSamsungSpec());
        var r = SrwReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    private static TestSrwBuilder.IfdSpec MinimalSamsungSpec() => new()
    {
        Width = 4, Height = 4, BitsPerSample = 8, SamplesPerPixel = 3,
        Compression = 1, Photometric = 2, NewSubFileType = 0,
        StripPayload = new byte[4 * 4 * 3],
        Make = "SAMSUNG",
    };
}
