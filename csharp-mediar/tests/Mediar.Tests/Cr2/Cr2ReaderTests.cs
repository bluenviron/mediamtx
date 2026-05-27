using Mediar.Imaging;
using Mediar.Imaging.Cr2;
using Xunit;

namespace Mediar.Tests.Cr2;

public sealed class Cr2ReaderTests
{
    private static TestCr2Builder.IfdSpec MakeRgbStrip(int w, int h, string? make = null, string? model = null)
    {
        var payload = new byte[w * h * 3];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)((i * 13) & 0xFF);
        return new TestCr2Builder.IfdSpec
        {
            Width = w,
            Height = h,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            StripPayload = payload,
            Make = make,
            Model = model,
        };
    }

    [Fact]
    public void Rejects_File_Without_Cr_Sentinel()
    {
        // Plain TIFF (no "CR" at offset 8) — must be rejected by Cr2Reader.
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        // Overwrite the CR sentinel with zeros (the builder always emits CR).
        bytes[8] = 0; bytes[9] = 0;
        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
        Assert.Contains("CR", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49, 0x2A, 0x00, 0x10, 0x00];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
    }

    [Fact]
    public void Rejects_Non_LittleEndian()
    {
        // Build a normal CR2 then flip the BOM to MM (big-endian); the
        // reader should reject because CR2 is little-endian by spec.
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)], raw: null);
        bytes[0] = (byte)'M'; bytes[1] = (byte)'M';
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
    }

    [Fact]
    public void Parses_Cr2_Header_Version_And_RawIfd_Offset()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4, "Canon", "EOS 5D Mark IV")],
            raw: MakeRgbStrip(8, 8),
            majorVersion: 2, minorVersion: 0);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Equal(2, cr2.Cr2.MajorVersion);
        Assert.Equal(0, cr2.Cr2.MinorVersion);
        Assert.NotEqual(0u, cr2.Cr2.RawIfdOffset);

        Assert.Equal("Canon", cr2.Metadata.CameraMake);
        Assert.Equal("EOS 5D Mark IV", cr2.Metadata.CameraModel);
        Assert.Equal("2.0", cr2.Metadata.Tags["CR2:Version"]);
    }

    [Fact]
    public void Discovers_Chain_Plus_Raw_Ifd_As_SubImages()
    {
        // Two chained IFDs (thumb + preview) plus a raw IFD.
        byte[] bytes = TestCr2Builder.Build(
            chain: [
                MakeRgbStrip(4, 4),    // IFD0 thumbnail
                MakeRgbStrip(2, 2),    // IFD1 alternate thumbnail
                MakeRgbStrip(16, 16),  // IFD2 full preview
            ],
            raw: MakeRgbStrip(32, 32));  // raw sensor

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Equal(4, cr2.SubImages.Count);
        Assert.Equal(Cr2IfdRole.Thumbnail, cr2.SubImages[0].Role);
        Assert.Equal(Cr2IfdRole.AlternateThumbnail, cr2.SubImages[1].Role);
        Assert.Equal(Cr2IfdRole.FullPreview, cr2.SubImages[2].Role);
        Assert.Equal(Cr2IfdRole.RawSensor, cr2.SubImages[3].Role);

        Assert.Equal(32, cr2.SubImages[3].Width);
        Assert.Equal(32, cr2.SubImages[3].Height);

        // Primary should be the largest (the raw IFD).
        Assert.Equal(32, cr2.Info.Width);
        Assert.Equal(32, cr2.Info.Height);
        Assert.Equal(ImageFormat.Cr2, cr2.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Largest_Chained_Frame_Through_Tiff()
    {
        // The raw IFD is NOT chained, so TiffReader (which walks chains)
        // will produce the full preview as the largest visible frame.
        const int W = 8, H = 4;
        var preview = MakeRgbStrip(W, H);
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(2, 2), preview],
            raw: null);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);
        Assert.True(cr2.CanDecodePixels);
        Assert.Equal(W, cr2.Info.Width);
        Assert.Equal(H, cr2.Info.Height);

        ImageFrame? frame = null;
        await foreach (var f in cr2.ReadFramesAsync())
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
            Assert.Equal(preview.StripPayload, frame.Pixels.ToArray());
        }
    }

    [Fact]
    public void Format_Detector_Recognizes_Cr2_Magic()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        var detected = ImageFormatDetector.Detect(bytes.AsSpan(0, 16));
        Assert.Equal(ImageFormat.Cr2, detected);
    }
}
