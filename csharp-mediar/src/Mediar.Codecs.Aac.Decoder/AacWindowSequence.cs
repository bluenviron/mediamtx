namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC window-sequence selector (ISO/IEC 14496-3 Table 4.74). Selects
/// the time/frequency tiling of an audio element: a single 2048-sample
/// long window, a long-to-short transition pair, eight 256-sample
/// short windows, or a short-to-long transition pair.
/// </summary>
public enum AacWindowSequence : byte
{
    /// <summary>One 2048-sample long window (1 group, 1 window).</summary>
    OnlyLong = 0,

    /// <summary>Long start window preceding an EIGHT_SHORT sequence.</summary>
    LongStart = 1,

    /// <summary>Eight 256-sample short windows split into 1..8 groups.</summary>
    EightShort = 2,

    /// <summary>Long stop window following an EIGHT_SHORT sequence.</summary>
    LongStop = 3,
}

/// <summary>
/// AAC window-shape selector (ISO/IEC 14496-3 §4.6.11).
/// </summary>
public enum AacWindowShape : byte
{
    /// <summary>Sine window.</summary>
    Sine = 0,

    /// <summary>Kaiser-Bessel-Derived window.</summary>
    KaiserBesselDerived = 1,
}
