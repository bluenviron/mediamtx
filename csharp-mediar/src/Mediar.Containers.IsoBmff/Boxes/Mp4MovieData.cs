namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Parsed contents of a single MP4 track at demux time.
/// </summary>
internal sealed class Mp4TrackData
{
    public required uint TrackId { get; init; }
    public required uint TimeScale { get; init; }
    public required ulong DurationInTimeScale { get; init; }
    public required string Handler { get; init; }
    public required string Language { get; init; }
    public required CodecId Codec { get; init; }
    public CodecParameters? CodecParameters { get; set; }
    public SampleRecord[] Samples { get; set; } = [];
}

/// <summary>
/// Parsed top-level movie metadata.
/// </summary>
internal sealed class Mp4MovieData
{
    public required uint MovieTimeScale { get; init; }
    public required ulong DurationInMovieTimeScale { get; init; }
    public required List<Mp4TrackData> Tracks { get; init; }
}
