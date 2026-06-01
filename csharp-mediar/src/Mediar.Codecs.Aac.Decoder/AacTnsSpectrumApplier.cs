namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Per-frame driver for AAC Temporal Noise Shaping inverse filtering
/// per ISO/IEC 14496-3 §4.6.9.2. Walks every filter declared by an
/// <see cref="AacTnsData"/>, composes
/// <see cref="AacTnsInverseQuant"/> (raw → PARCOR),
/// <see cref="AacTnsLpcStepUp"/> (PARCOR → direct-form LPC) and
/// <see cref="AacTnsInverseFilter"/> (IIR inverse on a spectral slice)
/// and applies the result in place to the dequantised MDCT spectrum.
/// </summary>
/// <remarks>
/// <para>
/// The recursion in §4.6.9.2 walks filters within a window from the
/// highest SFB down: <c>top</c> starts at the SWB count, the next
/// filter's <c>top</c> equals the previous filter's <c>bottom</c> and
/// each filter has <c>bottom = max(top − length, 0)</c>. Filter band
/// ranges therefore tile the spectrum without overlap.
/// </para>
/// <para>
/// The clamp on <c>start</c> uses the encoded <c>max_sfb</c> from
/// <see cref="AacIcsInfo.MaxSfb"/> (above it the spectrum is zero
/// anyway) while the clamp on <c>end</c> uses the spec table value
/// <c>tns_max_sfb</c> (above it TNS is disallowed regardless of
/// whether the spectrum carries data). Mirroring libfaad's
/// asymmetric <c>start = swb_offset[min(bottom, max_sfb)]</c> /
/// <c>end = swb_offset[min(top, tns_max_sfb)]</c> intentionally.
/// </para>
/// <para>
/// <b>Spectrum layout contract</b>: <see cref="Apply"/> expects the
/// 1024-line MDCT spectrum in <i>per-window</i> layout, not the
/// grouped + interleaved spec-bitstream layout produced by
/// <see cref="AacSpectralData"/>. Specifically:
/// </para>
/// <list type="bullet">
///   <item>
///     <description>
///       <see cref="AacWindowSequence.OnlyLong"/>,
///       <see cref="AacWindowSequence.LongStart"/>,
///       <see cref="AacWindowSequence.LongStop"/>: one contiguous
///       1024-line window at <c>spectrum[0..1024]</c>.
///     </description>
///   </item>
///   <item>
///     <description>
///       <see cref="AacWindowSequence.EightShort"/>: eight contiguous
///       128-line physical windows at
///       <c>spectrum[w * 128 .. (w + 1) * 128]</c> for w = 0..7. Any
///       window grouping from <see cref="AacIcsInfo.WindowsPerGroup"/>
///       must already have been undone (deinterleaved) by the caller.
///     </description>
///   </item>
/// </list>
/// <para>
/// The applier inverse-quantises the full transmitted
/// <see cref="AacTnsFilter.Coefficients"/> array but only feeds the
/// first <c>min(filter.Order, tnsMaxOrder)</c> PARCORs into the
/// Levinson step-up. Higher-order transmitted coefficients are
/// silently dropped, matching libfaad's
/// <c>min(order, tns_max_order)</c> clamp.
/// </para>
/// </remarks>
public static class AacTnsSpectrumApplier
{
    /// <summary>
    /// Apply every TNS filter declared in <paramref name="tnsData"/> to
    /// <paramref name="spectrum"/> in place.
    /// </summary>
    /// <param name="tnsData">
    /// Parsed TNS element. Its <see cref="AacTnsData.WindowSequence"/>
    /// must match <paramref name="icsInfo"/>.
    /// </param>
    /// <param name="icsInfo">
    /// Active <c>ics_info()</c>; supplies the window sequence and the
    /// <see cref="AacIcsInfo.MaxSfb"/> used for the <c>start</c> clamp.
    /// </param>
    /// <param name="spectrum">
    /// 1024-line MDCT spectrum in per-window layout (see remarks).
    /// Mutated in place.
    /// </param>
    /// <param name="swbOffsets">
    /// SWB offset table for the window length implied by
    /// <see cref="AacIcsInfo.WindowSequence"/>: the long table (closing
    /// at 1024) for long sequences or the short table (closing at 128)
    /// for <see cref="AacWindowSequence.EightShort"/>.
    /// </param>
    /// <param name="tnsMaxSfb">
    /// Maximum SFB at which TNS is allowed for the active window
    /// sequence + sample rate (spec Table 4.A.10). Must satisfy
    /// <c>0 ≤ tnsMaxSfb ≤ swbOffsets.Length − 1</c>.
    /// </param>
    /// <param name="tnsMaxOrder">
    /// Maximum filter order allowed by the spec for the active window
    /// sequence + sample rate + object type. Must satisfy
    /// <c>0 ≤ tnsMaxOrder ≤ <see cref="AacTnsLpcStepUp.MaxOrder"/></c>.
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="tnsData"/> or <paramref name="icsInfo"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// Any of the structural validations fail (see remarks).
    /// </exception>
    public static void Apply(
        AacTnsData tnsData,
        AacIcsInfo icsInfo,
        Span<float> spectrum,
        ReadOnlySpan<int> swbOffsets,
        int tnsMaxSfb,
        int tnsMaxOrder)
    {
        ArgumentNullException.ThrowIfNull(tnsData);
        ArgumentNullException.ThrowIfNull(icsInfo);

        if (tnsData.WindowSequence != icsInfo.WindowSequence)
        {
            throw new ArgumentException(
                $"tnsData.WindowSequence ({tnsData.WindowSequence}) does not match " +
                $"icsInfo.WindowSequence ({icsInfo.WindowSequence}).",
                nameof(tnsData));
        }

        bool isShort = icsInfo.WindowSequence == AacWindowSequence.EightShort;
        int expectedWindows = isShort ? AacTnsData.ShortWindowCount : 1;
        int windowLength = isShort
            ? AacSwbOffsets.ShortTransformLength
            : AacSwbOffsets.LongTransformLength;
        const int TotalLength = AacSwbOffsets.LongTransformLength;

        if (tnsData.Windows.Length != expectedWindows)
        {
            throw new ArgumentException(
                $"tnsData.Windows.Length ({tnsData.Windows.Length}) must equal " +
                $"{expectedWindows} for window sequence {icsInfo.WindowSequence}.",
                nameof(tnsData));
        }

        if (spectrum.Length != TotalLength)
        {
            throw new ArgumentException(
                $"spectrum length must be {TotalLength}, was {spectrum.Length}.",
                nameof(spectrum));
        }

        if (swbOffsets.Length < 2)
        {
            throw new ArgumentException(
                "swbOffsets must contain at least 2 entries (one SWB + closing offset).",
                nameof(swbOffsets));
        }

        if (swbOffsets[^1] != windowLength)
        {
            throw new ArgumentException(
                $"swbOffsets[^1] ({swbOffsets[^1]}) must equal the per-window transform " +
                $"length ({windowLength}) for {icsInfo.WindowSequence}.",
                nameof(swbOffsets));
        }

        int numSwb = swbOffsets.Length - 1;

        if (icsInfo.MaxSfb < 0 || icsInfo.MaxSfb > numSwb)
        {
            throw new ArgumentException(
                $"icsInfo.MaxSfb ({icsInfo.MaxSfb}) must lie in [0, {numSwb}].",
                nameof(icsInfo));
        }

        if (tnsMaxSfb < 0 || tnsMaxSfb > numSwb)
        {
            throw new ArgumentException(
                $"tnsMaxSfb ({tnsMaxSfb}) must lie in [0, {numSwb}].",
                nameof(tnsMaxSfb));
        }

        if (tnsMaxOrder < 0 || tnsMaxOrder > AacTnsLpcStepUp.MaxOrder)
        {
            throw new ArgumentException(
                $"tnsMaxOrder ({tnsMaxOrder}) must lie in " +
                $"[0, {AacTnsLpcStepUp.MaxOrder}].",
                nameof(tnsMaxOrder));
        }

        // Hot path. Stack-allocated work buffers sized once at the top
        // (worst case across all filters in the frame), reused between
        // filters via slicing.
        Span<float> parcorWork = stackalloc float[AacTnsLpcStepUp.MaxOrder];
        Span<float> lpcWork = stackalloc float[AacTnsLpcStepUp.MaxOrder];

        for (int w = 0; w < tnsData.Windows.Length; w++)
        {
            var window = tnsData.Windows[w];
            var filters = window.Filters;
            if (filters.IsDefaultOrEmpty) continue;

            Span<float> windowSpectrum = isShort
                ? spectrum.Slice(w * windowLength, windowLength)
                : spectrum;

            int bottom = numSwb;

            for (int f = 0; f < filters.Length; f++)
            {
                var filter = filters[f];

                if (filter.Length < 0)
                {
                    throw new ArgumentException(
                        $"window {w} filter {f} has negative length ({filter.Length}).",
                        nameof(tnsData));
                }
                if (filter.Order < 0)
                {
                    throw new ArgumentException(
                        $"window {w} filter {f} has negative order ({filter.Order}).",
                        nameof(tnsData));
                }

                int top = bottom;
                bottom = Math.Max(top - filter.Length, 0);

                int effectiveOrder = Math.Min(filter.Order, tnsMaxOrder);
                if (effectiveOrder == 0) continue;

                if (filter.Coefficients.Length != filter.Order)
                {
                    throw new ArgumentException(
                        $"window {w} filter {f} declares order {filter.Order} but carries " +
                        $"{filter.Coefficients.Length} coefficient(s).",
                        nameof(tnsData));
                }

                // Inverse-quantise the full transmitted coefficient set
                // (the raw width validation lives inside AacTnsInverseQuant).
                Span<float> parcorFull = parcorWork[..filter.Order];
                AacTnsInverseQuant.Compute(filter, window.CoefResHigh, parcorFull);

                // Step-up uses only the effective-order prefix; higher
                // coefficients are silently dropped per libfaad's clamp.
                Span<float> parcor = parcorFull[..effectiveOrder];
                Span<float> lpc = lpcWork[..effectiveOrder];
                AacTnsLpcStepUp.Compute(parcor, lpc);

                // Asymmetric clamp per §4.6.9.2 / libfaad: start uses
                // ics max_sfb (above it data is zero), end uses
                // tns_max_sfb (above it TNS is disallowed).
                int start = swbOffsets[Math.Min(bottom, icsInfo.MaxSfb)];
                int end = swbOffsets[Math.Min(top, tnsMaxSfb)];
                if (end <= start) continue;

                AacTnsInverseFilter.Apply(
                    windowSpectrum.Slice(start, end - start),
                    lpc,
                    filter.Direction);
            }
        }
    }
}
