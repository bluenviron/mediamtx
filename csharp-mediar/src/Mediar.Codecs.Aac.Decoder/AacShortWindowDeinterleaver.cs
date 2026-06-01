namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Converts an AAC EIGHT_SHORT-window dequantised spectrum between
/// the two storage layouts used inside the decoder:
/// <list type="bullet">
///   <item><description>
///     <b>Group-major / SFB-window-interleaved</b> (the layout
///     produced by <see cref="AacDequantizedSpectrum"/>): groups
///     are concatenated; within a group of <c>N</c> windows each
///     scale-factor band's <c>N</c> windows are stored
///     contiguously, SFBs in spec order. This is the layout the
///     dequantiser and scale-factor application use because a
///     single scale factor applies to all <c>N</c> windows of a
///     band.
///   </description></item>
///   <item><description>
///     <b>Window-major</b>: eight contiguous 128-coefficient
///     blocks, window 0 through window 7. This is the layout
///     <see cref="AacTnsSpectrumApplier"/> and the synthesis
///     filterbank (<see cref="AacSynthesisFilterbank.ProcessEightShortBlock"/>)
///     consume because TNS and IMDCT act per-window.
///   </description></item>
/// </list>
/// </summary>
/// <remarks>
/// <para>
/// Both directions only touch bins that fall inside <c>[0,
/// shortSwbOffsets[MaxSfb])</c>. Bins beyond <c>MaxSfb</c> in the
/// per-window destination are not written by
/// <see cref="ToWindowMajor"/> nor read by
/// <see cref="ToGroupMajor"/>; callers must initialise / consume
/// those regions themselves if they matter.
/// </para>
/// <para>
/// Both methods are pure functions over their inputs (no
/// allocations on the hot path) and are safe to call repeatedly
/// during steady-state decoding.
/// </para>
/// </remarks>
public static class AacShortWindowDeinterleaver
{
    /// <summary>
    /// Total number of windows in an EIGHT_SHORT frame
    /// (the sum of <see cref="AacIcsInfo.WindowsPerGroup"/> entries
    /// is always 8).
    /// </summary>
    public const int WindowCount = 8;

    /// <summary>Per-window transform length (128 samples).</summary>
    public const int WindowLength = AacSwbOffsets.ShortTransformLength;

    /// <summary>
    /// Total spectrum length for a short-window frame
    /// (<see cref="WindowCount"/> * <see cref="WindowLength"/>
    /// = 1024).
    /// </summary>
    public const int TotalLength = WindowCount * WindowLength;

    /// <summary>
    /// Project the group-major / SFB-window-interleaved layout
    /// in <paramref name="grouped"/> into the window-major layout
    /// in <paramref name="windowMajor"/>.
    /// </summary>
    /// <param name="grouped">Source layout (length 1024).</param>
    /// <param name="ics">
    /// ICS info whose <see cref="AacIcsInfo.WindowSequence"/> must
    /// be <see cref="AacWindowSequence.EightShort"/>.
    /// </param>
    /// <param name="shortSwbOffsets">
    /// SWB offset table for the relevant sample rate, ending at
    /// 128. Length must be at least <c>ics.MaxSfb + 1</c>.
    /// </param>
    /// <param name="windowMajor">Destination (length 1024).</param>
    public static void ToWindowMajor(
        ReadOnlySpan<float> grouped,
        AacIcsInfo ics,
        ReadOnlySpan<int> shortSwbOffsets,
        Span<float> windowMajor)
    {
        Validate(ics, shortSwbOffsets, grouped.Length, windowMajor.Length,
            nameOfGrouped: nameof(grouped), nameOfWindowMajor: nameof(windowMajor));

        ReadOnlySpan<byte> windowsPerGroup = ics.WindowsPerGroup.Span;
        int absoluteWindow = 0;
        int groupBase = 0;
        for (int g = 0; g < ics.WindowGroupCount; g++)
        {
            int windowsInGroup = windowsPerGroup[g];
            for (int wInGroup = 0; wInGroup < windowsInGroup; wInGroup++, absoluteWindow++)
            {
                int windowDestBase = absoluteWindow * WindowLength;
                for (int sfb = 0; sfb < ics.MaxSfb; sfb++)
                {
                    int bandStart = shortSwbOffsets[sfb];
                    int bandWidth = shortSwbOffsets[sfb + 1] - bandStart;
                    if (bandWidth <= 0) continue;

                    int groupedBase = groupBase + bandStart * windowsInGroup + wInGroup * bandWidth;
                    grouped.Slice(groupedBase, bandWidth).CopyTo(
                        windowMajor.Slice(windowDestBase + bandStart, bandWidth));
                }
            }
            groupBase += windowsInGroup * WindowLength;
        }
    }

