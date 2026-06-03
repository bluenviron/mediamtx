using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Opus.Encoder.Silk;

/// <summary>
/// Fixed-point Q-format arithmetic helpers used throughout the SILK
/// encoder. Direct ports of the saturating / rounding multipliers from
/// libopus <c>silk/macros.h</c> and <c>silk/SigProc_FIX.h</c>.
/// </summary>
/// <remarks>
/// <para>
/// Q-format conventions (RFC 6716 §4.2 and libopus):
/// <list type="bullet">
///   <item><description>Inputs are 16-bit linear PCM in Q0.</description></item>
///   <item><description>Autocorrelations and LPC residual energies are 32-bit, dynamically headroom-shifted; the shift is returned out-of-band.</description></item>
///   <item><description>LPC and LTP coefficients are commonly stored in Q12 (LPC) or Q7 (LTP).</description></item>
///   <item><description>Inverse quantities (gain inverses, autocorrelation reciprocals) use Q30 or higher.</description></item>
/// </list>
/// </para>
/// <para>
/// All helpers are <see cref="MethodImplOptions.AggressiveInlining"/> so
/// the JIT collapses them into the surrounding analysis loop. The
/// arithmetic follows libopus' "wb / ww / ll" naming:
/// <c>smulwb(a, b)</c> multiplies a 32-bit value by the low 16 bits of
/// another 32-bit value and shifts right by 16; <c>smulww</c> does the
/// same but on the high half; <c>smull</c> returns a 64-bit product.
/// </para>
/// </remarks>
internal static class SilkInt
{
    /// <summary>Saturate <paramref name="x"/> to a signed 16-bit range.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static short Sat16(int x)
    {
        if (x > short.MaxValue) return short.MaxValue;
        if (x < short.MinValue) return short.MinValue;
        return (short)x;
    }

    /// <summary>Saturate <paramref name="x"/> to a signed 32-bit range.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int Sat32(long x)
    {
        if (x > int.MaxValue) return int.MaxValue;
        if (x < int.MinValue) return int.MinValue;
        return (int)x;
    }

    /// <summary>Saturating add of two 32-bit signed integers (<c>silk_ADD_SAT32</c>).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int AddSat32(int a, int b) => Sat32((long)a + b);

    /// <summary>Saturating subtract of two 32-bit signed integers (<c>silk_SUB_SAT32</c>).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int SubSat32(int a, int b) => Sat32((long)a - b);

    /// <summary><c>silk_SMULBB(a,b)</c>: 16x16 → 32 multiply of the low halves.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int SmulBB(int a, int b) => (short)a * (short)b;

    /// <summary><c>silk_SMULWB(a,b)</c>: 32x16 → 32 multiply with the low 16 of <paramref name="b"/>, shifted right by 16.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int SmulWB(int a, int b) => (int)((a * (long)(short)b) >> 16);

    /// <summary><c>silk_SMULWW(a,b)</c>: 32x32 → 32, shifted right by 16. Often used for Q15 * Q16 products.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int SmulWW(int a, int b) => (int)((a * (long)b) >> 16);

    /// <summary><c>silk_SMULL(a,b)</c>: full 32x32 → 64 multiply.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static long Smull(int a, int b) => a * (long)b;

    /// <summary>Multiply-accumulate <c>a + b*c</c> with the same shape as <see cref="SmulWB"/>.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int SmlaWB(int a, int b, int c) => a + (int)((b * (long)(short)c) >> 16);

    /// <summary><c>silk_RSHIFT_ROUND(a, shift)</c>: signed arithmetic right shift with round-to-nearest.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int RShiftRound(int a, int shift)
    {
        if (shift <= 0) return a;
        return (a + (1 << (shift - 1))) >> shift;
    }

    /// <summary>Approximate <c>silk_CLZ32</c> — count of leading zero bits in a non-negative 32-bit value.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int Clz32(int x)
    {
        if (x <= 0) return x == 0 ? 32 : 0;
        return System.Numerics.BitOperations.LeadingZeroCount((uint)x);
    }

    /// <summary>
    /// Float helper: convert a Q-format integer back to <see cref="float"/>.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static float FromQ(int x, int q) => x * (1f / (1 << q));

    /// <summary>
    /// Float helper: convert a <see cref="float"/> to Q-format with saturation to 32-bit.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int ToQ(float x, int q)
    {
        double v = (double)x * (1 << q);
        if (v >= int.MaxValue) return int.MaxValue;
        if (v <= int.MinValue) return int.MinValue;
        return (int)System.Math.Round(v, System.MidpointRounding.AwayFromZero);
    }
}
