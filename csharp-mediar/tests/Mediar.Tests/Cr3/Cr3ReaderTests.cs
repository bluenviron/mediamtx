using Mediar.Imaging;
using Mediar.Imaging.Cr3;
using Xunit;

namespace Mediar.Tests.Cr3;

/// <summary>
/// Tests for <see cref="Cr3Reader"/>, covering ftyp brand validation,
/// Canon UUID box discovery, CMT1 TIFF IFD parsing, THMB / PRVW JPEG
/// sub-image extraction and end-to-end JPEG decode delegation.
/// </summary>
public sealed class Cr3ReaderTests
{
    // Tiny 16x16 solid-red baseline JPEG fixture (same as other RAW tests).
    private const string RedJpegBase64 =
        "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAQCAwMDAgQDAwMEBAQEBQkGBQUFBQsICAYJDQsNDQ0LDAwOEBQRDg8TDwwMEhgSExUWFxcXDhEZGxkWGhQWFxb/" +
        "2wBDAQQEBAUFBQoGBgoWDwwPFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhb/wAARCAAQABADASIAAhEBAxEB/8QA" +
        "HwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2Jyggk" +
        "KFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMX" +
        "Gx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAEC" +
        "AxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOE" +
        "hYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxeiiivyk/" +
        "v4//2Q==";

    private static byte[] LoadRedJpeg() => Convert.FromBase64String(RedJpegBase64);

    [Fact]
    public void Rejects_Truncated_File()
    {
        byte[] tiny = [0x00, 0x00, 0x00];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Missing_Ftyp()
    {
        // A box that isn't 'ftyp'.
        byte[] bytes =
        [
            0x00, 0x00, 0x00, 0x10, // size = 16
            (byte)'m', (byte)'d', (byte)'a', (byte)'t',
            0, 0, 0, 0, 0, 0, 0, 0,
        ];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Wrong_Brand()
    {
        var spec = new TestCr3Builder.Cr3Spec { Brand = "heic" };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr3Reader.Open(ms));
    }

    [Fact]
    public void Parses_Ftyp_Brand_And_Compatible_Brands()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Brand = "crx ",
            CompatibleBrands = ["crx ", "isom", "mp41"],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Equal(ImageFormat.Cr3, cr3.Format);
        Assert.Equal("crx ", cr3.Cr3.MajorBrand);
        Assert.Equal(3, cr3.Cr3.CompatibleBrands.Count);
        Assert.Contains("isom", cr3.Cr3.CompatibleBrands);
        Assert.Contains("mp41", cr3.Cr3.CompatibleBrands);
    }

