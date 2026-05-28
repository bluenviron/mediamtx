namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Syntactic element identifiers carried inside an AAC raw_data_block per
/// ISO/IEC 14496-3 Table 4.71. The 3-bit <c>id_syn_ele</c> field at the
/// start of every element selects which parser the dispatcher invokes.
/// </summary>
public enum AacSyntacticElementType
{
    /// <summary>Single Channel Element (mono channel ICS body).</summary>
    SingleChannelElement = 0,

    /// <summary>Channel Pair Element (stereo ICS body, optionally M/S coded).</summary>
    ChannelPairElement = 1,

    /// <summary>Coupling Channel Element.</summary>
    CouplingChannelElement = 2,

    /// <summary>LFE Channel Element (single-channel low-frequency body).</summary>
    LfeChannelElement = 3,

    /// <summary>Data Stream Element - opaque byte payload addressed by tag.</summary>
    DataStreamElement = 4,

    /// <summary>Program Configuration Element (custom channel layout).</summary>
    ProgramConfigElement = 5,

    /// <summary>Fill Element (padding + extension payloads such as SBR / DRC).</summary>
    FillElement = 6,

    /// <summary>End-of-raw_data_block sentinel; the dispatcher stops here.</summary>
    End = 7,
}
