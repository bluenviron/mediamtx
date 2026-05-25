namespace Mediar;

/// <summary>
/// Codec-specific parameters that describe how to interpret samples on a track.
/// Concrete subtypes carry kind-specific fields.
/// </summary>
public abstract class CodecParameters
{
    /// <summary>The codec used for samples on this track.</summary>
    public required CodecId Codec { get; init; }

    /// <summary>Opaque codec-private data (e.g. AVC <c>avcC</c>, AAC AudioSpecificConfig).</summary>
    public ReadOnlyMemory<byte> ExtraData { get; init; }

    /// <summary>The general media kind this parameter set describes.</summary>
    public abstract StreamKind Kind { get; }
}

/// <summary>Video-specific codec parameters.</summary>
public sealed class VideoCodecParameters : CodecParameters
{
    /// <inheritdoc/>
    public override StreamKind Kind => StreamKind.Video;

    /// <summary>Pixel width of the coded video.</summary>
    public int Width { get; init; }

    /// <summary>Pixel height of the coded video.</summary>
    public int Height { get; init; }

    /// <summary>Sample aspect ratio (pixel aspect ratio).</summary>
    public Rational SampleAspectRatio { get; init; } = Rational.One;

    /// <summary>Average frame rate (may be approximate).</summary>
    public Rational FrameRate { get; init; }
}

/// <summary>Audio-specific codec parameters.</summary>
public sealed class AudioCodecParameters : CodecParameters
{
    /// <inheritdoc/>
    public override StreamKind Kind => StreamKind.Audio;

    /// <summary>Samples per second per channel.</summary>
    public int SampleRate { get; init; }

    /// <summary>Channel count.</summary>
    public int Channels { get; init; }

    /// <summary>Bits per coded sample (uncompressed) or 0 for compressed codecs.</summary>
    public int BitsPerSample { get; init; }
}

/// <summary>Subtitle / timed text codec parameters.</summary>
public sealed class SubtitleCodecParameters : CodecParameters
{
    /// <inheritdoc/>
    public override StreamKind Kind => StreamKind.Subtitle;

    /// <summary>BCP-47 language tag (e.g. <c>en</c>, <c>de-DE</c>).</summary>
    public string Language { get; init; } = "und";
}
