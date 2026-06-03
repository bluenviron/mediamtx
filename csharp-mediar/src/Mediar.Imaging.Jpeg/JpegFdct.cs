using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Forward 8×8 DCT used by the JPEG baseline encoder. Mirror of the
/// integer inverse DCT in <c>Mediar.Acceleration.ScalarIdct8x8</c>:
/// same fixed-point constants and same Loeffler-Lightenberg-Moschytz
/// (LLM 1989) butterfly so that encode→decode round-trips through a
/// known-stable arithmetic path. Inputs are pre-shifted sample values
/// (<c>raw − 128</c>); outputs are 64 short coefficients in natural
/// (row-major, not zig-zag) order.
/// </summary>
internal static class JpegFdct
{
    // Same precision tier as the scalar IDCT so encode + decode share constants.
    private const int ConstBits = 13;
    private const int Pass1Bits = 2;

    private const int F_0_298631336 = 2446;
    private const int F_0_390180644 = 3196;
    private const int F_0_541196100 = 4433;
    private const int F_0_765366865 = 6270;
    private const int F_0_899976223 = 7373;
    private const int F_1_175875602 = 9633;
    private const int F_1_501321110 = 12299;
    private const int F_1_847759065 = 15137;
    private const int F_1_961570560 = 16069;
    private const int F_2_053119869 = 16819;
    private const int F_2_562915447 = 20995;
    private const int F_3_072711026 = 25172;

    /// <summary>
    /// Forward 8×8 DCT of <paramref name="block"/> (64 pre-shifted samples,
    /// natural order) into <paramref name="output"/> (64 short coefficients,
    /// natural order). The output is scaled by <c>8 * 8 * sqrt(2)/2</c>
    /// relative to a pure mathematical DCT — i.e. it is the libjpeg-style
    /// "ifast"-compatible scaling that pairs with the matching IDCT.
    /// </summary>
    public static void Forward(ReadOnlySpan<short> block, Span<short> output)
    {
        if (block.Length != 64) throw new ArgumentException("FDCT input must be 64 samples.", nameof(block));
        if (output.Length != 64) throw new ArgumentException("FDCT output must be 64 entries.", nameof(output));

        Span<int> tmp = stackalloc int[64];

        // ---- Row pass. ----
        for (int row = 0; row < 8; row++)
        {
            int o = row * 8;
            int d0 = block[o + 0];
            int d1 = block[o + 1];
            int d2 = block[o + 2];
            int d3 = block[o + 3];
            int d4 = block[o + 4];
            int d5 = block[o + 5];
            int d6 = block[o + 6];
            int d7 = block[o + 7];

            // Even part: outputs y0/y2/y4/y6.
            int t0 = d0 + d7;
            int t7 = d0 - d7;
            int t1 = d1 + d6;
            int t6 = d1 - d6;
            int t2 = d2 + d5;
            int t5 = d2 - d5;
            int t3 = d3 + d4;
            int t4 = d3 - d4;

            int t10 = t0 + t3;
            int t13 = t0 - t3;
            int t11 = t1 + t2;
            int t12 = t1 - t2;

            tmp[o + 0] = (t10 + t11) << Pass1Bits;
            tmp[o + 4] = (t10 - t11) << Pass1Bits;

            int z1 = (t12 + t13) * F_0_541196100;
            tmp[o + 2] = Descale(z1 + t13 * F_0_765366865, ConstBits - Pass1Bits);
            tmp[o + 6] = Descale(z1 + t12 * (-F_1_847759065), ConstBits - Pass1Bits);

            // Odd part: outputs y1/y3/y5/y7.
            int z1o = t4 + t7;
            int z2o = t5 + t6;
            int z3o = t4 + t6;
            int z4o = t5 + t7;
            int z5o = (z3o + z4o) * F_1_175875602;

            int t4c = t4 * F_0_298631336;
            int t5c = t5 * F_2_053119869;
            int t6c = t6 * F_3_072711026;
            int t7c = t7 * F_1_501321110;
            int z1c = z1o * (-F_0_899976223);
            int z2c = z2o * (-F_2_562915447);
            int z3c = z3o * (-F_1_961570560) + z5o;
            int z4c = z4o * (-F_0_390180644) + z5o;

            tmp[o + 7] = Descale(t4c + z1c + z3c, ConstBits - Pass1Bits);
            tmp[o + 5] = Descale(t5c + z2c + z4c, ConstBits - Pass1Bits);
            tmp[o + 3] = Descale(t6c + z2c + z3c, ConstBits - Pass1Bits);
            tmp[o + 1] = Descale(t7c + z1c + z4c, ConstBits - Pass1Bits);
        }

        // ---- Column pass. ----
        for (int col = 0; col < 8; col++)
        {
            int d0 = tmp[0 * 8 + col];
            int d1 = tmp[1 * 8 + col];
            int d2 = tmp[2 * 8 + col];
            int d3 = tmp[3 * 8 + col];
            int d4 = tmp[4 * 8 + col];
            int d5 = tmp[5 * 8 + col];
            int d6 = tmp[6 * 8 + col];
            int d7 = tmp[7 * 8 + col];

            int t0 = d0 + d7;
            int t7 = d0 - d7;
            int t1 = d1 + d6;
            int t6 = d1 - d6;
            int t2 = d2 + d5;
            int t5 = d2 - d5;
            int t3 = d3 + d4;
            int t4 = d3 - d4;

            int t10 = t0 + t3;
            int t13 = t0 - t3;
            int t11 = t1 + t2;
            int t12 = t1 - t2;

            output[0 * 8 + col] = (short)Descale(t10 + t11, Pass1Bits + 3);
            output[4 * 8 + col] = (short)Descale(t10 - t11, Pass1Bits + 3);

            int z1 = (t12 + t13) * F_0_541196100;
            output[2 * 8 + col] = (short)Descale(z1 + t13 * F_0_765366865, ConstBits + Pass1Bits + 3);
            output[6 * 8 + col] = (short)Descale(z1 + t12 * (-F_1_847759065), ConstBits + Pass1Bits + 3);

            int z1o = t4 + t7;
            int z2o = t5 + t6;
            int z3o = t4 + t6;
            int z4o = t5 + t7;
            int z5o = (z3o + z4o) * F_1_175875602;

            int t4c = t4 * F_0_298631336;
            int t5c = t5 * F_2_053119869;
            int t6c = t6 * F_3_072711026;
            int t7c = t7 * F_1_501321110;
            int z1c = z1o * (-F_0_899976223);
            int z2c = z2o * (-F_2_562915447);
            int z3c = z3o * (-F_1_961570560) + z5o;
            int z4c = z4o * (-F_0_390180644) + z5o;

            output[7 * 8 + col] = (short)Descale(t4c + z1c + z3c, ConstBits + Pass1Bits + 3);
            output[5 * 8 + col] = (short)Descale(t5c + z2c + z4c, ConstBits + Pass1Bits + 3);
            output[3 * 8 + col] = (short)Descale(t6c + z2c + z3c, ConstBits + Pass1Bits + 3);
            output[1 * 8 + col] = (short)Descale(t7c + z1c + z4c, ConstBits + Pass1Bits + 3);
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Descale(int v, int n)
    {
        if (n <= 0) return v;
        return (v + (1 << (n - 1))) >> n;
    }
}
