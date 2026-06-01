using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacHuffmanCodebookTests
{
    [Fact]
    public void FromCanonicalLengths_EmptySpan_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromCanonicalLengths(ReadOnlySpan<int>.Empty));
    }

    [Fact]
    public void FromCanonicalLengths_AllZero_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromCanonicalLengths(new int[] { 0, 0, 0 }));
    }

    [Fact]
    public void FromCanonicalLengths_NegativeLength_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacHuffmanCodebook.FromCanonicalLengths(new int[] { 1, -1 }));
    }

    [Fact]
    public void FromCanonicalLengths_LengthAbove32_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacHuffmanCodebook.FromCanonicalLengths(new int[] { 33 }));
    }

    [Fact]
    public void FromCanonicalLengths_KraftOverflow_Throws()
    {
        // Three codes of length 1 cannot fit (max 2 codes of length 1).
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromCanonicalLengths(new int[] { 1, 1, 1 }));
    }

    [Fact]
    public void FromCanonicalLengths_SingleSymbol_HasOneLeaf()
    {
        var book = AacHuffmanCodebook.FromCanonicalLengths(new int[] { 1 });
        Assert.Equal(1, book.SymbolCount);
        Assert.Equal(1, book.Capacity);
        Assert.Equal(1, book.MaxCodeLength);
    }

    [Fact]
    public void TryDecode_TwoSymbols_RoundTrip()
    {
        // Symbols 0 -> code "0" (1 bit), 1 -> code "1" (1 bit).
        var book = AacHuffmanCodebook.FromCanonicalLengths(new int[] { 1, 1 });
        // Bitstream "0110" should decode to 0, 1, 1, 0.
        byte[] data = { 0b0110_0000 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var s0));
        Assert.Equal(0, s0);
        Assert.True(book.TryDecode(ref reader, out var s1));
        Assert.Equal(1, s1);
        Assert.True(book.TryDecode(ref reader, out var s2));
        Assert.Equal(1, s2);
        Assert.True(book.TryDecode(ref reader, out var s3));
        Assert.Equal(0, s3);
    }

    [Fact]
    public void TryDecode_FiveSymbolCanonicalCode_RoundTrip()
    {
        // Symbol 0 length 2 -> code "00"
        // Symbol 1 length 2 -> code "01"
        // Symbol 2 length 2 -> code "10"
        // Symbol 3 length 3 -> code "110"
        // Symbol 4 length 3 -> code "111"
        var lengths = new int[] { 2, 2, 2, 3, 3 };
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        Assert.Equal(5, book.SymbolCount);
        Assert.Equal(3, book.MaxCodeLength);

        // Encode 0, 1, 2, 3, 4 -> "00", "01", "10", "110", "111"
        // Concatenated: "00 01 10 110 111" = "0001 1011 0111" = 0x1B7 ... need bytes:
        // 12 bits: 0001_1011_0111 -> 0x1B7 -> 2 bytes: 0001_1011, 0111_0000 = 0x1B, 0x70.
        byte[] data = { 0x1B, 0x70 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(0, v0);
        Assert.True(book.TryDecode(ref reader, out var v1));
        Assert.Equal(1, v1);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(2, v2);
        Assert.True(book.TryDecode(ref reader, out var v3));
        Assert.Equal(3, v3);
        Assert.True(book.TryDecode(ref reader, out var v4));
        Assert.Equal(4, v4);
    }

    [Fact]
    public void TryDecode_UnusedSymbolSkipped()
    {
        // Symbols 0 length 1, 1 unused, 2 length 2, 3 length 2 ->
        //   0 -> "0", 2 -> "10", 3 -> "11". Symbol 1 should be skipped.
        var lengths = new int[] { 1, 0, 2, 2 };
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        Assert.Equal(3, book.SymbolCount);
        Assert.Equal(4, book.Capacity);

        // Decode "0 10 11" = 0_10_11 = 6 bits "010110" left-aligned = 0x58.
        byte[] data = { 0x58 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(0, v0);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(2, v2);
        Assert.True(book.TryDecode(ref reader, out var v3));
        Assert.Equal(3, v3);
    }

    [Fact]
    public void TryDecode_StreamUnderflow_ReturnsFalse()
    {
        // Length 3 for symbol 0 means code is at least 3 bits long.
        // (But Kraft would be violated unless we add more symbols.)
        // Use 8-symbol balanced tree with length 3.
        var lengths = new int[] { 3, 3, 3, 3, 3, 3, 3, 3 };
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Only 2 bits in the buffer - underflow.
        byte[] data = { 0b1100_0000 };
        var reader = new BitReader(data);
        // Burn 6 bits, leaving 2.
        reader.ReadBits(6);
        Assert.False(book.TryDecode(ref reader, out var sym));
        Assert.Equal(-1, sym);
    }

    [Fact]
    public void TryDecode_IncompleteCodebook_LandsOnUnallocatedReturnsFalse()
    {
        // Lengths 2, 2: only two codes "00" and "01" - leaves "10" and "11" prefixes unassigned.
        var lengths = new int[] { 2, 2 };
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        Assert.Equal(2, book.SymbolCount);

        // Decode "10" -> should fail (no symbol assigned).
        byte[] data = { 0b1000_0000 };
        var reader = new BitReader(data);
        Assert.False(book.TryDecode(ref reader, out var sym));
        Assert.Equal(-1, sym);
    }

    [Fact]
    public void FromCanonicalLengths_LargeSparseCodebook_ConstructsAndDecodes()
    {
        // 16-symbol balanced tree, length 4 each.
        var lengths = new int[16];
        for (int i = 0; i < 16; i++) lengths[i] = 4;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        Assert.Equal(16, book.SymbolCount);
        Assert.Equal(4, book.MaxCodeLength);

        // Symbol N has code N as 4 bits. Encode all 16 codes back to back: 64 bits = 8 bytes.
        byte[] data = new byte[8];
        for (int i = 0; i < 16; i++)
        {
            int byteIdx = i / 2;
            if ((i & 1) == 0) data[byteIdx] = (byte)(i << 4);
            else data[byteIdx] |= (byte)i;
        }
        var reader = new BitReader(data);
        for (int i = 0; i < 16; i++)
        {
            Assert.True(book.TryDecode(ref reader, out var sym));
            Assert.Equal(i, sym);
        }
    }

    [Fact]
    public void FromExplicitCodes_NullArgsAreAllowedAsEmpty_StillThrowsForEmpty()
    {
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                ReadOnlySpan<uint>.Empty, ReadOnlySpan<int>.Empty));
    }

    [Fact]
    public void FromExplicitCodes_MismatchedLengths_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0, 1 }, new int[] { 1 }));
    }

    [Fact]
    public void FromExplicitCodes_AllZero_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0, 0, 0 }, new int[] { 0, 0, 0 }));
    }

    [Fact]
    public void FromExplicitCodes_NegativeLength_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0, 0 }, new int[] { 1, -1 }));
    }

    [Fact]
    public void FromExplicitCodes_LengthAbove32_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0 }, new int[] { 33 }));
    }

    [Fact]
    public void FromExplicitCodes_CodeDoesNotFitInDeclaredLength_Throws()
    {
        // Length 3, code 0b1000 = 8 - does not fit in 3 bits (max 7).
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 8 }, new int[] { 3 }));
    }

    [Fact]
    public void FromExplicitCodes_KraftOverflow_Throws()
    {
        // Three length-1 codes cannot fit (max 2).
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0, 1, 0 }, new int[] { 1, 1, 1 }));
    }

    [Fact]
    public void FromExplicitCodes_PrefixCollision_Throws()
    {
        // Symbol 0: code 0 length 1 ("0"). Symbol 1: code 0 length 2 ("00").
        // Symbol 1's first bit "0" collides with symbol 0's leaf.
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 0, 0 }, new int[] { 1, 2 }));
    }

    [Fact]
    public void FromExplicitCodes_DuplicateCode_Throws()
    {
        // Two symbols claim the same 2-bit code "10".
        Assert.Throws<ArgumentException>(() =>
            AacHuffmanCodebook.FromExplicitCodes(
                new uint[] { 2, 2 }, new int[] { 2, 2 }));
    }

    [Fact]
    public void FromExplicitCodes_NonCanonicalLayout_DecodesCorrectly()
    {
        // Non-canonical assignment: symbol 0 gets code "00" (2 bits),
        // symbol 1 gets "1" (1 bit), symbol 2 gets "01" (2 bits).
        // A canonical builder would have given symbol 1 the 1-bit "0"
        // and symbols 0, 2 the 2-bit codes "10", "11".
        var codes = new uint[] { 0b00, 0b1, 0b01 };
        var bits = new int[] { 2, 1, 2 };
        var book = AacHuffmanCodebook.FromExplicitCodes(codes, bits);
        Assert.Equal(3, book.SymbolCount);
        Assert.Equal(2, book.MaxCodeLength);

        // Decode "00 1 01 1 00" -> 0, 1, 2, 1, 0
        // = "00101100" = 8 bits left-aligned in 1 byte = 0x2C.
        byte[] data = { 0x2C };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(0, v0);
        Assert.True(book.TryDecode(ref reader, out var v1));
        Assert.Equal(1, v1);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(2, v2);
        Assert.True(book.TryDecode(ref reader, out var v3));
        Assert.Equal(1, v3);
        Assert.True(book.TryDecode(ref reader, out var v4));
        Assert.Equal(0, v4);
    }

    [Fact]
    public void FromExplicitCodes_FromAacScaleFactorFragment_DecodesAsExpected()
    {
        // Fragment of the canonical AAC scalefactor codebook (FFmpeg
        // ff_aac_scalefactor_code / ff_aac_scalefactor_bits, indices
        // 57..63). These codes are NOT in canonical order within
        // length 4 (symbol 1 has 0x0b="1011" but appears before symbol 4
        // which has the smaller 0x0a="1010").
        // Local idx 0 -> AAC 57: code 0x1a, bits 5
        // Local idx 1 -> AAC 58: code 0x0b, bits 4
        // Local idx 2 -> AAC 59: code 0x04, bits 3
        // Local idx 3 -> AAC 60: code 0x00, bits 1
        // Local idx 4 -> AAC 61: code 0x0a, bits 4
        // Local idx 5 -> AAC 62: code 0x0c, bits 4
        // Local idx 6 -> AAC 63: code 0x1b, bits 5
        var codes = new uint[] { 0x1a, 0x0b, 0x04, 0x00, 0x0a, 0x0c, 0x1b };
        var bits = new int[] { 5, 4, 3, 1, 4, 4, 5 };
        var book = AacHuffmanCodebook.FromExplicitCodes(codes, bits);
        Assert.Equal(7, book.SymbolCount);
        Assert.Equal(5, book.MaxCodeLength);

        // Decode "0" (idx 3) "1011" (idx 1) "100" (idx 2) "11010" (idx 0)
        // = "0 1011 100 11010" = 13 bits -> "0101_1100_1101_0xxx" left-aligned.
        // 0101_1100 = 0x5C; 1101_0000 = 0xD0.
        byte[] data = { 0x5C, 0xD0 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(3, v0);
        Assert.True(book.TryDecode(ref reader, out var v1));
        Assert.Equal(1, v1);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(2, v2);
        Assert.True(book.TryDecode(ref reader, out var v3));
        Assert.Equal(0, v3);
    }

    [Fact]
    public void FromExplicitCodes_UnusedSymbolSkipped()
    {
        // Symbol 1 unused. Symbol 0 -> "0", symbol 2 -> "10", symbol 3 -> "11".
        var codes = new uint[] { 0, 0, 2, 3 };
        var bits = new int[] { 1, 0, 2, 2 };
        var book = AacHuffmanCodebook.FromExplicitCodes(codes, bits);
        Assert.Equal(3, book.SymbolCount);
        Assert.Equal(4, book.Capacity);

        // Decode "0 10 11" = "010110" left-aligned 0x58.
        byte[] data = { 0x58 };
        var reader = new BitReader(data);
        Assert.True(book.TryDecode(ref reader, out var v0));
        Assert.Equal(0, v0);
        Assert.True(book.TryDecode(ref reader, out var v2));
        Assert.Equal(2, v2);
        Assert.True(book.TryDecode(ref reader, out var v3));
        Assert.Equal(3, v3);
    }

    [Fact]
    public void FromExplicitCodes_MaximumLengthCode_Builds()
    {
        // A 32-bit code is the upper bound. Use a length-1 + length-32
        // pair that satisfies Kraft.
        var codes = new uint[] { 0, 0x80000000u };
        var bits = new int[] { 1, 32 };
        var book = AacHuffmanCodebook.FromExplicitCodes(codes, bits);
        Assert.Equal(2, book.SymbolCount);
        Assert.Equal(32, book.MaxCodeLength);
    }
}
