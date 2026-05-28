namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Canonical Huffman decoder used by the AAC spectral and scale-factor
/// pipeline (ISO/IEC 14496-3 §4.6.3, Tables 4.A.2..4.A.13). The codebook
/// is constructed from per-symbol code lengths only - the actual
/// codewords are generated canonically (RFC 1951 / ISO 14496-3
/// §4.6.3.3): codes are sorted by length and, within a length, by
/// symbol index. Decode walks an internal binary tree representation
/// MSB-first one bit at a time.
/// </summary>
/// <remarks>
/// <para>
/// A length of <c>0</c> indicates an unused symbol. Valid lengths are
/// <c>1..32</c>. Lengths are validated against the Kraft inequality;
/// over-specified codebooks (sum of <c>2^-len</c> &gt; 1) are rejected.
/// </para>
/// <para>
/// The tree is a flat <c>int[]</c> where each internal node occupies
/// two adjacent slots: <c>_tree[2*n]</c> is the <c>0</c>-bit child and
/// <c>_tree[2*n+1]</c> is the <c>1</c>-bit child. Slot values use the
/// encoding: <c>&gt;= 0</c> = index of the next internal node;
/// <c>&lt; 0</c> = leaf with symbol index <c>-(slot + 1)</c>;
/// <see cref="int.MinValue"/> = no child assigned (decoding into this
/// slot returns <c>false</c>).
/// </para>
/// </remarks>
public sealed class AacHuffmanCodebook
{
    private const int Unallocated = int.MinValue;

    private readonly int[] _tree;
    private readonly int _maxCodeLength;
    private readonly int _symbolCount;
    private readonly int _totalLengths;

    private AacHuffmanCodebook(int[] tree, int maxCodeLength, int symbolCount, int totalLengths)
    {
        _tree = tree;
        _maxCodeLength = maxCodeLength;
        _symbolCount = symbolCount;
        _totalLengths = totalLengths;
    }

    /// <summary>Number of symbols with a non-zero code length.</summary>
    public int SymbolCount => _symbolCount;

    /// <summary>Total number of symbol slots (including zero-length entries) supplied at construction.</summary>
    public int Capacity => _totalLengths;

    /// <summary>Length of the longest codeword in bits.</summary>
    public int MaxCodeLength => _maxCodeLength;

    /// <summary>
    /// Build a canonical Huffman codebook from per-symbol code lengths.
    /// The symbol index used by <see cref="TryDecode"/> is the index in
    /// the input array. A length of <c>0</c> marks the symbol as unused;
    /// any other value must be in <c>[1, 32]</c>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// Thrown when <paramref name="codeLengths"/> is empty.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// Thrown when a length is outside <c>[0, 32]</c>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// Thrown when all lengths are zero or when the lengths violate the
    /// Kraft inequality (i.e. the code is over-specified).
    /// </exception>
    public static AacHuffmanCodebook FromCanonicalLengths(ReadOnlySpan<int> codeLengths)
    {
        if (codeLengths.IsEmpty)
            throw new ArgumentException("Code length array must be non-empty.", nameof(codeLengths));

        int maxLen = 0;
        int symbolCount = 0;
        foreach (var l in codeLengths)
        {
            if (l < 0 || l > 32)
                throw new ArgumentOutOfRangeException(nameof(codeLengths), l, "Code lengths must be in [0, 32].");
            if (l > maxLen) maxLen = l;
            if (l > 0) symbolCount++;
        }
        if (maxLen == 0)
            throw new ArgumentException("All code lengths are zero - no symbols.", nameof(codeLengths));

        // Kraft inequality: sum of 2^-len must be <= 1.
        long kraftScale = 1L << maxLen;
        long kraftSum = 0;
        for (int i = 0; i < codeLengths.Length; i++)
        {
            int l = codeLengths[i];
            if (l > 0) kraftSum += kraftScale >> l;
        }
        if (kraftSum > kraftScale)
            throw new ArgumentException("Kraft inequality violated - codebook is over-specified.", nameof(codeLengths));

        // Canonical first-code-per-length: code[len] = (code[len-1] + count[len-1]) << 1.
        Span<int> blCount = stackalloc int[maxLen + 1];
        for (int i = 0; i < codeLengths.Length; i++) blCount[codeLengths[i]]++;
        blCount[0] = 0;

        Span<int> nextCode = stackalloc int[maxLen + 1];
        int code = 0;
        for (int len = 1; len <= maxLen; len++)
        {
            code = (code + blCount[len - 1]) << 1;
            nextCode[len] = code;
        }

        // Generous initial tree capacity: at most (symbolCount * maxLen) internal nodes.
        int initialNodes = Math.Max(2, symbolCount * maxLen + 2);
        var tree = new int[initialNodes * 2];
        for (int i = 0; i < tree.Length; i++) tree[i] = Unallocated;
        int nextNode = 1; // node 0 reserved as the root

        for (int sym = 0; sym < codeLengths.Length; sym++)
        {
            int len = codeLengths[sym];
            if (len == 0) continue;

            int symCode = nextCode[len];
            nextCode[len] = symCode + 1;

            int node = 0;
            for (int b = len - 1; b >= 0; b--)
            {
                int bit = (symCode >> b) & 1;
                int childSlot = node * 2 + bit;
                if (b == 0)
                {
                    if (tree[childSlot] != Unallocated)
                        throw new ArgumentException("Codebook collision while assigning canonical codes.", nameof(codeLengths));
                    tree[childSlot] = -(sym + 1);
                }
                else
                {
                    int existing = tree[childSlot];
                    if (existing == Unallocated)
                    {
                        int newNode = nextNode++;
                        if (newNode * 2 + 1 >= tree.Length)
                        {
                            int newSize = tree.Length * 2;
                            var grown = new int[newSize];
                            Array.Copy(tree, grown, tree.Length);
                            for (int i = tree.Length; i < newSize; i++) grown[i] = Unallocated;
                            tree = grown;
                        }
                        tree[childSlot] = newNode;
                        node = newNode;
                    }
                    else if (existing < 0)
                    {
                        throw new ArgumentException("Codebook collision: prefix shadows leaf.", nameof(codeLengths));
                    }
                    else
                    {
                        node = existing;
                    }
                }
            }
        }

        return new AacHuffmanCodebook(tree, maxLen, symbolCount, codeLengths.Length);
    }

    /// <summary>
    /// Decode the next symbol from <paramref name="reader"/> MSB-first.
    /// Returns <see langword="false"/> when the stream underflows before
    /// a complete codeword is consumed or when the decoded prefix lands
    /// on an unassigned tree slot (which can only occur on incomplete
    /// codebooks).
    /// </summary>
    internal bool TryDecode(scoped ref BitReader reader, out int symbolIndex)
    {
        symbolIndex = -1;
        int node = 0;
        for (int i = 0; i < _maxCodeLength; i++)
        {
            if (reader.Remaining == 0) return false;
            int bit = (int)reader.ReadBits(1);
            int next = _tree[node * 2 + bit];
            if (next == Unallocated) return false;
            if (next < 0)
            {
                symbolIndex = -next - 1;
                return true;
            }
            node = next;
        }
        return false;
    }
}
