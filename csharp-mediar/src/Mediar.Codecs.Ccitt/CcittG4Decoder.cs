namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Decoder for ITU-T Rec. T.6 Group 4 (MMR) fax-compressed scanlines as
/// used by TIFF compression code 4. Output is 1-bpp packed MSB-first
/// with <c>1 = black</c>, <c>0 = white</c>.
/// </summary>
/// <remarks>
/// MMR is the two-dimensional MR coding scheme without EOL markers. The
/// encoded bit stream may be terminated by an EOFB (two consecutive EOL
/// markers); this decoder stops after <c>height</c> rows regardless, so
/// an absent EOFB is tolerated.
/// </remarks>
public static class CcittG4Decoder
{
    /// <summary>
    /// Decode a T.6 (G4) bit stream of the given logical dimensions to
    /// 1-bpp packed bytes (MSB-first, 1 = black).
    /// </summary>
    /// <param name="encoded">Encoded fax bytes.</param>
    /// <param name="width">Pixel width of the image.</param>
    /// <param name="height">Pixel height (number of rows).</param>
    public static byte[] Decode(ReadOnlyMemory<byte> encoded, int width, int height)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);

        var bits = new CcittBitReader(encoded);
        int rowBytes = CcittBitmap.RowBytes(width);
        var output = new byte[rowBytes * height];
        var refLine = new byte[rowBytes];

        for (int y = 0; y < height; y++)
        {
            Span<byte> codingLine = output.AsSpan(y * rowBytes, rowBytes);
            DecodeRow(ref bits, refLine, codingLine, width);
            codingLine.CopyTo(refLine);
        }

        return output;
    }

    private static void DecodeRow(ref CcittBitReader bits, ReadOnlySpan<byte> refLine,
                                  Span<byte> codingLine, int width)
    {
        int a0 = -1;
        int colourOfA0 = 0; // white

        while (a0 < width)
        {
            uint peek = bits.Peek(CcittTables.TwoDPeekBits);
            var sym = CcittTables.TwoDLookup[peek];
            if (sym.Mode == TwoDMode.Invalid)
            {
                throw new InvalidDataException(
                    $"Invalid CCITT T.6 2D code at bit {bits.BitPosition}.");
            }
            bits.Skip(sym.Length);

            var (b1, b2) = CcittReferenceLine.FindB1B2(refLine, width, a0, colourOfA0);
            int writeStart = Math.Max(a0, 0);
            int newA0;
            bool flipColour = true;

            switch (sym.Mode)
            {
                case TwoDMode.Pass:
                    newA0 = b2;
                    WriteRun(codingLine, writeStart, b2, colourOfA0);
                    flipColour = false;
                    break;
                case TwoDMode.Horizontal:
                {
                    int run1 = DecodeMhRun(ref bits, colourOfA0);
                    int run2 = DecodeMhRun(ref bits, 1 - colourOfA0);
                    int end1 = Math.Min(writeStart + run1, width);
                    int end2 = Math.Min(end1 + run2, width);
                    WriteRun(codingLine, writeStart, end1, colourOfA0);
                    WriteRun(codingLine, end1, end2, 1 - colourOfA0);
                    newA0 = writeStart + run1 + run2;
                    flipColour = false;
                    break;
                }
                case TwoDMode.V0: newA0 = b1; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vr1: newA0 = b1 + 1; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vr2: newA0 = b1 + 2; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vr3: newA0 = b1 + 3; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vl1: newA0 = b1 - 1; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vl2: newA0 = b1 - 2; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                case TwoDMode.Vl3: newA0 = b1 - 3; WriteRun(codingLine, writeStart, newA0, colourOfA0); break;
                default:
                    throw new InvalidDataException("Unhandled CCITT 2D mode.");
            }

            if (newA0 < writeStart)
            {
                throw new InvalidDataException(
                    "CCITT T.6 decode produced a backwards cursor; encoding is corrupt.");
            }
            a0 = newA0;
            if (flipColour) colourOfA0 = 1 - colourOfA0;
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

    private static void WriteRun(Span<byte> row, int from, int to, int colour)
    {
        if (colour == 0) return;
        if (from < 0) from = 0;
        int maxPixel = row.Length << 3;
        if (to > maxPixel) to = maxPixel;
        for (int x = from; x < to; x++)
        {
            CcittBitmap.SetBlack(row, x);
        }
    }
}
