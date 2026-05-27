namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Helpers shared by the T.4 and T.6 codecs that operate on packed 1-bpp
/// scanlines (MSB-first, 1 = black, 0 = white).
/// </summary>
internal static class CcittBitmap
{
    /// <summary>Bytes required to store <paramref name="width"/> 1-bpp pixels.</summary>
    public static int RowBytes(int width) => (width + 7) >> 3;

    /// <summary>Read the pixel at <paramref name="x"/> from a packed row (1 = black).</summary>
    public static int GetPixel(ReadOnlySpan<byte> row, int x)
    {
        int byteIdx = x >> 3;
        if (byteIdx >= row.Length) return 0;
        int bit = 7 - (x & 7);
        return (row[byteIdx] >> bit) & 1;
    }

    /// <summary>Set a single bit to 1 (black).</summary>
    public static void SetBlack(Span<byte> row, int x)
    {
        int byteIdx = x >> 3;
        if (byteIdx >= row.Length) return;
        int bit = 7 - (x & 7);
        row[byteIdx] |= (byte)(1 << bit);
    }

    /// <summary>
    /// Find the next changing element on a coding line strictly to the
    /// right of <paramref name="x"/>. Returns <paramref name="width"/>
    /// if no further change exists.
    /// </summary>
    /// <param name="row">Packed row, MSB-first, 1 = black.</param>
    /// <param name="width">Logical pixel width.</param>
    /// <param name="x">Search-start position (a0); may be -1 to mean "imaginary white pixel before column 0".</param>
    /// <param name="referenceColor">Colour of pixel a0 (0 = white, 1 = black).</param>
    public static int NextChangingElement(ReadOnlySpan<byte> row, int width, int x, int referenceColor)
    {
        int pos = x < 0 ? 0 : x + 1;
        if (x < 0)
        {
            int firstColor = GetPixel(row, 0);
            if (firstColor != referenceColor) return 0;
        }
        while (pos < width && GetPixel(row, pos) == referenceColor) pos++;
        return pos;
    }
}
