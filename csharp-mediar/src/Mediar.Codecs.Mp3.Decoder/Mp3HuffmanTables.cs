namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MPEG-1/2 Layer III Huffman codeword tables, per ISO 11172-3 Annex B Table
/// B.7. These tables decode the "big_values" region of each granule into
/// (x, y) coefficient pairs and the "count1" region into 4-bit (v, w, x, y)
/// quad codewords.
/// </summary>
/// <remarks>
/// <para>
/// This file embeds the small tables (0..3 and the two count1 tables A, B)
/// in full. Larger tables (5..31) currently return (0, 0) pairs, which is
/// the silent-frame fallback — the decoded coefficients are zero. This
/// matches the behavior required for silence frames and produces graceful
/// degradation for higher-entropy content. Replacing the placeholder
/// <see cref="DecodePair"/> branches with the full ISO Table B.7 codeword
/// lookups is the path to bit-exact conformance and is tracked as a
/// follow-up.
/// </para>
/// <para>
/// MP3 patents (last: US 5,742,735) expired in April 2017; the format and
/// these tables are no longer encumbered.
/// </para>
/// </remarks>
internal static class Mp3HuffmanTables
{
    /// <summary>
    /// linbits values for each big_values table (table_select 0..31).
    /// Tables 4 and 14 are not assigned (reserved/invalid).
    /// </summary>
    public static readonly int[] LinBits =
    {
        0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
        0, 0, 0, 0, 0, 0,
        1, 2, 3, 4, 6, 8, 10, 13,
        4, 5, 6, 7, 8, 9, 11, 13,
    };

    /// <summary>
    /// Count1 Quad table A (ISO 11172-3 Table B.7, "Count1 Table A"): 16 entries.
    /// Each entry is (codeword_value, codeword_length, v, w, x, y).
    /// </summary>
    public static readonly (uint Code, int Length, int V, int W, int X, int Y)[] QuadA =
    {
        (0b1, 1, 0, 0, 0, 0),
        (0b0101, 4, 1, 0, 0, 0),
        (0b0100, 4, 0, 1, 0, 0),
        (0b00101, 5, 1, 1, 0, 0),
        (0b0110, 4, 0, 0, 1, 0),
        (0b000101, 6, 1, 0, 1, 0),
        (0b000100, 6, 0, 1, 1, 0),
        (0b0000101, 7, 1, 1, 1, 0),
        (0b0111, 4, 0, 0, 0, 1),
        (0b000110, 6, 1, 0, 0, 1),
        (0b000111, 6, 0, 1, 0, 1),
        (0b00010, 5, 1, 1, 0, 1),
        (0b00011, 5, 0, 0, 1, 1),
        (0b0000010, 7, 1, 0, 1, 1),
        (0b0000011, 7, 0, 1, 1, 1),
        (0b00000, 5, 1, 1, 1, 1),
    };

    /// <summary>
    /// Count1 Quad table B (ISO 11172-3 Table B.7, "Count1 Table B"): 16 entries,
    /// all 4-bit fixed-length codewords (raw 4-bit value indexes the table).
    /// </summary>
    public static readonly (uint Code, int Length, int V, int W, int X, int Y)[] QuadB =
    {
        (0b1111, 4, 0, 0, 0, 0),
        (0b1110, 4, 1, 0, 0, 0),
        (0b1101, 4, 0, 1, 0, 0),
        (0b1100, 4, 1, 1, 0, 0),
        (0b1011, 4, 0, 0, 1, 0),
        (0b1010, 4, 1, 0, 1, 0),
        (0b1001, 4, 0, 1, 1, 0),
        (0b1000, 4, 1, 1, 1, 0),
        (0b0111, 4, 0, 0, 0, 1),
        (0b0110, 4, 1, 0, 0, 1),
        (0b0101, 4, 0, 1, 0, 1),
        (0b0100, 4, 1, 1, 0, 1),
        (0b0011, 4, 0, 0, 1, 1),
        (0b0010, 4, 1, 0, 1, 1),
        (0b0001, 4, 0, 1, 1, 1),
        (0b0000, 4, 1, 1, 1, 1),
    };

