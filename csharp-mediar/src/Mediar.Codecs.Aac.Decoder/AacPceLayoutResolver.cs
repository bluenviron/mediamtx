namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Speaker region for a PCE-described AAC channel layout. Maps to
/// the four positional regions in
/// <see cref="AacProgramConfigurationElement"/> plus the auxiliary
/// coupling-channel region.
/// </summary>
public enum AacPceChannelRegion
{
    /// <summary>Front speaker region (FrontElements).</summary>
    Front,

    /// <summary>Side speaker region (SideElements).</summary>
    Side,

    /// <summary>Back / rear speaker region (BackElements).</summary>
    Back,

    /// <summary>LFE region (LfeElements).</summary>
    Lfe,

    /// <summary>Auxiliary coupling-channel region (CouplingElements).</summary>
    Coupling,
}

/// <summary>
/// One resolved PCE-described channel slot: pairs a parsed
/// raw_data_block audio element with the PCE positional metadata
/// (region + index within the region) that selected it. Unlike
/// the channelConfiguration 1..7 resolver this DOES NOT carry a
/// concrete speaker label, because PCE-described layouts have no
/// universally-agreed canonical speaker mapping: front / side /
/// back slot indices are positional hints whose interpretation is
/// implementation-defined.
/// </summary>
public sealed record AacPceResolvedEntry
{
    /// <summary>The raw_data_block entry this slot resolves to.</summary>
    public required AacRawDataBlockEntry RawEntry { get; init; }

    /// <summary>Which positional region of the PCE this slot belongs to.</summary>
    public required AacPceChannelRegion Region { get; init; }

    /// <summary>
    /// Zero-based index of this slot WITHIN <see cref="Region"/>,
    /// matching the position in
    /// <see cref="AacProgramConfigurationElement.FrontElements"/>
    /// (etc.) that selected it.
    /// </summary>
    public required int RegionIndex { get; init; }
}

/// <summary>
/// Resolves PCE-described AAC channel layouts: walks the four
/// positional regions of a parsed
/// <see cref="AacProgramConfigurationElement"/> and matches each
/// slot to the corresponding audio element in a parsed
/// <see cref="AacRawDataBlock"/> by element_instance_tag.
/// </summary>
/// <remarks>
/// <para>
/// PCE-described layouts (channelConfiguration == 0) cannot use
/// the positional pairing performed by
/// <see cref="AacChannelLayoutResolver"/> because the PCE
/// specifies element kinds (SCE vs CPE) AND specific
/// element_instance_tag values rather than relying on the
/// raw_data_block element order.
/// </para>
/// <para>
/// The output order is: front slots, then side slots, then back
/// slots, then LFE slots, then coupling slots. Within each region
/// the order matches the PCE's region list order.
/// </para>
/// <para>
/// Associated-data and mixdown element tags are NOT surfaced; they
/// are handled at the data-stream / mixdown layer, not by the
/// channel routing.
/// </para>
/// </remarks>
public static class AacPceLayoutResolver
{
    /// <summary>
    /// Resolve <paramref name="pce"/>'s positional slots against
    /// the audio elements in <paramref name="block"/>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="block"/> or <paramref name="pce"/> is null.
    /// </exception>
    /// <exception cref="InvalidOperationException">
    /// A PCE slot references an element_instance_tag that is not
    /// present in <paramref name="block"/>, or the matched
    /// raw_data_block entry kind does not agree with the PCE
    /// slot's expected kind (SCE / CPE / LFE / CCE).
    /// </exception>
    public static IReadOnlyList<AacPceResolvedEntry> Resolve(
        AacRawDataBlock block,
        AacProgramConfigurationElement pce)
    {
        ArgumentNullException.ThrowIfNull(block);
        ArgumentNullException.ThrowIfNull(pce);

        var resolved = new List<AacPceResolvedEntry>();

        ResolveChannelRegion(block, pce.FrontElements, AacPceChannelRegion.Front, resolved);
        ResolveChannelRegion(block, pce.SideElements, AacPceChannelRegion.Side, resolved);
        ResolveChannelRegion(block, pce.BackElements, AacPceChannelRegion.Back, resolved);
        ResolveLfeRegion(block, pce.LfeElements, resolved);
        ResolveCouplingRegion(block, pce.CouplingElements, resolved);

        return resolved;
    }

    private static void ResolveChannelRegion(
        AacRawDataBlock block,
        IReadOnlyList<AacPceChannelSlot> slots,
        AacPceChannelRegion region,
        List<AacPceResolvedEntry> output)
    {
        for (int i = 0; i < slots.Count; i++)
        {
            var slot = slots[i];
            var expectedKind = slot.IsCpe
                ? AacSyntacticElementType.ChannelPairElement
                : AacSyntacticElementType.SingleChannelElement;
            var raw = FindAudioElementByTag(block, expectedKind, slot.TagSelect, region, i);
            output.Add(new AacPceResolvedEntry
            {
                RawEntry = raw,
                Region = region,
                RegionIndex = i,
            });
        }
    }

    private static void ResolveLfeRegion(
        AacRawDataBlock block,
        IReadOnlyList<int> tags,
        List<AacPceResolvedEntry> output)
    {
        for (int i = 0; i < tags.Count; i++)
        {
            var raw = FindAudioElementByTag(
                block, AacSyntacticElementType.LfeChannelElement, tags[i],
                AacPceChannelRegion.Lfe, i);
            output.Add(new AacPceResolvedEntry
            {
                RawEntry = raw,
                Region = AacPceChannelRegion.Lfe,
                RegionIndex = i,
            });
        }
    }

    private static void ResolveCouplingRegion(
        AacRawDataBlock block,
        IReadOnlyList<AacPceCouplingSlot> slots,
        List<AacPceResolvedEntry> output)
    {
        for (int i = 0; i < slots.Count; i++)
        {
            var raw = FindAudioElementByTag(
                block, AacSyntacticElementType.CouplingChannelElement, slots[i].TagSelect,
                AacPceChannelRegion.Coupling, i);
            output.Add(new AacPceResolvedEntry
            {
                RawEntry = raw,
                Region = AacPceChannelRegion.Coupling,
                RegionIndex = i,
            });
        }
    }

    private static AacRawDataBlockEntry FindAudioElementByTag(
        AacRawDataBlock block,
        AacSyntacticElementType expectedKind,
        int tag,
        AacPceChannelRegion region,
        int regionIndex)
    {
        foreach (var entry in block.Entries)
        {
            if (entry.Type != expectedKind) continue;
            int? entryTag = entry.Type switch
            {
                AacSyntacticElementType.SingleChannelElement => entry.SingleChannel?.ElementInstanceTag,
                AacSyntacticElementType.ChannelPairElement => entry.ChannelPair?.ElementInstanceTag,
                AacSyntacticElementType.LfeChannelElement => entry.LowFrequency?.ElementInstanceTag,
                AacSyntacticElementType.CouplingChannelElement => entry.CouplingChannel?.ElementInstanceTag,
                _ => null,
            };
            if (entryTag == tag)
            {
                return entry;
            }
        }

        throw new InvalidOperationException(
            $"PCE {region} slot #{regionIndex} expects a {expectedKind} with " +
            $"element_instance_tag={tag} but no such element was found in the " +
            $"raw_data_block. Either the block was parsed via the boundary " +
            $"overload (typed payloads not populated) or the stream layout " +
            $"does not match its PCE.");
    }
}
