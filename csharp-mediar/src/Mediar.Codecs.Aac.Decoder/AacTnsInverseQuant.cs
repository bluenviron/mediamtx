namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Inverse quantiser for AAC TNS (temporal noise shaping) filter
/// coefficients per ISO/IEC 14496-3 §4.6.9.3. Converts the raw
/// <c>CoefBits</c>-wide unsigned values stored in
/// <see cref="AacTnsFilter.Coefficients"/> into PARCOR (reflection)
/// coefficients suitable for <see cref="AacTnsLpcStepUp"/>.
/// </summary>
/// <remarks>
/// <para>
/// The inverse-quantisation step is:
/// </para>
/// <list type="number">
///   <item>
///     Sign-extend each raw value to a signed
///     <c>CoefBits</c>-bit integer.
///   </item>
///   <item>
///     Compute <c>parcor = sin(signed · π / 2 / Q)</c> where
///     <c>Q</c> is the per-window full-scale step:
///     <c>Q = 2^(CoefRes + 2) − 0.5</c> for non-negative inputs and
///     <c>Q = 2^(CoefRes + 2) + 0.5</c> for negative inputs. That is,
///     <c>CoefRes = 1</c> (4-bit base) uses 7.5 / 8.5 and
///     <c>CoefRes = 0</c> (3-bit base) uses 3.5 / 4.5.
///   </item>
/// </list>
/// <para>
/// The slightly asymmetric step sizes (<c>±0.5</c>) are the AAC
/// specification's choice for keeping the resulting PARCOR magnitudes
/// strictly below 1, which guarantees that the
/// <see cref="AacTnsLpcStepUp"/> output describes a stable all-pole
/// filter for <see cref="AacTnsInverseFilter"/>.
/// </para>
/// <para>
/// This routine is allocation-free on the hot path (writes into a
/// caller-provided span) with an allocating convenience overload.
/// </para>
/// </remarks>
public static class AacTnsInverseQuant
{
    /// <summary>
    /// Convert <paramref name="filter"/>'s raw coefficients into PARCOR
    /// values and write them to <paramref name="parcor"/>.
    /// </summary>
    /// <param name="filter">
    /// Parsed TNS filter carrying raw unsigned coefficients and the
    /// per-filter <see cref="AacTnsFilter.CoefBits"/>.
    /// </param>
    /// <param name="coefResHigh">
    /// Window-level <c>coef_res</c> flag (<see langword="true"/> =
    /// 4-bit base, <see langword="false"/> = 3-bit base). Selects the
    /// asymmetric step size used by the spec.
    /// </param>
    /// <param name="parcor">
    /// Destination span. Must have the same length as
    /// <see cref="AacTnsFilter.Order"/>.
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="filter"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="parcor"/>'s length does not equal the filter's
    /// order, or the filter's <see cref="AacTnsFilter.CoefBits"/> is
    /// outside the supported range 2..4, or the raw coefficients are
    /// out of range for the declared width.
    /// </exception>
    public static void Compute(AacTnsFilter filter, bool coefResHigh, Span<float> parcor)
    {
        ArgumentNullException.ThrowIfNull(filter);

        int order = filter.Order;
        if (parcor.Length != order)
        {
            throw new ArgumentException(
                $"parcor length ({parcor.Length}) must match filter order ({order}).",
                nameof(parcor));
        }

        if (order == 0) return;

        int coefBits = filter.CoefBits;
        if (coefBits is < 2 or > 4)
        {
            throw new ArgumentException(
                $"Filter CoefBits {coefBits} is outside the supported range 2..4.",
                nameof(filter));
        }

        // Step size denominators per §4.6.9.3 (asymmetric).
        // For 4-bit base: 7.5 / 8.5; for 3-bit base: 3.5 / 4.5.
        double basePower = coefResHigh ? 8.0 : 4.0;
        double iqfac = (basePower - 0.5) / (Math.PI / 2.0);    // for >= 0
        double iqfacM = (basePower + 0.5) / (Math.PI / 2.0);   // for <  0

        int signBit = 1 << (coefBits - 1);
        int range = 1 << coefBits;

        var raw = filter.Coefficients;
        if (raw.Length != order)
        {
            throw new ArgumentException(
                $"Filter has order {order} but only {raw.Length} raw coefficients.",
                nameof(filter));
        }

        for (int i = 0; i < order; i++)
        {
            int value = raw[i];
            if ((uint)value >= (uint)range)
            {
                throw new ArgumentException(
                    $"Raw coefficient {value} exceeds the declared {coefBits}-bit field width.",
                    nameof(filter));
            }

            int signed = value >= signBit ? value - range : value;

            double tmp = signed >= 0 ? signed / iqfac : signed / iqfacM;
            parcor[i] = (float)Math.Sin(tmp);
        }
    }

    /// <summary>
    /// Allocating overload of
    /// <see cref="Compute(AacTnsFilter, bool, Span{float})"/>; returns a
    /// freshly allocated array of PARCOR coefficients of length
    /// <see cref="AacTnsFilter.Order"/>.
    /// </summary>
    public static float[] Compute(AacTnsFilter filter, bool coefResHigh)
    {
        ArgumentNullException.ThrowIfNull(filter);
        var parcor = new float[filter.Order];
        Compute(filter, coefResHigh, parcor);
        return parcor;
    }
}
