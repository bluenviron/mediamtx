namespace Mediar.Codecs.Opus.Encoder.Silk;

/// <summary>
/// SILK autocorrelation and Burg-method LPC analysis. Port of the
/// algorithmic core of libopus <c>silk/burg_modified.c</c>
/// (<c>silk_burg_modified</c>) plus the autocorrelation accumulator
/// from <c>silk/autocorrelation.c</c>.
/// </summary>
/// <remarks>
/// <para>
/// Burg's maximum-entropy method (Burg 1975) computes the
/// reflection coefficients (PARCORs) and the equivalent direct-form
/// LPC coefficients <c>a[1..P]</c> from a finite-length sample window,
/// without an explicit windowing step. Compared to autocorrelation +
/// Levinson-Durbin it has lower bias on short windows and is the
/// method libopus uses inside <c>silk/find_LPC.c</c> for sub-frame
/// LPC analysis.
/// </para>
/// <para>
/// This port is in <see cref="float"/> precision to match the rest of
/// the Mediar Opus pipeline (the decoder's CELT path is also a float
/// build). When the SILK encoder graduates to bit-exact stream
/// production (a later slice, gated on SILK decoder Phases 3-4
/// landing), this file's <see cref="Burg(System.ReadOnlySpan{float}, System.Span{float}, int)"/>
/// will be replaced with the fixed-point Q-format version that
/// matches libopus' <c>burg_modified_FIX.c</c> bit-for-bit.
/// </para>
/// <para>References:</para>
/// <list type="bullet">
///   <item><description>RFC 6716 §4.2 (SILK decoder spec the encoder must produce a stream for).</description></item>
///   <item><description>libopus <c>silk/burg_modified.c</c>, <c>silk/find_LPC.c</c>, <c>silk/autocorrelation.c</c>.</description></item>
///   <item><description>J. P. Burg, <i>Maximum entropy spectral analysis</i>, PhD thesis, Stanford University, 1975.</description></item>
///   <item><description>B. S. Atal, <i>Predictive coding of speech at low bit rates</i>, IEEE Transactions on Communications, vol. 30, no. 4, pp. 600-614, 1982.</description></item>
/// </list>
/// </remarks>
public static class SilkAutocorr
{
    /// <summary>
    /// Compute the autocorrelation sequence
    /// <c>r[k] = Σ x[n] · x[n+k]</c> for <c>k = 0 … order</c>.
    /// Mirrors libopus <c>silk_autocorrelation</c>.
    /// </summary>
    /// <param name="x">Input samples.</param>
    /// <param name="r">Destination of length <c>order + 1</c>.</param>
    /// <param name="order">Number of autocorrelation lags after lag 0.</param>
    public static void Autocorrelation(ReadOnlySpan<float> x, Span<float> r, int order)
    {
        if (r.Length < order + 1)
            throw new ArgumentException("Destination span shorter than order + 1.", nameof(r));
        if (order < 0)
            throw new ArgumentOutOfRangeException(nameof(order));

        int n = x.Length;
        for (int k = 0; k <= order; k++)
        {
            double s = 0.0;
            for (int i = 0; i + k < n; i++)
                s += (double)x[i] * x[i + k];
            r[k] = (float)s;
        }
    }

