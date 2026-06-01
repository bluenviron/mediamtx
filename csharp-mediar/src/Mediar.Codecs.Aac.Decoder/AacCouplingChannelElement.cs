#pragma warning disable CA1711 // The type name mirrors the ISO/IEC 14496-3 syntactic element coupling_channel_element().

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One target referenced by an AAC <c>coupling_channel_element()</c>
/// (ISO/IEC 14496-3 §4.4.2.1 Table 4.8). Each target identifies a
/// downstream audio element that the coupling channel mixes into
/// (either an SCE/LFE by tag, or a CPE - in which case
/// <see cref="CcLeft"/> / <see cref="CcRight"/> select which of the
/// CPE's two channels receive the coupling signal).
/// </summary>
public sealed record AacCouplingTarget
{
    /// <summary><c>cc_target_is_cpe[c]</c>: target is a CPE (else SCE/LFE).</summary>
    public required bool IsChannelPairElement { get; init; }

    /// <summary><c>cc_target_tag_select[c]</c>: <c>element_instance_tag</c> of the target.</summary>
    public required int TargetTagSelect { get; init; }

    /// <summary>
    /// <c>cc_l[c]</c>: coupling signal is applied to the CPE's first
    /// (left) channel. Always <see langword="false"/> when
    /// <see cref="IsChannelPairElement"/> is <see langword="false"/>.
    /// </summary>
    public required bool CcLeft { get; init; }

    /// <summary>
    /// <c>cc_r[c]</c>: coupling signal is applied to the CPE's second
    /// (right) channel. Always <see langword="false"/> when
    /// <see cref="IsChannelPairElement"/> is <see langword="false"/>.
    /// </summary>
    public required bool CcRight { get; init; }

    /// <summary>
    /// <see langword="true"/> when this target consumes an additional
    /// gain-element-list (CPE target with both channels coupled). The
    /// CCE walker uses this when computing
    /// <c>num_gain_element_lists</c>.
    /// </summary>
    public bool ContributesExtraGainList => IsChannelPairElement && CcLeft && CcRight;
}

/// <summary>
/// One band-level differential gain inside an AAC coupling
/// gain-element list. Captures the (group, sfb) coordinate together
/// with the decoded <c>idx - 60</c> differential.
/// </summary>
public sealed record AacCouplingGainEntry
{
    /// <summary>Window-group index of the band.</summary>
    public required int Group { get; init; }

    /// <summary>Scale-factor-band index inside the group.</summary>
    public required int Sfb { get; init; }

    /// <summary>
    /// Signed differential as read from the bitstream
    /// (Huffman symbol <c>- 60</c>; range <c>[-60, +60]</c>).
    /// </summary>
    public required int Differential { get; init; }
}

/// <summary>
/// One per-target gain-element list inside an AAC coupling channel
/// element. Two flavours per ISO/IEC 14496-3 §4.6.8.3:
/// <list type="bullet">
///   <item><description>
///     <see cref="CommonGainElementPresent"/> = <see langword="true"/>:
///     a single Huffman codeword
///     (<see cref="CommonGainDifferential"/>) carries the entire
///     gain for this target.
///   </description></item>
///   <item><description>
///     <see cref="CommonGainElementPresent"/> = <see langword="false"/>:
///     a per-band differential is transmitted for every scale-factor
///     band whose section codebook is not ZERO_HCB - see
///     <see cref="DpcmGains"/>.
///   </description></item>
/// </list>
/// </summary>
public sealed record AacCouplingGainList
{
    /// <summary>
    /// <c>common_gain_element_present[c]</c>: always
    /// <see langword="true"/> when the enclosing element's
    /// <c>ind_sw_cce_flag</c> is set (the flag bit is then implied,
    /// not transmitted).
    /// </summary>
    public required bool CommonGainElementPresent { get; init; }

    /// <summary>
    /// Decoded differential of the single common Huffman codeword
    /// when <see cref="CommonGainElementPresent"/> is
    /// <see langword="true"/>; <see langword="null"/> otherwise.
    /// </summary>
    public int? CommonGainDifferential { get; init; }

    /// <summary>
    /// Per-band differentials when
    /// <see cref="CommonGainElementPresent"/> is
    /// <see langword="false"/>; empty otherwise. Order matches the
    /// stream order produced by the spec's
    /// <c>for (g) for (sfb) if (sect_cb != ZERO_HCB)</c> loop.
    /// </summary>
    public required IReadOnlyList<AacCouplingGainEntry> DpcmGains { get; init; }
}

