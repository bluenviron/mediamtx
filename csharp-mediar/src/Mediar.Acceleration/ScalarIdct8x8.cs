using System.Runtime.CompilerServices;

namespace Mediar.Acceleration;

/// <summary>
/// Portable scalar reference 8×8 inverse DCT for JPEG. Implements the
/// integer Loeffler-Lightenberg-Moschytz 11-multiply IDCT with
/// <see cref="ConstBits"/> fractional bits of precision in the constants
/// and <see cref="PassShift"/> fractional bits in the row/column passes.
/// </summary>
/// <remarks>
/// Reference: C. Loeffler, A. Lightenberg, G. S. Moschytz,
/// "Practical fast 1-D DCT algorithms with 11 multiplications",
/// ICASSP 1989, pp. 988-991. The fixed-point constants and rounding
/// offsets match libjpeg-turbo <c>jidctint.c</c> exactly so that hardware
/// SIMD implementations can produce byte-for-byte identical output.
/// </remarks>
public sealed class ScalarIdct8x8 : IIdct8x8
{
    /// <summary>Singleton instance — stateless.</summary>
    public static ScalarIdct8x8 Instance { get; } = new();

    /// <inheritdoc/>
    public AccelerationTier IsaTier => AccelerationTier.Scalar;

    internal const int ConstBits = 13;
    internal const int PassShift = 11;
    private const int Pass1Bits = ConstBits - PassShift; // = 2
    private const int Pass1Round = 1 << (Pass1Bits - 1); // = 2

    internal const int FixM_0_298631336 = 2446;
    internal const int FixM_0_390180644 = 3196;
    internal const int FixM_0_541196100 = 4433;
    internal const int FixM_0_765366865 = 6270;
    internal const int FixM_0_899976223 = 7373;
    internal const int FixM_1_175875602 = 9633;
    internal const int FixM_1_501321110 = 12299;
    internal const int FixM_1_847759065 = 15137;
    internal const int FixM_1_961570560 = 16069;
    internal const int FixM_2_053119869 = 16819;
    internal const int FixM_2_562915447 = 20995;
    internal const int FixM_3_072711026 = 25172;

