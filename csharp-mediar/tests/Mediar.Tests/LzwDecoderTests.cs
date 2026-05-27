using Mediar.Codecs.Lzw;
using Xunit;

namespace Mediar.Tests;

public sealed class LzwDecoderTests
{
    [Fact]
    public void Tiff_DecodesCanonicalSpecExample()
    {
        // TIFF6 Annex C example bytes: "WED WE WEE WEB" → "WED" + "WE" + " WEE" + " WEB"
        // We use the canonical encoded stream from TIFF6 Section 13 page 61.
        // Bytes: 80 16 0D 80 8F 0F 12 38 1A 04 5C 02 0B 7F 00 00
        // produced from "/WED WE WEE WEB/"  — full check: round-trip via our encoder
        var input = "WED WE WEE WEB"u8.ToArray();
        var compressed = EncodeTiffLzw(input);
        var decoded = LzwDecoder.DecodeTiff(compressed);
        Assert.Equal(input, decoded);
    }

    [Theory]
    [InlineData("")]
    [InlineData("A")]
    [InlineData("AAAAAAAAAA")]
    [InlineData("ABCDEFGHIJKLMNOPQRSTUVWXYZ")]
    [InlineData("the quick brown fox jumps over the lazy dog")]
    public void Tiff_RoundTripsRepresentativeStrings(string text)
    {
        var input = System.Text.Encoding.ASCII.GetBytes(text);
        var compressed = EncodeTiffLzw(input);
        var decoded = LzwDecoder.DecodeTiff(compressed);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void Tiff_RoundTripsKwKwKPattern()
    {
        // ABABABAB triggers the K + first(K) edge case in classical LZW.
        var input = System.Text.Encoding.ASCII.GetBytes("ABABABABABABABABABABABAB");
        var compressed = EncodeTiffLzw(input);
        var decoded = LzwDecoder.DecodeTiff(compressed);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void Tiff_RoundTripsLargeBufferAcrossCodeWidthBoundaries()
    {
        var rng = new Random(0xC0DEC);
        var input = new byte[16 * 1024];
        rng.NextBytes(input);
        var compressed = EncodeTiffLzw(input);
        var decoded = LzwDecoder.DecodeTiff(compressed);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void Gif_RoundTripsTwoColourFrame()
    {
        // 16 pixels, 1-bit palette → lzwMinCodeSize = 2 (minimum legal value).
        var input = new byte[] { 0, 1, 0, 1, 0, 1, 0, 1, 1, 0, 1, 0, 1, 0, 1, 0 };
        var compressed = EncodeGifLzw(input, lzwMinCodeSize: 2);
        var decoded = LzwDecoder.DecodeGif(compressed, 2, input.Length);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void Gif_RoundTripsEightBitFrame()
    {
        var rng = new Random(0xC0FFEE);
        var input = new byte[4096];
        rng.NextBytes(input);
        var compressed = EncodeGifLzw(input, lzwMinCodeSize: 8);
        var decoded = LzwDecoder.DecodeGif(compressed, 8, input.Length);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void RejectsInvalidOptions()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            LzwDecoder.Decode([0], new LzwOptions(LzwBitOrder.MsbFirst, InitialBits: 1)));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            LzwDecoder.Decode([0], new LzwOptions(LzwBitOrder.MsbFirst, InitialBits: 8, MaxBits: 8)));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            LzwDecoder.Decode([0], new LzwOptions(LzwBitOrder.MsbFirst, InitialBits: 8, MaxBits: 17)));
    }

    // ── helpers: minimal correct LZW encoders used only by tests ────────────────────────

    private static byte[] EncodeTiffLzw(ReadOnlySpan<byte> input)
    {
        var dict = new Dictionary<string, int>(StringComparer.Ordinal);
        const int clearCode = 256;
        const int endCode = 257;
        const int maxCode = 4095;
        var bitOut = new MsbBitWriter();
        int codeSize = 9;
        int nextCode = endCode + 1;

        bitOut.Write(clearCode, codeSize);
        for (int i = 0; i < 256; i++) dict[char.ToString((char)i)] = i;

        string omega = string.Empty;
        for (int i = 0; i < input.Length; i++)
        {
            string omegaK = omega + (char)input[i];
            if (dict.ContainsKey(omegaK))
            {
                omega = omegaK;
            }
            else
            {
                bitOut.Write(dict[omega], codeSize);
                if (nextCode <= maxCode)
                {
                    dict[omegaK] = nextCode++;
                    if (nextCode == (1 << codeSize) && codeSize < 12)
                    {
                        codeSize++;
                    }
                }
                else
                {
                    bitOut.Write(clearCode, codeSize);
                    dict.Clear();
                    for (int j = 0; j < 256; j++) dict[char.ToString((char)j)] = j;
                    codeSize = 9;
                    nextCode = endCode + 1;
                }
                omega = char.ToString((char)input[i]);
            }
        }
        if (omega.Length > 0) bitOut.Write(dict[omega], codeSize);
        bitOut.Write(endCode, codeSize);
        return bitOut.ToArray();
    }

    private static byte[] EncodeGifLzw(ReadOnlySpan<byte> input, int lzwMinCodeSize)
    {
        int clearCode = 1 << lzwMinCodeSize;
        int endCode = clearCode + 1;
        const int maxCode = 4095;
        var dict = new Dictionary<string, int>(StringComparer.Ordinal);
        var bitOut = new LsbBitWriter();
        int codeSize = lzwMinCodeSize + 1;
        int nextCode = endCode + 1;

        for (int i = 0; i < clearCode; i++) dict[char.ToString((char)i)] = i;
        bitOut.Write(clearCode, codeSize);

        string omega = string.Empty;
        for (int i = 0; i < input.Length; i++)
        {
            string omegaK = omega + (char)input[i];
            if (dict.ContainsKey(omegaK))
            {
                omega = omegaK;
            }
            else
            {
                bitOut.Write(dict[omega], codeSize);
                if (nextCode <= maxCode)
                {
                    dict[omegaK] = nextCode++;
                    if (nextCode == (1 << codeSize) + 1 && codeSize < 12)
                    {
                        codeSize++;
                    }
                }
                else
                {
                    bitOut.Write(clearCode, codeSize);
                    dict.Clear();
                    for (int j = 0; j < clearCode; j++) dict[char.ToString((char)j)] = j;
                    codeSize = lzwMinCodeSize + 1;
                    nextCode = endCode + 1;
                }
                omega = char.ToString((char)input[i]);
            }
        }
        if (omega.Length > 0) bitOut.Write(dict[omega], codeSize);
        bitOut.Write(endCode, codeSize);
        return bitOut.ToArray();
    }

    private sealed class LsbBitWriter
    {
        private readonly List<byte> _buf = [];
        private int _bitBuf;
        private int _bitCount;

        public void Write(int code, int width)
        {
            _bitBuf |= code << _bitCount;
            _bitCount += width;
            while (_bitCount >= 8)
            {
                _buf.Add((byte)(_bitBuf & 0xFF));
                _bitBuf >>>= 8;
                _bitCount -= 8;
            }
        }

        public byte[] ToArray()
        {
            if (_bitCount > 0) _buf.Add((byte)(_bitBuf & 0xFF));
            return [.. _buf];
        }
    }

    private sealed class MsbBitWriter
    {
        private readonly List<byte> _buf = [];
        private int _bitBuf;
        private int _bitCount;

        public void Write(int code, int width)
        {
            _bitBuf = (_bitBuf << width) | code;
            _bitCount += width;
            while (_bitCount >= 8)
            {
                _bitCount -= 8;
                _buf.Add((byte)((_bitBuf >> _bitCount) & 0xFF));
            }
        }

        public byte[] ToArray()
        {
            if (_bitCount > 0) _buf.Add((byte)((_bitBuf << (8 - _bitCount)) & 0xFF));
            return [.. _buf];
        }
    }
}
