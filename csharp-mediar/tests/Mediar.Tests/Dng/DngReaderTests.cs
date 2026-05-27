using Mediar.Imaging;
using Mediar.Imaging.Dng;
using Xunit;

namespace Mediar.Tests.Dng;

public sealed class DngReaderTests
{
    private static readonly uint[] ExpectedBlackLevel = [16u, 16u, 16u, 16u];
    private static readonly uint[] ExpectedWhiteLevel = [4095u, 4095u, 4095u, 4095u];

    [Fact]
    public void Rejects_File_Without_DngVersion_Tag()
    {
        // A perfectly valid TIFF without DNGVersion should be rejected.
        var spec = new TestDngBuilder.IfdSpec
        {
            Width = 2,
            Height = 2,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[2 * 2 * 3],
            // intentionally no DngVersion
        };
        byte[] bytes = TestDngBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        var ex = Assert.Throws<ImageFormatException>(() => DngReader.Open(ms));
        Assert.Contains("DNGVersion", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Rejects_Truncated_Header()
    {
        byte[] tiny = [0x49, 0x49];  // II, then nothing
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => DngReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Byte_Order_Mark()
    {
        byte[] bytes = [0x00, 0x00, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => DngReader.Open(ms));
    }

    [Fact]
    public void Rejects_Bad_Magic()
    {
        // II then magic 41 instead of 42
        byte[] bytes = [0x49, 0x49, 0x29, 0x00, 0x08, 0x00, 0x00, 0x00];
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => DngReader.Open(ms));
    }

    [Fact]
    public void Discovers_SubIfd_And_Picks_It_As_Primary()
    {
        // IFD0 = 4x4 RGB thumbnail (NewSubFileType=1 = reduced-res preview)
        // SubIFD = 8x8 single-component raw Gray8 (NewSubFileType=0 = primary)
        var raw = new TestDngBuilder.IfdSpec
        {
            Width = 8,
            Height = 8,
            BitsPerSample = 8,
            SamplesPerPixel = 1,
            Compression = 1,
            Photometric = 1,  // BlackIsZero
            NewSubFileType = 0,
            StripPayload = new byte[8 * 8],
        };
        var thumb = new TestDngBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 1,
            StripPayload = new byte[4 * 4 * 3],
            DngVersion = [1, 7, 0, 0],
            UniqueCameraModel = "TestCam X1",
            Make = "Mediar",
            Model = "TestCam X1",
            SubIfds = [raw],
        };
        byte[] bytes = TestDngBuilder.Build(thumb);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dng = DngReader.Open(ms);

        Assert.Equal(2, dng.SubImages.Count);
        // First entry is IFD0 (the thumbnail).
        Assert.Equal(4, dng.SubImages[0].Width);
        Assert.Equal(0, dng.SubImages[0].SubIfdLevel);
        Assert.Equal(1, dng.SubImages[0].NewSubFileType);
        // Second entry is the SubIFD with the raw.
        Assert.Equal(8, dng.SubImages[1].Width);
        Assert.Equal(1, dng.SubImages[1].SubIfdLevel);
        Assert.Equal(0, dng.SubImages[1].NewSubFileType);

        // Primary should be the SubIFD (NewSubFileType == 0).
        Assert.Equal(8, dng.Info.Width);
        Assert.Equal(8, dng.Info.Height);
        Assert.Equal(ImageFormat.Dng, dng.Format);
    }

    [Fact]
    public void Parses_Dng_Metadata_Fields()
    {
        var spec = new TestDngBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = new byte[4 * 4 * 3],
            DngVersion = [1, 4, 0, 0],
            DngBackwardVersion = [1, 1, 0, 0],
            UniqueCameraModel = "Acme XR-7",
            Make = "Acme",
            Model = "XR-7",
            Software = "Mediar Test Harness",
            CfaPattern = [0, 1, 1, 2],  // RGGB
            BlackLevel = [16u, 16u, 16u, 16u],
            WhiteLevel = [4095u, 4095u, 4095u, 4095u],
        };
        byte[] bytes = TestDngBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dng = DngReader.Open(ms);

        Assert.Equal([1, 4, 0, 0], dng.Dng.DngVersion.ToArray());
        Assert.Equal([1, 1, 0, 0], dng.Dng.DngBackwardVersion.ToArray());
        Assert.Equal("Acme XR-7", dng.Dng.UniqueCameraModel);
        Assert.Equal("Acme", dng.Dng.Make);
        Assert.Equal("XR-7", dng.Dng.Model);
        Assert.Equal("Mediar Test Harness", dng.Dng.Software);
        Assert.Equal([0, 1, 1, 2], dng.Dng.CfaPattern.ToArray());
        Assert.Equal(ExpectedBlackLevel, dng.Dng.BlackLevel);
        Assert.Equal(ExpectedWhiteLevel, dng.Dng.WhiteLevel);

        // ImageMetadata projection.
        Assert.Equal("Acme", dng.Metadata.CameraMake);
        Assert.Equal("XR-7", dng.Metadata.CameraModel);
        Assert.Equal("Mediar Test Harness", dng.Metadata.Software);
        Assert.True(dng.Metadata.Tags.ContainsKey("DNG:Version"));
        Assert.Equal("1.4.0.0", dng.Metadata.Tags["DNG:Version"]);
        Assert.Equal("Acme XR-7", dng.Metadata.Tags["DNG:UniqueCameraModel"]);
        Assert.Equal("16,16,16,16", dng.Metadata.Tags["DNG:BlackLevel"]);
        Assert.Equal("4095,4095,4095,4095", dng.Metadata.Tags["DNG:WhiteLevel"]);
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_Uncompressed_Rgb_Through_Tiff()
    {
        // Synthesise a primary IFD with uncompressed RGB pixels. The DNG
        // reader should delegate to TiffReader and emit the same bytes.
        const int W = 4, H = 4;
        var strip = new byte[W * H * 3];
        for (int i = 0; i < strip.Length; i++) strip[i] = (byte)((i * 7) & 0xFF);

        var spec = new TestDngBuilder.IfdSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 8,
            SamplesPerPixel = 3,
            Compression = 1,
            Photometric = 2,
            NewSubFileType = 0,
            StripPayload = strip,
            DngVersion = [1, 7, 0, 0],
        };
        byte[] bytes = TestDngBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dng = DngReader.Open(ms);
        Assert.True(dng.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in dng.ReadFramesAsync())
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
            // Pixels should round-trip byte-for-byte (TIFF copy path).
            var got = frame.Pixels.ToArray();
            Assert.Equal(strip, got);
        }
    }

    [Fact]
    public void Reports_Unsupported_Compression_As_CanDecodePixels_False()
    {
        // Compression 34892 = "Lossy JPEG" — not on TiffReader's supported list.
        var spec = new TestDngBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 1,
            Compression = 34892,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            DngVersion = [1, 7, 0, 0],
        };
        byte[] bytes = TestDngBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dng = DngReader.Open(ms);

        Assert.False(dng.CanDecodePixels);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Pixels_Cannot_Be_Decoded()
    {
        var spec = new TestDngBuilder.IfdSpec
        {
            Width = 4,
            Height = 4,
            BitsPerSample = 8,
            SamplesPerPixel = 1,
            Compression = 34892,
            Photometric = 1,
            NewSubFileType = 0,
            StripPayload = new byte[16],
            DngVersion = [1, 7, 0, 0],
        };
        byte[] bytes = TestDngBuilder.Build(spec);

        using var ms = new MemoryStream(bytes, writable: false);
        using var dng = DngReader.Open(ms);

        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in dng.ReadFramesAsync())
            {
                // unreachable
            }
        });
    }
}
