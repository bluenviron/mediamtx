namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One paired entry in a resolved AAC channel layout: a parsed
/// audio element from <see cref="AacRawDataBlock"/> bound to the
/// corresponding speaker mapping from
/// <see cref="AacChannelMapping"/>. Coupling Channel Elements are
/// surfaced with <see cref="Mapping"/> = <see langword="null"/>
/// because they are auxiliary inputs to a downstream coupling
/// gain, not standalone speaker outputs.
/// </summary>
public sealed record AacResolvedChannelEntry
{
    /// <summary>The raw_data_block entry this pairing wraps.</summary>
    public required AacRawDataBlockEntry RawEntry { get; init; }

    /// <summary>
    /// The speaker mapping for this audio element, or
    /// <see langword="null"/> for auxiliary CCEs.
    /// </summary>
    public AacChannelMappingEntry? Mapping { get; init; }
}

/// <summary>
/// Resolves the ordered audio elements inside a parsed
/// <see cref="AacRawDataBlock"/> against the speaker layout
/// dictated by a given <c>channelConfiguration</c>. Filters out
/// codec-state-free elements (PCE / DSE / FIL / END) so callers
/// can drive the per-element decoders directly against the
/// returned list.
/// </summary>
/// <remarks>
/// <para>
/// The expected ordering follows
/// <see cref="AacChannelMapping.GetForConfiguration"/>:
/// non-CCE audio elements (SCE / CPE / LFE) in the
/// raw_data_block must appear in the exact element-type sequence
/// dictated by the mapping. CCEs may be interleaved at any point
/// and are surfaced as auxiliary entries
/// (<see cref="AacResolvedChannelEntry.Mapping"/> = <see langword="null"/>);
/// they do not contribute to the speaker-count balance check.
/// </para>
/// <para>
/// <c>channelConfiguration == 0</c> is the PCE-described layout
/// case: this resolver throws because the caller must instead
/// build the layout from the explicit
/// <see cref="AacProgramConfigurationElement"/> in the stream.
/// A separate PCE-based resolver is a future ship.
/// </para>
/// </remarks>
public static class AacChannelLayoutResolver
{
    /// <summary>
    /// Resolve <paramref name="block"/>'s audio elements against
    /// the channel mapping for
    /// <paramref name="channelConfiguration"/>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="block"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="channelConfiguration"/> is outside [1, 7].
    /// (Configuration 0 uses an explicit PCE; configurations &gt;
    /// 7 are reserved.)
    /// </exception>
    /// <exception cref="InvalidOperationException">
    /// The audio element sequence in <paramref name="block"/>
    /// does not match the expected mapping element-type sequence,
    /// either in count or in element kind.
    /// </exception>
    public static IReadOnlyList<AacResolvedChannelEntry> Resolve(
        AacRawDataBlock block,
        int channelConfiguration)
    {
        ArgumentNullException.ThrowIfNull(block);

        if (channelConfiguration is < 1 or > 7)
        {
            throw new ArgumentOutOfRangeException(
                nameof(channelConfiguration),
                channelConfiguration,
                "channelConfiguration must be in [1, 7]. " +
                "Configuration 0 requires explicit-PCE resolution; values > 7 are reserved.");
        }

        var mapping = AacChannelMapping.GetForConfiguration(channelConfiguration);
        var resolved = new List<AacResolvedChannelEntry>();
        int mappingIdx = 0;

        foreach (var entry in block.Entries)
        {
            switch (entry.Type)
            {
                case AacSyntacticElementType.SingleChannelElement:
                case AacSyntacticElementType.ChannelPairElement:
                case AacSyntacticElementType.LfeChannelElement:
                    if (mappingIdx >= mapping.Count)
                    {
                        throw new InvalidOperationException(
                            $"raw_data_block contains more audio elements than channel " +
                            $"configuration {channelConfiguration} expects (mapping has " +
                            $"{mapping.Count} slots).");
                    }
                    var slot = mapping[mappingIdx];
                    if (slot.Element != entry.Type)
                    {
                        throw new InvalidOperationException(
                            $"raw_data_block audio element #{mappingIdx} is {entry.Type} but " +
                            $"channel configuration {channelConfiguration} expects {slot.Element} " +
                            $"at this position.");
                    }
                    resolved.Add(new AacResolvedChannelEntry
                    {
                        RawEntry = entry,
                        Mapping = slot,
                    });
                    mappingIdx++;
                    break;

                case AacSyntacticElementType.CouplingChannelElement:
                    resolved.Add(new AacResolvedChannelEntry
                    {
                        RawEntry = entry,
                        Mapping = null,
                    });
                    break;

                case AacSyntacticElementType.DataStreamElement:
                case AacSyntacticElementType.ProgramConfigElement:
                case AacSyntacticElementType.FillElement:
                case AacSyntacticElementType.End:
                    // Codec-state-free; not part of the speaker routing.
                    break;

                default:
                    throw new InvalidOperationException(
                        $"Unknown syntactic element type: {entry.Type}.");
            }
        }

        if (mappingIdx != mapping.Count)
        {
            throw new InvalidOperationException(
                $"raw_data_block has only {mappingIdx} audio elements but channel " +
                $"configuration {channelConfiguration} expects {mapping.Count}.");
        }

        return resolved;
    }

    /// <summary>
    /// Convenience filter: returns just the speaker-bound (non-CCE)
    /// resolved entries from a <see cref="Resolve"/> result.
    /// </summary>
    public static IReadOnlyList<AacResolvedChannelEntry> FilterSpeakerEntries(
        IReadOnlyList<AacResolvedChannelEntry> resolved)
    {
        ArgumentNullException.ThrowIfNull(resolved);
        var list = new List<AacResolvedChannelEntry>(resolved.Count);
        foreach (var entry in resolved)
        {
            if (entry.Mapping is not null) list.Add(entry);
        }
        return list;
    }

    /// <summary>
    /// Convenience filter: returns just the auxiliary CCE
    /// resolved entries from a <see cref="Resolve"/> result.
    /// </summary>
    public static IReadOnlyList<AacResolvedChannelEntry> FilterCouplingEntries(
        IReadOnlyList<AacResolvedChannelEntry> resolved)
    {
        ArgumentNullException.ThrowIfNull(resolved);
        var list = new List<AacResolvedChannelEntry>();
        foreach (var entry in resolved)
        {
            if (entry.Mapping is null
                && entry.RawEntry.Type == AacSyntacticElementType.CouplingChannelElement)
            {
                list.Add(entry);
            }
        }
        return list;
    }
}
