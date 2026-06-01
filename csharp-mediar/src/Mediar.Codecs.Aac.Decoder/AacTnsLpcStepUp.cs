namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Step-up conversion from PARCOR (reflection / lattice) coefficients
/// to direct-form linear-prediction coefficients via the Levinson
/// recursion. Used by the AAC TNS (temporal noise shaping) stage,
/// where the bitstream carries quantised reflection coefficients and
/// the inverse filter operates on the direct form (ISO/IEC 14496-3
/// §4.6.9.3).
/// </summary>
/// <remarks>
/// <para>
/// The step-up recursion is the textbook companion to the
/// Levinson-Durbin algorithm. Given PARCOR coefficients
/// <c>k[1..order]</c> the routine computes
/// <c>a[1..order]</c> such that the all-pole transfer function
/// </para>
/// <code>
///     1 / (1 - a[1] z^{-1} - a[2] z^{-2} - ... - a[order] z^{-order})
/// </code>
/// <para>
/// realised in direct form is equivalent to the lattice form built
/// from <c>k</c>. The recurrence used here matches the AAC TNS
/// formulation directly:
/// </para>
/// <code>
///     for i = 1 .. order
///       tmp[j]   = a[j] + k[i] * a[i - j]   for j = 1 .. i - 1
///       a[1..i-1] = tmp[1..i-1]
///       a[i]     = k[i]
/// </code>
/// <para>
/// The implementation is in-place with respect to the output buffer
/// and uses a stack-allocated scratch row so it allocates nothing on
/// the heap.
/// </para>
/// </remarks>
public static class AacTnsLpcStepUp
{
    /// <summary>
    /// Maximum filter order supported. Matches the AAC TNS long-window
    /// upper bound (<see cref="AacTnsData.MaxOrderLong"/> = 31).
    /// </summary>
    public const int MaxOrder = AacTnsData.MaxOrderLong;

    /// <summary>
    /// Compute the direct-form LPC coefficients from
    /// <paramref name="parcor"/> and write them to <paramref name="lpc"/>.
    /// </summary>
    /// <param name="parcor">
    /// Reflection coefficients <c>k[1..order]</c>, in stream order
    /// (i.e. the first entry is <c>k[1]</c>). The length determines
    /// the filter order.
    /// </param>
    /// <param name="lpc">
    /// Output buffer for the direct-form coefficients
    /// <c>a[1..order]</c>. Must have the same length as
    /// <paramref name="parcor"/>.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="lpc"/> has a different length than
    /// <paramref name="parcor"/>, or <paramref name="parcor"/> has more
    /// than <see cref="MaxOrder"/> entries.
    /// </exception>
    public static void Compute(ReadOnlySpan<float> parcor, Span<float> lpc)
    {
        if (lpc.Length != parcor.Length)
        {
            throw new ArgumentException(
                $"lpc length ({lpc.Length}) must match parcor length ({parcor.Length}).",
                nameof(lpc));
        }

        int order = parcor.Length;
        if (order > MaxOrder)
        {
            throw new ArgumentException(
                $"PARCOR order {order} exceeds the supported maximum {MaxOrder}.",
                nameof(parcor));
        }

        if (order == 0) return;

        Span<float> tmp = stackalloc float[MaxOrder];

        for (int i = 0; i < order; i++)
        {
            // The recurrence is 1-indexed in spec terms; the local
            // variable 'i' is 0-indexed so the live row indices run
            // 0..i-1 and the just-introduced coefficient lands at i.
            float k = parcor[i];

            for (int j = 0; j < i; j++)
            {
                tmp[j] = lpc[j] + k * lpc[i - 1 - j];
            }
            for (int j = 0; j < i; j++)
            {
                lpc[j] = tmp[j];
            }
            lpc[i] = k;
        }
    }

    /// <summary>
    /// Allocating overload of
    /// <see cref="Compute(ReadOnlySpan{float}, Span{float})"/>;
    /// returns a freshly allocated array of direct-form LPC
    /// coefficients.
    /// </summary>
    /// <param name="parcor">Reflection coefficients <c>k[1..order]</c>.</param>
    /// <returns>Direct-form LPC coefficients <c>a[1..order]</c>.</returns>
    public static float[] Compute(ReadOnlySpan<float> parcor)
    {
        var lpc = new float[parcor.Length];
        Compute(parcor, lpc);
        return lpc;
    }
}
