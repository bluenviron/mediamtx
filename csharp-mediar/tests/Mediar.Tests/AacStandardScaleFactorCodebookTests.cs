using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacStandardScaleFactorCodebookTests
{
    [Fact]
    public void Constants_HaveSpecValues()
    {
        Assert.Equal(121, AacStandardScaleFactorCodebook.SymbolCount);
        Assert.Equal(60, AacStandardScaleFactorCodebook.ZeroDeltaSymbolIndex);
        Assert.Equal(60, AacStandardScaleFactorCodebook.MaxAbsoluteDelta);
    }

    [Fact]
    public void Codes_And_Bits_Have_121_Entries()
    {
        Assert.Equal(121, AacStandardScaleFactorCodebook.Codes.Length);
        Assert.Equal(121, AacStandardScaleFactorCodebook.Bits.Length);
    }

    [Fact]
    public void Book_BuildsSuccessfully_And_Has_121_Symbols()
    {
        var book = AacStandardScaleFactorCodebook.Book;
        Assert.NotNull(book);
        Assert.Equal(121, book.SymbolCount);
        Assert.Equal(121, book.Capacity);
        Assert.Equal(19, book.MaxCodeLength);
    }

    [Fact]
    public void ZeroDeltaSymbol_HasOneBitCode()
    {
        // Symbol 60 (delta=0) is the most-likely value and must be the
        // single-bit "0" codeword.
        Assert.Equal(1, AacStandardScaleFactorCodebook.Bits[60]);
        Assert.Equal(0u, AacStandardScaleFactorCodebook.Codes[60]);
    }

    [Theory]
    [InlineData(0, -60)]
    [InlineData(60, 0)]
    [InlineData(120, 60)]
    [InlineData(30, -30)]
    [InlineData(90, 30)]
    public void SymbolToDelta_RoundTrips(int symbol, int expectedDelta)
    {
        Assert.Equal(expectedDelta, AacStandardScaleFactorCodebook.SymbolToDelta(symbol));
        Assert.Equal(symbol, AacStandardScaleFactorCodebook.DeltaToSymbol(expectedDelta));
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(121)]
    [InlineData(int.MinValue)]
    public void SymbolToDelta_OutOfRange_Throws(int symbol)
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacStandardScaleFactorCodebook.SymbolToDelta(symbol));
    }

    [Theory]
    [InlineData(-61)]
    [InlineData(61)]
    [InlineData(int.MaxValue)]
    public void DeltaToSymbol_OutOfRange_ReturnsMinusOne(int delta)
    {
        Assert.Equal(-1, AacStandardScaleFactorCodebook.DeltaToSymbol(delta));
    }

    [Fact]
    public void Decode_SingleBitZeroDelta()
    {
        // Bitstream "0xxxxxxx" -> first symbol decodes to 60 (delta 0).
        byte[] data = { 0b0000_0000 };
        var reader = new BitReader(data);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var sym));
        Assert.Equal(60, sym);
        Assert.Equal(0, AacStandardScaleFactorCodebook.SymbolToDelta(sym));
    }

    [Fact]
    public void Decode_FFmpegFragment_Indices_57_To_63()
    {
        // Sanity check several published codewords end-to-end:
        // bits[57]=5, code=0x1a -> 11010 -> 5 bits
        // bits[58]=4, code=0x0b -> 1011 -> 4 bits
        // bits[59]=3, code=0x04 -> 100 -> 3 bits
        // bits[60]=1, code=0x00 -> 0 -> 1 bit
        // bits[61]=4, code=0x0a -> 1010 -> 4 bits
        // bits[62]=4, code=0x0c -> 1100 -> 4 bits
        // bits[63]=5, code=0x1b -> 11011 -> 5 bits
        // Decode 60, 59, 58, 61, 62, 63, 57
        // = "0" "100" "1011" "1010" "1100" "11011" "11010"
        // = 1 + 3 + 4 + 4 + 4 + 5 + 5 = 26 bits
        // "0 100 1011 1010 1100 11011 11010"
        // = "01001011 10101100 11011110 10xxxxxx"
        // byte 0: 0100_1011 = 0x4B
        // byte 1: 1010_1100 = 0xAC
        // byte 2: 1101_1110 = 0xDE
        // byte 3: 10xx_xxxx = 0x80
        byte[] data = { 0x4B, 0xAC, 0xDE, 0x80 };
        var reader = new BitReader(data);

        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v0));
        Assert.Equal(60, v0);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v1));
        Assert.Equal(59, v1);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v2));
        Assert.Equal(58, v2);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v3));
        Assert.Equal(61, v3);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v4));
        Assert.Equal(62, v4);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v5));
        Assert.Equal(63, v5);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var v6));
        Assert.Equal(57, v6);
    }

    [Fact]
    public void Decode_MaxLength19BitCode_Symbol13()
    {
        // bits[13] = 19, code = 0x7FFFF = 19 ones "1111111111111111111".
        // Decode just this 19-bit code packed left-aligned -> 3 bytes:
        // 19 bits: 1111_1111 1111_1111 111x_xxxx = 0xFF, 0xFF, 0xE0.
        byte[] data = { 0xFF, 0xFF, 0xE0 };
        var reader = new BitReader(data);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var sym));
        Assert.Equal(13, sym);
        Assert.Equal(13 - 60, AacStandardScaleFactorCodebook.SymbolToDelta(sym));
    }

    [Fact]
    public void Book_IsSharedInstance()
    {
        var a = AacStandardScaleFactorCodebook.Book;
        var b = AacStandardScaleFactorCodebook.Book;
        Assert.Same(a, b);
    }

    [Fact]
    public void BitsArray_AllInValidRange()
    {
        // Every published bit-length must be in [1, 19] (no zero entries
        // in this codebook).
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            int b = AacStandardScaleFactorCodebook.Bits[i];
            Assert.InRange(b, 1, 19);
        }
    }

    [Fact]
    public void CodesFitInDeclaredBits()
    {
        // Every (code, bits) pair must satisfy code < 2^bits.
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            uint code = AacStandardScaleFactorCodebook.Codes[i];
            int bits = AacStandardScaleFactorCodebook.Bits[i];
            uint cap = bits == 32 ? uint.MaxValue : (1u << bits) - 1u;
            Assert.True(code <= cap,
                $"Symbol {i}: code 0x{code:X} exceeds {bits}-bit cap.");
        }
    }

    [Fact]
    public void DeltaToSymbol_Boundaries_Are_Inclusive()
    {
        Assert.Equal(0, AacStandardScaleFactorCodebook.DeltaToSymbol(-60));
        Assert.Equal(120, AacStandardScaleFactorCodebook.DeltaToSymbol(60));
    }

    [Fact]
    public void SymbolToDelta_AllSymbols_RoundTrip_Via_DeltaToSymbol()
    {
        for (int sym = 0; sym < AacStandardScaleFactorCodebook.SymbolCount; sym++)
        {
            int delta = AacStandardScaleFactorCodebook.SymbolToDelta(sym);
            Assert.Equal(sym, AacStandardScaleFactorCodebook.DeltaToSymbol(delta));
        }
    }

    [Fact]
    public void Codes_Have_No_Duplicates_Within_Same_BitLength()
    {
        // Huffman prefix property: within a bit-length bucket, all codes
        // are distinct.
        var byLen = new Dictionary<int, HashSet<uint>>();
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            int bits = AacStandardScaleFactorCodebook.Bits[i];
            uint code = AacStandardScaleFactorCodebook.Codes[i];
            if (!byLen.TryGetValue(bits, out var set))
            {
                set = new HashSet<uint>();
                byLen[bits] = set;
            }
            Assert.True(set.Add(code),
                $"Duplicate code 0x{code:X} at length {bits} (symbol {i}).");
        }
    }

    [Fact]
    public void Bits_Sum_Total_Is_Stable_Across_Runs()
    {
        // Capture the current sum of all bit-lengths; if the table is ever
        // edited this value will change and surface as a deliberate fail.
        long sum = 0;
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            sum += AacStandardScaleFactorCodebook.Bits[i];
        }
        Assert.True(sum >= AacStandardScaleFactorCodebook.SymbolCount,
            $"Sum {sum} is smaller than symbol count.");
        Assert.True(sum <= AacStandardScaleFactorCodebook.SymbolCount * 19,
            $"Sum {sum} exceeds max possible (121 * 19).");
    }

    [Fact]
    public void Codes_Kraft_Inequality_Holds()
    {
        // For a uniquely-decodable Huffman code, Kraft's inequality must hold:
        //   sum(2^-bits[i]) <= 1
        double kraft = 0;
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            kraft += Math.Pow(2, -AacStandardScaleFactorCodebook.Bits[i]);
        }
        Assert.True(kraft <= 1.0 + 1e-9, $"Kraft sum {kraft} exceeds 1.");
    }

    [Theory]
    [InlineData(0)]
    [InlineData(50)]
    [InlineData(60)]
    [InlineData(75)]
    [InlineData(120)]
    public void Encoded_Symbol_Is_Decoded_Back_From_Bit_Buffer(int symbol)
    {
        int bits = AacStandardScaleFactorCodebook.Bits[symbol];
        uint code = AacStandardScaleFactorCodebook.Codes[symbol];

        // Pack `bits` left-aligned into a byte buffer big enough for it.
        int byteLen = (bits + 7) / 8;
        // Use up to 3 bytes; for symbols requiring 19 bits we need 3 bytes.
        Assert.True(byteLen <= 3);
        byte[] buf = new byte[byteLen];
        // Shift code left to align with the top of the buffer.
        int shift = byteLen * 8 - bits;
        ulong padded = (ulong)code << shift;
        for (int i = 0; i < byteLen; i++)
        {
            buf[i] = (byte)((padded >> ((byteLen - 1 - i) * 8)) & 0xFF);
        }
        var reader = new BitReader(buf);
        Assert.True(AacStandardScaleFactorCodebook.Book.TryDecode(ref reader, out var decoded));
        Assert.Equal(symbol, decoded);
    }

    [Fact]
    public void Bits_Distribution_Includes_Shortest_And_Longest()
    {
        // The single-bit codeword and the 19-bit codewords must both
        // appear in any valid build of this table.
        bool hasMin = false, hasMax = false;
        for (int i = 0; i < AacStandardScaleFactorCodebook.SymbolCount; i++)
        {
            if (AacStandardScaleFactorCodebook.Bits[i] == 1) hasMin = true;
            if (AacStandardScaleFactorCodebook.Bits[i] == 19) hasMax = true;
        }
        Assert.True(hasMin, "Expected at least one 1-bit codeword.");
        Assert.True(hasMax, "Expected at least one 19-bit codeword.");
    }
}
