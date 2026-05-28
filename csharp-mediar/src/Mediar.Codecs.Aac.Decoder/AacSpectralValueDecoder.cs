namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Decodes one tuple of 2 or 4 signed quantized spectral
/// coefficients from a spectral Huffman codebook (ISO/IEC 14496-3
/// §4.6.3.3). The reader is a thin layer over
/// <see cref="AacHuffmanCodebook"/> and
/// <see cref="AacSpectralCodebookGeometry"/> that handles:
/// <list type="bullet">
///   <item><description>Decoding the next codeword to a symbol
///     index;</description></item>
///   <item><description>Decomposing the symbol index into 2 or 4
///     components per the codebook geometry;</description></item>
///   <item><description>Reading one sign bit per non-zero component
///     for unsigned codebooks (3, 4, 7..11);</description></item>
///   <item><description>Reading the unary-prefix + magnitude escape
///     sequence for codebook 11 components whose in-table magnitude
///     equals 16;</description></item>
///   <item><description>Returning the final signed quantized
///     values.</description></item>
/// </list>
/// </summary>
public static class AacSpectralValueDecoder
{
    /// <summary>
    /// Maximum number of unary <c>1</c>-bits allowed in a codebook
    /// 11 escape sequence (i.e. the magnitude can extend up to
    /// <c>(1 &lt;&lt; (4 + 8)) + ((1 &lt;&lt; (4 + 8)) - 1) = 8191</c>).
    /// </summary>
    public const int MaxEscapePrefix = 8;

    /// <summary>
    /// Decode the next tuple from <paramref name="reader"/> using
    /// <paramref name="geometry"/> + <paramref name="codebook"/>.
    /// </summary>
    /// <param name="reader">Active bit reader; advanced past the tuple on success.</param>
    /// <param name="geometry">Codebook geometry (dimension, signedness, LAV, escape).</param>
    /// <param name="codebook">Huffman codebook matching the geometry's symbol count.</param>
    /// <param name="values">
    /// Destination span. Must have length at least
    /// <see cref="AacSpectralCodebookGeometry.Dimension"/>; only that
    /// many entries are written.
    /// </param>
    /// <returns>
    /// <see langword="true"/> on success. Returns <see langword="false"/>
    /// when the bit stream underflows, when the codebook decodes a
    /// symbol index outside <c>[0, SymbolCount)</c>, when an escape
    /// sequence overruns <see cref="MaxEscapePrefix"/>, or when
    /// <paramref name="codebook"/>'s capacity does not match
    /// <paramref name="geometry"/>'s <c>SymbolCount</c>.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacSpectralCodebookGeometry geometry,
        AacHuffmanCodebook codebook,
        scoped Span<int> values)
    {
        ArgumentNullException.ThrowIfNull(geometry);
        ArgumentNullException.ThrowIfNull(codebook);
        if (codebook.Capacity != geometry.SymbolCount) return false;
        if (values.Length < geometry.Dimension) return false;

        if (!codebook.TryDecode(ref reader, out int symbolIndex)) return false;
        if (symbolIndex < 0 || symbolIndex >= geometry.SymbolCount) return false;

        geometry.Decompose(symbolIndex, values);

        if (geometry.IsSigned)
        {
            // Signed codebooks (1, 2, 5, 6) are already centred; nothing more to read.
            return true;
        }

        // Unsigned codebooks (3, 4, 7..11): read one sign bit per non-zero magnitude.
        for (int i = 0; i < geometry.Dimension; i++)
        {
            if (values[i] == 0) continue;
            if (reader.Remaining < 1) return false;
            if (reader.ReadBit()) values[i] = -values[i];
        }

        if (!geometry.HasEscape) return true;

        // Codebook 11: in-table magnitude 16 signals an escape sequence
        // that replaces the magnitude with (1 << (prefix + 4)) + extension.
        for (int i = 0; i < geometry.Dimension; i++)
        {
            int magnitude = values[i] < 0 ? -values[i] : values[i];
            if (magnitude != 16) continue;

            int prefix = 0;
            while (true)
            {
                if (reader.Remaining < 1) return false;
                if (!reader.ReadBit()) break;
                prefix++;
                if (prefix > MaxEscapePrefix) return false;
            }

            int extBits = prefix + 4;
            if (reader.Remaining < extBits) return false;
            int extValue = (int)reader.ReadBits(extBits);
            int finalMag = (1 << extBits) + extValue;
            values[i] = values[i] < 0 ? -finalMag : finalMag;
        }

        return true;
    }
}
