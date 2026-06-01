namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// The eleven standard AAC spectral Huffman codebooks (cb 1..11) per
/// ISO/IEC 14496-3 Annex 4.A.2.2..4.A.2.12 / Tables 4.A.2..4.A.12.
/// Each codebook is built once at static init via
/// <see cref="AacHuffmanCodebook.FromExplicitCodes"/> from FFmpeg's
/// canonical <c>codesN</c> / <c>bitsN</c> tables in
/// <c>libavcodec/aactab.c</c>. The geometry of each codebook
/// (tuple size, signedness, maximum absolute value, escape) is
/// returned by <see cref="AacSpectralCodebookGeometry"/>.
/// </summary>
/// <remarks>
/// <para>Per ISO/IEC 14496-3 Table 4.A.1: cb 1, 2 use 4-tuples of
/// signed values in [-1..+1] (81 symbols, 3^4). cb 3, 4 use 4-tuples
/// of unsigned values in [0..2] (81 symbols). cb 5, 6 use 2-tuples
/// of signed values in [-4..+4] (81 symbols, 9^2). cb 7, 8 use
/// 2-tuples of unsigned values in [0..7] (64 symbols, 8^2). cb 9,
/// 10 use 2-tuples of unsigned values in [0..12] (169 symbols,
/// 13^2). cb 11 covers 2-tuples up to value 16 with an escape
/// (289 symbols, 17^2).</para>
/// </remarks>
public static class AacStandardSpectralCodebooks
{
    private static readonly Lazy<AacHuffmanCodebook>[] BookLookup =
    [
        new Lazy<AacHuffmanCodebook>(BuildCb1),
        new Lazy<AacHuffmanCodebook>(BuildCb2),
        new Lazy<AacHuffmanCodebook>(BuildCb3),
        new Lazy<AacHuffmanCodebook>(BuildCb4),
        new Lazy<AacHuffmanCodebook>(BuildCb5),
        new Lazy<AacHuffmanCodebook>(BuildCb6),
        new Lazy<AacHuffmanCodebook>(BuildCb7),
        new Lazy<AacHuffmanCodebook>(BuildCb8),
        new Lazy<AacHuffmanCodebook>(BuildCb9),
        new Lazy<AacHuffmanCodebook>(BuildCb10),
        new Lazy<AacHuffmanCodebook>(BuildCb11),
    ];

    /// <summary>Get the standard codebook for cb number <paramref name="cbNumber"/> (1..11).</summary>
    /// <exception cref="ArgumentOutOfRangeException">When <paramref name="cbNumber"/> is outside [1, 11].</exception>
    public static AacHuffmanCodebook GetCodebook(int cbNumber)
    {
        if (cbNumber < 1 || cbNumber > 11)
        {
            throw new ArgumentOutOfRangeException(
                nameof(cbNumber), cbNumber, "cb number must be in [1, 11].");
        }
        return BookLookup[cbNumber - 1].Value;
    }

    /// <summary>Standard spectral codebook cb 1 (Table 4.A.1).</summary>
    public static AacHuffmanCodebook Cb1 => BookLookup[0].Value;

    private static readonly uint[] Codes1 =
    [
        0x07f8, 0x01f1, 0x07fd, 0x03f5, 0x0068, 0x03f0, 0x07f7, 0x01ec, 
        0x07f5, 0x03f1, 0x0072, 0x03f4, 0x0074, 0x0011, 0x0076, 0x01eb, 
        0x006c, 0x03f6, 0x07fc, 0x01e1, 0x07f1, 0x01f0, 0x0061, 0x01f6, 
        0x07f2, 0x01ea, 0x07fb, 0x01f2, 0x0069, 0x01ed, 0x0077, 0x0017, 
        0x006f, 0x01e6, 0x0064, 0x01e5, 0x0067, 0x0015, 0x0062, 0x0012, 
        0x0000, 0x0014, 0x0065, 0x0016, 0x006d, 0x01e9, 0x0063, 0x01e4, 
        0x006b, 0x0013, 0x0071, 0x01e3, 0x0070, 0x01f3, 0x07fe, 0x01e7, 
        0x07f3, 0x01ef, 0x0060, 0x01ee, 0x07f0, 0x01e2, 0x07fa, 0x03f3, 
        0x006a, 0x01e8, 0x0075, 0x0010, 0x0073, 0x01f4, 0x006e, 0x03f7, 
        0x07f6, 0x01e0, 0x07f9, 0x03f2, 0x0066, 0x01f5, 0x07ff, 0x01f7, 
        0x07f4,
    ];

