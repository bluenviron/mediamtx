namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Loudspeaker positions used by the AAC channel-mapping table to
/// describe which physical output a particular element-channel
/// targets. Mirrors the labels used in ISO/IEC 14496-3 Table 1.19
/// and the long-form spec text.
/// </summary>
public enum AacSpeaker
{
    /// <summary>Unassigned (used when no second speaker exists for a SCE/LFE entry).</summary>
    None = 0,

    /// <summary>Front centre (C).</summary>
    FrontCentre,

    /// <summary>Front left (L).</summary>
    FrontLeft,

    /// <summary>Front right (R).</summary>
    FrontRight,

    /// <summary>Surround left (Ls) - rear-side in 5.1, side in 7.1.</summary>
    SurroundLeft,

    /// <summary>Surround right (Rs).</summary>
    SurroundRight,

    /// <summary>Back / rear-surround left (Lsr / Lrs) - 7.1 only.</summary>
    BackLeft,

    /// <summary>Back / rear-surround right (Rsr / Rrs) - 7.1 only.</summary>
    BackRight,

    /// <summary>Mono back surround (Cs) - 4.0 only.</summary>
    BackCentre,

    /// <summary>Low-frequency effects channel (LFE).</summary>
    Lfe,
}

/// <summary>
/// One entry in the AAC channel-mapping table: a syntactic element
/// (SCE / CPE / LFE) plus the speaker(s) its decoded channel(s)
/// drive. For SCE / LFE entries
/// <see cref="SecondSpeaker"/> is <see cref="AacSpeaker.None"/>;
/// for CPE entries the first/second decoded channel correspond to
/// the left/right speakers.
/// </summary>
public sealed record AacChannelMappingEntry
{
    /// <summary>Element kind in raw_data_block scan order.</summary>
    public required AacSyntacticElementType Element { get; init; }

    /// <summary>Speaker driven by the element's first (or only) channel.</summary>
    public required AacSpeaker FirstSpeaker { get; init; }

    /// <summary>
    /// Speaker driven by the element's second channel (CPE only);
    /// <see cref="AacSpeaker.None"/> for SCE / LFE.
    /// </summary>
    public AacSpeaker SecondSpeaker { get; init; } = AacSpeaker.None;
}

/// <summary>
/// MPEG-4 channel-configuration to element-order + speaker-mapping
/// table per ISO/IEC 14496-3 Table 1.19. Maps a
/// <c>channelConfiguration</c> value (1..7) to the ordered list of
/// raw_data_block elements expected by a conformant frame, with
/// each element's decoded channels annotated against the standard
/// speaker labels.
/// </summary>
/// <remarks>
/// <para>
/// <c>channelConfiguration == 0</c> is the explicit-layout case:
/// the decoder must consult the
/// <see cref="AacProgramConfigurationElement"/> instead, and this
/// helper returns an empty list (callers must not call it for 0).
/// </para>
/// <para>
/// All standard layouts begin with a centre SCE and a front L/R
/// CPE (for configurations 3+). Surround pairs are added as CPEs,
/// the rear centre (Cs) for 4.0 is a trailing SCE, and the LFE is
/// a trailing LFE element for 5.1 / 7.1.
/// </para>
/// </remarks>
public static class AacChannelMapping
{
    /// <summary>
    /// Return the ordered element list for
    /// <paramref name="channelConfiguration"/>. Returns an empty
    /// list for the explicit-PCE case (configuration 0).
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="channelConfiguration"/> &lt; 0 or &gt; 7.
    /// </exception>
    public static IReadOnlyList<AacChannelMappingEntry> GetForConfiguration(int channelConfiguration)
    {
        if (channelConfiguration is < 0 or > 7)
        {
            throw new ArgumentOutOfRangeException(
                nameof(channelConfiguration),
                channelConfiguration,
                "channelConfiguration must be in [0, 7].");
        }

        return channelConfiguration switch
        {
            0 => Array.Empty<AacChannelMappingEntry>(),

            // 1 channel: SCE -> C
            1 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
            },

            // 2 channels: CPE -> L, R
            2 => new[]
            {
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
            },

            // 3 channels (3.0): SCE C, CPE L R
            3 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
            },

            // 4 channels (4.0): SCE C, CPE L R, SCE Cs
            4 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
                Sce(AacSpeaker.BackCentre),
            },

            // 5 channels (5.0): SCE C, CPE L R, CPE Ls Rs
            5 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
                Cpe(AacSpeaker.SurroundLeft, AacSpeaker.SurroundRight),
            },

            // 6 channels (5.1): 5.0 + LFE
            6 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
                Cpe(AacSpeaker.SurroundLeft, AacSpeaker.SurroundRight),
                Lfe(),
            },

            // 8 channels (7.1): SCE C, CPE L R, CPE Lss Rss, CPE Lsr Rsr, LFE
            // (per Table 1.19 in MPEG-4 v2: side surround + back surround)
            7 => new[]
            {
                Sce(AacSpeaker.FrontCentre),
                Cpe(AacSpeaker.FrontLeft, AacSpeaker.FrontRight),
                Cpe(AacSpeaker.SurroundLeft, AacSpeaker.SurroundRight),
                Cpe(AacSpeaker.BackLeft, AacSpeaker.BackRight),
                Lfe(),
            },

            _ => throw new ArgumentOutOfRangeException(nameof(channelConfiguration)),
        };
    }

    /// <summary>
    /// Count the speaker outputs implied by
    /// <paramref name="channelConfiguration"/>. This is the audible
    /// channel count; equal to
    /// <see cref="AacChannelConfigurations.SpeakerCount"/>.
    /// </summary>
    public static int SpeakerCount(int channelConfiguration)
        => AacChannelConfigurations.SpeakerCount(channelConfiguration);

    private static AacChannelMappingEntry Sce(AacSpeaker s) => new()
    {
        Element = AacSyntacticElementType.SingleChannelElement,
        FirstSpeaker = s,
    };

    private static AacChannelMappingEntry Cpe(AacSpeaker first, AacSpeaker second) => new()
    {
        Element = AacSyntacticElementType.ChannelPairElement,
        FirstSpeaker = first,
        SecondSpeaker = second,
    };

    private static AacChannelMappingEntry Lfe() => new()
    {
        Element = AacSyntacticElementType.LfeChannelElement,
        FirstSpeaker = AacSpeaker.Lfe,
    };
}
