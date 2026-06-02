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

    [Fact]
    public void Without_Raw_Ifd_Has_Only_Chain_SubImages()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4), MakeRgbStrip(8, 8)],
            raw: null);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Equal(2, cr2.SubImages.Count);
        Assert.Equal(0u, cr2.Cr2.RawIfdOffset);
        Assert.DoesNotContain(cr2.SubImages, s => s.Role == Cr2IfdRole.RawSensor);
    }

    [Fact]
    public void Without_Camera_Make_And_Model_Metadata_Is_Null()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Null(cr2.Metadata.CameraMake);
        Assert.Null(cr2.Metadata.CameraModel);
    }

    [Fact]
    public void Version_Major_Minor_Persists_In_Metadata_Tag()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null,
            majorVersion: 1, minorVersion: 3);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Equal(1, cr2.Cr2.MajorVersion);
        Assert.Equal(3, cr2.Cr2.MinorVersion);
        Assert.Equal("1.3", cr2.Metadata.Tags["CR2:Version"]);
    }

    [Fact]
    public void Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
    }

    [Fact]
    public void Single_Chain_Ifd_Sets_Primary_To_It()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(12, 6)],
            raw: null);

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.Single(cr2.SubImages);
        Assert.Equal(12, cr2.Info.Width);
        Assert.Equal(6, cr2.Info.Height);
        Assert.Equal(Cr2IfdRole.Thumbnail, cr2.SubImages[0].Role);
    }

    [Fact]
    public void Detector_Does_Not_Recognize_Cr2_Without_Sentinel()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        bytes[8] = 0; bytes[9] = 0;
        var detected = ImageFormatDetector.Detect(bytes.AsSpan(0, 16));
        Assert.NotEqual(ImageFormat.Cr2, detected);
    }

    [Fact]
    public void RawIfdOffset_Points_To_Valid_Section_When_Raw_Present()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(2, 2)],
            raw: MakeRgbStrip(16, 16));

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);

        Assert.NotEqual(0u, cr2.Cr2.RawIfdOffset);
        Assert.True(cr2.Cr2.RawIfdOffset < bytes.Length);
        var rawSub = Assert.Single(cr2.SubImages, s => s.Role == Cr2IfdRole.RawSensor);
        Assert.Equal(16, rawSub.Width);
        Assert.Equal(16, rawSub.Height);
    }

    [Fact]
    public void Open_With_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => Cr2Reader.Open((Stream)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        var ms = new MemoryStream(bytes, writable: false);
        using (var cr2 = Cr2Reader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Cr2, cr2.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        using var ms = new MemoryStream(bytes, writable: false);
        using (var cr2 = Cr2Reader.Open(ms))
        {
            Assert.Equal(ImageFormat.Cr2, cr2.Format);
        }
        // Stream is still usable after the reader disposes.
        ms.Position = 0;
        Assert.Equal((byte)'I', (byte)ms.ReadByte());
    }

    [Fact]
    public void Rejects_Invalid_Tiff_Magic()
    {
        // II byte-order mark but a magic word != 42.
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        bytes[2] = 99; bytes[3] = 0;
        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
        Assert.Contains("magic", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Rejects_Zero_Ifd0_Offset()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        bytes[4] = 0; bytes[5] = 0; bytes[6] = 0; bytes[7] = 0;
        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => Cr2Reader.Open(ms));
        Assert.Contains("no IFD", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Largest_Sub_Image_Becomes_Primary_When_Preview_Is_Larger_Than_Raw()
    {
        // Preview (32x32 = 1024 px) is larger than the raw IFD (8x8 = 64 px).
        // The preview should still be selected as the primary.
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4), MakeRgbStrip(32, 32)],
            raw: MakeRgbStrip(8, 8));

        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);
        Assert.Equal(32, cr2.Info.Width);
        Assert.Equal(32, cr2.Info.Height);
    }

    [Fact]
    public void Info_ColorSpace_Is_RAW()
    {
        byte[] bytes = TestCr2Builder.Build(
            chain: [MakeRgbStrip(4, 4)],
            raw: null);
        using var ms = new MemoryStream(bytes, writable: false);
        using var cr2 = Cr2Reader.Open(ms);
        Assert.Equal("RAW", cr2.Info.ColorSpace);
        Assert.Equal(ImageFormat.Cr2, cr2.Info.Format);
        Assert.False(cr2.Info.HasAlpha);
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => Cr2Reader.Open((Stream)null!));
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = TestCr2Builder.Build(chain: [MakeRgbStrip(4, 4)], raw: null);
        var r = Cr2Reader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Format_Is_Cr2()
    {
        byte[] bytes = TestCr2Builder.Build(chain: [MakeRgbStrip(4, 4)], raw: null);
        using var cr2 = Cr2Reader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Cr2, cr2.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        byte[] bytes = TestCr2Builder.Build(chain: [MakeRgbStrip(4, 4)], raw: null);
        using var cr2 = Cr2Reader.Open(new MemoryStream(bytes, writable: false));
        if (!cr2.CanDecodePixels) return;
        using var cts = new System.Threading.CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in cr2.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => Cr2Reader.Open((string)null!));
    }
}
