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
}
