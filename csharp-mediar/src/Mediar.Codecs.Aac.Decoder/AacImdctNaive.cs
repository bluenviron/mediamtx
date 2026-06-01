namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Reference (O(N²)) inverse modified discrete cosine transform
/// used by the AAC time-frequency synthesis per
/// ISO/IEC 14496-3 §4.6.11.1:
/// </summary>
/// <remarks>
/// <para>
/// <c>x[n] = (2/N) · Σ_{k=0..M-1} spec[k] · cos((2π/N) · (n + n₀) · (k + 1/2))</c>
/// </para>
/// <para>
/// where:
/// </para>
/// <list type="bullet">
///   <item><c>M</c> = number of spectral coefficients
///         (<c>1024</c> long, <c>128</c> short)</item>
///   <item><c>N = 2M</c> = number of output samples</item>
///   <item><c>n₀ = (M + 1) / 2</c> = AAC's MDCT phase shift</item>
/// </list>
/// <para>
/// This implementation walks the full double loop and is intended as
/// a correctness reference that the eventual fast (split-radix /
/// post-rotated-FFT) implementation will be validated against. For
/// short blocks (<c>M = 128</c>) it is fast enough for production
/// use; for long blocks (<c>M = 1024</c>) it costs ~2 M floating-
/// point operations per frame per channel, which is workable but
/// far from the fast-path target.
/// </para>
/// </remarks>
public static class AacImdctNaive
{
    /// <summary>Long-block input length (1024 coefficients → 2048 samples).</summary>
    public const int LongInputLength = 1024;

    /// <summary>Short-block input length (128 coefficients → 256 samples).</summary>
    public const int ShortInputLength = 128;

    /// <summary>
    /// Inverse MDCT in place: writes <c>2·M</c> samples to
    /// <paramref name="samples"/> from <c>M</c>
    /// <paramref name="coefficients"/>.
    /// </summary>
    /// <param name="coefficients">
    /// Spectral input of length <c>M</c>. Empty is treated as a
    /// no-op.
    /// </param>
    /// <param name="samples">
    /// Time-domain output of length <c>2 · M</c>.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="samples"/> length is not exactly twice
    /// <paramref name="coefficients"/> length.
    /// </exception>
    public static void Inverse(ReadOnlySpan<float> coefficients, Span<float> samples)
    {
        if (samples.Length != 2 * coefficients.Length)
        {
            throw new ArgumentException(
                $"Samples length ({samples.Length}) must be exactly 2x coefficients length ({coefficients.Length}).",
                nameof(samples));
        }

        if (coefficients.IsEmpty)
        {
            return;
        }

        int m = coefficients.Length;
        int n = 2 * m;
        double scale = 2.0 / n;
        double n0 = (m + 1) / 2.0;
        double omega = 2.0 * Math.PI / n;

        for (int i = 0; i < n; i++)
        {
            double sum = 0.0;
            double phase = omega * (i + n0);
            for (int k = 0; k < m; k++)
            {
                sum += coefficients[k] * Math.Cos(phase * (k + 0.5));
            }
            samples[i] = (float)(scale * sum);
        }
    }

    /// <summary>
    /// Allocating convenience overload returning a new sample buffer
    /// of length <c>2 · coefficients.Length</c>.
    /// </summary>
    public static float[] Inverse(ReadOnlySpan<float> coefficients)
    {
        var output = new float[2 * coefficients.Length];
        Inverse(coefficients, output.AsSpan());
        return output;
    }
}
