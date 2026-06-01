using System.Collections.Immutable;
using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC Kaiser-Bessel-Derived (KBD) analysis/synthesis window per
/// ISO/IEC 14496-3 §4.6.11.3.2. Used when <c>ics_info().window_shape
/// == 1</c>; the alternative <c>0</c> shape is
/// <see cref="AacSineWindow"/>.
/// </summary>
/// <remarks>
/// <para>
/// The KBD window is derived from a Kaiser window of length
/// <c>N + 1</c> via the cumulative-sum / square-root construction:
/// </para>
/// <list type="number">
///   <item>Compute the Kaiser kernel
///         <c>W(p) = I₀(π·α·√(1 − ((p − N/2) / (N/2))²))</c> for
///         <c>p = 0 .. N</c>.</item>
///   <item>Form running sums <c>S(n) = Σ_{p=0..n} W(p)</c> and the
///         total <c>S(N)</c>.</item>
///   <item>Normalised cumulative
///         <c>W'(n) = S(n) / S(N)</c>.</item>
///   <item>Rising left half
///         <c>w_kbd(n) = √W'(n)</c> for <c>n = 0 .. N-1</c>;
///         falling right half is the mirror reflection.</item>
/// </list>
/// <para>
/// AAC uses <c>α = 4</c> for long blocks (<c>N = 1024</c>) and
/// <c>α = 6</c> for short blocks (<c>N = 128</c>), exposed via
/// <see cref="LongAlpha"/> / <see cref="ShortAlpha"/>.
/// </para>
/// <para>
/// Like the sine window, KBD satisfies the MDCT TDAC
/// perfect-reconstruction condition
/// <c>w(n)² + w(N + n)² = 1</c>.
/// </para>
/// </remarks>
public static class AacKbdWindow
{
    /// <summary>Kaiser shape parameter for long blocks (<c>α = 4</c>).</summary>
    public const double LongAlpha = 4.0;

    /// <summary>Kaiser shape parameter for short blocks (<c>α = 6</c>).</summary>
    public const double ShortAlpha = 6.0;

    /// <summary>Number of samples in a long-window half (<c>N = 1024</c>).</summary>
    public const int LongHalfLength = AacSineWindow.LongHalfLength;

    /// <summary>Number of samples in a short-window half (<c>N = 128</c>).</summary>
    public const int ShortHalfLength = AacSineWindow.ShortHalfLength;

    /// <summary>
    /// Compute the rising left half of the KBD window of length
    /// <paramref name="halfLength"/> using shape parameter
    /// <paramref name="alpha"/>.
    /// </summary>
    public static ImmutableArray<float> ComputeRisingHalf(int halfLength, double alpha)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(halfLength, 0);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(alpha);

        var buffer = new float[halfLength];
        WriteRisingHalfInternal(buffer, halfLength, alpha);
        return ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    /// <summary>
    /// Compute the full symmetric KBD window of length
    /// <c>2 · <paramref name="halfLength"/></c>.
    /// </summary>
    public static ImmutableArray<float> ComputeFull(int halfLength, double alpha)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(halfLength, 0);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(alpha);

        var buffer = new float[2 * halfLength];
        WriteRisingHalfInternal(buffer.AsSpan(0, halfLength), halfLength, alpha);
        for (int n = 0; n < halfLength; n++)
        {
            buffer[2 * halfLength - 1 - n] = buffer[n];
        }
        return ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    /// <summary>
    /// Write the rising left half of the KBD window into
    /// <paramref name="destination"/> using shape parameter
    /// <paramref name="alpha"/>.
    /// </summary>
    public static void WriteRisingHalf(Span<float> destination, double alpha)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(alpha);
        if (destination.IsEmpty) return;
        WriteRisingHalfInternal(destination, destination.Length, alpha);
    }

    private static void WriteRisingHalfInternal(Span<float> destination, int halfLength, double alpha)
    {
        // Kaiser kernel of length N + 1 (indices 0..N).
        Span<double> kernel = halfLength + 1 <= 2048
            ? stackalloc double[halfLength + 1]
            : new double[halfLength + 1];

        double piAlpha = Math.PI * alpha;
        double halfN = halfLength / 2.0;
        for (int p = 0; p <= halfLength; p++)
        {
            double t = (p - halfN) / halfN;
            double inner = 1.0 - t * t;
            if (inner < 0.0) inner = 0.0;
            kernel[p] = ModifiedBesselI0(piAlpha * Math.Sqrt(inner));
        }

        // Running sum and total.
        Span<double> cum = halfLength + 1 <= 2048
            ? stackalloc double[halfLength + 1]
            : new double[halfLength + 1];
        double running = 0.0;
        for (int p = 0; p <= halfLength; p++)
        {
            running += kernel[p];
            cum[p] = running;
        }
        double total = cum[halfLength];

        for (int n = 0; n < halfLength; n++)
        {
            double normalised = cum[n] / total;
            destination[n] = (float)Math.Sqrt(normalised);
        }
    }

    /// <summary>
    /// Modified Bessel function of the first kind, order zero,
    /// computed via its absolutely-convergent power series:
    /// <c>I₀(x) = Σ_{k≥0} (x/2)^(2k) / (k!)²</c>.
    /// </summary>
    internal static double ModifiedBesselI0(double x)
    {
        double y = x * x * 0.25;
        double term = 1.0;
        double sum = 1.0;
        for (int k = 1; k < 60; k++)
        {
            term *= y / (k * k);
            sum += term;
            if (term < 1e-18 * sum)
            {
                break;
            }
        }
        return sum;
    }
}
