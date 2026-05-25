namespace Mediar;

/// <summary>
/// Describes a single track inside a container (one elementary stream).
/// </summary>
public sealed class MediaTrack
{
    /// <summary>Zero-based index inside the parent container.</summary>
    public required int Index { get; init; }

    /// <summary>
    /// Container-specific track identifier (e.g. MP4 <c>track_ID</c>).
    /// Two tracks in the same container always have distinct values.
    /// </summary>
    public required uint Id { get; init; }

    /// <summary>The kind of media carried by this track.</summary>
    public StreamKind Kind => Codec.Kind;

    /// <summary>Codec parameters for this track.</summary>
    public required CodecParameters Codec { get; init; }

    /// <summary>Time-base used for <see cref="MediaSample.Pts"/> and <c>Dts</c>.</summary>
    public required Rational TimeBase { get; init; }

    /// <summary>Total track duration expressed in <see cref="TimeBase"/> ticks; -1 if unknown.</summary>
    public long DurationTicks { get; init; } = -1;

    /// <summary>BCP-47 language tag, or "und".</summary>
    public string Language { get; init; } = "und";

    /// <summary>True if the muxer should treat this as the default track of its kind.</summary>
    public bool IsDefault { get; init; }

    /// <summary>Optional human-readable name.</summary>
    public string? Name { get; init; }

    /// <inheritdoc/>
    public override string ToString() =>
        $"#{Index} {Kind} {Codec.Codec} tb={TimeBase} dur={DurationTicks}";
}
