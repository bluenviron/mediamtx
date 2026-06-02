using Mediar.Imaging;
using Mediar.Imaging.Rw2;
using Xunit;

namespace Mediar.Tests.Rw2;

/// <summary>
/// Tests for <see cref="Rw2Reader"/>, covering RW2 magic validation,
/// Panasonic-specific tag parsing, sub-image discovery and end-to-end
/// pixel decode delegation to <see cref="Mediar.Imaging.Tiff.TiffReader"/>.
/// </summary>
public sealed class Rw2ReaderTests
{
    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49, 0x55];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => Rw2Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x4D, 0x4D, 0x00, 0x55, 0x00, 0x00, 0x00, 0x08];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Rw2Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Standard_Tiff_Magic()
    {
        var spec = new TestRw2Builder.Rw2Spec { Magic = 0x002A };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Rw2Reader.Open(ms));
    }

    [Fact]
    public void Parses_Panasonic_Metadata_Tags()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            Model = "DC-S5M2",
            Software = "Ver.1.1",
            DateTime = "2024:08:12 14:30:00",
            Artist = "Test Photographer",
            Copyright = "(c) 2024 Test",
            PanasonicRawVersion = "0\0\0",
            SensorWidth = 6112,
            SensorHeight = 4080,
            SensorTopBorder = 16,
            SensorLeftBorder = 8,
            SensorBottomBorder = 4076,
            SensorRightBorder = 6108,
            CfaPattern = 1,
            Iso = 200,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));

        Assert.Equal(ImageFormat.Rw2, rw2.Format);
        Assert.Equal("Panasonic", rw2.Rw2.Make);
        Assert.Equal("DC-S5M2", rw2.Rw2.Model);
        Assert.Equal("Ver.1.1", rw2.Rw2.Software);
        Assert.Equal("2024:08:12 14:30:00", rw2.Rw2.DateTime);
        Assert.Equal("Test Photographer", rw2.Rw2.Artist);
        Assert.Equal("(c) 2024 Test", rw2.Rw2.Copyright);

        Assert.Equal(6112, rw2.Rw2.SensorWidth);
        Assert.Equal(4080, rw2.Rw2.SensorHeight);
        Assert.Equal(16, rw2.Rw2.SensorTopBorder);
        Assert.Equal(8, rw2.Rw2.SensorLeftBorder);
        Assert.Equal(4076, rw2.Rw2.SensorBottomBorder);
        Assert.Equal(6108, rw2.Rw2.SensorRightBorder);
        Assert.Equal(1, rw2.Rw2.CfaPattern);
        Assert.Equal(200, rw2.Rw2.Iso);

        Assert.Equal("Panasonic", rw2.Metadata.CameraMake);
        Assert.Equal("DC-S5M2", rw2.Metadata.CameraModel);
        Assert.Equal(200, rw2.Metadata.IsoSpeed);
    }

    [Fact]
    public void Falls_Back_To_Sensor_Dimensions_When_Tiff_Dimensions_Absent()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            SensorWidth = 4000,
            SensorHeight = 3000,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));

        Assert.Equal(4000, rw2.Info.Width);
        Assert.Equal(3000, rw2.Info.Height);
    }

    [Fact]
    public void Reports_Panasonic_Compressed_RAW_As_Undecodable()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            TiffWidth = 4000,
            TiffHeight = 3000,
            BitsPerSample = 12,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));

        Assert.False(rw2.CanDecodePixels);
        Assert.Equal(34316, rw2.SubImages[0].CompressionTag);
    }

    [Fact]
    public async Task Decodes_Uncompressed_Rgb_Through_Tiff_Reader()
    {
        int w = 8, h = 4;
        byte[] strip = new byte[w * h * 3];
        for (int i = 0; i < strip.Length; i += 3)
        {
            strip[i] = 0xFF;
            strip[i + 1] = 0x00;
            strip[i + 2] = 0x00;
        }
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            TiffWidth = (uint)w,
            TiffHeight = (uint)h,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Photometric = 2,
            Compression = 1,
            IncludeStripData = true,
            StripBytes = strip,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));

        Assert.True(rw2.CanDecodePixels);
        ImageFrame? frame = null;
        await foreach (var f in rw2.ReadFramesAsync())
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
            Assert.Equal(0xFF, pixels[0]);
            Assert.Equal(0x00, pixels[1]);
            Assert.Equal(0x00, pixels[2]);
        }
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Primary_Uses_Panasonic_Compression()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            TiffWidth = 4000,
            TiffHeight = 3000,
            BitsPerSample = 12,
            Compression = 34316,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in rw2.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }

    [Fact]
    public void Detector_Recognises_Rw2_Magic()
    {
        byte[] bytes = [0x49, 0x49, 0x55, 0x00, 0x08, 0x00, 0x00, 0x00];
        Assert.Equal(ImageFormat.Rw2, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void Records_PanasonicRawVersion()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            PanasonicRawVersion = "0\0\0",
            Make = "Panasonic",
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.False(string.IsNullOrEmpty(rw2.Rw2.PanasonicRawVersion));
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => Rw2Reader.Open(ms));
    }

    [Fact]
    public void Without_Make_Tag_CameraMake_Is_Null()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            SensorWidth = 4000,
            SensorHeight = 3000,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.Null(rw2.Rw2.Make);
        Assert.Null(rw2.Metadata.CameraMake);
    }

    [Fact]
    public void Without_Iso_Tag_IsoSpeed_Is_Zero()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            SensorWidth = 4000,
            SensorHeight = 3000,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.Equal(0, rw2.Rw2.Iso);
        Assert.Null(rw2.Metadata.IsoSpeed);
    }

    [Fact]
    public void SensorBorders_All_Recorded_When_Provided()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            SensorWidth = 6000,
            SensorHeight = 4000,
            SensorTopBorder = 8,
            SensorLeftBorder = 4,
            SensorBottomBorder = 3996,
            SensorRightBorder = 5996,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.Equal(8, rw2.Rw2.SensorTopBorder);
        Assert.Equal(4, rw2.Rw2.SensorLeftBorder);
        Assert.Equal(3996, rw2.Rw2.SensorBottomBorder);
        Assert.Equal(5996, rw2.Rw2.SensorRightBorder);
    }

    [Fact]
    public void CfaPattern_Defaults_Zero_When_Absent()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            SensorWidth = 4000,
            SensorHeight = 3000,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.Equal(0, rw2.Rw2.CfaPattern);
    }

    [Fact]
    public void Records_SubImage_Compression_Tag_On_Panasonic_Compression()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            TiffWidth = 4000,
            TiffHeight = 3000,
            BitsPerSample = 12,
            Compression = 34316,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        using var rw2 = Rw2Reader.Open(new MemoryStream(bytes));
        Assert.NotEmpty(rw2.SubImages);
        Assert.Equal(34316, rw2.SubImages[0].CompressionTag);
    }

    [Fact]
    public void Reader_Disposes_OwnedStream_On_Dispose()
    {
        var spec = new TestRw2Builder.Rw2Spec
        {
            Make = "Panasonic",
            SensorWidth = 100,
            SensorHeight = 100,
        };
        byte[] bytes = TestRw2Builder.Build(spec);
        var ms = new MemoryStream(bytes);
        var rw2 = Rw2Reader.Open(ms, ownsStream: true);
        rw2.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }
}
