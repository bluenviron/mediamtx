using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Per-band gain computation for AAC scale-factor application per
/// ISO/IEC 14496-3 §4.6.2 step 7:
/// <c>gain = 2^((sf - <see cref="SfOffset"/>) / 4)</c>.
/// </summary>
/// <remarks>
/// <para>
/// After <see cref="AacInverseQuantization"/> turns integer quantised
/// coefficients into a floating-point spectrum, the dequantization
/// stage multiplies each scale-factor band's coefficients by the
/// per-band gain derived from the absolute scale factor produced by
/// <see cref="AacAbsoluteScaleFactors"/>. The gain formula
/// <c>2^((sf - 100) / 4)</c> is fixed by the spec; the <c>SF_OFFSET</c>
/// constant of 100 normalises <c>global_gain</c>'s mid-range
/// (typical AAC streams cluster around <c>sf ≈ 100</c>, giving a
/// near-unity gain) and ensures the formula works smoothly for
/// streams that quantise softly (large <c>sf</c>) and aggressively
/// (small <c>sf</c>) alike.
/// </para>
/// <para>
/// Scale-factor values fall in the range <c>[0, 255]</c> for spectral
/// bands. For PNS bands the absolute value can be negative because
/// the PNS energy is signed and <c>global_gain</c>-relative; this
/// helper still computes a valid (small) gain for those.
/// </para>
/// </remarks>
public static class AacScaleFactorGain
{
    /// <summary>
    /// Constant offset subtracted from the absolute scale factor in
    /// the gain formula (<c>SF_OFFSET</c> in the spec).
    /// </summary>
    public const int SfOffset = 100;

    /// <summary>Reciprocal of 4 used to scale the exponent.</summary>
    private const double Quarter = 0.25;

    /// <summary>
    /// Compute the per-band linear gain for an absolute scale factor.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static float Gain(int absoluteScaleFactor)
    {
        double exponent = (absoluteScaleFactor - SfOffset) * Quarter;
        return (float)Math.Pow(2.0, exponent);
    }

    /// <summary>
    /// Multiply <paramref name="band"/> in place by the gain derived
    /// from <paramref name="absoluteScaleFactor"/>. The gain is
    /// computed once and applied to every coefficient in
    /// <paramref name="band"/>.
    /// </summary>
    public static void ApplyTo(Span<float> band, int absoluteScaleFactor)
    {
        float gain = Gain(absoluteScaleFactor);
        for (int i = 0; i < band.Length; i++)
        {
            band[i] *= gain;
        }
    }

    /// <summary>
    /// Multiply each coefficient in <paramref name="source"/> by the
    /// gain derived from <paramref name="absoluteScaleFactor"/> and
    /// write the result to the corresponding slot in
    /// <paramref name="destination"/>. The destination must be at
    /// least as long as the source.
    /// </summary>
    /// <exception cref="ArgumentException">
    /// <paramref name="destination"/> is shorter than <paramref name="source"/>.
    /// </exception>
    public static void Apply(
        ReadOnlySpan<float> source,
        Span<float> destination,
        int absoluteScaleFactor)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException(
                $"Destination span (length {destination.Length}) is shorter than source (length {source.Length}).",
                nameof(destination));
        }
        float gain = Gain(absoluteScaleFactor);
        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = source[i] * gain;
        }
    }
}
