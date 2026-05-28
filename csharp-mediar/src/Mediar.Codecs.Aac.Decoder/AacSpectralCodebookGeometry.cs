namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Static geometry of an AAC spectral Huffman codebook per ISO/IEC
/// 14496-3 §4.6.3. The 11 spectral codebooks fall into a small number
/// of families parameterised by:
/// <list type="bullet">
///   <item><description><see cref="Dimension"/> - 4 (quad) for
///     codebooks 1..4, 2 (pair) for codebooks 5..11.</description></item>
///   <item><description><see cref="IsSigned"/> - <see langword="true"/>
///     for codebooks 1, 2, 5, 6 (symbol index directly encodes signed
///     values); <see langword="false"/> for codebooks 3, 4, 7..11
///     (symbol index encodes unsigned magnitudes, sign bits follow the
///     codeword for each non-zero component).</description></item>
///   <item><description><see cref="LargestAbsoluteValue"/> - 1, 2, 4,
///     7, 12, or 16 depending on the codebook family.</description></item>
///   <item><description><see cref="HasEscape"/> - <see langword="true"/>
///     only for codebook 11, which signals out-of-range magnitudes via
///     a unary-prefix escape sequence.</description></item>
/// </list>
/// </summary>
/// <remarks>
/// The geometry does <i>not</i> carry the canonical Huffman length
/// table; that wires in separately (one
/// <see cref="AacHuffmanCodebook"/> per codebook number). Splitting
/// the geometry from the codebook table keeps the symbol-to-tuple
/// decomposition unit-testable without depending on the (large)
/// static length tables.
/// </remarks>
public sealed record AacSpectralCodebookGeometry
{
    /// <summary>Codebook number (1..11) as written in <c>section_data()</c>.</summary>
    public required int CodebookNumber { get; init; }

    /// <summary>Number of values per codeword (2 or 4).</summary>
    public required int Dimension { get; init; }

    /// <summary>
    /// <see langword="true"/> when the symbol index already encodes
    /// signed components (codebooks 1, 2, 5, 6); <see langword="false"/>
    /// when sign bits follow the codeword for each non-zero component
    /// (codebooks 3, 4, 7..11).
    /// </summary>
    public required bool IsSigned { get; init; }

    /// <summary>
    /// Largest absolute value any component can take inside the
    /// codeword. Codebook 11 uses 16 as the in-table LAV; magnitudes
    /// of exactly 16 are then extended by the escape sequence.
    /// </summary>
    public required int LargestAbsoluteValue { get; init; }

    /// <summary>True only for codebook 11.</summary>
    public required bool HasEscape { get; init; }

    /// <summary>Number of distinct codewords in the codebook.</summary>
    public required int SymbolCount { get; init; }

    private static readonly AacSpectralCodebookGeometry[] Catalogue = new AacSpectralCodebookGeometry[]
    {
        new() { CodebookNumber = 1,  Dimension = 4, IsSigned = true,  LargestAbsoluteValue = 1,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 2,  Dimension = 4, IsSigned = true,  LargestAbsoluteValue = 1,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 3,  Dimension = 4, IsSigned = false, LargestAbsoluteValue = 2,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 4,  Dimension = 4, IsSigned = false, LargestAbsoluteValue = 2,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 5,  Dimension = 2, IsSigned = true,  LargestAbsoluteValue = 4,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 6,  Dimension = 2, IsSigned = true,  LargestAbsoluteValue = 4,  HasEscape = false, SymbolCount = 81 },
        new() { CodebookNumber = 7,  Dimension = 2, IsSigned = false, LargestAbsoluteValue = 7,  HasEscape = false, SymbolCount = 64 },
        new() { CodebookNumber = 8,  Dimension = 2, IsSigned = false, LargestAbsoluteValue = 7,  HasEscape = false, SymbolCount = 64 },
        new() { CodebookNumber = 9,  Dimension = 2, IsSigned = false, LargestAbsoluteValue = 12, HasEscape = false, SymbolCount = 169 },
        new() { CodebookNumber = 10, Dimension = 2, IsSigned = false, LargestAbsoluteValue = 12, HasEscape = false, SymbolCount = 169 },
        new() { CodebookNumber = 11, Dimension = 2, IsSigned = false, LargestAbsoluteValue = 16, HasEscape = true,  SymbolCount = 289 },
    };

