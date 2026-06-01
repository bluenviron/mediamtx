namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC pulse-data applier per ISO/IEC 14496-3 §4.6.2.1: modifies up
/// to four quantised spectral coefficients in place by adding
/// (or subtracting, for negative coefficients) per-pulse amplitudes
/// at offsets derived from <c>swb_offset_long_window</c>.
/// </summary>
/// <remarks>
/// <para>
/// Pulse data is a noiseless-coding extension that the encoder uses
/// to "fix up" individual quantised coefficients without paying for
/// a full extra section of high-magnitude entries. Per the spec the
/// fix-up happens in the quantised integer domain
/// <strong>before</strong> <see cref="AacInverseQuantization"/>;
/// applying pulses after dequant would lose the
/// <c>x^(4/3)</c> non-linearity and break bit-exact decoding.
/// </para>
/// <para>
/// Pulse data is only legal when
/// <c>window_sequence != EIGHT_SHORT_SEQUENCE</c>; the parser
/// already enforces that on read. The first pulse lives at
/// <c>swb_offset[pulse_start_sfb] + pulse_offset[0]</c>; each
/// subsequent pulse is the previous position plus
/// <c>pulse_offset[i]</c>.
/// </para>
/// <para>
/// Sign-preserving update per spec §4.6.2.1:
/// <code>
/// if (quant[p] &gt; 0) quant[p] += amplitude;
/// else               quant[p] -= amplitude;
/// </code>
/// Coefficients that are exactly zero fall into the <c>else</c>
/// branch and become <c>-amplitude</c>.
/// </para>
/// </remarks>
public static class AacPulseApplier
{
    /// <summary>
    /// Apply <paramref name="pulses"/> in place to
    /// <paramref name="quantizedCoefficients"/> using
    /// <paramref name="longSwbOffsets"/> as the long-window SWB
    /// offset table.
    /// </summary>
    /// <param name="quantizedCoefficients">
    /// Spectral coefficients in the quantised integer domain
    /// (i.e., before <see cref="AacInverseQuantization"/>). Must
    /// hold at least the maximum target position; for AAC long
    /// windows this is normally 1024 entries.
    /// </param>
    /// <param name="pulses">Parsed pulse data.</param>
    /// <param name="longSwbOffsets">
    /// Long-window SWB offset table for the current sample rate
    /// (see <see cref="AacSwbOffsets.GetLongOffsets"/>).
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="pulses"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <c>pulse_start_sfb</c> is beyond <paramref name="longSwbOffsets"/>
    /// or a pulse position would land outside
    /// <paramref name="quantizedCoefficients"/>.
    /// </exception>
    public static void ApplyToQuantised(
        Span<int> quantizedCoefficients,
        AacPulseData pulses,
        ReadOnlySpan<int> longSwbOffsets)
    {
        ArgumentNullException.ThrowIfNull(pulses);

        int startSfb = pulses.StartScaleFactorBand;
        if ((uint)startSfb >= (uint)longSwbOffsets.Length)
        {
            throw new ArgumentOutOfRangeException(
                nameof(pulses),
                $"pulse_start_sfb {startSfb} is outside the SWB table (length {longSwbOffsets.Length}).");
        }

        int position = longSwbOffsets[startSfb];
        var pulseList = pulses.Pulses;
        for (int i = 0; i < pulseList.Length; i++)
        {
            var pulse = pulseList[i];
            position += pulse.Offset;
            if ((uint)position >= (uint)quantizedCoefficients.Length)
            {
                throw new ArgumentOutOfRangeException(
                    nameof(pulses),
                    $"pulse[{i}] target position {position} is outside the coefficient buffer (length {quantizedCoefficients.Length}).");
            }

            if (quantizedCoefficients[position] > 0)
            {
                quantizedCoefficients[position] += pulse.Amplitude;
            }
            else
            {
                quantizedCoefficients[position] -= pulse.Amplitude;
            }
        }
    }
}
