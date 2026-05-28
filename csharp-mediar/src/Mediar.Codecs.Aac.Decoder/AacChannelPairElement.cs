#pragma warning disable CA1711 // The type name mirrors the ISO/IEC 14496-3 syntactic element channel_pair_element().

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC <c>ms_mask_present</c> field values per ISO/IEC 14496-3 Table 4.6.
/// Selects how M/S stereo coding is signalled for a CPE.
/// </summary>
public enum AacMsMaskPresent
{
    /// <summary>No M/S stereo coding is used in this frame.</summary>
    None = 0,

    /// <summary>
    /// M/S coding is used selectively; a per-band <c>ms_used[g][sfb]</c>
    /// bit array follows the field to identify which bands carry M/S
    /// rather than independent L/R coefficients.
    /// </summary>
    PerBand = 1,

    /// <summary>
    /// M/S coding is applied to every scale-factor band (no per-band
    /// flag array is transmitted).
    /// </summary>
    AllBands = 2,

    /// <summary>Reserved by the AAC specification; rejected on parse.</summary>
    Reserved = 3,
}

/// <summary>
/// Parsed view of an AAC <c>channel_pair_element()</c> (CPE) per
/// ISO/IEC 14496-3 §4.4.2.1 Table 4.5. Composes the 4-bit
/// <c>element_instance_tag</c> prefix plus the <c>common_window</c>
/// machinery plus two <see cref="AacIndividualChannelStream"/> bodies.
/// The bodies parse through the optional pulse_data, tns_data and
/// gain_control_data_present flag — the <c>spectral_data()</c> body
/// itself remains deferred (blocked on the swb_offset tables from
/// Annex 4.A).
/// </summary>
/// <remarks>
/// CPE is the stereo / dual-mono audio element type. When
/// <c>common_window</c> is 1, both individual channel streams share a
/// single <c>ics_info()</c> aggregation; when 0, each parses its own.
/// </remarks>
public sealed record AacChannelPairElement
{
    /// <summary>Maximum value of <c>element_instance_tag</c> (4-bit field).</summary>
    public const int MaxElementInstanceTag = 15;

    /// <summary>4-bit <c>element_instance_tag</c> identifying this CPE within the raw_data_block.</summary>
    public required int ElementInstanceTag { get; init; }

    /// <summary>
    /// <c>common_window</c> flag: true means both channels share the
    /// <see cref="SharedIcsInfo"/> aggregator; false means each channel
    /// stream carries its own ics_info.
    /// </summary>
    public required bool CommonWindow { get; init; }

    /// <summary>
    /// Shared <c>ics_info()</c> applied to both channel streams when
    /// <see cref="CommonWindow"/> is true; <see langword="null"/>
    /// otherwise.
    /// </summary>
    public required AacIcsInfo? SharedIcsInfo { get; init; }

    /// <summary>
    /// <c>ms_mask_present</c> field; meaningful only when
    /// <see cref="CommonWindow"/> is true. Defaults to
    /// <see cref="AacMsMaskPresent.None"/> when there is no
    /// common window.
    /// </summary>
    public required AacMsMaskPresent MsMaskPresent { get; init; }

    /// <summary>
    /// Per-band <c>ms_used[g][sfb]</c> flag array when
    /// <see cref="MsMaskPresent"/> is <see cref="AacMsMaskPresent.PerBand"/>;
    /// empty otherwise. Outer dimension is window-group index, inner is
    /// scale-factor-band index (max_sfb entries).
    /// </summary>
    public required IReadOnlyList<IReadOnlyList<bool>> MsUsed { get; init; }

    /// <summary>Parsed first <c>individual_channel_stream()</c> body.</summary>
    public required AacIndividualChannelStream FirstStream { get; init; }

    /// <summary>Parsed second <c>individual_channel_stream()</c> body.</summary>
    public required AacIndividualChannelStream SecondStream { get; init; }

    /// <summary>
    /// Total bits consumed by the prefix + common_window block + both
    /// stream bodies. The bits that follow inside the parent
    /// raw_data_block belong to <c>spectral_data()</c> and are not
    /// consumed here.
    /// </summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Read a <c>channel_pair_element()</c> from <paramref name="reader"/>
    /// positioned at <c>element_instance_tag</c>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at element_instance_tag.</param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="element">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns><see langword="true"/> when the prefix, common-window block and both bodies parsed cleanly.</returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacChannelPairElement? element)
    {
        element = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);

        int startBits = reader.Position;
        if (reader.Remaining < 4 + 1) return false;
        int elementInstanceTag = (int)reader.ReadBits(4);
        bool commonWindow = reader.ReadBits(1) == 1;

        AacIcsInfo? sharedIcsInfo = null;
        var msMaskPresent = AacMsMaskPresent.None;
        IReadOnlyList<IReadOnlyList<bool>> msUsed = Array.Empty<IReadOnlyList<bool>>();

        if (commonWindow)
        {
            if (!AacIcsInfo.TryParse(ref reader, out var info) || info is null)
            {
                return false;
            }
            sharedIcsInfo = info;

            if (reader.Remaining < 2) return false;
            int rawMs = (int)reader.ReadBits(2);
            msMaskPresent = (AacMsMaskPresent)rawMs;
            if (msMaskPresent == AacMsMaskPresent.Reserved) return false;

            if (msMaskPresent == AacMsMaskPresent.PerBand)
            {
                int maxSfb = info.MaxSfb;
                int groups = info.WindowGroupCount;
                int totalBits = maxSfb * groups;
                if (reader.Remaining < totalBits) return false;

                var msUsedFlat = new bool[groups][];
                for (int g = 0; g < groups; g++)
                {
                    var groupFlags = new bool[maxSfb];
                    for (int sfb = 0; sfb < maxSfb; sfb++)
                    {
                        groupFlags[sfb] = reader.ReadBits(1) == 1;
                    }
                    msUsedFlat[g] = groupFlags;
                }
                msUsed = msUsedFlat;
            }
        }

        if (!AacIndividualChannelStream.TryRead(
                ref reader,
                sharedIcsInfo,
                scaleFlag: false,
                scaleFactorCodebook,
                out var first)
            || first is null)
        {
            return false;
        }

        if (!AacIndividualChannelStream.TryRead(
                ref reader,
                sharedIcsInfo,
                scaleFlag: false,
                scaleFactorCodebook,
                out var second)
            || second is null)
        {
            return false;
        }

        element = new AacChannelPairElement
        {
            ElementInstanceTag = elementInstanceTag,
            CommonWindow = commonWindow,
            SharedIcsInfo = sharedIcsInfo,
            MsMaskPresent = msMaskPresent,
            MsUsed = msUsed,
            FirstStream = first,
            SecondStream = second,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>channel_pair_element()</c> body from
    /// <paramref name="bytes"/> starting at the first bit.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacChannelPairElement? element)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, scaleFactorCodebook, out element);
    }
}
