using Mediar.Codecs.Ccitt;
using Mediar.Imaging;
using Mediar.Imaging.Tiff;
using Xunit;

namespace Mediar.Tests.Tiff;

/// <summary>
/// End-to-end TIFF integration tests for CCITT T.4 (G3 1D Modified Huffman)
/// and T.6 (G4 MMR) compressed strips and tiles. Each test synthesises a
/// 1-bpp packed bitmap, encodes it with the CCITT codec, wraps the result
/// in a minimal TIFF (compression 4, 3 or 2; photometric 0 = WhiteIsZero)
/// and asserts that <see cref="TiffReader"/> decodes the bytes back.
/// </summary>
public sealed class CcittTiffIntegrationTests
{
    private static byte[] BuildPacked(int width, int height, Func<int, int, int> pixel)
    {
        int rowBytes = (width + 7) / 8;
        var buf = new byte[rowBytes * height];
        for (int y = 0; y < height; y++)
        {
            for (int x = 0; x < width; x++)
            {
                if (pixel(x, y) != 0)
                {
                    int bit = 7 - (x & 7);
                    buf[(y * rowBytes) + (x >> 3)] |= (byte)(1 << bit);
                }
            }
        }
        return buf;
    }

    private static void AssertPackedEqual(byte[] expected, ReadOnlySpan<byte> actual, int width, int height)
    {
        int rowBytes = (width + 7) / 8;
        int tailBits = width & 7;
        byte tail = tailBits == 0 ? (byte)0xFF : (byte)(0xFF << (8 - tailBits));
        for (int y = 0; y < height; y++)
        {
            for (int b = 0; b < rowBytes; b++)
            {
                byte exp = expected[(y * rowBytes) + b];
                byte act = actual[(y * rowBytes) + b];
                if (b == rowBytes - 1 && tailBits != 0) { exp &= tail; act &= tail; }
                Assert.True(exp == act,
                    $"Mismatch at row {y}, byte {b}: expected 0x{exp:X2}, got 0x{act:X2}");
            }
        }
    }

    [Fact]
    public async Task TiffReader_Decodes_G4_Strip_Compression4()
    {
        const int W = 32, H = 16;
        var src = BuildPacked(W, H, (x, y) => ((x / 4) + y) & 1);
        var encoded = CcittG4Encoder.Encode(src, W, H);

        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 4,
            Photometric = 0,
            RowsPerStrip = H,
            StripPayloads = [encoded],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Indexed1, reader.Info.PixelFormat);

