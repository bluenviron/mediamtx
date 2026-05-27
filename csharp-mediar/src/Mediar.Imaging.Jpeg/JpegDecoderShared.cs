using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Helpers shared between <see cref="JpegBaselineDecoder"/> and
/// <see cref="JpegProgressiveDecoder"/>: zig-zag ordering, IDCT cosine
/// table, Huffman decode + signed-magnitude extension, level-shift /
/// IDCT 8×8, and the YCbCr → RGB conversion that builds the final
/// <see cref="ImageFrame"/>.
/// </summary>
internal static class JpegDecoderShared
{
    /// <summary>Zig-zag scan order: maps scan-order index → natural 8×8 index.</summary>
    public static readonly int[] Zigzag =
    [
         0,  1,  8, 16,  9,  2,  3, 10,
        17, 24, 32, 25, 18, 11,  4,  5,
        12, 19, 26, 33, 40, 48, 41, 34,
        27, 20, 13,  6,  7, 14, 21, 28,
        35, 42, 49, 56, 57, 50, 43, 36,
        29, 22, 15, 23, 30, 37, 44, 51,
        58, 59, 52, 45, 38, 31, 39, 46,
        53, 60, 61, 54, 47, 55, 62, 63,
    ];

    /// <summary>Pre-computed IDCT cosine table: index [u*8 + x] = C(u) * 0.5 * cos((2x+1)uπ/16).</summary>
    public static readonly float[] IdctCos = ComputeIdctCos();

    private static float[] ComputeIdctCos()
    {
        var t = new float[64];
        for (int u = 0; u < 8; u++)
        {
            float cu = u == 0 ? MathF.Sqrt(0.5f) : 1f;
            for (int x = 0; x < 8; x++)
            {
                t[u * 8 + x] = cu * 0.5f * MathF.Cos((2 * x + 1) * u * MathF.PI / 16f);
            }
        }
        return t;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int DecodeHuffman(ref JpegBitReader reader, HuffmanTable table)
    {
        // Fast path: peek 9 bits and look up in a 512-entry table.
        int peek = reader.Peek9();
        if (peek >= 0)
        {
            int fast = table.FastTable[peek];
            if (fast >= 0)
            {
                int len = fast >> 8;
                reader.Consume(len);
                return fast & 0xFF;
            }
        }
        int code = 0;
        for (int l = 1; l <= 16; l++)
        {
            code = (code << 1) | reader.ReadBit();
            if (code <= table.MaxCode[l])
            {
                int j = table.ValPtr[l] + (code - table.MinCode[l]);
                return table.HuffVal[j];
            }
        }
        throw new ImageFormatException("Bad JPEG Huffman code.");
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int ReceiveExtend(ref JpegBitReader reader, int s)
    {
        int v = reader.ReadBits(s);
        if (v < (1 << (s - 1)))
        {
            v -= (1 << s) - 1;
        }
        return v;
    }

    /// <summary>IDCT 8×8 with level-shift to [0,255] and clamp; output to a single-byte plane.</summary>
    public static void Idct8x8(ReadOnlySpan<short> input, byte[] outBytes, int outStride, int outOff)
    {
        Span<float> temp = stackalloc float[64];

        for (int row = 0; row < 8; row++)
        {
            int ri = row * 8;
            for (int x = 0; x < 8; x++)
            {
                float sum = 0;
                for (int u = 0; u < 8; u++)
                {
                    sum += input[ri + u] * IdctCos[u * 8 + x];
                }
                temp[ri + x] = sum;
            }
        }

        for (int col = 0; col < 8; col++)
        {
            for (int y = 0; y < 8; y++)
            {
                float sum = 0;
                for (int v = 0; v < 8; v++)
                {
                    sum += temp[v * 8 + col] * IdctCos[v * 8 + y];
                }
                int s = (int)MathF.Round(sum + 128f);
                if (s < 0) s = 0;
                else if (s > 255) s = 255;
                outBytes[outOff + y * outStride + col] = (byte)s;
            }
        }
    }

    public static ImageFrame BuildGrayscaleFrame(int w, int h, byte[] plane, int planeStride)
    {
        var (frame, buf) = ImageFrame.Rent(w, h, PixelFormat.Gray8, w);
        for (int y = 0; y < h; y++)
        {
            Buffer.BlockCopy(plane, y * planeStride, buf, y * w, w);
        }
        return frame;
    }

    public static ImageFrame BuildRgbFrame(
        int w, int h, byte[][] planes, int[] planeWidths,
        JpegComponent[] components, int hMax, int vMax)
    {
        var (frame, buf) = ImageFrame.Rent(w, h, PixelFormat.Rgb24, w * 3);

        var yPlane = planes[0];
        var cbPlane = planes[1];
        var crPlane = planes[2];
        int yStride = planeWidths[0];
        int cbStride = planeWidths[1];
        int crStride = planeWidths[2];

        int yhDiv = hMax / components[0].HSampling;
        int yvDiv = vMax / components[0].VSampling;
        int cbhDiv = hMax / components[1].HSampling;
        int cbvDiv = vMax / components[1].VSampling;
        int crhDiv = hMax / components[2].HSampling;
        int crvDiv = vMax / components[2].VSampling;

        for (int py = 0; py < h; py++)
        {
            int yOff = (py / yvDiv) * yStride;
            int cbOff = (py / cbvDiv) * cbStride;
            int crOff = (py / crvDiv) * crStride;
            int rowOff = py * w * 3;
            for (int px = 0; px < w; px++)
            {
                int y = yPlane[yOff + px / yhDiv];
                int cb = cbPlane[cbOff + px / cbhDiv] - 128;
                int cr = crPlane[crOff + px / crhDiv] - 128;

                int r = y + (int)MathF.Round(1.402f * cr);
                int g = y - (int)MathF.Round(0.344136f * cb + 0.714136f * cr);
                int b = y + (int)MathF.Round(1.772f * cb);

                if (r < 0) r = 0; else if (r > 255) r = 255;
                if (g < 0) g = 0; else if (g > 255) g = 255;
                if (b < 0) b = 0; else if (b > 255) b = 255;

                int o = rowOff + px * 3;
                buf[o + 0] = (byte)r;
                buf[o + 1] = (byte)g;
                buf[o + 2] = (byte)b;
            }
        }
        return frame;
    }
}