    /// <summary>
    /// Project the window-major layout in
    /// <paramref name="windowMajor"/> back into the group-major /
    /// SFB-window-interleaved layout in <paramref name="grouped"/>.
    /// </summary>
    public static void ToGroupMajor(
        ReadOnlySpan<float> windowMajor,
        AacIcsInfo ics,
        ReadOnlySpan<int> shortSwbOffsets,
        Span<float> grouped)
    {
        Validate(ics, shortSwbOffsets, grouped.Length, windowMajor.Length,
            nameOfGrouped: nameof(grouped), nameOfWindowMajor: nameof(windowMajor));

        ReadOnlySpan<byte> windowsPerGroup = ics.WindowsPerGroup.Span;
        int absoluteWindow = 0;
        int groupBase = 0;
        for (int g = 0; g < ics.WindowGroupCount; g++)
        {
            int windowsInGroup = windowsPerGroup[g];
            for (int wInGroup = 0; wInGroup < windowsInGroup; wInGroup++, absoluteWindow++)
            {
                int windowSrcBase = absoluteWindow * WindowLength;
                for (int sfb = 0; sfb < ics.MaxSfb; sfb++)
                {
                    int bandStart = shortSwbOffsets[sfb];
                    int bandWidth = shortSwbOffsets[sfb + 1] - bandStart;
                    if (bandWidth <= 0) continue;

                    int groupedBase = groupBase + bandStart * windowsInGroup + wInGroup * bandWidth;
                    windowMajor.Slice(windowSrcBase + bandStart, bandWidth).CopyTo(
                        grouped.Slice(groupedBase, bandWidth));
                }
            }
            groupBase += windowsInGroup * WindowLength;
        }
    }

    private static void Validate(
        AacIcsInfo ics,
        ReadOnlySpan<int> shortSwbOffsets,
        int groupedLength,
        int windowMajorLength,
        string nameOfGrouped,
        string nameOfWindowMajor)
    {
        ArgumentNullException.ThrowIfNull(ics);

        if (ics.WindowSequence != AacWindowSequence.EightShort)
        {
            throw new ArgumentException(
                $"WindowSequence must be EightShort, was {ics.WindowSequence}.",
                nameof(ics));
        }
        if (groupedLength != TotalLength)
        {
            throw new ArgumentException(
                $"grouped length must be {TotalLength}, was {groupedLength}.",
                nameOfGrouped);
        }
        if (windowMajorLength != TotalLength)
        {
            throw new ArgumentException(
                $"windowMajor length must be {TotalLength}, was {windowMajorLength}.",
                nameOfWindowMajor);
        }
        if (shortSwbOffsets.Length < ics.MaxSfb + 1)
        {
            throw new ArgumentException(
                $"shortSwbOffsets length ({shortSwbOffsets.Length}) must be at least " +
                $"MaxSfb + 1 ({ics.MaxSfb + 1}).",
                nameof(shortSwbOffsets));
        }
        if (ics.MaxSfb > 0 && shortSwbOffsets[ics.MaxSfb] > WindowLength)
        {
            throw new ArgumentException(
                $"shortSwbOffsets[MaxSfb] ({shortSwbOffsets[ics.MaxSfb]}) exceeds the per-window " +
                $"length ({WindowLength}).",
                nameof(shortSwbOffsets));
        }

        ReadOnlySpan<byte> windowsPerGroup = ics.WindowsPerGroup.Span;
        if (windowsPerGroup.Length < ics.WindowGroupCount)
        {
            throw new ArgumentException(
                $"ics.WindowsPerGroup length ({windowsPerGroup.Length}) is shorter than " +
                $"WindowGroupCount ({ics.WindowGroupCount}).",
                nameof(ics));
        }

        int totalWindowsAcrossGroups = 0;
        for (int g = 0; g < ics.WindowGroupCount; g++)
        {
            totalWindowsAcrossGroups += windowsPerGroup[g];
        }
        if (totalWindowsAcrossGroups != WindowCount)
        {
            throw new ArgumentException(
                $"sum of WindowsPerGroup ({totalWindowsAcrossGroups}) must equal {WindowCount}.",
                nameof(ics));
        }
    }
}
