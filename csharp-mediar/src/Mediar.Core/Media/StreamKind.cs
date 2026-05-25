namespace Mediar;

/// <summary>The kind of media carried by a track.</summary>
public enum StreamKind
{
    /// <summary>Unknown / unspecified.</summary>
    Unknown = 0,
    /// <summary>Compressed or uncompressed video.</summary>
    Video,
    /// <summary>Compressed or uncompressed audio.</summary>
    Audio,
    /// <summary>Timed text subtitles or closed captions.</summary>
    Subtitle,
    /// <summary>Generic timed data (e.g. metadata).</summary>
    Data,
}