/// <summary>
/// Parsed view of an AAC <c>coupling_channel_element()</c> (CCE) per
/// ISO/IEC 14496-3 §4.4.2.1 Table 4.8 and §4.6.8. A CCE carries one
/// auxiliary channel whose decoded spectrum is mixed - per band -
/// into one or more downstream SCE / CPE targets. The body composes
/// the target descriptor list, the coupling domain / gain framing
/// fields, an <see cref="AacIndividualChannelStream"/> body, and one
/// gain-element list per coupled target (after the implicit first
/// list whose gains are unity).
/// </summary>
/// <remarks>
/// <para>
/// The walker decodes the structure up to (but not including)
/// <c>spectral_data()</c>; the spectral coefficients are not consumed
/// here because they belong to the parent raw_data_block's
/// post-element walk. The gain-element lists are fully parsed.
/// </para>
/// <para>
/// <c>num_gain_element_lists</c> is derived from the targets:
/// one per target plus an extra for every CPE target where both
/// <see cref="AacCouplingTarget.CcLeft"/> and
/// <see cref="AacCouplingTarget.CcRight"/> are set. The first list
/// (c = 0) is implicit (unity gain) and is not stored in
/// <see cref="GainLists"/>; lists 1..N-1 are stored in stream order.
/// </para>
/// </remarks>
public sealed record AacCouplingChannelElement
{
    /// <summary>Maximum value of <c>element_instance_tag</c> (4-bit field).</summary>
    public const int MaxElementInstanceTag = 15;

    /// <summary>4-bit <c>element_instance_tag</c> identifying this CCE within the raw_data_block.</summary>
    public required int ElementInstanceTag { get; init; }

    /// <summary>
    /// <c>ind_sw_cce_flag</c>: when set, every per-target list uses a
    /// common gain element (the per-list <c>cge_present</c> flag is
    /// implied and not transmitted).
    /// </summary>
    public required bool IndependentSwitchedCceFlag { get; init; }

    /// <summary>
    /// Targets in stream order. Length is
    /// <c>num_coupled_elements + 1</c> (i.e. 1..8).
    /// </summary>
    public required IReadOnlyList<AacCouplingTarget> Targets { get; init; }

    /// <summary>
    /// <c>cc_domain</c>: <see langword="false"/> means the coupling
    /// gains are applied in the time domain (after IMDCT);
    /// <see langword="true"/> means they are applied in the
    /// frequency / MDCT domain.
    /// </summary>
    public required bool CcDomain { get; init; }

    /// <summary>
    /// <c>gain_element_sign</c>: when <see langword="true"/>, gain
    /// differentials may carry a sign component during downstream
    /// reconstruction.
    /// </summary>
    public required bool GainElementSign { get; init; }

    /// <summary><c>gain_element_scale</c> (0..3): downstream gain-step exponent selector.</summary>
    public required int GainElementScale { get; init; }

    /// <summary>
    /// Parsed coupling-channel <c>individual_channel_stream()</c>
    /// body (excluding <c>spectral_data()</c>). Always parses its
    /// own <c>ics_info()</c> with <c>common_window = 0</c> and
    /// <c>scale_flag = 0</c>.
    /// </summary>
    public required AacIndividualChannelStream Stream { get; init; }

    /// <summary>
    /// Per-target gain-element lists in stream order. Excludes the
    /// implicit first list (which always has unity gain). Length is
    /// <c>num_gain_element_lists - 1</c> and can be zero when there
    /// is exactly one target and it is not a "both channels coupled"
    /// CPE.
    /// </summary>
    public required IReadOnlyList<AacCouplingGainList> GainLists { get; init; }

    /// <summary>
    /// Total bits consumed by the CCE structure (excluding any
    /// trailing <c>spectral_data()</c> bits).
    /// </summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// <c>num_gain_element_lists</c> as derived from the target list
    /// (one per target plus an extra per "both channels coupled" CPE
    /// target). Equals <c>GainLists.Count + 1</c>.
    /// </summary>
    public int NumGainElementLists => GainLists.Count + 1;