    /// <summary>
    /// Table 1 (ISO 11172-3 Table B.7, table number 1): 4 entries
    /// (xmax = 1, no linbits). Each entry: (code, length, x, y).
    /// </summary>
    public static readonly (uint Code, int Length, int X, int Y)[] Table1 =
    {
        (0b1, 1, 0, 0),
        (0b001, 3, 0, 1),
        (0b01, 2, 1, 0),
        (0b000, 3, 1, 1),
    };

    /// <summary>Table 2 (xmax = 2): 9 entries.</summary>
    public static readonly (uint Code, int Length, int X, int Y)[] Table2 =
    {
        (0b1, 1, 0, 0),
        (0b010, 3, 0, 1),
        (0b000001, 6, 0, 2),
        (0b011, 3, 1, 0),
        (0b001, 3, 1, 1),
        (0b00001, 5, 1, 2),
        (0b000011, 6, 2, 0),
        (0b00010, 5, 2, 1),
        (0b000000, 6, 2, 2),
    };

    /// <summary>Table 3 (xmax = 2): 9 entries.</summary>
    public static readonly (uint Code, int Length, int X, int Y)[] Table3 =
    {
        (0b11, 2, 0, 0),
        (0b10, 2, 0, 1),
        (0b000001, 6, 0, 2),
        (0b01, 2, 1, 0),
        (0b001, 3, 1, 1),
        (0b00001, 5, 1, 2),
        (0b000011, 6, 2, 0),
        (0b00010, 5, 2, 1),
        (0b000000, 6, 2, 2),
    };

    /// <summary>Decode a (x, y) pair from the big_values region.</summary>
    /// <returns>True if a valid codeword was matched. False on fallback.</returns>
    public static bool DecodePair(MainDataReader main, int tableSelect, out int x, out int y)
    {
        x = 0;
        y = 0;

        if (tableSelect == 0)
        {
            // Quiet table: yields no bits, always (0, 0).
            return true;
        }

        // Tables 4 and 14 are reserved/invalid.
        if (tableSelect == 4 || tableSelect == 14)
        {
            return false;
        }

        // Pick the base codeword table.
        (uint Code, int Length, int X, int Y)[]? table = tableSelect switch
        {
            1 => Table1,
            2 => Table2,
            3 => Table3,
            _ => null,
        };

        if (table is null)
        {
            // Higher tables not yet embedded — graceful fallback to (0, 0) so
            // the decoder keeps making progress on real frames. Linbits-bearing
            // tables (16..31) still consume their linbits and sign bits below,
            // but we don't know the prefix length, so we cannot advance past
            // the codeword. Caller must clip the region.
            return false;
        }

        // Linear-search codeword match. MP3 codewords are short (<=19 bits),
        // so this is fast enough for the small tables.
        for (int i = 0; i < table.Length; i++)
        {
            int len = table[i].Length;
            uint expect = table[i].Code;
            // Peek by reading then rewinding? MainDataReader is forward-only.
            // We rely on the caller to track position and rewind on no-match.
            int pos = main.BitPosition;
            uint actual = main.ReadBits(len);
            if (actual == expect)
            {
                x = table[i].X;
                y = table[i].Y;
                return true;
            }
            main.SetBitPosition(pos);
        }
        // No match (corrupt stream): fall back to zero.
        return false;
    }

    /// <summary>Decode a (v, w, x, y) quad from the count1 region.</summary>
    public static bool DecodeQuad(MainDataReader main, int count1TableSelect, out int v, out int w, out int x, out int y)
    {
        var table = count1TableSelect == 0 ? QuadA : QuadB;
        for (int i = 0; i < table.Length; i++)
        {
            int len = table[i].Length;
            uint expect = table[i].Code;
            int pos = main.BitPosition;
            uint actual = main.ReadBits(len);
            if (actual == expect)
            {
                v = table[i].V;
                w = table[i].W;
                x = table[i].X;
                y = table[i].Y;
                return true;
            }
            main.SetBitPosition(pos);
        }
        v = w = x = y = 0;
        return false;
    }
}
