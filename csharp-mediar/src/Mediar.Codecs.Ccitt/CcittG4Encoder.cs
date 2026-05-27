namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Encoder for ITU-T Rec. T.6 Group 4 (MMR) fax compression. Produces
/// the bit-stream consumed by <see cref="CcittG4Decoder"/>; emits an
/// EOFB (two EOL markers, byte-aligned via <see cref="CcittBitWriter"/>)
/// at the end of the image.
/// </summary>
/// <remarks>
/// The encoding strategy is the straightforward "choose Pass when b2 is
/// past a1; otherwise choose Vertical for offsets ≤ 3; otherwise fall
/// back to Horizontal" algorithm from T.6 §2.2.4. It produces valid (if
/// not always optimally short) MMR output that round-trips through
/// <see cref="CcittG4Decoder"/>.
/// </remarks>
public static class CcittG4Encoder
{
    /// <summary>
    /// Encode 1-bpp packed pixels (MSB-first, 1 = black) into a T.6 MMR bit stream.
    /// </summary>
    public static byte[] Encode(ReadOnlySpan<byte> packedPixels, int width, int height)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        int rowBytes = CcittBitmap.RowBytes(width);
        if (packedPixels.Length < rowBytes * height)
            throw new ArgumentException("Buffer shorter than width * height / 8.", nameof(packedPixels));

        var writer = new CcittBitWriter();
        var refLine = new byte[rowBytes];

        for (int y = 0; y < height; y++)
        {
            var coding = packedPixels.Slice(y * rowBytes, rowBytes);
            EncodeRow(writer, refLine, coding, width);
            coding.CopyTo(refLine);
        }

        // EOFB = two EOL markers back to back.
        WriteEol(writer);
        WriteEol(writer);
        return writer.ToArray();
    }

    private static void WriteEol(CcittBitWriter writer)
    {
        // 000000000001 = 0x001 (12 bits)
        writer.Write(0x001, 12);
    }

    private static void EncodeRow(CcittBitWriter writer, ReadOnlySpan<byte> refLine,
                                   ReadOnlySpan<byte> codingLine, int width)
    {
        int a0 = -1;
        int colourOfA0 = 0;

        while (a0 < width)
        {
            int a1 = CcittBitmap.NextChangingElement(codingLine, width, a0, colourOfA0);
            int a2 = a1 < width
                ? CcittBitmap.NextChangingElement(codingLine, width, a1, 1 - colourOfA0)
                : width;

            var (b1, b2) = CcittReferenceLine.FindB1B2(refLine, width, a0, colourOfA0);

            // Pass mode preferred when reference run b1..b2 is entirely
            // before a1.
            if (b2 < a1)
            {
                WriteCode(writer, "0001");
                a0 = b2;
                continue;
            }

            int delta = a1 - b1;
            if (delta is >= -3 and <= 3)
            {
                switch (delta)
                {
                    case 0: WriteCode(writer, "1"); break;
                    case 1: WriteCode(writer, "011"); break;
                    case 2: WriteCode(writer, "000011"); break;
                    case 3: WriteCode(writer, "0000011"); break;
                    case -1: WriteCode(writer, "010"); break;
                    case -2: WriteCode(writer, "000010"); break;
                    case -3: WriteCode(writer, "0000010"); break;
                }
                a0 = a1;
                colourOfA0 = 1 - colourOfA0;
            }
            else
            {
                // Horizontal mode.
                int writeStart = Math.Max(a0, 0);
                int run1 = a1 - writeStart;
                int run2 = a2 - a1;
                WriteCode(writer, "001");
                CcittRunEncoder.WriteRun(writer, colourOfA0, run1);
                CcittRunEncoder.WriteRun(writer, 1 - colourOfA0, run2);
                a0 = a2;
                // colour stays
            }
        }
    }

    private static void WriteCode(CcittBitWriter writer, string code)
    {
        uint bits = 0;
        for (int i = 0; i < code.Length; i++)
        {
            bits = (bits << 1) | (uint)(code[i] - '0');
        }
        writer.Write(bits, code.Length);
    }
}
