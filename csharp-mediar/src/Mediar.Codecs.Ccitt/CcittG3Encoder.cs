namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Encoder for ITU-T Rec. T.4 Group 3 one-dimensional (Modified Huffman)
/// fax compression. Produces the bit stream consumed by
/// <see cref="CcittG3Decoder"/>.
/// </summary>
public static class CcittG3Encoder
{
    /// <summary>Encoder options for T.4 Group 3 / Modified Huffman.</summary>
    /// <param name="EmitEolMarkers">When <c>true</c>, prefix every row with an EOL.</param>
    /// <param name="EolByteAlign">When <c>true</c>, byte-align each EOL by zero-padding before it.</param>
    /// <param name="EmitRtc">When <c>true</c>, append six EOL markers (Return-To-Control) at the end.</param>
    public readonly record struct Options(bool EmitEolMarkers, bool EolByteAlign, bool EmitRtc);

    /// <summary>Encode 1-bpp packed bytes into a T.4 1D / MH bit stream.</summary>
    public static byte[] Encode(ReadOnlySpan<byte> packedPixels, int width, int height, Options options)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        int rowBytes = CcittBitmap.RowBytes(width);
        if (packedPixels.Length < rowBytes * height)
            throw new ArgumentException("Buffer shorter than width * height / 8.", nameof(packedPixels));

        var writer = new CcittBitWriter();

        for (int y = 0; y < height; y++)
        {
            if (options.EmitEolMarkers)
            {
                if (options.EolByteAlign) writer.FlushByte();
                writer.Write(0x001, 12);
            }
            EncodeRow(writer, packedPixels.Slice(y * rowBytes, rowBytes), width);
        }

        if (options.EmitRtc)
        {
            if (options.EolByteAlign) writer.FlushByte();
            for (int i = 0; i < 6; i++) writer.Write(0x001, 12);
        }

        return writer.ToArray();
    }

    private static void EncodeRow(CcittBitWriter writer, ReadOnlySpan<byte> codingLine, int width)
    {
        int x = 0;
        int colour = 0; // first run is white
        while (x < width)
        {
            int run = 0;
            while (x + run < width && CcittBitmap.GetPixel(codingLine, x + run) == colour) run++;
            CcittRunEncoder.WriteRun(writer, colour, run);
            x += run;
            colour = 1 - colour;
        }
    }
}
