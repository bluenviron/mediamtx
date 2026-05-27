using System.Buffers.Binary;
using Mediar.Imaging;
using Mediar.Imaging.Raf;
using Xunit;

namespace Mediar.Tests.Raf;

/// <summary>
/// Tests for <see cref="RafReader"/>, covering header validation,
/// magic-byte rejection, offset/length sanity checks, embedded-JPEG
/// preview decoding, and CFA sub-image discovery.
/// </summary>
public sealed class RafReaderTests
{
    // Tiny 16x16 solid-red baseline JPEG (the same fixture used by
    // TiffReaderTests.JpegInTiff_*). Base64-encoded so the test file
    // remains pure source. Decodes to a self-contained SOF0 bitstream.
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
    public void Rejects_NotFuji_Magic()
    {
        var spec = new TestRafBuilder.RafSpec
        {
            JpegBytes = LoadRedJpeg(),
            OverrideMagic = "NOTAFUJIFILMRAW"u8.ToArray(),
        };
        var bytes = TestRafBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => RafReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_TruncatedHeader()
    {
        var spec = new TestRafBuilder.RafSpec
        {
            JpegBytes = LoadRedJpeg(),
            Truncate = true,
            TruncateTo = 64,
        };
        var bytes = TestRafBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => RafReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_ZeroJpegOffset()
    {
        var spec = new TestRafBuilder.RafSpec
        {
            JpegBytes = LoadRedJpeg(),
            ZeroJpegSlot = true,
        };
        var bytes = TestRafBuilder.Build(spec);
        Assert.Throws<ImageFormatException>(() => RafReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Rejects_JpegSliceBeyondEof()
    {
        var spec = new TestRafBuilder.RafSpec { JpegBytes = LoadRedJpeg() };
        var bytes = TestRafBuilder.Build(spec);

        // Patch the JPEG length field to point past EOF.
        BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x58, 4), 0xFFFFFFFFu);
        Assert.Throws<ImageFormatException>(() => RafReader.Open(new MemoryStream(bytes)));
    }

    [Fact]
    public void Parses_HeaderFields_And_CameraModel()
    {
        var spec = new TestRafBuilder.RafSpec
        {
            FormatVersion = "0201",
            CameraModel = "X-T4",
            DirectoryVersion = "0159",
            JpegBytes = LoadRedJpeg(),
        };
        var bytes = TestRafBuilder.Build(spec);
        using var r = RafReader.Open(new MemoryStream(bytes));

        Assert.Equal(ImageFormat.Raf, r.Format);
        Assert.Equal("0201", r.Raf.FormatVersion);
        Assert.Equal("X-T4", r.Raf.CameraModel);
        Assert.Equal("0159", r.Raf.DirectoryVersion);
        Assert.Equal((uint)0x6C, r.Raf.JpegOffset);
        Assert.Equal((uint)spec.JpegBytes.Length, r.Raf.JpegLength);

        Assert.Equal("FUJIFILM", r.Metadata.CameraMake);
        Assert.Equal("X-T4", r.Metadata.CameraModel);
    }

    [Fact]
    public void Discovers_JpegPreview_AsPrimary_SubImage()
    {
        var spec = new TestRafBuilder.RafSpec { JpegBytes = LoadRedJpeg() };
        var bytes = TestRafBuilder.Build(spec);
        using var r = RafReader.Open(new MemoryStream(bytes));

        Assert.Single(r.SubImages);
        var preview = r.SubImages[0];
        Assert.Equal(RafSubImageKind.JpegPreview, preview.Kind);
        Assert.True(preview.CanDecodePixels);
        Assert.Equal(16, preview.Width);
        Assert.Equal(16, preview.Height);
        Assert.Equal((uint)0x6C, preview.Offset);
        Assert.Equal((uint)spec.JpegBytes.Length, preview.Length);
        Assert.Equal(PixelFormat.Rgb24, preview.PixelFormat);
    }

    [Fact]
    public void Exposes_Cfa_SubImage_When_Present_And_MarksUndecodable()
    {
        // Construct a tiny but valid LE TIFF as the CFA payload. Only IFD0
        // is needed; the strip data can be empty since RafReader does not
        // attempt to decode the CFA pixels.
        var cfa = BuildMinimalTiff(width: 32, height: 24);

        var spec = new TestRafBuilder.RafSpec
        {
            JpegBytes = LoadRedJpeg(),
            CfaBytes = cfa,
        };
        var bytes = TestRafBuilder.Build(spec);
        using var r = RafReader.Open(new MemoryStream(bytes));

        Assert.Equal(2, r.SubImages.Count);
        var cfaSub = r.SubImages[1];
        Assert.Equal(RafSubImageKind.Cfa, cfaSub.Kind);
        Assert.False(cfaSub.CanDecodePixels);
        Assert.Equal(32, cfaSub.Width);
        Assert.Equal(24, cfaSub.Height);
        Assert.Equal((uint)cfa.Length, cfaSub.Length);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_EmbeddedJpegPreview()
    {
        var spec = new TestRafBuilder.RafSpec { JpegBytes = LoadRedJpeg() };
        var bytes = TestRafBuilder.Build(spec);
        using var r = RafReader.Open(new MemoryStream(bytes));
        Assert.True(r.CanDecodePixels);

        int frames = 0;
        await foreach (var frame in r.ReadFramesAsync())
        {
            frames++;
            Assert.Equal(16, frame.Width);
            Assert.Equal(16, frame.Height);
            Assert.Equal(PixelFormat.Rgb24, frame.PixelFormat);

            // Spot-check: the embedded fixture is solid red.
            var pixels = frame.Pixels.Span;
            Assert.True(pixels[0] >= 200, "Expected red channel near 255, got " + pixels[0]);
            Assert.True(pixels[1] < 50, "Expected green channel near 0, got " + pixels[1]);
            Assert.True(pixels[2] < 50, "Expected blue channel near 0, got " + pixels[2]);

            frame.Dispose();
        }
        Assert.Equal(1, frames);
    }

    [Fact]
    public void Detector_Recognizes_FujiFilm_Magic()
    {
        var spec = new TestRafBuilder.RafSpec { JpegBytes = LoadRedJpeg() };
        var bytes = TestRafBuilder.Build(spec);
        Assert.Equal(ImageFormat.Raf, ImageFormatDetector.Detect(bytes));
    }

    [Fact]
    public void Meta_Container_Is_Accepted_And_Bounds_Validated()
    {
        var spec = new TestRafBuilder.RafSpec
        {
            JpegBytes = LoadRedJpeg(),
            MetaBytes = [0, 0, 0, 1, 0x01, 0x00, 0x00, 0x04, 1, 2, 3, 4],
        };
        var bytes = TestRafBuilder.Build(spec);
        using var r = RafReader.Open(new MemoryStream(bytes));
        Assert.True(r.Raf.MetaOffset > 0);
        Assert.Equal(12u, r.Raf.MetaLength);
    }

    private static byte[] BuildMinimalTiff(int width, int height)
    {
        // II + 0x2A + uint32 IFD0 offset(8). 8 IFD entries:
        //   ImageWidth     0x0100 SHORT  count=1 value=<width>
        //   ImageLength    0x0101 SHORT  count=1 value=<height>
        //   BitsPerSample  0x0102 SHORT  count=1 value=16
        //   Compression    0x0103 SHORT  count=1 value=1
        //   Photometric    0x0106 SHORT  count=1 value=32803 (CFA)
        //   StripOffsets   0x0111 LONG   count=1 value=<after-IFD>
        //   RowsPerStrip   0x0116 SHORT  count=1 value=<height>
        //   StripByteCounts 0x0117 LONG  count=1 value=0
        // Then next-IFD = 0.
        const int ifdEntryCount = 8;
        int ifdSize = 2 + ifdEntryCount * 12 + 4;
        int totalLen = 8 + ifdSize;
        var buf = new byte[totalLen];

        buf[0] = (byte)'I'; buf[1] = (byte)'I';
        BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(2, 2), 42);
        BinaryPrimitives.WriteUInt32LittleEndian(buf.AsSpan(4, 4), 8u);

        int o = 8;
        BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o, 2), (ushort)ifdEntryCount);
        o += 2;

        WriteEntry(buf, ref o, 0x0100, 3, 1, (uint)width);
        WriteEntry(buf, ref o, 0x0101, 3, 1, (uint)height);
        WriteEntry(buf, ref o, 0x0102, 3, 1, 16);
        WriteEntry(buf, ref o, 0x0103, 3, 1, 1);
        WriteEntry(buf, ref o, 0x0106, 3, 1, 32803);
        WriteEntry(buf, ref o, 0x0111, 4, 1, (uint)totalLen);
        WriteEntry(buf, ref o, 0x0116, 3, 1, (uint)height);
        WriteEntry(buf, ref o, 0x0117, 4, 1, 0u);

        BinaryPrimitives.WriteUInt32LittleEndian(buf.AsSpan(o, 4), 0u);
        return buf;
    }

    private static void WriteEntry(byte[] buf, ref int o, int tag, int type, uint count, uint value)
    {
        BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o, 2), (ushort)tag);
        BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o + 2, 2), (ushort)type);
        BinaryPrimitives.WriteUInt32LittleEndian(buf.AsSpan(o + 4, 4), count);
        BinaryPrimitives.WriteUInt32LittleEndian(buf.AsSpan(o + 8, 4), value);
        o += 12;
    }
}
