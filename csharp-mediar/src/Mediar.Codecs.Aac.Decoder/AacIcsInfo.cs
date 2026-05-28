namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of the AAC <c>ics_info()</c> header that opens each
/// individual_channel_stream() body (ISO/IEC 14496-3 §4.4.2.3,
/// Table 4.74). Captures the window sequence + shape and, for short
/// sequences, the derived window-group structure used by
/// <c>section_data()</c>, <c>scale_factor_data()</c> and
/// <c>spectral_data()</c>.
/// </summary>
/// <remarks>
/// <para>
/// This phase-1 parser supports <i>only</i>
/// <c>predictor_data_present = 0</c>. Both main-profile prediction
/// (audio_object_type == 1) and LC-LTP (audio_object_type == 19) need
/// a separate sub-parser plus a maintained-state predictor that is
/// not yet implemented; encountering <c>predictor_data_present = 1</c>
/// therefore returns <see langword="false"/>.
/// </para>
/// <para>
/// <see cref="WindowsPerGroup"/> is derived from the 7-bit
/// <see cref="ScaleFactorGrouping"/> field for EIGHT_SHORT sequences,
/// where bit 6 (MSB) of <c>scale_factor_grouping</c> describes whether
/// window 1 stays in the same group as window 0, bit 5 describes
/// window 2 vs 1, and so on down to bit 0 for window 7 vs 6. A
/// <c>1</c> bit means "continue the current group", a <c>0</c> bit
/// starts a new group. For long sequences <see cref="WindowsPerGroup"/>
/// is always a single entry <c>[1]</c>.
/// </para>
/// </remarks>
public sealed record AacIcsInfo
{
    /// <summary>Window-sequence selector (2 bits).</summary>
    public required AacWindowSequence WindowSequence { get; init; }

    /// <summary>Window-shape selector (1 bit).</summary>
    public required AacWindowShape WindowShape { get; init; }

    /// <summary>
    /// <c>max_sfb</c>: index of the last scale-factor band that
    /// carries data. 4 bits wide for EIGHT_SHORT, 6 bits otherwise.
    /// </summary>
    public required int MaxSfb { get; init; }

    /// <summary>
    /// Raw 7-bit <c>scale_factor_grouping</c> field. Present only for
    /// EIGHT_SHORT sequences; <see langword="null"/> for long sequences.
    /// </summary>
    public byte? ScaleFactorGrouping { get; init; }

    /// <summary>
    /// Number of window groups in this audio element (1..8). Always
    /// <c>1</c> for long sequences; <c>1..8</c> for EIGHT_SHORT.
    /// </summary>
    public required int WindowGroupCount { get; init; }

    /// <summary>
    /// Number of windows in each group, in stream order. Sums to
    /// <c>1</c> for long sequences and to <c>8</c> for EIGHT_SHORT.
    /// </summary>
    public required ReadOnlyMemory<byte> WindowsPerGroup { get; init; }

    /// <summary>
    /// <see langword="true"/> when the encoder signalled
    /// <c>predictor_data_present</c>. Phase-1 only allows
    /// <see langword="false"/>; <see cref="TryParse"/> rejects the
    /// stream otherwise.
    /// </summary>
    public bool PredictorDataPresent { get; init; }

    /// <summary>
    /// Parse an <c>ics_info()</c> structure from the current
    /// <paramref name="reader"/> position. Returns <see langword="false"/>
    /// when the stream underflows or when an unsupported
    /// <c>predictor_data_present = 1</c> is encountered.
    /// </summary>
    /// <param name="reader">Active MSB-first bit reader; advanced past <c>ics_info()</c> on success.</param>
    /// <param name="info">Populated on success; otherwise <see langword="null"/>.</param>
    internal static bool TryParse(scoped ref BitReader reader, out AacIcsInfo? info)
    {
        info = null;
        // ics_info() = ics_reserved_bit(1) + window_sequence(2) + window_shape(1)
        //            + max_sfb(4 or 6) + [scale_factor_grouping(7) | predictor_data_present(1)]
        // Worst case: 1 + 2 + 1 + 6 + 1 = 11 bits when predictor_data_present is the trailing bit.
        if (reader.Remaining < 11) return false;

        try
        {
            _ = reader.ReadBits(1); // ics_reserved_bit - per spec must be 0; common decoders ignore.
            var windowSequence = (AacWindowSequence)reader.ReadBits(2);
            var windowShape = (AacWindowShape)reader.ReadBits(1);

            int maxSfb;
            byte? sfg = null;
            int groupCount;
            byte[] windowsPerGroup;
            bool predictor = false;

            if (windowSequence == AacWindowSequence.EightShort)
            {
                // 4 bits max_sfb + 7 bits scale_factor_grouping = 11 bits.
                maxSfb = (int)reader.ReadBits(4);
                if (maxSfb > 15) return false; // 4-bit field caps at 15 already.
                byte grouping = (byte)reader.ReadBits(7);
                sfg = grouping;
                windowsPerGroup = DeriveShortWindowGroups(grouping, out groupCount);
            }
            else
            {
                // 6 bits max_sfb + 1 bit predictor_data_present = 7 bits.
                maxSfb = (int)reader.ReadBits(6);
                if (maxSfb > 63) return false;
                bool pred = reader.ReadBit();
                if (pred) return false; // Phase-1 rejects main prediction / LTP.
                predictor = false;
                groupCount = 1;
                windowsPerGroup = SingleLongGroup;
            }

            info = new AacIcsInfo
            {
                WindowSequence = windowSequence,
                WindowShape = windowShape,
                MaxSfb = maxSfb,
                ScaleFactorGrouping = sfg,
                WindowGroupCount = groupCount,
                WindowsPerGroup = windowsPerGroup,
                PredictorDataPresent = predictor,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            info = null;
            return false;
        }
    }

    private static readonly byte[] SingleLongGroup = new byte[] { 1 };

    private static byte[] DeriveShortWindowGroups(byte scaleFactorGrouping, out int groupCount)
    {
        // 8 windows, scale_factor_grouping has 7 bits where bit 6 (MSB) controls
        // grouping between window 0 and window 1, bit 5 between 1 and 2, ..., bit 0
        // between window 6 and window 7. A '1' bit means "stay in the current group";
        // a '0' bit starts a new group.
        Span<byte> sizes = stackalloc byte[8];
        int gc = 1;
        sizes[0] = 1;
        for (int w = 1; w < 8; w++)
        {
            int bit = (scaleFactorGrouping >> (7 - w)) & 1;
            if (bit == 1)
            {
                sizes[gc - 1]++;
            }
            else
            {
                sizes[gc++] = 1;
            }
        }

        var result = new byte[gc];
        for (int i = 0; i < gc; i++) result[i] = sizes[i];
        groupCount = gc;
        return result;
    }
}