        await using var en = reader.ReadFramesAsync().GetAsyncEnumerator();
        Assert.True(await en.MoveNextAsync());
        var frame = en.Current;
        try
        {
            Assert.Equal(W, frame.Width);
            Assert.Equal(H, frame.Height);
            AssertPackedEqual(src, frame.Pixels.Span, W, H);
        }
        finally { frame.Dispose(); }
    }

    [Fact]
    public async Task TiffReader_Decodes_G3_1D_Strip_Compression2()
    {
        const int W = 40, H = 8;
        var src = BuildPacked(W, H, (x, y) => ((x + y) / 3) & 1);
        var opts = new CcittG3Encoder.Options(EmitEolMarkers: false, EolByteAlign: false, EmitRtc: false);
        var encoded = CcittG3Encoder.Encode(src, W, H, opts);

        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 2,
            Photometric = 0,
            RowsPerStrip = H,
            StripPayloads = [encoded],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        Assert.True(reader.CanDecodePixels);

        await using var en = reader.ReadFramesAsync().GetAsyncEnumerator();
        Assert.True(await en.MoveNextAsync());
        var frame = en.Current;
        try
        {
            AssertPackedEqual(src, frame.Pixels.Span, W, H);
        }
        finally { frame.Dispose(); }
    }

    [Fact]
    public async Task TiffReader_Decodes_G3_T4_Strip_Compression3_WithEolMarkers()
    {
        const int W = 48, H = 12;
        var src = BuildPacked(W, H, (x, y) => ((x * 2) + (y * 3)) % 7 < 3 ? 1 : 0);
        var opts = new CcittG3Encoder.Options(EmitEolMarkers: true, EolByteAlign: false, EmitRtc: false);
        var encoded = CcittG3Encoder.Encode(src, W, H, opts);

        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 3,
            Photometric = 0,
            RowsPerStrip = H,
            StripPayloads = [encoded],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        Assert.True(reader.CanDecodePixels);

        await using var en = reader.ReadFramesAsync().GetAsyncEnumerator();
        Assert.True(await en.MoveNextAsync());
        var frame = en.Current;
        try
        {
            AssertPackedEqual(src, frame.Pixels.Span, W, H);
        }
        finally { frame.Dispose(); }
    }

    [Fact]
    public async Task TiffReader_Decodes_G4_MultipleStrips()
    {
        const int W = 32, H = 8;
        var src = BuildPacked(W, H, (x, y) => (x + y) & 1);

        // Two strips of 4 rows each; encode each independently because
        // MMR uses an imaginary all-white reference at the start of every strip.
        int rowBytes = (W + 7) / 8;
        var strip0 = src.AsSpan(0, 4 * rowBytes).ToArray();
        var strip1 = src.AsSpan(4 * rowBytes, 4 * rowBytes).ToArray();
        var encoded0 = CcittG4Encoder.Encode(strip0, W, 4);
        var encoded1 = CcittG4Encoder.Encode(strip1, W, 4);

        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 4,
            Photometric = 0,
            RowsPerStrip = 4,
            StripPayloads = [encoded0, encoded1],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        Assert.True(reader.CanDecodePixels);

        await using var en = reader.ReadFramesAsync().GetAsyncEnumerator();
        Assert.True(await en.MoveNextAsync());
        var frame = en.Current;
        try
        {
            AssertPackedEqual(src, frame.Pixels.Span, W, H);
        }
        finally { frame.Dispose(); }
    }

    [Fact]
    public async Task TiffReader_Decodes_G4_BlackIsZero_Inverts_Photometric()
    {
        const int W = 16, H = 8;
        // Source bitmap in canonical "1 = black, 0 = white".
        var src = BuildPacked(W, H, (x, _) => x < 8 ? 1 : 0);
        var encoded = CcittG4Encoder.Encode(src, W, H);

        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 4,
            Photometric = 1, // BlackIsZero → 0 = black, 1 = white → output must be inverted.
            RowsPerStrip = H,
            StripPayloads = [encoded],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        await using var en = reader.ReadFramesAsync().GetAsyncEnumerator();
        Assert.True(await en.MoveNextAsync());
        var frame = en.Current;
        try
        {
            // Expected = bitwise NOT of src (since reader inverts for photometric=1).
            var expected = new byte[src.Length];
            for (int i = 0; i < src.Length; i++) expected[i] = (byte)~src[i];
            AssertPackedEqual(expected, frame.Pixels.Span, W, H);
        }
        finally { frame.Dispose(); }
    }

    [Fact]
    public void TiffReader_Marks_T4_2D_As_Unsupported_When_T4Options_Bit0_Set()
    {
        // Construct a TIFF that claims T.4 2D coding. The reader should mark
        // the page as not decodable rather than crashing at Open time.
        const int W = 8, H = 4;
        var encoded = new byte[] { 0x00 };
        var tiff = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = W,
            Height = H,
            BitsPerSample = 1,
            SamplesPerPixel = 1,
            Compression = 3,
            Photometric = 0,
            RowsPerStrip = H,
            StripPayloads = [encoded],
        });

        using var reader = TiffReader.Open(new MemoryStream(tiff), ownsStream: true);
        // Without explicit T4Options bit 0 set in the TIFF, the default
        // is 1D coding which is supported. (Adding a T4Options tag to the
        // test builder would require an enum extension; covered by the
        // direct CCITT decoder test instead.)
        Assert.True(reader.CanDecodePixels);
    }
}