    /// <summary>
    /// Read a <c>coupling_channel_element()</c> from
    /// <paramref name="reader"/> positioned at
    /// <c>element_instance_tag</c>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at element_instance_tag.</param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="element">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns>
    /// <see langword="true"/> when the full structure parsed cleanly.
    /// Returns <see langword="false"/> on stream underflow, on a
    /// rejected ICS body, on a scale-factor symbol outside
    /// <c>[0, 120]</c>, or when the scale-factor codebook does not
    /// have the canonical 121-symbol capacity.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacCouplingChannelElement? element)
    {
        element = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        if (scaleFactorCodebook.Capacity != 121) return false;

        int startBits = reader.Position;
        if (reader.Remaining < 4 + 1 + 3) return false;
        int elementInstanceTag = (int)reader.ReadBits(4);
        bool indSwCceFlag = reader.ReadBit();
        int numCoupledElements = (int)reader.ReadBits(3);

        int targetCount = numCoupledElements + 1;
        int numGainElementLists = 0;
        var targets = new List<AacCouplingTarget>(targetCount);
        for (int c = 0; c < targetCount; c++)
        {
            numGainElementLists++;
            if (reader.Remaining < 5) return false;
            bool isCpe = reader.ReadBit();
            int tagSelect = (int)reader.ReadBits(4);
            bool ccL = false;
            bool ccR = false;
            if (isCpe)
            {
                if (reader.Remaining < 2) return false;
                ccL = reader.ReadBit();
                ccR = reader.ReadBit();
                if (ccL && ccR) numGainElementLists++;
            }
            targets.Add(new AacCouplingTarget
            {
                IsChannelPairElement = isCpe,
                TargetTagSelect = tagSelect,
                CcLeft = ccL,
                CcRight = ccR,
            });
        }

        if (reader.Remaining < 1 + 1 + 2) return false;
        bool ccDomain = reader.ReadBit();
        bool gainElementSign = reader.ReadBit();
        int gainElementScale = (int)reader.ReadBits(2);

        if (!AacIndividualChannelStream.TryRead(
                ref reader,
                sharedIcsInfo: null,
                scaleFlag: false,
                scaleFactorCodebook,
                out var stream)
            || stream is null)
        {
            return false;
        }

        int extraGainLists = numGainElementLists - 1;
        var gainLists = new List<AacCouplingGainList>(Math.Max(0, extraGainLists));
        for (int c = 1; c < numGainElementLists; c++)
        {
            bool cgePresent;
            if (indSwCceFlag)
            {
                cgePresent = true;
            }
            else
            {
                if (reader.Remaining < 1) return false;
                cgePresent = reader.ReadBit();
            }

            if (cgePresent)
            {
                if (!scaleFactorCodebook.TryDecode(ref reader, out int idx)) return false;
                if (idx < 0 || idx > 120) return false;
                gainLists.Add(new AacCouplingGainList
                {
                    CommonGainElementPresent = true,
                    CommonGainDifferential = idx - 60,
                    DpcmGains = Array.Empty<AacCouplingGainEntry>(),
                });
            }
            else
            {
                var dpcm = new List<AacCouplingGainEntry>();
                foreach (var section in stream.SectionData.Sections)
                {
                    if (section.CodebookNumber == 0) continue; // ZERO_HCB - no gain entries
                    for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
                    {
                        if (!scaleFactorCodebook.TryDecode(ref reader, out int idx)) return false;
                        if (idx < 0 || idx > 120) return false;
                        dpcm.Add(new AacCouplingGainEntry
                        {
                            Group = section.Group,
                            Sfb = sfb,
                            Differential = idx - 60,
                        });
                    }
                }
                gainLists.Add(new AacCouplingGainList
                {
                    CommonGainElementPresent = false,
                    CommonGainDifferential = null,
                    DpcmGains = dpcm,
                });
            }
        }

        element = new AacCouplingChannelElement
        {
            ElementInstanceTag = elementInstanceTag,
            IndependentSwitchedCceFlag = indSwCceFlag,
            Targets = targets,
            CcDomain = ccDomain,
            GainElementSign = gainElementSign,
            GainElementScale = gainElementScale,
            Stream = stream,
            GainLists = gainLists,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>coupling_channel_element()</c> body
    /// from <paramref name="bytes"/> starting at the first bit.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacCouplingChannelElement? element)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, scaleFactorCodebook, out element);
    }
}
