namespace Mediar;

/// <summary>
/// Stable codec identifiers used across container boundaries.
/// Values are deliberately stable. Codec implementations are pluggable;
/// this enum only names what was encoded, it does not imply Mediar can decode it.
/// </summary>
public enum CodecId
{
    /// <summary>Unknown / unrecognized codec.</summary>
    Unknown = 0,

    // --- Audio
    /// <summary>Uncompressed PCM (interpretation depends on codec parameters).</summary>
    PcmS16Le = 0x0100,
    /// <summary>Uncompressed PCM big-endian 16-bit signed.</summary>
    PcmS16Be,
    /// <summary>Uncompressed PCM 24-bit signed little-endian.</summary>
    PcmS24Le,
    /// <summary>Uncompressed PCM 32-bit signed little-endian.</summary>
    PcmS32Le,
    /// <summary>Uncompressed PCM 32-bit IEEE float little-endian.</summary>
    PcmF32Le,
    /// <summary>MPEG-1/2 Layer III audio (MP3).</summary>
    Mp3,
    /// <summary>FLAC lossless audio.</summary>
    Flac,
    /// <summary>MPEG-4 AAC (LC, HE, etc.).</summary>
    Aac,
    /// <summary>Opus.</summary>
    Opus,
    /// <summary>Vorbis.</summary>
    Vorbis,
    /// <summary>ALAC (Apple lossless).</summary>
    Alac,
    /// <summary>AC-3.</summary>
    Ac3,
    /// <summary>E-AC-3.</summary>
    EAc3,
    /// <summary>ITU-T G.711 µ-law (8-bit companded).</summary>
    G711MuLaw,
    /// <summary>ITU-T G.711 A-law (8-bit companded).</summary>
    G711ALaw,
    /// <summary>Uncompressed PCM 8-bit unsigned.</summary>
    PcmU8,
    /// <summary>Uncompressed PCM 8-bit signed.</summary>
    PcmS8,
    /// <summary>MPEG-1/2 Layer II audio (MP2).</summary>
    Mp2,
    /// <summary>MPEG-1/2 Layer I audio (MP1).</summary>
    Mp1,
    /// <summary>ETSI GSM 06.10 full-rate (13 kbit/s).</summary>
    Gsm610,
    /// <summary>Adaptive Multi-Rate Narrowband (AMR-NB, 3GPP TS 26.071).</summary>
    AmrNb,
    /// <summary>Adaptive Multi-Rate Wideband (AMR-WB, 3GPP TS 26.171).</summary>
    AmrWb,
    /// <summary>Qualcomm PureVoice (QCELP-13K).</summary>
    Qcelp,
    /// <summary>WavPack lossless / hybrid.</summary>
    WavPack,
    /// <summary>4-bit IMA / Dialogic / OKI ADPCM (VOC, WAV).</summary>
    ImaAdpcm,
    /// <summary>Microsoft 4-bit ADPCM (formatTag 0x0002 in WAV).</summary>
    MsAdpcm,
    /// <summary>Creative ADPCM (used in VOC files).</summary>
    CreativeAdpcm,
    /// <summary>Fibonacci-delta-encoded 8-bit audio used by 8SVX.</summary>
    Fibonacci8Svx,

    // --- Video
    /// <summary>H.264 / AVC.</summary>
    H264 = 0x0200,
    /// <summary>H.265 / HEVC.</summary>
    H265,
    /// <summary>AV1.</summary>
    Av1,
    /// <summary>
    /// AV2 — Alliance for Open Media's successor codec to AV1. The AV2
    /// bitstream specification is still under active development (as of
    /// 2026) and no Mediar decoder is provided. This identifier exists so
    /// that container-level operations (demux, remux, sample passthrough
    /// from MKV/MP4 into another container) can transparently carry AV2
    /// samples once they begin to appear in the wild.
    /// </summary>
    Av2,
    /// <summary>VP8.</summary>
    Vp8,
    /// <summary>VP9.</summary>
    Vp9,
    /// <summary>MPEG-4 part 2.</summary>
    Mpeg4,

    // --- Subtitle
    /// <summary>SubRip text subtitle.</summary>
    SubRip = 0x0300,
    /// <summary>WebVTT text subtitle.</summary>
    WebVtt,
    /// <summary>3GPP Timed Text (tx3g atom in MP4).</summary>
    Tx3g,
    /// <summary>SubStation Alpha / Advanced SSA.</summary>
    Ass,
}

/// <summary>Helpers for <see cref="CodecId"/>.</summary>
public static class CodecIdExtensions
{
    /// <summary>Best-effort mapping from CodecId to <see cref="StreamKind"/>.</summary>
    public static StreamKind Kind(this CodecId id) => (int)id switch
    {
        >= 0x0100 and < 0x0200 => StreamKind.Audio,
        >= 0x0200 and < 0x0300 => StreamKind.Video,
        >= 0x0300 and < 0x0400 => StreamKind.Subtitle,
        _ => StreamKind.Unknown,
    };
}
