using Mediar.Codecs.Ccitt;
using Xunit;

namespace Mediar.Tests.Codecs.Ccitt;

/// <summary>
/// Round-trip tests for the CCITT T.4 (G3 1D Modified Huffman) and T.6
/// (G4 MMR) codecs. Each test encodes a synthetic packed bitmap and
/// asserts that decoding the result reproduces the original bytes.
/// </summary>
public sealed class CcittRoundTripTests
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

    [Theory]
    [InlineData(8, 4)]
    [InlineData(16, 16)]
    [InlineData(73, 11)]
    public void G4_AllWhite_RoundTrips(int w, int h)
    {
        var src = BuildPacked(w, h, (_, _) => 0);
        var encoded = CcittG4Encoder.Encode(src, w, h);
        Assert.NotEmpty(encoded);
        var decoded = CcittG4Decoder.Decode(encoded, w, h);
        Assert.Equal(src, decoded);
    }

    [Theory]
    [InlineData(8, 4)]
    [InlineData(16, 16)]
    [InlineData(73, 11)]
    public void G4_AllBlack_RoundTrips(int w, int h)
    {
        var src = BuildPacked(w, h, (_, _) => 1);
        var encoded = CcittG4Encoder.Encode(src, w, h);
        var decoded = CcittG4Decoder.Decode(encoded, w, h);
        AssertPixelsEqual(src, decoded, w, h);
    }

    [Fact]
    public void G4_VerticalStripes_RoundTrips()
    {
        const int W = 32, H = 8;
        var src = BuildPacked(W, H, (x, _) => (x / 4) & 1);
        var encoded = CcittG4Encoder.Encode(src, W, H);
        var decoded = CcittG4Decoder.Decode(encoded, W, H);
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void G4_HorizontalStripes_RoundTrips()
    {
        const int W = 24, H = 16;
        var src = BuildPacked(W, H, (_, y) => (y / 2) & 1);
        var encoded = CcittG4Encoder.Encode(src, W, H);
        var decoded = CcittG4Decoder.Decode(encoded, W, H);
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void G4_Checkerboard_RoundTrips()
    {
        const int W = 24, H = 24;
        var src = BuildPacked(W, H, (x, y) => (x + y) & 1);
        var encoded = CcittG4Encoder.Encode(src, W, H);
        var decoded = CcittG4Decoder.Decode(encoded, W, H);
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void G4_LongRun_GreaterThan2560_RoundTrips()
    {
        // 3000 pixels wide exercises the extended make-up code path.
        const int W = 3000, H = 1;
        var src = BuildPacked(W, H, (x, _) => x >= 100 && x < 2800 ? 1 : 0);
        var encoded = CcittG4Encoder.Encode(src, W, H);
        var decoded = CcittG4Decoder.Decode(encoded, W, H);
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void G4_DiagonalLine_RoundTrips()
    {
        const int W = 64, H = 64;
        var src = BuildPacked(W, H, (x, y) => x == y ? 1 : 0);
        var encoded = CcittG4Encoder.Encode(src, W, H);
        var decoded = CcittG4Decoder.Decode(encoded, W, H);
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Theory]
    [InlineData(8, 4)]
    [InlineData(73, 11)]
    public void G3_1D_AllWhite_RoundTrips(int w, int h)
    {
        var src = BuildPacked(w, h, (_, _) => 0);
        var opts = new CcittG3Encoder.Options(EmitEolMarkers: false, EolByteAlign: false, EmitRtc: false);
        var encoded = CcittG3Encoder.Encode(src, w, h, opts);
        var decoded = CcittG3Decoder.Decode(encoded, w, h,
            new CcittG3Decoder.Options(HasEolMarkers: false, EolByteAligned: false));
        AssertPixelsEqual(src, decoded, w, h);
    }

    [Fact]
    public void G3_1D_Checkerboard_RoundTrips()
    {
        const int W = 32, H = 16;
        var src = BuildPacked(W, H, (x, y) => (x + y) & 1);
        var opts = new CcittG3Encoder.Options(EmitEolMarkers: false, EolByteAlign: false, EmitRtc: false);
        var encoded = CcittG3Encoder.Encode(src, W, H, opts);
        var decoded = CcittG3Decoder.Decode(encoded, W, H,
            new CcittG3Decoder.Options(HasEolMarkers: false, EolByteAligned: false));
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void G3_T4_WithEolMarkers_RoundTrips()
    {
        const int W = 64, H = 8;
        var src = BuildPacked(W, H, (x, y) => ((x + y) / 3) & 1);
        var opts = new CcittG3Encoder.Options(EmitEolMarkers: true, EolByteAlign: false, EmitRtc: false);
        var encoded = CcittG3Encoder.Encode(src, W, H, opts);
        var decoded = CcittG3Decoder.Decode(encoded, W, H,
            new CcittG3Decoder.Options(HasEolMarkers: true, EolByteAligned: false));
        AssertPixelsEqual(src, decoded, W, H);
    }

    [Fact]
    public void Decoder_Rejects_Unsupported_Compression()
    {
        Assert.Throws<NotSupportedException>(() =>
            CcittDecoder.Decode(new byte[] { 0 }, 8, 8, tiffCompression: 5));
    }

    [Fact]
    public void Decoder_Throws_On_T4_Two_Dimensional()
    {
        // T4Options bit 0 = 2D coding (MR), not yet implemented.
        Assert.Throws<NotSupportedException>(() =>
            CcittDecoder.Decode(new byte[] { 0 }, 8, 8, tiffCompression: 3, t4Options: 1));
    }

    [Fact]
    public void Decoder_Width_Zero_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            CcittG4Decoder.Decode(new byte[] { 0 }, width: 0, height: 1));
    }

    private static void AssertPixelsEqual(byte[] expected, byte[] actual, int width, int height)
    {
        Assert.Equal(expected.Length, actual.Length);
        int rowBytes = (width + 7) / 8;
        int tailMaskBits = width & 7;
        byte tailMask = tailMaskBits == 0
            ? (byte)0xFF
            : (byte)(0xFF << (8 - tailMaskBits));
        for (int y = 0; y < height; y++)
        {
            int row = y * rowBytes;
            for (int b = 0; b < rowBytes; b++)
            {
                byte exp = expected[row + b];
                byte act = actual[row + b];
                if (b == rowBytes - 1 && tailMaskBits != 0)
                {
                    exp &= tailMask;
                    act &= tailMask;
                }
                Assert.True(exp == act,
                    $"Pixel mismatch at row {y}, byte {b}: expected 0x{exp:X2}, got 0x{act:X2}");
            }
        }
    }
}