    /// <inheritdoc/>
    public void Idct8x8(ReadOnlySpan<short> input, Span<byte> output, int outputStride)
    {
        if (input.Length != 64)
        {
            throw new ArgumentException("IDCT input must be exactly 64 entries.", nameof(input));
        }
        if (outputStride < 8)
        {
            throw new ArgumentException("Output stride must be at least 8.", nameof(outputStride));
        }
        if (output.Length < 7 * outputStride + 8)
        {
            throw new ArgumentException("Output span too short for 8×8 block.", nameof(output));
        }

        Span<int> tmp = stackalloc int[64];

        // ---- Pass 1: rows. Each row produces 8 ints with PassShift+Pass1Bits fractional bits. ----
        for (int row = 0; row < 8; row++)
        {
            int o = row * 8;
            int z2 = input[o + 0];
            int z3 = input[o + 2];
            int z4 = input[o + 4];
            int z5 = input[o + 6];
            int t0 = input[o + 1];
            int t1 = input[o + 3];
            int t2 = input[o + 5];
            int t3 = input[o + 7];

            // Fast path: row of pure DC.
            if ((t0 | t1 | t2 | t3 | z3 | z4 | z5) == 0)
            {
                int dc = z2 << Pass1Bits;
                for (int i = 0; i < 8; i++) tmp[o + i] = dc;
                continue;
            }

            // Even part.
            int e0 = (z2 + z4) << ConstBits;
            int e1 = (z2 - z4) << ConstBits;
            int z1c = (z3 + z5) * FixM_0_541196100;
            int e2 = z1c + z5 * (-FixM_1_847759065);
            int e3 = z1c + z3 * FixM_0_765366865;
            int ev0 = e0 + e3;
            int ev3 = e0 - e3;
            int ev1 = e1 + e2;
            int ev2 = e1 - e2;

            // Odd part.
            int oo0 = t3;
            int oo1 = t2;
            int oo2 = t1;
            int oo3 = t0;
            int z1 = oo0 + oo3;
            int z2b = oo1 + oo2;
            int z3b = oo0 + oo2;
            int z4b = oo1 + oo3;
            int z5b = (z3b + z4b) * FixM_1_175875602;

            int oo0c = oo0 * FixM_0_298631336;
            int oo1c = oo1 * FixM_2_053119869;
            int oo2c = oo2 * FixM_3_072711026;
            int oo3c = oo3 * FixM_1_501321110;
            int z1c2 = z1 * (-FixM_0_899976223);
            int z2c = z2b * (-FixM_2_562915447);
            int z3c = z3b * (-FixM_1_961570560) + z5b;
            int z4c = z4b * (-FixM_0_390180644) + z5b;

            oo0c += z1c2 + z3c;
            oo1c += z2c + z4c;
            oo2c += z2c + z3c;
            oo3c += z1c2 + z4c;

            tmp[o + 0] = Descale(ev0 + oo3c, PassShift);
            tmp[o + 7] = Descale(ev0 - oo3c, PassShift);
            tmp[o + 1] = Descale(ev1 + oo2c, PassShift);
            tmp[o + 6] = Descale(ev1 - oo2c, PassShift);
            tmp[o + 2] = Descale(ev2 + oo1c, PassShift);
            tmp[o + 5] = Descale(ev2 - oo1c, PassShift);
            tmp[o + 3] = Descale(ev3 + oo0c, PassShift);
            tmp[o + 4] = Descale(ev3 - oo0c, PassShift);
        }

        // ---- Pass 2: columns. Output is level-shifted and clamped. ----
        for (int col = 0; col < 8; col++)
        {
            int z2 = tmp[0 * 8 + col];
            int z3 = tmp[2 * 8 + col];
            int z4 = tmp[4 * 8 + col];
            int z5 = tmp[6 * 8 + col];
            int t0 = tmp[1 * 8 + col];
            int t1 = tmp[3 * 8 + col];
            int t2 = tmp[5 * 8 + col];
            int t3 = tmp[7 * 8 + col];

            int e0 = (z2 + z4) << ConstBits;
            int e1 = (z2 - z4) << ConstBits;
            int z1c = (z3 + z5) * FixM_0_541196100;
            int e2 = z1c + z5 * (-FixM_1_847759065);
            int e3 = z1c + z3 * FixM_0_765366865;
            int ev0 = e0 + e3;
            int ev3 = e0 - e3;
            int ev1 = e1 + e2;
            int ev2 = e1 - e2;

            int oo0 = t3;
            int oo1 = t2;
            int oo2 = t1;
            int oo3 = t0;
            int z1 = oo0 + oo3;
            int z2b = oo1 + oo2;
            int z3b = oo0 + oo2;
            int z4b = oo1 + oo3;
            int z5b = (z3b + z4b) * FixM_1_175875602;

            int oo0c = oo0 * FixM_0_298631336;
            int oo1c = oo1 * FixM_2_053119869;
            int oo2c = oo2 * FixM_3_072711026;
            int oo3c = oo3 * FixM_1_501321110;
            int z1c2 = z1 * (-FixM_0_899976223);
            int z2c = z2b * (-FixM_2_562915447);
            int z3c = z3b * (-FixM_1_961570560) + z5b;
            int z4c = z4b * (-FixM_0_390180644) + z5b;

            oo0c += z1c2 + z3c;
            oo1c += z2c + z4c;
            oo2c += z2c + z3c;
            oo3c += z1c2 + z4c;

            const int totalShift = PassShift + 3;
            output[0 * outputStride + col] = ClampWithBias(ev0 + oo3c, totalShift);
            output[7 * outputStride + col] = ClampWithBias(ev0 - oo3c, totalShift);
            output[1 * outputStride + col] = ClampWithBias(ev1 + oo2c, totalShift);
            output[6 * outputStride + col] = ClampWithBias(ev1 - oo2c, totalShift);
            output[2 * outputStride + col] = ClampWithBias(ev2 + oo1c, totalShift);
            output[5 * outputStride + col] = ClampWithBias(ev2 - oo1c, totalShift);
            output[3 * outputStride + col] = ClampWithBias(ev3 + oo0c, totalShift);
            output[4 * outputStride + col] = ClampWithBias(ev3 - oo0c, totalShift);
        }

        _ = Pass1Round;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    internal static int Descale(int v, int n)
    {
        return (v + (1 << (n - 1))) >> n;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    internal static byte ClampWithBias(int v, int totalShift)
    {
        // Apply (v + round) >> totalShift, add level-shift 128, clamp [0,255].
        int s = Descale(v, totalShift) + 128;
        if (s < 0) return 0;
        if (s > 255) return 255;
        return (byte)s;
    }
}
