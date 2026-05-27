namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Decoder for ITU-T Rec. T.4 Group 3 one-dimensional (Modified Huffman)
/// fax-compressed scanlines as used by TIFF compression codes 2 and 3.
/// Output is 1-bpp packed MSB-first with <c>1 = black</c>, <c>0 = white</c>.
/// </summary>
/// <remarks>
/// <para>
/// For TIFF compression 2 (Modified Huffman) rows are encoded back to
/// back with no EOL markers; rows are not byte-aligned. Pass
/// <c>HasEolMarkers</c> = <c>false</c>.
/// </para>
/// <para>
/// For TIFF compression 3 (CCITT T.4 Group 3) rows are preceded by an
/// EOL = <c>000000000001</c> marker; if the encoder set the "byte-align
/// EOL" T4Options bit, EOL is preceded by 0..7 fill bits so it ends on
/// a byte boundary. Pass <c>HasEolMarkers</c> = <c>true</c>.
/// </para>
/// </remarks>
public static class CcittG3Decoder
{
    /// <summary>
    /// Decoder options for T.4 Group 3 / Modified Huffman.
    /// </summary>
    /// <param name="HasEolMarkers">When <c>true</c>, expect an EOL prefix on every row.</param>
    /// <param name="EolByteAligned">When <c>true</c>, EOL markers are byte-aligned (T4Options bit 2).</param>
    public readonly record struct Options(bool HasEolMarkers, bool EolByteAligned);

    /// <summary>Decode a 1D MH bit stream of the given dimensions.</summary>
    public static byte[] Decode(ReadOnlyMemory<byte> encoded, int width, int height, Options options)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);

        var bits = new CcittBitReader(encoded);
        int rowBytes = CcittBitmap.RowBytes(width);
        var output = new byte[rowBytes * height];

        for (int y = 0; y < height; y++)
        {
            if (options.HasEolMarkers)
            {
                ConsumeEol(ref bits, options.EolByteAligned);
            }
            DecodeRow(ref bits, output.AsSpan(y * rowBytes, rowBytes), width);
        }

        return output;
    }

    private static void ConsumeEol(ref CcittBitReader bits, bool eolByteAligned)
    {
        if (eolByteAligned)
        {
            // Skip 0..7 zero bits until EOL is byte-aligned (i.e. the
            // trailing 1 of the EOL marker lands on a byte boundary).
            // In T.4 §2.2.2 the rule is "EOL ends at the last bit of a
            // byte"; here we approximate by walking forward over zeros
            // until we see a 1 and then verifying it's an EOL.
            while (!bits.IsAtEnd && bits.Peek(1) == 0) bits.Skip(1);
        }
        else
        {
            // Skip arbitrary zero fill bits before an EOL.
            while (!bits.IsAtEnd && bits.Peek(1) == 0) bits.Skip(1);
        }

        // The leading zeros have been consumed; verify EOL terminator bit.
        if (bits.IsAtEnd) return;
        uint trailing = bits.Peek(1);
        if (trailing != 1)
            throw new InvalidDataException("Expected EOL marker.");
        bits.Skip(1);
    }

    private static void DecodeRow(ref CcittBitReader bits, Span<byte> codingLine, int width)
    {
        int x = 0;
        int colour = 0; // first run on the row is always white per T.4 §2.2.1.
        while (x < width)
        {
            int run = DecodeMhRun(ref bits, colour);
            int end = Math.Min(x + run, width);
            if (colour == 1)
            {
                for (int p = x; p < end; p++) CcittBitmap.SetBlack(codingLine, p);
            }
            x = end;
            colour = 1 - colour;
        }
    }

    private static int DecodeMhRun(ref CcittBitReader bits, int colour)
    {
        var table = colour == 0 ? CcittTables.WhiteLookup : CcittTables.BlackLookup;
        int total = 0;
        while (true)
        {
            uint peek = bits.Peek(CcittTables.MhPeekBits);
            var sym = table[peek];
            if (sym.Length == 0 || sym.Run == CcittTables.EolRun)
            {
                throw new InvalidDataException(
                    $"Invalid MH run code at bit {bits.BitPosition}.");
            }
            bits.Skip(sym.Length);
            total += sym.Run;
            if (sym.Run < 64) return total;
        }
    }
}
