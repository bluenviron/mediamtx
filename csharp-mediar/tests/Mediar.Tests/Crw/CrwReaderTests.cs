using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.Crw;
using Xunit;

namespace Mediar.Tests.Crw;

/// <summary>
/// Tests for <see cref="CrwReader"/>, covering CIFF header validation,
/// heap-directory walking, embedded JPEG thumbnail decode, sub-heap
/// recursion, and metadata extraction.
/// </summary>
public sealed class CrwReaderTests
{
    // Tiny 16x16 solid-red baseline JPEG (reused from RafReaderTests). Decodes
    // to a self-contained SOF0 bitstream so JpegReader can open it.
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
    public void Rejects_TruncatedFile()
    {
        var bytes = new byte[20]; // < 30 (header 26 + 4-byte trailer)
        Assert.Throws<ImageFormatException>(() => CrwReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_InvalidByteOrderMark()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            OverrideByteOrderMark = new byte[] { (byte)'X', (byte)'Y' },
        };
        var bytes = TestCrwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => CrwReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_MissingHeapCcdrSignature()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            OverrideSignature = "NOTHEAPC"u8.ToArray(),
        };
        var bytes = TestCrwBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => CrwReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Open_ParsesHeaderFields_For_EmptyHeap()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            Version = 0x00010002, // v1.2
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.Equal(ImageFormat.Crw, r.Format);
        Assert.Equal("II", r.Crw.ByteOrderMark);
        Assert.Equal(26u, r.Crw.HeaderLength);
        Assert.Equal("HEAPCCDR", r.Crw.Type);
        Assert.Equal(0x00010002u, r.Crw.Version);
        Assert.Equal(0, r.Crw.TopLevelEntryCount);
        Assert.Equal(0, r.Crw.TotalEntryCount);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void DiscoversTopLevelEntries_For_CameraTypeAndFirmware()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x080A, Payload = TestCrwBuilder.CameraTypePayload("Canon", "Canon EOS-D30") },
                new() { Tag = 0x080B, Payload = TestCrwBuilder.AsciiPayload("Firmware Version 1.0.5") },
                new() { Tag = 0x0810, Payload = TestCrwBuilder.AsciiPayload("Test Owner") },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.Equal(3, r.Crw.TopLevelEntryCount);
        Assert.Equal(3, r.Crw.TotalEntryCount);
        Assert.Equal("Canon", r.Crw.Make);
        Assert.Equal("Canon EOS-D30", r.Crw.Model);
        Assert.Equal("Firmware Version 1.0.5", r.Crw.FirmwareVersion);
        Assert.Equal("Test Owner", r.Crw.OwnerName);
        Assert.Equal("Canon", r.Metadata.CameraMake);
        Assert.Equal("Canon EOS-D30", r.Metadata.CameraModel);
    }

    [Fact]
    public void ParsesImageSpec_AndCaptureTime()
    {
        const uint width = 2160;
        const uint height = 1440;
        const uint epoch = 1_234_567_890; // 2009-02-13 23:31:30 UTC
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x1810, Payload = TestCrwBuilder.ImageSpecPayload(width, height, 1, 1, 0, 12, 24, le: true) },
                new() { Tag = 0x180E, Payload = TestCrwBuilder.CaptureTimePayload(epoch, le: true) },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.Equal(width, r.Crw.SensorWidth);
        Assert.Equal(height, r.Crw.SensorHeight);
        Assert.Equal(1u, r.Crw.PixelAspectNumerator);
        Assert.Equal(1u, r.Crw.PixelAspectDenominator);
        Assert.Equal(12u, r.Crw.ComponentBitDepth);
        Assert.Equal(epoch, r.Crw.CaptureTimeSeconds);
        Assert.Equal(new DateTimeOffset(2009, 2, 13, 23, 31, 30, TimeSpan.Zero), r.Metadata.CapturedAt);
    }

    [Fact]
    public void DiscoversEmbeddedJpegThumbnail_AsPrimary()
    {
        var jpeg = LoadRedJpeg();
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x2007, Payload = jpeg },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.True(r.CanDecodePixels);
        Assert.Single(r.SubImages);
        var thumb = r.SubImages[0];
        Assert.Equal(CrwSubImageKind.JpegThumbnail, thumb.Kind);
        Assert.Equal((ushort)0x2007, thumb.Tag);
        Assert.Equal((uint)jpeg.Length, thumb.Length);
        Assert.Equal(16, thumb.Width);
        Assert.Equal(16, thumb.Height);
        Assert.True(thumb.CanDecodePixels);
        Assert.Equal(16, r.Info.Width);
        Assert.Equal(16, r.Info.Height);
        Assert.Equal(PixelFormat.Rgb24, r.Info.PixelFormat);
    }

    [Fact]
    public void RawImageData_IsSurfaced_AsUndecodable()
    {
        // tag 0x2005 = raw CCD data (Canon CIFF spec). Treated as
        // undecodable until the per-camera lossless JPEG predictor
        // tables are parsed.
        var rawPayload = new byte[256];
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x2005, Payload = rawPayload },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.False(r.CanDecodePixels);
        Assert.Single(r.SubImages);
        Assert.Equal(CrwSubImageKind.RawImageData, r.SubImages[0].Kind);
        Assert.False(r.SubImages[0].CanDecodePixels);
    }

    [Fact]
    public void WalksSubHeap_Recursively()
    {
        // Sub-heap (tag 0x300A = ImageProps) containing two child entries.
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new()
                {
                    Tag = 0x300A,
                    Children =
                    [
                        new() { Tag = 0x080A, Payload = TestCrwBuilder.CameraTypePayload("Canon", "Canon EOS-10D") },
                        new() { Tag = 0x080B, Payload = TestCrwBuilder.AsciiPayload("1.0.0") },
                    ],
                },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.Equal(1, r.Crw.TopLevelEntryCount);
        Assert.Equal(3, r.Crw.TotalEntryCount);
        // Children should be recorded with DirectoryDepth = 1.
        var depth1 = r.SubImages.Where(s => s.DirectoryDepth == 1).ToList();
        Assert.Equal(2, depth1.Count);
        Assert.Contains(depth1, s => s.Tag == 0x080A);
        Assert.Contains(depth1, s => s.Tag == 0x080B);
        Assert.Equal("Canon EOS-10D", r.Crw.Model);
        Assert.Equal("1.0.0", r.Crw.FirmwareVersion);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_EmbeddedJpegThumbnail()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x080A, Payload = TestCrwBuilder.CameraTypePayload("Canon", "Canon EOS-D30") },
                new() { Tag = 0x2007, Payload = LoadRedJpeg() },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        var frames = new List<ImageFrame>();
        await foreach (var f in r.ReadFramesAsync())
        {
            frames.Add(f);
        }
        Assert.Single(frames);
        Assert.Equal(16, frames[0].Width);
        Assert.Equal(16, frames[0].Height);
        // Red baseline - centre pixel should be roughly red.
        var centre = ReadPixel(frames[0], 8, 8);
        Assert.True(centre.R > centre.G + 30, $"R={centre.R} G={centre.G}");
        Assert.True(centre.R > centre.B + 30, $"R={centre.R} B={centre.B}");
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_NoDecodableSubImage()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            Entries =
            [
                new() { Tag = 0x2005, Payload = new byte[256] },
                new() { Tag = 0x080A, Payload = TestCrwBuilder.CameraTypePayload("Canon", "Canon EOS-D30") },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.False(r.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in r.ReadFramesAsync()) { }
        });
    }

    [Fact]
    public void Detector_RecognisesCrwHeaderMagic()
    {
        var bytes = TestCrwBuilder.Build(new TestCrwBuilder.CrwSpec());
        var detected = ImageFormatDetector.Detect(bytes.AsSpan(0, Math.Min(bytes.Length, 64)));
        Assert.Equal(ImageFormat.Crw, detected);
    }

    [Fact]
    public void BigEndian_File_IsRecognised_And_Parsed()
    {
        var spec = new TestCrwBuilder.CrwSpec
        {
            LittleEndian = false,
            Entries =
            [
                new() { Tag = 0x080A, Payload = TestCrwBuilder.CameraTypePayload("Canon", "Canon EOS-1D") },
            ],
        };
        var bytes = TestCrwBuilder.Build(spec);
        using var r = CrwReader.Open(new MemoryStream(bytes));
        Assert.Equal("MM", r.Crw.ByteOrderMark);
        Assert.Equal("Canon EOS-1D", r.Crw.Model);
    }

    [Fact]
    public void OutOfBoundsDirectoryOffset_IsRejected()
    {
        // Build a normal CRW then patch the trailing directory-offset to a
        // value that overflows the heap.
        var bytes = TestCrwBuilder.Build(new TestCrwBuilder.CrwSpec());
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(bytes.Length - 4), 0x7FFF_FFFFu);
        Assert.Throws<ImageFormatException>(() => CrwReader.Open(new MemoryStream(bytes)));
    }

    private static (byte R, byte G, byte B) ReadPixel(ImageFrame frame, int x, int y)
    {
        // Frame data is row-major Rgb24 (8 bpc) per CrwReader's contract.
        int stride = frame.Stride;
        int off = y * stride + x * 3;
        var data = frame.Pixels.Span;
        return (data[off], data[off + 1], data[off + 2]);
    }
}