    [Fact]
    public void Parses_Canon_Cmt1_Tiff_Tags()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Model = "Canon EOS R5",
            Software = "EOS R5 Firmware 2.0.1",
            DateTime = "2024:08:11 18:22:09",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCanonUuid);
        Assert.True(cr3.Cr3.HasCmt1);
        Assert.Equal("Canon", cr3.Cr3.Make);
        Assert.Equal("Canon EOS R5", cr3.Cr3.Model);
        Assert.Equal("EOS R5 Firmware 2.0.1", cr3.Cr3.Software);
        Assert.Equal("2024:08:11 18:22:09", cr3.Cr3.DateTime);
        Assert.Equal("Test Photographer", cr3.Cr3.Artist);
        Assert.Equal("(c) 2024 Test", cr3.Cr3.Copyright);

        Assert.Equal("Canon", cr3.Metadata.CameraMake);
        Assert.Equal("Canon EOS R5", cr3.Metadata.CameraModel);
    }

    [Fact]
    public void Discovers_Thumbnail_SubImage()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        var thumb = cr3.SubImages.Single(s => s.Kind == Cr3SubImageKind.Thumbnail);
        Assert.Equal(16, thumb.Width);
        Assert.Equal(16, thumb.Height);
        Assert.True(thumb.CanDecodePixels);
        Assert.True(cr3.CanDecodePixels);
    }

    [Fact]
    public void Discovers_Preview_And_Picks_It_As_Primary_Over_Thumbnail()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
            PrvwJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Contains(cr3.SubImages, s => s.Kind == Cr3SubImageKind.Thumbnail);
        Assert.Contains(cr3.SubImages, s => s.Kind == Cr3SubImageKind.Preview);
        Assert.True(cr3.CanDecodePixels);
        // Both JPEGs are 16x16 in this synthetic test, so primary tie-break picks one;
        // the important property is that Info reflects the largest sub-image.
        Assert.Equal(16, cr3.Info.Width);
        Assert.Equal(16, cr3.Info.Height);
    }

    [Fact]
    public void Mdat_SurfacedAs_RawMosaic_SubImage_Undecodable()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
            MdatPayload = new byte[1024],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        var raw = cr3.SubImages.Single(s => s.Kind == Cr3SubImageKind.RawMosaic);
        Assert.Equal(1024, raw.Length);
        Assert.False(raw.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Thumbnail_When_Only_Thumbnail_Present()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        ImageFrame? frame = null;
        await foreach (var f in cr3.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        using (frame)
        {
            Assert.Equal(16, frame.Width);
            Assert.Equal(16, frame.Height);
            Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);
        }
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_No_Decodable_SubImage_Present()
    {
        // CR3 with only an mdat raw payload — no JPEG sub-images at all.
        var spec = new TestCr3Builder.Cr3Spec
        {
            MdatPayload = new byte[64],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));
        Assert.False(cr3.CanDecodePixels);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in cr3.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Cmt2_ExifSubIfd_Parses_ExposureTime_FNumber_Iso_FocalLength()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Model = "Canon EOS R5",
            ExposureTime = (1, 250),       // 1/250 s
            FNumber = (40, 10),            // f/4.0
            IsoSpeedRatings = 800,
            FocalLength = (50, 1),         // 50 mm
            DateTimeOriginal = "2024:11:22 14:30:00",
            DateTimeDigitized = "2024:11:22 14:30:01",
            ExposureBiasValue = (-1, 3),   // -0.333 EV
            LensModel = "RF 50mm F1.2 L USM",
            LensMake = "Canon",
            Flash = 16,                    // off
            MeteringMode = 5,
            ExposureProgram = 3,
            WhiteBalance = 0,
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt2);
        Assert.NotNull(cr3.Cr3.Exif);
        var exif = cr3.Cr3.Exif!;
        Assert.Equal(1.0 / 250.0, exif.ExposureTimeSeconds);
        Assert.Equal(4.0, exif.FNumber);
        Assert.Equal((ushort)800, exif.IsoSpeedRatings);
        Assert.Equal(50.0, exif.FocalLengthMm);
        Assert.Equal("2024:11:22 14:30:00", exif.DateTimeOriginal);
        Assert.Equal("2024:11:22 14:30:01", exif.DateTimeDigitized);
        Assert.Equal(-1.0 / 3.0, exif.ExposureBiasValue!.Value, precision: 6);
        Assert.Equal("RF 50mm F1.2 L USM", exif.LensModel);
        Assert.Equal("Canon", exif.LensMake);
        Assert.Equal((ushort)16, exif.Flash);
        Assert.Equal((ushort)5, exif.MeteringMode);
        Assert.Equal((ushort)3, exif.ExposureProgram);

        // ImageMetadata bridge fields populated from Cmt2 take precedence
        // over Cmt1 DateTime.
        Assert.Equal("2024:11:22 14:30:00", cr3.Metadata.CapturedAtRaw);
        Assert.Equal(1.0 / 250.0, cr3.Metadata.ExposureTimeSeconds);
        Assert.Equal(4.0, cr3.Metadata.FNumber);
        Assert.Equal(800, cr3.Metadata.IsoSpeed);
        Assert.Equal(50.0, cr3.Metadata.FocalLengthMm);
        Assert.Equal("RF 50mm F1.2 L USM", cr3.Metadata.LensModel);

        Assert.Equal("1", cr3.Metadata.Tags["CR3:HasCmt2"]);
        Assert.Equal("800", cr3.Metadata.Tags["Exif:ISOSpeedRatings"]);
        Assert.Equal("RF 50mm F1.2 L USM", cr3.Metadata.Tags["Exif:LensModel"]);
    }

    [Fact]
    public void Cmt4_GpsIfd_Parses_Coordinates_Altitude_TimeStamp()
    {
        // Latitude 37° 25' 50.4" N, longitude 122° 5' 10.8" W, altitude 30.5 m.
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Model = "Canon EOS R5",
            GpsLatitudeRef = "N",
            GpsLatitudeDms = (37, 1, 25, 1, 504, 10),
            GpsLongitudeRef = "W",
            GpsLongitudeDms = (122, 1, 5, 1, 108, 10),
            GpsAltitudeRef = 0,
            GpsAltitude = (305, 10),
            GpsTimeStampHms = (14, 1, 30, 1, 15, 1),
            GpsDateStamp = "2024:11:22",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt4);
        var gps = cr3.Cr3.Gps;
        Assert.NotNull(gps);

        Assert.NotNull(gps!.LatitudeDegrees);
        Assert.Equal(37.43066666, gps.LatitudeDegrees!.Value, precision: 4);
        Assert.NotNull(gps.LongitudeDegrees);
        Assert.Equal(-122.086333333, gps.LongitudeDegrees!.Value, precision: 4);
        Assert.Equal(30.5, gps.AltitudeMeters);
        Assert.Equal("N", gps.LatitudeRef);
        Assert.Equal("W", gps.LongitudeRef);
        Assert.Equal("14:30:15", gps.TimeStampUtc);
        Assert.Equal("2024:11:22", gps.DateStamp);

        // ImageMetadata bridge fields.
        Assert.Equal(gps.LatitudeDegrees, cr3.Metadata.GpsLatitude);
        Assert.Equal(gps.LongitudeDegrees, cr3.Metadata.GpsLongitude);
        Assert.Equal(30.5, cr3.Metadata.GpsAltitudeMeters);

        Assert.Equal("1", cr3.Metadata.Tags["CR3:HasCmt4"]);
        Assert.Equal("W", cr3.Metadata.Tags["Gps:LongitudeRef"]);
        Assert.Equal("2024:11:22", cr3.Metadata.Tags["Gps:DateStamp"]);
    }

    [Fact]
    public void Cmt4_Gps_Below_Sea_Level_Has_Negative_Altitude()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            GpsAltitudeRef = 1, // below sea level
            GpsAltitude = (12, 1), // magnitude 12 m
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));
        Assert.True(cr3.Cr3.HasCmt4);
        Assert.Equal(-12.0, cr3.Cr3.Gps!.AltitudeMeters);
    }

    [Fact]
    public void Cmt4_Gps_Southern_And_Eastern_Hemisphere_Signs()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            GpsLatitudeRef = "S",
            GpsLatitudeDms = (33, 1, 51, 1, 30, 1),
            GpsLongitudeRef = "E",
            GpsLongitudeDms = (151, 1, 12, 1, 30, 1),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));
        var gps = cr3.Cr3.Gps!;
        Assert.True(gps.LatitudeDegrees < 0);
        Assert.True(gps.LongitudeDegrees > 0);
    }

    [Fact]
    public void Cmt3_RawMakerNote_Surfaces_HasFlag_And_Length()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Cmt3RawPayload = new byte[1024],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt3);
        Assert.Equal(1024, cr3.Cr3.Cmt3ByteLength);
        Assert.Equal("1", cr3.Metadata.Tags["CR3:HasCmt3"]);
        Assert.Equal("1024", cr3.Metadata.Tags["CR3:MakerNoteLength"]);
    }

    [Fact]
    public void Cmt2_And_Cmt4_Coexist_With_Cmt1_In_Same_Canon_Uuid()
    {
        // Combined test: CMT1 baseline + CMT2 EXIF + CMT4 GPS in one file.
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Model = "Canon EOS R6 Mark II",
            DateTime = "2024:11:22 14:00:00",
            ExposureTime = (1, 60),
            FNumber = (28, 10),
            IsoSpeedRatings = 400,
            FocalLength = (35, 1),
            DateTimeOriginal = "2024:11:22 14:30:00",
            GpsLatitudeRef = "N",
            GpsLatitudeDms = (48, 1, 51, 1, 24, 1),
            GpsLongitudeRef = "E",
            GpsLongitudeDms = (2, 1, 21, 1, 6, 1),
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt1);
        Assert.True(cr3.Cr3.HasCmt2);
        Assert.True(cr3.Cr3.HasCmt4);
        Assert.False(cr3.Cr3.HasCmt3);

        Assert.Equal("Canon EOS R6 Mark II", cr3.Cr3.Model);
        Assert.Equal(2.8, cr3.Cr3.Exif!.FNumber);
        Assert.True(cr3.Cr3.Gps!.LatitudeDegrees > 48 && cr3.Cr3.Gps.LatitudeDegrees < 49);
    }

    [Fact]
    public void Cmt2_With_No_Fields_Set_Is_Not_Emitted()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt1);
        Assert.False(cr3.Cr3.HasCmt2);
        Assert.False(cr3.Cr3.HasCmt4);
        Assert.Null(cr3.Cr3.Exif);
        Assert.Null(cr3.Cr3.Gps);
    }

    [Fact]
    public void Cmt3_Typed_MakerNote_Parses_ImageType_Firmware_Owner()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            CanonImageType = "Canon EOS R5",
            CanonFirmwareRevision = "Firmware Version 1.6.0",
            CanonOwnerName = "J. Doe",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt3);
        Assert.NotNull(cr3.Cr3.MakerNote);
        Assert.Equal("Canon EOS R5", cr3.Cr3.MakerNote!.ImageType);
        Assert.Equal("Firmware Version 1.6.0", cr3.Cr3.MakerNote.FirmwareRevision);
        Assert.Equal("J. Doe", cr3.Cr3.MakerNote.OwnerName);

        Assert.Equal("Canon EOS R5", cr3.Metadata.Tags["Canon:ImageType"]);
        Assert.Equal("Firmware Version 1.6.0", cr3.Metadata.Tags["Canon:FirmwareRevision"]);
        Assert.Equal("J. Doe", cr3.Metadata.Tags["Canon:OwnerName"]);
    }

    [Fact]
    public void Cmt3_Typed_MakerNote_Parses_SerialNumber_And_ModelId()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            CanonSerialNumber = 123456789u,
            CanonModelId = 0x80000453u, // EOS R5 ID per Canon's body table.
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt3);
        Assert.Equal(123456789u, cr3.Cr3.MakerNote!.SerialNumber);
        Assert.Equal(0x80000453u, cr3.Cr3.MakerNote.ModelId);

        Assert.Equal("123456789", cr3.Metadata.Tags["Canon:SerialNumber"]);
        Assert.Equal("0x80000453", cr3.Metadata.Tags["Canon:ModelID"]);
    }

    [Fact]
    public void Cmt3_LensModel_Overrides_Cmt2_LensModel_In_ImageMetadata()
    {
        // Canon's MakerNote LensModel is the authoritative full string;
        // EXIF's LensModel often carries only an identifier. When both are
        // present the more authoritative MakerNote value must win.
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            LensModel = "EF24-105mm",
            CanonLensModel = "RF24-105mm F4 L IS USM",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Equal("EF24-105mm", cr3.Cr3.Exif!.LensModel);
        Assert.Equal("RF24-105mm F4 L IS USM", cr3.Cr3.MakerNote!.LensModel);
        Assert.Equal("RF24-105mm F4 L IS USM", cr3.Metadata.LensModel);
    }

    [Fact]
    public void Cmt3_InternalSerialNumber_Is_Surfaced()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            CanonInternalSerialNumber = "XB1234567890",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.Equal("XB1234567890", cr3.Cr3.MakerNote!.InternalSerialNumber);
        Assert.Equal("XB1234567890", cr3.Metadata.Tags["Canon:InternalSerialNumber"]);
    }

    [Fact]
    public void Cmt3_Raw_Payload_Still_Sets_Length_But_No_Typed_Values()
    {
        // Raw bytes path: HasCmt3 + byte length flow through but no typed
        // MakerNote fields are parsed (because the bytes aren't a TIFF stream).
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
            Cmt3RawPayload = new byte[256],
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.True(cr3.Cr3.HasCmt3);
        Assert.Equal(256, cr3.Cr3.Cmt3ByteLength);
        Assert.NotNull(cr3.Cr3.MakerNote);
        Assert.Null(cr3.Cr3.MakerNote!.ImageType);
        Assert.Null(cr3.Cr3.MakerNote.SerialNumber);
        Assert.DoesNotContain("Canon:ImageType", cr3.Metadata.Tags.Keys);
    }

    [Fact]
    public void Cmt3_Without_Any_Tags_Yields_HasCmt3_False()
    {
        var spec = new TestCr3Builder.Cr3Spec
        {
            Make = "Canon",
        };
        byte[] bytes = TestCr3Builder.Build(spec);
        using var cr3 = Cr3Reader.Open(new MemoryStream(bytes));

        Assert.False(cr3.Cr3.HasCmt3);
        Assert.Null(cr3.Cr3.MakerNote);
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => Cr3Reader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => Cr3Reader.Open((string)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] bytes = TestCr3Builder.Build(new TestCr3Builder.Cr3Spec());
        var ms = new MemoryStream(bytes, writable: false);
        using (var r = Cr3Reader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Cr3, r.Format);
        }
        Assert.False(ms.CanRead);
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestCr3Builder.Build(new TestCr3Builder.Cr3Spec());
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = Cr3Reader.Open(ms))
        {
            Assert.Equal(ImageFormat.Cr3, r.Format);
        }
        Assert.True(ms.CanRead);
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestCr3Builder.Build(new TestCr3Builder.Cr3Spec());
        var r = Cr3Reader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Info_Format_Equals_Cr3()
    {
        byte[] bytes = TestCr3Builder.Build(new TestCr3Builder.Cr3Spec());
        using var r = Cr3Reader.Open(new MemoryStream(bytes));
        Assert.Equal(ImageFormat.Cr3, r.Info.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestCr3Builder.Build(new TestCr3Builder.Cr3Spec
        {
            ThmbJpeg = LoadRedJpeg(),
        });
        using var r = Cr3Reader.Open(new MemoryStream(bytes));
        if (!r.CanDecodePixels) return;
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token)) { }
        });
    }
}