    /// <summary>
    /// Burg-method LPC analysis. Computes the order-<paramref name="order"/>
    /// LPC coefficients that minimise the geometric mean of forward and
    /// backward prediction errors over <paramref name="x"/>.
    /// </summary>
    /// <param name="x">Analysis window.</param>
    /// <param name="a">
    /// Output LPC coefficients, length <paramref name="order"/>. The
    /// implicit leading coefficient <c>a[0] = 1</c> is omitted; the
    /// prediction model is <c>x̂[n] = -Σ a[i-1] · x[n-i]</c> for
    /// <c>i = 1 … order</c>.
    /// </param>
    /// <param name="order">LPC order; <c>1 ≤ order ≤ x.Length</c>.</param>
    /// <returns>
    /// Residual energy after applying the inverse LPC filter to
    /// <paramref name="x"/>. Always non-negative.
    /// </returns>
    public static float Burg(ReadOnlySpan<float> x, Span<float> a, int order)
    {
        if (order < 1) throw new ArgumentOutOfRangeException(nameof(order));
        if (a.Length < order)
            throw new ArgumentException("Coefficient span shorter than order.", nameof(a));
        if (x.Length < order + 1)
            throw new ArgumentException("Window must contain at least order + 1 samples.", nameof(x));

        int n = x.Length;

        // f[i] = forward prediction error, b[i] = backward prediction error.
        // Initially both equal the input.
        Span<float> f = n <= 1024 ? stackalloc float[n] : new float[n];
        Span<float> b = n <= 1024 ? stackalloc float[n] : new float[n];
        for (int i = 0; i < n; i++) { f[i] = x[i]; b[i] = x[i]; }

        // Initial total error energy (E_0 in Burg's recursion). The PARCOR
        // denominator uses Σ(f² + b²), which equals 2·E_0 at the first step.
        double e = 0.0;
        for (int i = 0; i < n; i++) e += (double)x[i] * x[i];
        e *= 2.0;

        // Workspace for in-place coefficient update.
        Span<float> aPrev = order <= 64 ? stackalloc float[order] : new float[order];
        for (int i = 0; i < order; i++) { a[i] = 0f; aPrev[i] = 0f; }

        for (int m = 0; m < order; m++)
        {
            // Numerator: 2 · Σ_{i=m+1..n-1} f[i] · b[i-1].
            double num = 0.0;
            for (int i = m + 1; i < n; i++)
                num += (double)f[i] * b[i - 1];
            num *= 2.0;

            // The Burg denominator is approximately the current total error
            // energy (forward + backward) restricted to indices ≥ m+1; we
            // maintain it via the standard recursion below to avoid an
            // O(N) pass each iteration.
            // For the very first iteration, peel off the m=0 contributions
            // (f[0]² and b[n-1]²) that fall outside the recursion window.
            if (m == 0)
            {
                e -= (double)x[0] * x[0] + (double)x[n - 1] * x[n - 1];
            }

            float k = e > 0.0 ? (float)(num / e) : 0f;
            // Clamp PARCOR to (-1, 1) for stability — Burg guarantees this
            // in exact arithmetic, but float roundoff can push it outside.
            if (k > 0.99999f) k = 0.99999f;
            else if (k < -0.99999f) k = -0.99999f;

            // Direct-form coefficient update: a_new[i] = a[i] - k · a[m-1-i].
            for (int i = 0; i < m; i++) aPrev[i] = a[i];
            for (int i = 0; i < m; i++)
                a[i] = aPrev[i] - k * aPrev[m - 1 - i];
            a[m] = k;

            // Update forward and backward errors and the running energy
            // (Burg's recursion: walk from the high index downward so the
            // b update uses the un-rotated b[i-1]).
            for (int i = n - 1; i > m; i--)
            {
                float fi = f[i] - k * b[i - 1];
                float bi = b[i - 1] - k * f[i];
                f[i] = fi;
                b[i] = bi;
            }

            // E_{m+1} = (1 - k²) · E_m, then peel the boundary samples
            // that fall out of the next iteration's summation window.
            e = (1.0 - (double)k * k) * e;
            if (m + 1 < order)
            {
                int j = m + 1;
                e -= (double)f[j] * f[j] + (double)b[n - 1] * b[n - 1];
            }
        }

        // Final residual energy: sum of forward-error squares over the
        // valid post-recursion window.
        double res = 0.0;
        for (int i = order; i < n; i++) res += (double)f[i] * f[i];
        return (float)res;
    }

    /// <summary>
    /// Reference Levinson-Durbin recursion on a precomputed
    /// autocorrelation sequence. Used by tests and as a fallback when
    /// the input is already characterised by its autocorrelation.
    /// </summary>
    /// <param name="r">Autocorrelation, length <c>order + 1</c>.</param>
    /// <param name="a">Output LPC coefficients, length <paramref name="order"/>.</param>
    /// <param name="order">LPC order.</param>
    /// <returns>Residual energy <c>E_P</c> after the recursion.</returns>
    public static float LevinsonDurbin(ReadOnlySpan<float> r, Span<float> a, int order)
    {
        if (order < 1) throw new ArgumentOutOfRangeException(nameof(order));
        if (r.Length < order + 1)
            throw new ArgumentException("Autocorrelation shorter than order + 1.", nameof(r));
        if (a.Length < order)
            throw new ArgumentException("Coefficient span shorter than order.", nameof(a));

        double e = r[0];
        Span<double> ad = order <= 64 ? stackalloc double[order] : new double[order];
        Span<double> tmp = order <= 64 ? stackalloc double[order] : new double[order];

        for (int m = 0; m < order; m++)
        {
            double acc = r[m + 1];
            for (int i = 0; i < m; i++) acc += ad[i] * r[m - i];
            double k = e > 0.0 ? -acc / e : 0.0;

            for (int i = 0; i < m; i++) tmp[i] = ad[i];
            for (int i = 0; i < m; i++) ad[i] = tmp[i] + k * tmp[m - 1 - i];
            ad[m] = k;
            e *= 1.0 - k * k;
        }

        // Note the sign convention: Levinson here yields a[i] such that
        // x̂[n] = -Σ a[i] · x[n-i-1], matching <see cref="Burg"/>.
        for (int i = 0; i < order; i++) a[i] = (float)ad[i];
        return (float)e;
    }
}
