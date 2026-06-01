namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// The canonical AAC scalefactor Huffman codebook (cb 12) per
/// ISO/IEC 14496-3 Annex 4.A.2.1 / Table 4.A.13. Used by
/// <see cref="AacScaleFactorData"/> to decode the differential
/// scalefactor stream of an audio element.
/// </summary>
/// <remarks>
/// <para>
/// The 121-symbol alphabet covers signed scalefactor differences in
/// the range [-60..+60]; symbol index <c>i</c> maps to delta
/// <c>i − 60</c>. The most-likely value (delta = 0, symbol 60) is
/// encoded with the single-bit codeword "0"; large deltas climb to
/// 19-bit codewords.
/// </para>
/// <para>
/// Tables mirror FFmpeg's <c>ff_aac_scalefactor_code</c> and
/// <c>ff_aac_scalefactor_bits</c> (<c>libavcodec/aactab.c</c>). The
/// underlying <see cref="AacHuffmanCodebook"/> is built once via
/// <see cref="AacHuffmanCodebook.FromExplicitCodes"/> because the
/// spec's codewords are NOT in canonical Huffman order within each
/// length.
/// </para>
/// </remarks>
public static class AacStandardScaleFactorCodebook
{
    /// <summary>Number of symbols (deltas in [-60..+60]).</summary>
    public const int SymbolCount = 121;

    /// <summary>Symbol index corresponding to a delta of 0.</summary>
    public const int ZeroDeltaSymbolIndex = 60;

    /// <summary>Maximum absolute scalefactor delta representable.</summary>
    public const int MaxAbsoluteDelta = 60;

    private static readonly uint[] CodesArray =
    [
        0x3ffe8, 0x3ffe6, 0x3ffe7, 0x3ffe5, 0x7fff5, 0x7fff1, 0x7ffed, 0x7fff6,
        0x7ffee, 0x7ffef, 0x7fff0, 0x7fffc, 0x7fffd, 0x7ffff, 0x7fffe, 0x7fff7,
        0x7fff8, 0x7fffb, 0x7fff9, 0x3ffe4, 0x7fffa, 0x3ffe3, 0x1ffef, 0x1fff0,
        0x0fff5, 0x1ffee, 0x0fff2, 0x0fff3, 0x0fff4, 0x0fff1, 0x07ff6, 0x07ff7,
        0x03ff9, 0x03ff5, 0x03ff7, 0x03ff3, 0x03ff6, 0x03ff2, 0x01ff7, 0x01ff5,
        0x00ff9, 0x00ff7, 0x00ff6, 0x007f9, 0x00ff4, 0x007f8, 0x003f9, 0x003f7,
        0x003f5, 0x001f8, 0x001f7, 0x000fa, 0x000f8, 0x000f6, 0x00079, 0x0003a,
        0x00038, 0x0001a, 0x0000b, 0x00004, 0x00000, 0x0000a, 0x0000c, 0x0001b,
        0x00039, 0x0003b, 0x00078, 0x0007a, 0x000f7, 0x000f9, 0x001f6, 0x001f9,
        0x003f4, 0x003f6, 0x003f8, 0x007f5, 0x007f4, 0x007f6, 0x007f7, 0x00ff5,
        0x00ff8, 0x01ff4, 0x01ff6, 0x01ff8, 0x03ff8, 0x03ff4, 0x0fff0, 0x07ff4,
        0x0fff6, 0x07ff5, 0x3ffe2, 0x7ffd9, 0x7ffda, 0x7ffdb, 0x7ffdc, 0x7ffdd,
        0x7ffde, 0x7ffd8, 0x7ffd2, 0x7ffd3, 0x7ffd4, 0x7ffd5, 0x7ffd6, 0x7fff2,
        0x7ffdf, 0x7ffe7, 0x7ffe8, 0x7ffe9, 0x7ffea, 0x7ffeb, 0x7ffe6, 0x7ffe0,
        0x7ffe1, 0x7ffe2, 0x7ffe3, 0x7ffe4, 0x7ffe5, 0x7ffd7, 0x7ffec, 0x7fff4,
        0x7fff3,
    ];

    private static readonly int[] BitsArray =
    [
        18, 18, 18, 18, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19,
        19, 19, 19, 18, 19, 18, 17, 17, 16, 17, 16, 16, 16, 16, 15, 15,
        14, 14, 14, 14, 14, 14, 13, 13, 12, 12, 12, 11, 12, 11, 10, 10,
        10,  9,  9,  8,  8,  8,  7,  6,  6,  5,  4,  3,  1,  4,  4,  5,
         6,  6,  7,  7,  8,  8,  9,  9, 10, 10, 10, 11, 11, 11, 11, 12,
        12, 13, 13, 13, 14, 14, 16, 15, 16, 15, 18, 19, 19, 19, 19, 19,
        19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19, 19,
        19, 19, 19, 19, 19, 19, 19, 19, 19,
    ];

    private static readonly AacHuffmanCodebook BookInstance =
        BuildBook();

    private static AacHuffmanCodebook BuildBook()
    {
        if (CodesArray.Length != SymbolCount || BitsArray.Length != SymbolCount)
        {
            throw new InvalidOperationException(
                "AacStandardScaleFactorCodebook tables must contain exactly " +
                SymbolCount + " entries.");
        }
        return AacHuffmanCodebook.FromExplicitCodes(CodesArray, BitsArray);
    }

    /// <summary>
    /// The shared, lazily-built <see cref="AacHuffmanCodebook"/>
    /// instance for cb 12.
    /// </summary>
    public static AacHuffmanCodebook Book => BookInstance;

    /// <summary>
    /// Codeword values per symbol (read-only view). Each value is
    /// transmitted MSB-first using the corresponding entry of
    /// <see cref="Bits"/>.
    /// </summary>
    public static ReadOnlySpan<uint> Codes => CodesArray;

    /// <summary>
    /// Codeword bit-lengths per symbol (read-only view).
    /// </summary>
    public static ReadOnlySpan<int> Bits => BitsArray;

    /// <summary>
    /// Convert a decoded symbol index to a signed scalefactor delta in
    /// <c>[-MaxAbsoluteDelta..+MaxAbsoluteDelta]</c>.
    /// </summary>
    public static int SymbolToDelta(int symbolIndex)
    {
        if ((uint)symbolIndex >= (uint)SymbolCount)
        {
            throw new ArgumentOutOfRangeException(
                nameof(symbolIndex), symbolIndex,
                "symbolIndex must be in [0.." + (SymbolCount - 1) + "].");
        }
        return symbolIndex - ZeroDeltaSymbolIndex;
    }

    /// <summary>
    /// Convert a signed scalefactor delta to a symbol index. Returns
    /// <c>-1</c> when <paramref name="delta"/> is outside the
    /// representable range.
    /// </summary>
    public static int DeltaToSymbol(int delta)
    {
        int idx = delta + ZeroDeltaSymbolIndex;
        return (uint)idx < (uint)SymbolCount ? idx : -1;
    }
}