    /// <summary>
    /// Look up the geometry for codebook <paramref name="codebookNumber"/>
    /// (1..11). Returns <see langword="null"/> for values outside that
    /// range (codebook 0 is the all-zeros marker, 12 is intensity
    /// stereo, 13+ are reserved or LTP).
    /// </summary>
    public static AacSpectralCodebookGeometry? Get(int codebookNumber)
    {
        if (codebookNumber < 1 || codebookNumber > 11) return null;
        return Catalogue[codebookNumber - 1];
    }

    /// <summary>
    /// Decompose a Huffman symbol index into 2 or 4 component values
    /// per <see cref="Dimension"/>. For signed codebooks (1, 2, 5, 6)
    /// the components are already centred at zero (range
    /// <c>[-LargestAbsoluteValue, +LargestAbsoluteValue]</c>). For
    /// unsigned codebooks (3, 4, 7..11) the components are unsigned
    /// magnitudes in <c>[0, LargestAbsoluteValue]</c>; sign bits must
    /// be read separately by the caller for each non-zero component.
    /// </summary>
    /// <param name="symbolIndex">
    /// Huffman symbol index in <c>[0, SymbolCount)</c>.
    /// </param>
    /// <param name="components">
    /// Output span of length at least <see cref="Dimension"/>. Only
    /// the first <see cref="Dimension"/> entries are written.
    /// </param>
    /// <exception cref="ArgumentOutOfRangeException">
    /// Thrown when <paramref name="symbolIndex"/> is outside
    /// <c>[0, SymbolCount)</c> or when
    /// <paramref name="components"/>.Length is less than
    /// <see cref="Dimension"/>.
    /// </exception>
    public void Decompose(int symbolIndex, Span<int> components)
    {
        if (symbolIndex < 0 || symbolIndex >= SymbolCount)
            throw new ArgumentOutOfRangeException(nameof(symbolIndex), symbolIndex, $"Must be in [0, {SymbolCount}).");
        if (components.Length < Dimension)
            throw new ArgumentOutOfRangeException(nameof(components), components.Length, $"Need >= {Dimension} slots.");

        switch (CodebookNumber)
        {
            case 1:
            case 2:
                // 4D signed, base 3, range [-1, +1]. idx = ((w+1)*3 + (x+1))*3*3 + ...
                {
                    int idx = symbolIndex;
                    components[0] = idx / 27 - 1;
                    components[1] = (idx / 9) % 3 - 1;
                    components[2] = (idx / 3) % 3 - 1;
                    components[3] = idx % 3 - 1;
                    break;
                }
            case 3:
            case 4:
                // 4D unsigned, base 3, range [0, 2].
                {
                    int idx = symbolIndex;
                    components[0] = idx / 27;
                    components[1] = (idx / 9) % 3;
                    components[2] = (idx / 3) % 3;
                    components[3] = idx % 3;
                    break;
                }
            case 5:
            case 6:
                // 2D signed, base 9, range [-4, +4].
                {
                    int idx = symbolIndex;
                    components[0] = idx / 9 - 4;
                    components[1] = idx % 9 - 4;
                    break;
                }
            case 7:
            case 8:
                // 2D unsigned, base 8, range [0, 7].
                {
                    int idx = symbolIndex;
                    components[0] = idx / 8;
                    components[1] = idx % 8;
                    break;
                }
            case 9:
            case 10:
                // 2D unsigned, base 13, range [0, 12].
                {
                    int idx = symbolIndex;
                    components[0] = idx / 13;
                    components[1] = idx % 13;
                    break;
                }
            case 11:
                // 2D unsigned, base 17, range [0, 16]. Escape signalled
                // when a component is 16 - the caller reads the escape
                // sequence to recover the actual magnitude.
                {
                    int idx = symbolIndex;
                    components[0] = idx / 17;
                    components[1] = idx % 17;
                    break;
                }
            default:
                throw new InvalidOperationException($"Unsupported codebook number {CodebookNumber}.");
        }
    }
}
