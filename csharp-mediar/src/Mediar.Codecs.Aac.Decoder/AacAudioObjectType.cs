namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// MPEG-4 audio object type values (ISO/IEC 14496-3 Table 1.16). Codes that
/// require an explicit AOT-escape (audioObjectType == 31) are not enumerated
/// individually; the parser exposes the resolved numeric value.
/// </summary>
public enum AacAudioObjectType
{
    /// <summary>Reserved / not yet decoded.</summary>
    Null = 0,
    /// <summary>AAC main profile.</summary>
    AacMain = 1,
    /// <summary>AAC low complexity (AAC-LC) — by far the most common.</summary>
    AacLc = 2,
    /// <summary>AAC scalable sample rate (AAC-SSR).</summary>
    AacSsr = 3,
    /// <summary>AAC long term prediction.</summary>
    AacLtp = 4,
    /// <summary>Spectral band replication (HE-AAC v1 base layer).</summary>
    Sbr = 5,
    /// <summary>AAC scalable.</summary>
    AacScalable = 6,
    /// <summary>TwinVQ.</summary>
    TwinVq = 7,
    /// <summary>Code-excited linear prediction (CELP).</summary>
    Celp = 8,
    /// <summary>Harmonic vector excitation coding.</summary>
    Hvxc = 9,
    /// <summary>Error-resilient AAC-LC.</summary>
    ErAacLc = 17,
    /// <summary>Parametric stereo (HE-AAC v2 base layer).</summary>
    Ps = 29,
    /// <summary>AOT-escape marker (5 bits = 31, followed by 6-bit extension).</summary>
    Escape = 31,
}
