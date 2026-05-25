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

    // --- Video
    /// <summary>H.264 / AVC.</summary>
    H264 = 0x0200,
    /// <summary>H.265 / HEVC.</summary>
    H265,
    /// <summary>AV1.</summary>
    Av1,
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