    private static readonly int[] Bits1 =
    [
         11,   9,  11,  10,   7,  10,  11,   9,  11,  10,   7,  10,   7,   5,   7,   9, 
          7,  10,  11,   9,  11,   9,   7,   9,  11,   9,  11,   9,   7,   9,   7,   5, 
          7,   9,   7,   9,   7,   5,   7,   5,   1,   5,   7,   5,   7,   9,   7,   9, 
          7,   5,   7,   9,   7,   9,  11,   9,  11,   9,   7,   9,  11,   9,  11,  10, 
          7,   9,   7,   5,   7,   9,   7,  10,  11,   9,  11,  10,   7,   9,  11,   9, 
         11,
    ];

    private static AacHuffmanCodebook BuildCb1()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes1, Bits1);
    }

    /// <summary>Standard spectral codebook cb 2 (Table 4.A.2).</summary>
    public static AacHuffmanCodebook Cb2 => BookLookup[1].Value;

    private static readonly uint[] Codes2 =
    [
        0x01f3, 0x006f, 0x01fd, 0x00eb, 0x0023, 0x00ea, 0x01f7, 0x00e8, 
        0x01fa, 0x00f2, 0x002d, 0x0070, 0x0020, 0x0006, 0x002b, 0x006e, 
        0x0028, 0x00e9, 0x01f9, 0x0066, 0x00f8, 0x00e7, 0x001b, 0x00f1, 
        0x01f4, 0x006b, 0x01f5, 0x00ec, 0x002a, 0x006c, 0x002c, 0x000a, 
        0x0027, 0x0067, 0x001a, 0x00f5, 0x0024, 0x0008, 0x001f, 0x0009, 
        0x0000, 0x0007, 0x001d, 0x000b, 0x0030, 0x00ef, 0x001c, 0x0064, 
        0x001e, 0x000c, 0x0029, 0x00f3, 0x002f, 0x00f0, 0x01fc, 0x0071, 
        0x01f2, 0x00f4, 0x0021, 0x00e6, 0x00f7, 0x0068, 0x01f8, 0x00ee, 
        0x0022, 0x0065, 0x0031, 0x0002, 0x0026, 0x00ed, 0x0025, 0x006a, 
        0x01fb, 0x0072, 0x01fe, 0x0069, 0x002e, 0x00f6, 0x01ff, 0x006d, 
        0x01f6,
    ];

    private static readonly int[] Bits2 =
    [
          9,   7,   9,   8,   6,   8,   9,   8,   9,   8,   6,   7,   6,   5,   6,   7, 
          6,   8,   9,   7,   8,   8,   6,   8,   9,   7,   9,   8,   6,   7,   6,   5, 
          6,   7,   6,   8,   6,   5,   6,   5,   3,   5,   6,   5,   6,   8,   6,   7, 
          6,   5,   6,   8,   6,   8,   9,   7,   9,   8,   6,   8,   8,   7,   9,   8, 
          6,   7,   6,   4,   6,   8,   6,   7,   9,   7,   9,   7,   6,   8,   9,   7, 
          9,
    ];

    private static AacHuffmanCodebook BuildCb2()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes2, Bits2);
    }

    /// <summary>Standard spectral codebook cb 3 (Table 4.A.3).</summary>
    public static AacHuffmanCodebook Cb3 => BookLookup[2].Value;

    private static readonly uint[] Codes3 =
    [
        0x0000, 0x0009, 0x00ef, 0x000b, 0x0019, 0x00f0, 0x01eb, 0x01e6, 
        0x03f2, 0x000a, 0x0035, 0x01ef, 0x0034, 0x0037, 0x01e9, 0x01ed, 
        0x01e7, 0x03f3, 0x01ee, 0x03ed, 0x1ffa, 0x01ec, 0x01f2, 0x07f9, 
        0x07f8, 0x03f8, 0x0ff8, 0x0008, 0x0038, 0x03f6, 0x0036, 0x0075, 
        0x03f1, 0x03eb, 0x03ec, 0x0ff4, 0x0018, 0x0076, 0x07f4, 0x0039, 
        0x0074, 0x03ef, 0x01f3, 0x01f4, 0x07f6, 0x01e8, 0x03ea, 0x1ffc, 
        0x00f2, 0x01f1, 0x0ffb, 0x03f5, 0x07f3, 0x0ffc, 0x00ee, 0x03f7, 
        0x7ffe, 0x01f0, 0x07f5, 0x7ffd, 0x1ffb, 0x3ffa, 0xffff, 0x00f1, 
        0x03f0, 0x3ffc, 0x01ea, 0x03ee, 0x3ffb, 0x0ff6, 0x0ffa, 0x7ffc, 
        0x07f2, 0x0ff5, 0xfffe, 0x03f4, 0x07f7, 0x7ffb, 0x0ff7, 0x0ff9, 
        0x7ffa,
    ];

    private static readonly int[] Bits3 =
    [
          1,   4,   8,   4,   5,   8,   9,   9,  10,   4,   6,   9,   6,   6,   9,   9, 
          9,  10,   9,  10,  13,   9,   9,  11,  11,  10,  12,   4,   6,  10,   6,   7, 
         10,  10,  10,  12,   5,   7,  11,   6,   7,  10,   9,   9,  11,   9,  10,  13, 
          8,   9,  12,  10,  11,  12,   8,  10,  15,   9,  11,  15,  13,  14,  16,   8, 
         10,  14,   9,  10,  14,  12,  12,  15,  11,  12,  16,  10,  11,  15,  12,  12, 
         15,
    ];

    private static AacHuffmanCodebook BuildCb3()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes3, Bits3);
    }

    /// <summary>Standard spectral codebook cb 4 (Table 4.A.4).</summary>
    public static AacHuffmanCodebook Cb4 => BookLookup[3].Value;

    private static readonly uint[] Codes4 =
    [
        0x0007, 0x0016, 0x00f6, 0x0018, 0x0008, 0x00ef, 0x01ef, 0x00f3, 
        0x07f8, 0x0019, 0x0017, 0x00ed, 0x0015, 0x0001, 0x00e2, 0x00f0, 
        0x0070, 0x03f0, 0x01ee, 0x00f1, 0x07fa, 0x00ee, 0x00e4, 0x03f2, 
        0x07f6, 0x03ef, 0x07fd, 0x0005, 0x0014, 0x00f2, 0x0009, 0x0004, 
        0x00e5, 0x00f4, 0x00e8, 0x03f4, 0x0006, 0x0002, 0x00e7, 0x0003, 
        0x0000, 0x006b, 0x00e3, 0x0069, 0x01f3, 0x00eb, 0x00e6, 0x03f6, 
        0x006e, 0x006a, 0x01f4, 0x03ec, 0x01f0, 0x03f9, 0x00f5, 0x00ec, 
        0x07fb, 0x00ea, 0x006f, 0x03f7, 0x07f9, 0x03f3, 0x0fff, 0x00e9, 
        0x006d, 0x03f8, 0x006c, 0x0068, 0x01f5, 0x03ee, 0x01f2, 0x07f4, 
        0x07f7, 0x03f1, 0x0ffe, 0x03ed, 0x01f1, 0x07f5, 0x07fe, 0x03f5, 
        0x07fc,
    ];

    private static readonly int[] Bits4 =
    [
          4,   5,   8,   5,   4,   8,   9,   8,  11,   5,   5,   8,   5,   4,   8,   8, 
          7,  10,   9,   8,  11,   8,   8,  10,  11,  10,  11,   4,   5,   8,   4,   4, 
          8,   8,   8,  10,   4,   4,   8,   4,   4,   7,   8,   7,   9,   8,   8,  10, 
          7,   7,   9,  10,   9,  10,   8,   8,  11,   8,   7,  10,  11,  10,  12,   8, 
          7,  10,   7,   7,   9,  10,   9,  11,  11,  10,  12,  10,   9,  11,  11,  10, 
         11,
    ];

    private static AacHuffmanCodebook BuildCb4()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes4, Bits4);
    }

    /// <summary>Standard spectral codebook cb 5 (Table 4.A.5).</summary>
    public static AacHuffmanCodebook Cb5 => BookLookup[4].Value;

    private static readonly uint[] Codes5 =
    [
        0x1fff, 0x0ff7, 0x07f4, 0x07e8, 0x03f1, 0x07ee, 0x07f9, 0x0ff8, 
        0x1ffd, 0x0ffd, 0x07f1, 0x03e8, 0x01e8, 0x00f0, 0x01ec, 0x03ee, 
        0x07f2, 0x0ffa, 0x0ff4, 0x03ef, 0x01f2, 0x00e8, 0x0070, 0x00ec, 
        0x01f0, 0x03ea, 0x07f3, 0x07eb, 0x01eb, 0x00ea, 0x001a, 0x0008, 
        0x0019, 0x00ee, 0x01ef, 0x07ed, 0x03f0, 0x00f2, 0x0073, 0x000b, 
        0x0000, 0x000a, 0x0071, 0x00f3, 0x07e9, 0x07ef, 0x01ee, 0x00ef, 
        0x0018, 0x0009, 0x001b, 0x00eb, 0x01e9, 0x07ec, 0x07f6, 0x03eb, 
        0x01f3, 0x00ed, 0x0072, 0x00e9, 0x01f1, 0x03ed, 0x07f7, 0x0ff6, 
        0x07f0, 0x03e9, 0x01ed, 0x00f1, 0x01ea, 0x03ec, 0x07f8, 0x0ff9, 
        0x1ffc, 0x0ffc, 0x0ff5, 0x07ea, 0x03f3, 0x03f2, 0x07f5, 0x0ffb, 
        0x1ffe,
    ];

    private static readonly int[] Bits5 =
    [
         13,  12,  11,  11,  10,  11,  11,  12,  13,  12,  11,  10,   9,   8,   9,  10, 
         11,  12,  12,  10,   9,   8,   7,   8,   9,  10,  11,  11,   9,   8,   5,   4, 
          5,   8,   9,  11,  10,   8,   7,   4,   1,   4,   7,   8,  11,  11,   9,   8, 
          5,   4,   5,   8,   9,  11,  11,  10,   9,   8,   7,   8,   9,  10,  11,  12, 
         11,  10,   9,   8,   9,  10,  11,  12,  13,  12,  12,  11,  10,  10,  11,  12, 
         13,
    ];

    private static AacHuffmanCodebook BuildCb5()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes5, Bits5);
    }

    /// <summary>Standard spectral codebook cb 6 (Table 4.A.6).</summary>
    public static AacHuffmanCodebook Cb6 => BookLookup[5].Value;

    private static readonly uint[] Codes6 =
    [
        0x07fe, 0x03fd, 0x01f1, 0x01eb, 0x01f4, 0x01ea, 0x01f0, 0x03fc, 
        0x07fd, 0x03f6, 0x01e5, 0x00ea, 0x006c, 0x0071, 0x0068, 0x00f0, 
        0x01e6, 0x03f7, 0x01f3, 0x00ef, 0x0032, 0x0027, 0x0028, 0x0026, 
        0x0031, 0x00eb, 0x01f7, 0x01e8, 0x006f, 0x002e, 0x0008, 0x0004, 
        0x0006, 0x0029, 0x006b, 0x01ee, 0x01ef, 0x0072, 0x002d, 0x0002, 
        0x0000, 0x0003, 0x002f, 0x0073, 0x01fa, 0x01e7, 0x006e, 0x002b, 
        0x0007, 0x0001, 0x0005, 0x002c, 0x006d, 0x01ec, 0x01f9, 0x00ee, 
        0x0030, 0x0024, 0x002a, 0x0025, 0x0033, 0x00ec, 0x01f2, 0x03f8, 
        0x01e4, 0x00ed, 0x006a, 0x0070, 0x0069, 0x0074, 0x00f1, 0x03fa, 
        0x07ff, 0x03f9, 0x01f6, 0x01ed, 0x01f8, 0x01e9, 0x01f5, 0x03fb, 
        0x07fc,
    ];

    private static readonly int[] Bits6 =
    [
         11,  10,   9,   9,   9,   9,   9,  10,  11,  10,   9,   8,   7,   7,   7,   8, 
          9,  10,   9,   8,   6,   6,   6,   6,   6,   8,   9,   9,   7,   6,   4,   4, 
          4,   6,   7,   9,   9,   7,   6,   4,   4,   4,   6,   7,   9,   9,   7,   6, 
          4,   4,   4,   6,   7,   9,   9,   8,   6,   6,   6,   6,   6,   8,   9,  10, 
          9,   8,   7,   7,   7,   7,   8,  10,  11,  10,   9,   9,   9,   9,   9,  10, 
         11,
    ];

    private static AacHuffmanCodebook BuildCb6()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes6, Bits6);
    }

    /// <summary>Standard spectral codebook cb 7 (Table 4.A.7).</summary>
    public static AacHuffmanCodebook Cb7 => BookLookup[6].Value;

    private static readonly uint[] Codes7 =
    [
        0x0000, 0x0005, 0x0037, 0x0074, 0x00f2, 0x01eb, 0x03ed, 0x07f7, 
        0x0004, 0x000c, 0x0035, 0x0071, 0x00ec, 0x00ee, 0x01ee, 0x01f5, 
        0x0036, 0x0034, 0x0072, 0x00ea, 0x00f1, 0x01e9, 0x01f3, 0x03f5, 
        0x0073, 0x0070, 0x00eb, 0x00f0, 0x01f1, 0x01f0, 0x03ec, 0x03fa, 
        0x00f3, 0x00ed, 0x01e8, 0x01ef, 0x03ef, 0x03f1, 0x03f9, 0x07fb, 
        0x01ed, 0x00ef, 0x01ea, 0x01f2, 0x03f3, 0x03f8, 0x07f9, 0x07fc, 
        0x03ee, 0x01ec, 0x01f4, 0x03f4, 0x03f7, 0x07f8, 0x0ffd, 0x0ffe, 
        0x07f6, 0x03f0, 0x03f2, 0x03f6, 0x07fa, 0x07fd, 0x0ffc, 0x0fff,
    ];

    private static readonly int[] Bits7 =
    [
          1,   3,   6,   7,   8,   9,  10,  11,   3,   4,   6,   7,   8,   8,   9,   9, 
          6,   6,   7,   8,   8,   9,   9,  10,   7,   7,   8,   8,   9,   9,  10,  10, 
          8,   8,   9,   9,  10,  10,  10,  11,   9,   8,   9,   9,  10,  10,  11,  11, 
         10,   9,   9,  10,  10,  11,  12,  12,  11,  10,  10,  10,  11,  11,  12,  12,
    ];

    private static AacHuffmanCodebook BuildCb7()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes7, Bits7);
    }

    /// <summary>Standard spectral codebook cb 8 (Table 4.A.8).</summary>
    public static AacHuffmanCodebook Cb8 => BookLookup[7].Value;

    private static readonly uint[] Codes8 =
    [
        0x000e, 0x0005, 0x0010, 0x0030, 0x006f, 0x00f1, 0x01fa, 0x03fe, 
        0x0003, 0x0000, 0x0004, 0x0012, 0x002c, 0x006a, 0x0075, 0x00f8, 
        0x000f, 0x0002, 0x0006, 0x0014, 0x002e, 0x0069, 0x0072, 0x00f5, 
        0x002f, 0x0011, 0x0013, 0x002a, 0x0032, 0x006c, 0x00ec, 0x00fa, 
        0x0071, 0x002b, 0x002d, 0x0031, 0x006d, 0x0070, 0x00f2, 0x01f9, 
        0x00ef, 0x0068, 0x0033, 0x006b, 0x006e, 0x00ee, 0x00f9, 0x03fc, 
        0x01f8, 0x0074, 0x0073, 0x00ed, 0x00f0, 0x00f6, 0x01f6, 0x01fd, 
        0x03fd, 0x00f3, 0x00f4, 0x00f7, 0x01f7, 0x01fb, 0x01fc, 0x03ff,
    ];

    private static readonly int[] Bits8 =
    [
          5,   4,   5,   6,   7,   8,   9,  10,   4,   3,   4,   5,   6,   7,   7,   8, 
          5,   4,   4,   5,   6,   7,   7,   8,   6,   5,   5,   6,   6,   7,   8,   8, 
          7,   6,   6,   6,   7,   7,   8,   9,   8,   7,   6,   7,   7,   8,   8,  10, 
          9,   7,   7,   8,   8,   8,   9,   9,  10,   8,   8,   8,   9,   9,   9,  10,
    ];

    private static AacHuffmanCodebook BuildCb8()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes8, Bits8);
    }

    /// <summary>Standard spectral codebook cb 9 (Table 4.A.9).</summary>
    public static AacHuffmanCodebook Cb9 => BookLookup[8].Value;

    private static readonly uint[] Codes9 =
    [
        0x0000, 0x0005, 0x0037, 0x00e7, 0x01de, 0x03ce, 0x03d9, 0x07c8, 
        0x07cd, 0x0fc8, 0x0fdd, 0x1fe4, 0x1fec, 0x0004, 0x000c, 0x0035, 
        0x0072, 0x00ea, 0x00ed, 0x01e2, 0x03d1, 0x03d3, 0x03e0, 0x07d8, 
        0x0fcf, 0x0fd5, 0x0036, 0x0034, 0x0071, 0x00e8, 0x00ec, 0x01e1, 
        0x03cf, 0x03dd, 0x03db, 0x07d0, 0x0fc7, 0x0fd4, 0x0fe4, 0x00e6, 
        0x0070, 0x00e9, 0x01dd, 0x01e3, 0x03d2, 0x03dc, 0x07cc, 0x07ca, 
        0x07de, 0x0fd8, 0x0fea, 0x1fdb, 0x01df, 0x00eb, 0x01dc, 0x01e6, 
        0x03d5, 0x03de, 0x07cb, 0x07dd, 0x07dc, 0x0fcd, 0x0fe2, 0x0fe7, 
        0x1fe1, 0x03d0, 0x01e0, 0x01e4, 0x03d6, 0x07c5, 0x07d1, 0x07db, 
        0x0fd2, 0x07e0, 0x0fd9, 0x0feb, 0x1fe3, 0x1fe9, 0x07c4, 0x01e5, 
        0x03d7, 0x07c6, 0x07cf, 0x07da, 0x0fcb, 0x0fda, 0x0fe3, 0x0fe9, 
        0x1fe6, 0x1ff3, 0x1ff7, 0x07d3, 0x03d8, 0x03e1, 0x07d4, 0x07d9, 
        0x0fd3, 0x0fde, 0x1fdd, 0x1fd9, 0x1fe2, 0x1fea, 0x1ff1, 0x1ff6, 
        0x07d2, 0x03d4, 0x03da, 0x07c7, 0x07d7, 0x07e2, 0x0fce, 0x0fdb, 
        0x1fd8, 0x1fee, 0x3ff0, 0x1ff4, 0x3ff2, 0x07e1, 0x03df, 0x07c9, 
        0x07d6, 0x0fca, 0x0fd0, 0x0fe5, 0x0fe6, 0x1feb, 0x1fef, 0x3ff3, 
        0x3ff4, 0x3ff5, 0x0fe0, 0x07ce, 0x07d5, 0x0fc6, 0x0fd1, 0x0fe1, 
        0x1fe0, 0x1fe8, 0x1ff0, 0x3ff1, 0x3ff8, 0x3ff6, 0x7ffc, 0x0fe8, 
        0x07df, 0x0fc9, 0x0fd7, 0x0fdc, 0x1fdc, 0x1fdf, 0x1fed, 0x1ff5, 
        0x3ff9, 0x3ffb, 0x7ffd, 0x7ffe, 0x1fe7, 0x0fcc, 0x0fd6, 0x0fdf, 
        0x1fde, 0x1fda, 0x1fe5, 0x1ff2, 0x3ffa, 0x3ff7, 0x3ffc, 0x3ffd, 
        0x7fff,
    ];

    private static readonly int[] Bits9 =
    [
          1,   3,   6,   8,   9,  10,  10,  11,  11,  12,  12,  13,  13,   3,   4,   6, 
          7,   8,   8,   9,  10,  10,  10,  11,  12,  12,   6,   6,   7,   8,   8,   9, 
         10,  10,  10,  11,  12,  12,  12,   8,   7,   8,   9,   9,  10,  10,  11,  11, 
         11,  12,  12,  13,   9,   8,   9,   9,  10,  10,  11,  11,  11,  12,  12,  12, 
         13,  10,   9,   9,  10,  11,  11,  11,  12,  11,  12,  12,  13,  13,  11,   9, 
         10,  11,  11,  11,  12,  12,  12,  12,  13,  13,  13,  11,  10,  10,  11,  11, 
         12,  12,  13,  13,  13,  13,  13,  13,  11,  10,  10,  11,  11,  11,  12,  12, 
         13,  13,  14,  13,  14,  11,  10,  11,  11,  12,  12,  12,  12,  13,  13,  14, 
         14,  14,  12,  11,  11,  12,  12,  12,  13,  13,  13,  14,  14,  14,  15,  12, 
         11,  12,  12,  12,  13,  13,  13,  13,  14,  14,  15,  15,  13,  12,  12,  12, 
         13,  13,  13,  13,  14,  14,  14,  14,  15,
    ];

    private static AacHuffmanCodebook BuildCb9()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes9, Bits9);
    }

    /// <summary>Standard spectral codebook cb 10 (Table 4.A.10).</summary>
    public static AacHuffmanCodebook Cb10 => BookLookup[9].Value;

    private static readonly uint[] Codes10 =
    [
        0x0022, 0x0008, 0x001d, 0x0026, 0x005f, 0x00d3, 0x01cf, 0x03d0, 
        0x03d7, 0x03ed, 0x07f0, 0x07f6, 0x0ffd, 0x0007, 0x0000, 0x0001, 
        0x0009, 0x0020, 0x0054, 0x0060, 0x00d5, 0x00dc, 0x01d4, 0x03cd, 
        0x03de, 0x07e7, 0x001c, 0x0002, 0x0006, 0x000c, 0x001e, 0x0028, 
        0x005b, 0x00cd, 0x00d9, 0x01ce, 0x01dc, 0x03d9, 0x03f1, 0x0025, 
        0x000b, 0x000a, 0x000d, 0x0024, 0x0057, 0x0061, 0x00cc, 0x00dd, 
        0x01cc, 0x01de, 0x03d3, 0x03e7, 0x005d, 0x0021, 0x001f, 0x0023, 
        0x0027, 0x0059, 0x0064, 0x00d8, 0x00df, 0x01d2, 0x01e2, 0x03dd, 
        0x03ee, 0x00d1, 0x0055, 0x0029, 0x0056, 0x0058, 0x0062, 0x00ce, 
        0x00e0, 0x00e2, 0x01da, 0x03d4, 0x03e3, 0x07eb, 0x01c9, 0x005e, 
        0x005a, 0x005c, 0x0063, 0x00ca, 0x00da, 0x01c7, 0x01ca, 0x01e0, 
        0x03db, 0x03e8, 0x07ec, 0x01e3, 0x00d2, 0x00cb, 0x00d0, 0x00d7, 
        0x00db, 0x01c6, 0x01d5, 0x01d8, 0x03ca, 0x03da, 0x07ea, 0x07f1, 
        0x01e1, 0x00d4, 0x00cf, 0x00d6, 0x00de, 0x00e1, 0x01d0, 0x01d6, 
        0x03d1, 0x03d5, 0x03f2, 0x07ee, 0x07fb, 0x03e9, 0x01cd, 0x01c8, 
        0x01cb, 0x01d1, 0x01d7, 0x01df, 0x03cf, 0x03e0, 0x03ef, 0x07e6, 
        0x07f8, 0x0ffa, 0x03eb, 0x01dd, 0x01d3, 0x01d9, 0x01db, 0x03d2, 
        0x03cc, 0x03dc, 0x03ea, 0x07ed, 0x07f3, 0x07f9, 0x0ff9, 0x07f2, 
        0x03ce, 0x01e4, 0x03cb, 0x03d8, 0x03d6, 0x03e2, 0x03e5, 0x07e8, 
        0x07f4, 0x07f5, 0x07f7, 0x0ffb, 0x07fa, 0x03ec, 0x03df, 0x03e1, 
        0x03e4, 0x03e6, 0x03f0, 0x07e9, 0x07ef, 0x0ff8, 0x0ffe, 0x0ffc, 
        0x0fff,
    ];

    private static readonly int[] Bits10 =
    [
          6,   5,   6,   6,   7,   8,   9,  10,  10,  10,  11,  11,  12,   5,   4,   4, 
          5,   6,   7,   7,   8,   8,   9,  10,  10,  11,   6,   4,   5,   5,   6,   6, 
          7,   8,   8,   9,   9,  10,  10,   6,   5,   5,   5,   6,   7,   7,   8,   8, 
          9,   9,  10,  10,   7,   6,   6,   6,   6,   7,   7,   8,   8,   9,   9,  10, 
         10,   8,   7,   6,   7,   7,   7,   8,   8,   8,   9,  10,  10,  11,   9,   7, 
          7,   7,   7,   8,   8,   9,   9,   9,  10,  10,  11,   9,   8,   8,   8,   8, 
          8,   9,   9,   9,  10,  10,  11,  11,   9,   8,   8,   8,   8,   8,   9,   9, 
         10,  10,  10,  11,  11,  10,   9,   9,   9,   9,   9,   9,  10,  10,  10,  11, 
         11,  12,  10,   9,   9,   9,   9,  10,  10,  10,  10,  11,  11,  11,  12,  11, 
         10,   9,  10,  10,  10,  10,  10,  11,  11,  11,  11,  12,  11,  10,  10,  10, 
         10,  10,  10,  11,  11,  12,  12,  12,  12,
    ];

    private static AacHuffmanCodebook BuildCb10()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes10, Bits10);
    }

    /// <summary>Standard spectral codebook cb 11 (Table 4.A.11).</summary>
    public static AacHuffmanCodebook Cb11 => BookLookup[10].Value;

    private static readonly uint[] Codes11 =
    [
        0x0000, 0x0006, 0x0019, 0x003d, 0x009c, 0x00c6, 0x01a7, 0x0390, 
        0x03c2, 0x03df, 0x07e6, 0x07f3, 0x0ffb, 0x07ec, 0x0ffa, 0x0ffe, 
        0x038e, 0x0005, 0x0001, 0x0008, 0x0014, 0x0037, 0x0042, 0x0092, 
        0x00af, 0x0191, 0x01a5, 0x01b5, 0x039e, 0x03c0, 0x03a2, 0x03cd, 
        0x07d6, 0x00ae, 0x0017, 0x0007, 0x0009, 0x0018, 0x0039, 0x0040, 
        0x008e, 0x00a3, 0x00b8, 0x0199, 0x01ac, 0x01c1, 0x03b1, 0x0396, 
        0x03be, 0x03ca, 0x009d, 0x003c, 0x0015, 0x0016, 0x001a, 0x003b, 
        0x0044, 0x0091, 0x00a5, 0x00be, 0x0196, 0x01ae, 0x01b9, 0x03a1, 
        0x0391, 0x03a5, 0x03d5, 0x0094, 0x009a, 0x0036, 0x0038, 0x003a, 
        0x0041, 0x008c, 0x009b, 0x00b0, 0x00c3, 0x019e, 0x01ab, 0x01bc, 
        0x039f, 0x038f, 0x03a9, 0x03cf, 0x0093, 0x00bf, 0x003e, 0x003f, 
        0x0043, 0x0045, 0x009e, 0x00a7, 0x00b9, 0x0194, 0x01a2, 0x01ba, 
        0x01c3, 0x03a6, 0x03a7, 0x03bb, 0x03d4, 0x009f, 0x01a0, 0x008f, 
        0x008d, 0x0090, 0x0098, 0x00a6, 0x00b6, 0x00c4, 0x019f, 0x01af, 
        0x01bf, 0x0399, 0x03bf, 0x03b4, 0x03c9, 0x03e7, 0x00a8, 0x01b6, 
        0x00ab, 0x00a4, 0x00aa, 0x00b2, 0x00c2, 0x00c5, 0x0198, 0x01a4, 
        0x01b8, 0x038c, 0x03a4, 0x03c4, 0x03c6, 0x03dd, 0x03e8, 0x00ad, 
        0x03af, 0x0192, 0x00bd, 0x00bc, 0x018e, 0x0197, 0x019a, 0x01a3, 
        0x01b1, 0x038d, 0x0398, 0x03b7, 0x03d3, 0x03d1, 0x03db, 0x07dd, 
        0x00b4, 0x03de, 0x01a9, 0x019b, 0x019c, 0x01a1, 0x01aa, 0x01ad, 
        0x01b3, 0x038b, 0x03b2, 0x03b8, 0x03ce, 0x03e1, 0x03e0, 0x07d2, 
        0x07e5, 0x00b7, 0x07e3, 0x01bb, 0x01a8, 0x01a6, 0x01b0, 0x01b2, 
        0x01b7, 0x039b, 0x039a, 0x03ba, 0x03b5, 0x03d6, 0x07d7, 0x03e4, 
        0x07d8, 0x07ea, 0x00ba, 0x07e8, 0x03a0, 0x01bd, 0x01b4, 0x038a, 
        0x01c4, 0x0392, 0x03aa, 0x03b0, 0x03bc, 0x03d7, 0x07d4, 0x07dc, 
        0x07db, 0x07d5, 0x07f0, 0x00c1, 0x07fb, 0x03c8, 0x03a3, 0x0395, 
        0x039d, 0x03ac, 0x03ae, 0x03c5, 0x03d8, 0x03e2, 0x03e6, 0x07e4, 
        0x07e7, 0x07e0, 0x07e9, 0x07f7, 0x0190, 0x07f2, 0x0393, 0x01be, 
        0x01c0, 0x0394, 0x0397, 0x03ad, 0x03c3, 0x03c1, 0x03d2, 0x07da, 
        0x07d9, 0x07df, 0x07eb, 0x07f4, 0x07fa, 0x0195, 0x07f8, 0x03bd, 
        0x039c, 0x03ab, 0x03a8, 0x03b3, 0x03b9, 0x03d0, 0x03e3, 0x03e5, 
        0x07e2, 0x07de, 0x07ed, 0x07f1, 0x07f9, 0x07fc, 0x0193, 0x0ffd, 
        0x03dc, 0x03b6, 0x03c7, 0x03cc, 0x03cb, 0x03d9, 0x03da, 0x07d3, 
        0x07e1, 0x07ee, 0x07ef, 0x07f5, 0x07f6, 0x0ffc, 0x0fff, 0x019d, 
        0x01c2, 0x00b5, 0x00a1, 0x0096, 0x0097, 0x0095, 0x0099, 0x00a0, 
        0x00a2, 0x00ac, 0x00a9, 0x00b1, 0x00b3, 0x00bb, 0x00c0, 0x018f, 
        0x0004,
    ];

    private static readonly int[] Bits11 =
    [
          4,   5,   6,   7,   8,   8,   9,  10,  10,  10,  11,  11,  12,  11,  12,  12, 
         10,   5,   4,   5,   6,   7,   7,   8,   8,   9,   9,   9,  10,  10,  10,  10, 
         11,   8,   6,   5,   5,   6,   7,   7,   8,   8,   8,   9,   9,   9,  10,  10, 
         10,  10,   8,   7,   6,   6,   6,   7,   7,   8,   8,   8,   9,   9,   9,  10, 
         10,  10,  10,   8,   8,   7,   7,   7,   7,   8,   8,   8,   8,   9,   9,   9, 
         10,  10,  10,  10,   8,   8,   7,   7,   7,   7,   8,   8,   8,   9,   9,   9, 
          9,  10,  10,  10,  10,   8,   9,   8,   8,   8,   8,   8,   8,   8,   9,   9, 
          9,  10,  10,  10,  10,  10,   8,   9,   8,   8,   8,   8,   8,   8,   9,   9, 
          9,  10,  10,  10,  10,  10,  10,   8,  10,   9,   8,   8,   9,   9,   9,   9, 
          9,  10,  10,  10,  10,  10,  10,  11,   8,  10,   9,   9,   9,   9,   9,   9, 
          9,  10,  10,  10,  10,  10,  10,  11,  11,   8,  11,   9,   9,   9,   9,   9, 
          9,  10,  10,  10,  10,  10,  11,  10,  11,  11,   8,  11,  10,   9,   9,  10, 
          9,  10,  10,  10,  10,  10,  11,  11,  11,  11,  11,   8,  11,  10,  10,  10, 
         10,  10,  10,  10,  10,  10,  10,  11,  11,  11,  11,  11,   9,  11,  10,   9, 
          9,  10,  10,  10,  10,  10,  10,  11,  11,  11,  11,  11,  11,   9,  11,  10, 
         10,  10,  10,  10,  10,  10,  10,  10,  11,  11,  11,  11,  11,  11,   9,  12, 
         10,  10,  10,  10,  10,  10,  10,  11,  11,  11,  11,  11,  11,  12,  12,   9, 
          9,   8,   8,   8,   8,   8,   8,   8,   8,   8,   8,   8,   8,   8,   8,   9, 
          5,
    ];

    private static AacHuffmanCodebook BuildCb11()
    {
        return AacHuffmanCodebook.FromExplicitCodes(Codes11, Bits11);
    }

}
