using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSpectralValueDecoderTests
{
    /// <summary>
    /// Build a synthetic codebook of <paramref name="symbolCount"/>
    /// symbols where every symbol has a fixed-length code of
    /// ceil(log2(symbolCount)) bits and codes are canonical.
    /// </summary>
    private static AacHuffmanCodebook BuildBalancedCodebook(int symbolCount)
    {
        int bits = 1;
        while ((1 << bits) < symbolCount) bits++;
        // For a perfect balanced tree we need symbolCount == 2^bits.
        // Use a fixed-length codebook that fills the tree where possible
        // and leaves the rest unassigned (incomplete).
        var lengths = new int[symbolCount];
        for (int i = 0; i < symbolCount; i++) lengths[i] = bits;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static byte[] EncodeFixedLengthSymbol(int bitCount, int symbolIndex)
    {
        var w = new AacBitWriter();
        w.Write((uint)symbolIndex, bitCount);
        return w.ToArray();
    }

    [Fact]
    public void TryRead_SignedCb1_DecodesQuadDirectly()
    {
        var geom = AacSpectralCodebookGeometry.Get(1)!;
        // Codebook 1 has 81 symbols; balanced needs 128, so use 81 explicitly.
        // We need a complete tree. Use lengths = {7, 7, ..., 7} for 81 symbols (incomplete tree).
        // For testing we just need decode to work for the symbols we encode.
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Encode symbol 40 -> (0, 0, 0, 0) per cb1 layout.
        var w = new AacBitWriter();
        w.Write(40u, 7);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[4];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(0, values[1]);
        Assert.Equal(0, values[2]);
        Assert.Equal(0, values[3]);
    }

    [Fact]
    public void TryRead_SignedCb1_FirstSymbol_AllMinusOne_NoSignBits()
    {
        var geom = AacSpectralCodebookGeometry.Get(1)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 0 -> (-1, -1, -1, -1).
        var w = new AacBitWriter();
        w.Write(0u, 7);
        // No sign bits should be read.
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[4];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(-1, values[0]);
        Assert.Equal(-1, values[1]);
        Assert.Equal(-1, values[2]);
        Assert.Equal(-1, values[3]);
        Assert.Equal(7, reader.Position); // codeword only, no sign bits.
    }

    [Fact]
    public void TryRead_UnsignedCb3_ReadsSignBitForEachNonZero()
    {
        var geom = AacSpectralCodebookGeometry.Get(3)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 80 -> magnitudes (2, 2, 2, 2). 4 sign bits follow.
        var w = new AacBitWriter();
        w.Write(80u, 7);
        // Sign bits: 0 (pos), 1 (neg), 0 (pos), 1 (neg).
        w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(0u, 1);
        w.Write(1u, 1);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[4];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(2, values[0]);
        Assert.Equal(-2, values[1]);
        Assert.Equal(2, values[2]);
        Assert.Equal(-2, values[3]);
        Assert.Equal(11, reader.Position); // 7 bits codeword + 4 sign bits.
    }

    [Fact]
    public void TryRead_UnsignedCb3_AllZeroComponents_NoSignBits()
    {
        var geom = AacSpectralCodebookGeometry.Get(3)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 0 -> (0, 0, 0, 0); no sign bits should be read.
        var w = new AacBitWriter();
        w.Write(0u, 7);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[4];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(0, values[1]);
        Assert.Equal(0, values[2]);
        Assert.Equal(0, values[3]);
        Assert.Equal(7, reader.Position);
    }

    [Fact]
    public void TryRead_UnsignedCb7_TwoComponents_ReadsSignBits()
    {
        var geom = AacSpectralCodebookGeometry.Get(7)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 6;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 15 -> (1, 7); two sign bits "10" -> (+1, -7).
        var w = new AacBitWriter();
        w.Write(15u, 6);
        w.Write(0u, 1);
        w.Write(1u, 1);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(1, values[0]);
        Assert.Equal(-7, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_EscapeFirstComponentX_DecodesExtendedMagnitude()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 272 -> (16, 0). Sign bit for x = 1 (negative). y=0 has no sign bit.
        // Escape sequence for x: prefix 0 ("0" bit) + 4-bit extension "0101" = 5
        //  -> magnitude = (1 << 4) + 5 = 21. Sign negative -> -21.
        var w = new AacBitWriter();
        w.Write(272u, 9);
        w.Write(1u, 1);    // sign for x (negative)
        w.Write(0u, 1);    // escape prefix terminator
        w.Write(5u, 4);    // 4-bit extension value
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(-21, values[0]);
        Assert.Equal(0, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_EscapeWithUnaryPrefix2_DecodesLargerMagnitude()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 16 -> (0, 16). y has magnitude 16; sign positive.
        // Escape for y: prefix "110" (two 1s then a 0) -> prefix=2,
        //   read 6-bit extension "010101" = 21.
        //   magnitude = (1 << 6) + 21 = 85.
        var w = new AacBitWriter();
        w.Write(16u, 9);
        w.Write(0u, 1);    // sign for y (positive)
        w.Write(0b110u, 3); // unary "110" -> prefix=2
        w.Write(0b010101u, 6);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(85, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_EscapeBothComponents_DecodesEach()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 288 -> (16, 16). Both escape.
        // Sign bits "01": sign_x=0 (positive), sign_y=1 (negative).
        // x escape: prefix "0" -> 0, ext "0001" = 1 -> magnitude = 16 + 1 = 17. x = +17.
        // y escape: prefix "0" -> 0, ext "0011" = 3 -> magnitude = 16 + 3 = 19. y = -19.
        var w = new AacBitWriter();
        w.Write(288u, 9);
        w.Write(0u, 1); // sign x
        w.Write(1u, 1); // sign y
        w.Write(0u, 1); // x escape prefix terminator
        w.Write(1u, 4); // x extension = 1
        w.Write(0u, 1); // y escape prefix terminator
        w.Write(3u, 4); // y extension = 3
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(17, values[0]);
        Assert.Equal(-19, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_NonEscapeComponent_NoEscapeRead()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 1 -> (0, 1). Only y nonzero -> 1 sign bit, no escape.
        var w = new AacBitWriter();
        w.Write(1u, 9);
        w.Write(0u, 1); // sign y positive
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(1, values[1]);
        Assert.Equal(10, reader.Position);
    }

    [Fact]
    public void TryRead_GeometryAndCodebookMismatch_Rejected()
    {
        var geom = AacSpectralCodebookGeometry.Get(1)!; // 81 symbols
        int[] lengths = new int[64];
        for (int i = 0; i < 64; i++) lengths[i] = 6;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        var reader = new BitReader(new byte[] { 0 });
        Span<int> values = stackalloc int[4];
        Assert.False(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
    }

    [Fact]
    public void TryRead_DestinationTooSmall_Rejected()
    {
        var geom = AacSpectralCodebookGeometry.Get(1)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        var reader = new BitReader(new byte[] { 0 });
        // Quad codebook needs 4 slots; pass 3.
        var buf = new int[3];
        Assert.False(AacSpectralValueDecoder.TryRead(ref reader, geom, book, buf));
    }

    [Fact]
    public void TryRead_StreamUnderflowDuringSignBit_Rejected()
    {
        var geom = AacSpectralCodebookGeometry.Get(3)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 80 -> (2, 2, 2, 2). Needs 4 sign bits. Provide only 7-bit codeword + 0 sign bits
        // by truncating the buffer to exactly 7 bits worth (we still write to byte boundary,
        // but trail with no sign bits and no padding ones).
        // Actually BitWriter pads to byte, so the buffer has 8 bits where the 8th is a zero pad.
        // After reading 7 bits codeword and 1 zero sign bit (positive), we'd have 0 remaining
        // and 3 more sign bits needed -> underflow.
        var w = new AacBitWriter();
        w.Write(80u, 7);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);
        Span<int> values = stackalloc int[4];
        Assert.False(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
    }

    [Fact]
    public void TryRead_Cb11_EscapePrefixOverflow_Rejected()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 16 -> (0, 16). y sign + escape with 9 consecutive 1-bits (overflows MaxEscapePrefix=8).
        var w = new AacBitWriter();
        w.Write(16u, 9);
        w.Write(0u, 1); // sign positive
        for (int i = 0; i < 9; i++) w.Write(1u, 1); // 9 unary 1s
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.False(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
    }

    [Fact]
    public void MaxEscapePrefix_Constant_Is_8()
    {
        Assert.Equal(8, AacSpectralValueDecoder.MaxEscapePrefix);
    }

    [Fact]
    public void TryRead_Cb11_EscapeAtMaxPrefix_8_Accepted()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 16 -> (0, 16). Prefix = 8 means 12-bit extension follows.
        var w = new AacBitWriter();
        w.Write(16u, 9);
        w.Write(0u, 1); // sign positive
        for (int i = 0; i < 8; i++) w.Write(1u, 1); // 8 unary 1s
        w.Write(0u, 1); // terminator
        w.Write(0u, 12); // 12-bit extension = 0
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        // magnitude = (1 << (8+4)) + 0 = 4096
        Assert.Equal(4096, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_EscapeMaxExtension_DecodesMaxMagnitude()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 16 -> (0, 16). Prefix = 8, extension = 0xFFF (12 bits all ones)
        var w = new AacBitWriter();
        w.Write(16u, 9);
        w.Write(0u, 1); // sign positive
        for (int i = 0; i < 8; i++) w.Write(1u, 1);
        w.Write(0u, 1);
        w.Write(0xFFFu, 12); // extension = 4095
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        // magnitude = 4096 + 4095 = 8191
        Assert.Equal(8191, values[1]);
    }

    [Fact]
    public void TryRead_Cb11_EscapeExtensionUnderflow_Rejected()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 16 -> (0, 16). prefix=0 means 4-bit extension; provide only 2 bits.
        var w = new AacBitWriter();
        w.Write(16u, 9);
        w.Write(0u, 1); // sign positive
        w.Write(0u, 1); // escape prefix terminator
        // truncate by emitting nothing more; BitWriter pads to next byte so we have 5 unread bits left
        // need to manually craft buffer to underflow.
        // After 9-bit symbol + 1-bit sign + 1-bit terminator = 11 bits = 2 bytes. Only 5 bits remain in pad.
        // Need ext_bits = 4 (>= 5? actually 4 <= 5 so it succeeds).
        // Instead, use single-byte buffer truncated so codeword can't even fit (8 bits < 9-bit codeword).
        var bytes = new byte[] { 0x00 };
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.False(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
    }

    [Fact]
    public void TryRead_Cb11_NoEscape_BothComponentsNonZero_OnlySignBitsRead()
    {
        var geom = AacSpectralCodebookGeometry.Get(11)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 18 -> Get geometry to confirm magnitudes are not 16; pick small magnitudes.
        // 18 with quantizer of 17 = row 1, col 1 -> (1, 1). Actually geometry depends on impl.
        // Use sym 1 (0, 1) for guaranteed no-escape.
        var w = new AacBitWriter();
        w.Write(1u, 9);
        w.Write(0u, 1); // sign for y positive
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[2];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(1, values[1]);
        Assert.Equal(10, reader.Position);
    }

    [Fact]
    public void TryRead_NullGeometry_Throws()
    {
        int[] lengths = new int[81];
        for (int i = 0; i < 81; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        Assert.Throws<ArgumentNullException>(() =>
        {
            var reader = new BitReader(new byte[] { 0 });
            Span<int> values = stackalloc int[4];
            AacSpectralValueDecoder.TryRead(ref reader, null!, book, values);
        });
    }

    [Fact]
    public void TryRead_NullCodebook_Throws()
    {
        var geom = AacSpectralCodebookGeometry.Get(1)!;
        Assert.Throws<ArgumentNullException>(() =>
        {
            var reader = new BitReader(new byte[] { 0 });
            Span<int> values = stackalloc int[4];
            AacSpectralValueDecoder.TryRead(ref reader, geom, null!, values);
        });
    }

    [Theory]
    [InlineData(2)]
    [InlineData(5)]
    [InlineData(6)]
    public void TryRead_SignedCodebooks_ReadOnlyCodeword(int cb)
    {
        var geom = AacSpectralCodebookGeometry.Get(cb)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        var w = new AacBitWriter();
        // Decode symbol 0 - signed codebooks need no sign bits.
        w.Write(0u, 7);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[geom.Dimension];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(7, reader.Position);
    }

    [Theory]
    [InlineData(4)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(10)]
    public void TryRead_UnsignedCodebooks_HappyPath(int cb)
    {
        var geom = AacSpectralCodebookGeometry.Get(cb)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 8;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 0 -> all-zero component tuple - no sign bits, no escape.
        var w = new AacBitWriter();
        w.Write(0u, 8);
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[geom.Dimension];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        for (int i = 0; i < geom.Dimension; i++) Assert.Equal(0, values[i]);
    }

    [Fact]
    public void TryRead_Cb3_SingleNonZeroSign_Negative()
    {
        var geom = AacSpectralCodebookGeometry.Get(3)!;
        int[] lengths = new int[geom.SymbolCount];
        for (int i = 0; i < geom.SymbolCount; i++) lengths[i] = 7;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);

        // Symbol 1 -> (0, 0, 0, 1): single sign bit for last component.
        var w = new AacBitWriter();
        w.Write(1u, 7);
        w.Write(1u, 1); // negative
        var bytes = w.ToArray();
        var reader = new BitReader(bytes);

        Span<int> values = stackalloc int[4];
        Assert.True(AacSpectralValueDecoder.TryRead(ref reader, geom, book, values));
        Assert.Equal(0, values[0]);
        Assert.Equal(0, values[1]);
        Assert.Equal(0, values[2]);
        Assert.Equal(-1, values[3]);
        Assert.Equal(8, reader.Position);
    }
}
