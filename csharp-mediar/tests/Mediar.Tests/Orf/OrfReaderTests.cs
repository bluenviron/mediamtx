using Mediar.Imaging;
using Mediar.Imaging.Orf;
using Xunit;

namespace Mediar.Tests.Orf;

/// <summary>
/// Tests for <see cref="OrfReader"/>, covering Olympus magic
/// variants (0x002A standard TIFF + Make tag, 0x4F52 'RO',
/// 0x5253 'RS'), EXIF metadata parsing, sub-image discovery and
/// pixel decode through <see cref="Mediar.Imaging.Tiff.TiffReader"/>.
/// </summary>
public sealed class OrfReaderTests
{
    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49, 0x52];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0xAB, 0xCD, 0x52, 0x4F, 0x00, 0x00, 0x00, 0x08];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Magic_Word()
    {
        // II + 0x1234 - neither TIFF nor any known Olympus variant.
        byte[] bytes = [0x49, 0x49, 0x34, 0x12, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(ms));
    }

    [Fact]
    public void Rejects_Standard_Tiff_With_Non_Olympus_Make()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x002A,
            Make = "NIKON CORPORATION",
            TiffWidth = 4000,
            TiffHeight = 3000,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(ms));
    }

    [Theory]
    [InlineData((ushort)0x4F52, "OLYMPUS IMAGING CORP.")]
    [InlineData((ushort)0x5352, "OLYMPUS CORPORATION")]
    [InlineData((ushort)0x4F52, "OLYMPUS OPTICAL CO.,LTD")]
    [InlineData((ushort)0x4F52, "OM Digital Solutions")]
    [InlineData((ushort)0x002A, "OLYMPUS IMAGING CORP.")]
    public void Accepts_All_Known_Olympus_Variants(ushort magic, string make)
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = magic,
            Make = make,
            Model = "E-M1 MarkII",
            TiffWidth = 4640,
            TiffHeight = 3472,
            BitsPerSample = 12,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));

        Assert.Equal(ImageFormat.Orf, orf.Format);
        Assert.Equal(magic, orf.Orf.OlympusMagic);
        Assert.Equal(make, orf.Orf.Make);
        Assert.Equal("E-M1 MarkII", orf.Orf.Model);
    }

    [Fact]
    public void Parses_Olympus_Metadata_Tags()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OM Digital Solutions",
            Model = "OM-1",
            Software = "OM-1 Firmware Ver.1.4",
            DateTime = "2024:08:12 14:30:00",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            MakerNote = new byte[128],
            TiffWidth = 5184,
            TiffHeight = 3888,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));

        Assert.Equal("OM Digital Solutions", orf.Orf.Make);
        Assert.Equal("OM-1", orf.Orf.Model);
        Assert.Equal("OM-1 Firmware Ver.1.4", orf.Orf.Software);
        Assert.Equal("2024:08:12 14:30:00", orf.Orf.DateTime);
        Assert.Equal("Test Photographer", orf.Orf.Artist);
        Assert.Equal("(c) 2024 Test", orf.Orf.Copyright);
        Assert.Equal(128, orf.Orf.MakerNoteLength);

        Assert.Equal("OM Digital Solutions", orf.Metadata.CameraMake);
        Assert.Equal("OM-1", orf.Metadata.CameraModel);
    }

    [Fact]
    public async Task Decodes_Uncompressed_Rgb_Through_Tiff_Reader()
    {
        int w = 8, h = 4;
        byte[] strip = new byte[w * h * 3];
        for (int i = 0; i < strip.Length; i += 3)
        {
            strip[i] = 0x00;
            strip[i + 1] = 0xFF;
            strip[i + 2] = 0x00;
        }
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = (uint)w,
            TiffHeight = (uint)h,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Photometric = 2,
            Compression = 1,
            IncludeStripData = true,
            StripBytes = strip,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));

        Assert.True(orf.CanDecodePixels);
        ImageFrame? frame = null;
        await foreach (var f in orf.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(w, frame.Width);
            Assert.Equal(h, frame.Height);
            Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);
            var pixels = frame.Pixels.Span;
            Assert.Equal(0x00, pixels[0]);
            Assert.Equal(0xFF, pixels[1]);
            Assert.Equal(0x00, pixels[2]);
        }
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Primary_Uses_12bit_Mosaic()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 4640,
            TiffHeight = 3472,
            BitsPerSample = 12,
            SamplesPerPixel = 1,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));

        Assert.False(orf.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in orf.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Detector_Recognises_Olympus_Magic_Variants()
    {
        // IIRO: bytes 'I','I','R','O' = [0x49, 0x49, 0x52, 0x4F]
        byte[] ro = [0x49, 0x49, 0x52, 0x4F, 0x08, 0x00, 0x00, 0x00];
        Assert.Equal(ImageFormat.Orf, ImageFormatDetector.Detect(ro));
        // IIRS: bytes 'I','I','R','S' = [0x49, 0x49, 0x52, 0x53]
        byte[] rs = [0x49, 0x49, 0x52, 0x53, 0x08, 0x00, 0x00, 0x00];
        Assert.Equal(ImageFormat.Orf, ImageFormatDetector.Detect(rs));
        // MMOR: bytes 'M','M','O','R' = [0x4D, 0x4D, 0x4F, 0x52]
        byte[] orBe = [0x4D, 0x4D, 0x4F, 0x52, 0x00, 0x00, 0x00, 0x08];
        Assert.Equal(ImageFormat.Orf, ImageFormatDetector.Detect(orBe));
    }

    [Fact]
    public void Records_Olympus_Magic_In_Tags()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 4640,
            TiffHeight = 3472,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Equal("0x4F52", orf.Metadata.Tags["ORF:Magic"]);
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(ms));
    }

    [Fact]
    public void Without_Software_Tag_Field_Is_Null()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            Model = "E-M1",
            TiffWidth = 100,
            TiffHeight = 100,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Null(orf.Orf.Software);
    }

    [Fact]
    public void Without_MakerNote_Length_Is_Zero()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 100,
            TiffHeight = 100,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Equal(0, orf.Orf.MakerNoteLength);
    }

    [Fact]
    public void SubImages_Has_At_Least_Primary_Entry()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 4640,
            TiffHeight = 3472,
            BitsPerSample = 12,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.NotEmpty(orf.SubImages);
    }

    [Theory]
    [InlineData((ushort)0x4F52, "0x4F52")]
    [InlineData((ushort)0x5352, "0x5352")]
    public void Magic_Tag_String_Matches_Hex_Representation(ushort magic, string expected)
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = magic,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 100,
            TiffHeight = 100,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Equal(expected, orf.Metadata.Tags["ORF:Magic"]);
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestOrfBuilder.OrfSpec
        {
            Magic = 0x4F52,
            Make = "OLYMPUS IMAGING CORP.",
            TiffWidth = 100,
            TiffHeight = 100,
        };
        byte[] bytes = TestOrfBuilder.Build(spec);
        var ms = new MemoryStream(bytes);
        var orf = OrfReader.Open(ms, ownsStream: true);
        orf.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Format_Detector_Recognises_BigEndian_Variant()
    {
        // MMOR: bytes 'M','M','O','R' big-endian Olympus.
        byte[] orBe = [0x4D, 0x4D, 0x4F, 0x52, 0x00, 0x00, 0x00, 0x08];
        Assert.Equal(ImageFormat.Orf, ImageFormatDetector.Detect(orBe));
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => OrfReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestOrfBuilder.Build(MinimalOlympusSpec());
        using var ms = new MemoryStream(bytes);
        using (var r = OrfReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Orf, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public void Format_Property_Is_Orf()
    {
        byte[] bytes = TestOrfBuilder.Build(MinimalOlympusSpec());
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Equal(ImageFormat.Orf, orf.Format);
        Assert.Equal(ImageFormat.Orf, orf.Info.Format);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestOrfBuilder.Build(MinimalOlympusSpec());
        var r = OrfReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Bytes_Less_Than_Two_Throws_ImageFormatException()
    {
        Assert.Throws<ImageFormatException>(() => OrfReader.Open(new MemoryStream([0x49])));
    }

    [Fact]
    public void Make_Tag_Is_Surfaced_In_Raw_Metadata()
    {
        var spec = MinimalOlympusSpec() with { Model = "E-M1 Mark III" };
        byte[] bytes = TestOrfBuilder.Build(spec);
        using var orf = OrfReader.Open(new MemoryStream(bytes));
        Assert.Equal("OLYMPUS IMAGING CORP.", orf.Orf.Make);
        Assert.Equal("E-M1 Mark III", orf.Orf.Model);
    }

    private static TestOrfBuilder.OrfSpec MinimalOlympusSpec() => new()
    {
        Magic = 0x4F52,
        Make = "OLYMPUS IMAGING CORP.",
        TiffWidth = 100,
        TiffHeight = 100,
    };

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => OrfReader.Open((string)null!));
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_PreCancelled_Token()
    {
        byte[] bytes = TestOrfBuilder.Build(MinimalOlympusSpec());
        using var r = OrfReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        if (!r.CanDecodePixels) return;
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token))
            {
                f.Dispose();
            }
        });
    }
}
