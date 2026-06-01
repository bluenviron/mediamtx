namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Inverse all-pole (IIR) filter used to undo the AAC TNS encoder's
/// forward FIR shaping. Operates in place on a contiguous slice of
/// the dequantised MDCT spectrum per ISO/IEC 14496-3 §4.6.9.2.
/// </summary>
/// <remarks>
/// <para>
/// For each spectral sample <c>m</c> walked in the chosen direction,
/// the IIR step is
/// </para>
/// <code>
///     spectrum[m] += sum_{k = 1 .. order} lpc[k - 1] * past[k]
/// </code>
/// <para>
/// where <c>past[k]</c> is the previously filtered output
/// <c>k</c> steps ago in the walk direction. The convention matches
/// the libfaad TNS implementation, where the LPC coefficients carry
/// the PLUS sign from <see cref="AacTnsLpcStepUp"/> and the inverse
/// filter adds the past contributions rather than subtracting them.
/// </para>
/// <para>
/// Direction selects the walk through the spectrum slice:
/// </para>
/// <list type="bullet">
///   <item>
///     <description>
///       <c>reverseDirection = false</c>: walk low-to-high (ascending
///       index 0, 1, 2, ...). Matches TNS <c>direction = 0</c>.
///     </description>
///   </item>
///   <item>
///     <description>
///       <c>reverseDirection = true</c>: walk high-to-low (descending
///       index <c>Length - 1, Length - 2, ...</c>). Matches TNS
///       <c>direction = 1</c>.
///     </description>
///   </item>
/// </list>
/// <para>
/// The past-output state is initialised to all-zeros at the start of
/// each call; TNS does not propagate state across calls.
/// </para>
/// </remarks>
public static class AacTnsInverseFilter
{
    /// <summary>
    /// Maximum filter order supported, matching
    /// <see cref="AacTnsLpcStepUp.MaxOrder"/>.
    /// </summary>
    public const int MaxOrder = AacTnsLpcStepUp.MaxOrder;

    /// <summary>
    /// Apply the IIR inverse filter to <paramref name="spectrum"/>
    /// in-place using <paramref name="lpc"/> (direct-form LPC of
    /// length <c>order</c>, as produced by
    /// <see cref="AacTnsLpcStepUp"/>).
    /// </summary>
    /// <param name="spectrum">
    /// Contiguous spectral coefficients to filter; rewritten in-place.
    /// </param>
    /// <param name="lpc">
    /// Direct-form LPC coefficients <c>a[1..order]</c>. Length
    /// determines the filter order.
    /// </param>
    /// <param name="reverseDirection">
    /// <see langword="false"/> walks <paramref name="spectrum"/>
    /// low-to-high; <see langword="true"/> walks it high-to-low.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="lpc"/> exceeds <see cref="MaxOrder"/>.
    /// </exception>
    public static void Apply(
        Span<float> spectrum,
        ReadOnlySpan<float> lpc,
        bool reverseDirection)
    {
        int order = lpc.Length;
        if (order > MaxOrder)
        {
            throw new ArgumentException(
                $"TNS filter order {order} exceeds the supported maximum {MaxOrder}.",
                nameof(lpc));
        }
        if (order == 0 || spectrum.Length == 0) return;

        // Past-output ring buffer: state[0] is the most recent output
        // (one step back), state[order - 1] is the oldest.
        Span<float> state = stackalloc float[MaxOrder];
        state.Clear();

        int length = spectrum.Length;
        if (!reverseDirection)
        {
            for (int m = 0; m < length; m++)
            {
                float sum = spectrum[m];
                for (int k = 0; k < order; k++)
                {
                    sum += lpc[k] * state[k];
                }
                spectrum[m] = sum;
                ShiftStateRight(state, order, sum);
            }
        }
        else
        {
            for (int m = length - 1; m >= 0; m--)
            {
                float sum = spectrum[m];
                for (int k = 0; k < order; k++)
                {
                    sum += lpc[k] * state[k];
                }
                spectrum[m] = sum;
                ShiftStateRight(state, order, sum);
            }
        }
    }

    private static void ShiftStateRight(Span<float> state, int order, float newest)
    {
        // state[order-1] gets dropped, everything else slides right,
        // newest output goes to state[0].
        for (int k = order - 1; k > 0; k--)
        {
            state[k] = state[k - 1];
        }
        state[0] = newest;
    }
}
