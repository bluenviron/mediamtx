namespace Mediar;

/// <summary>
/// Reads tracks and samples from a container.
/// Implementations must be safe to enumerate from a single thread; concurrent
/// enumeration is not required.
/// </summary>
public interface IMediaDemuxer : IAsyncDisposable, IDisposable
{
    /// <summary>The container format name (e.g. <c>mp4</c>, <c>wav</c>).</summary>
    string FormatName { get; }

    /// <summary>The list of tracks discovered in the container.</summary>
    IReadOnlyList<MediaTrack> Tracks { get; }

    /// <summary>
    /// File-level metadata (title, artist, geolocation, etc.) extracted from
    /// the container. Demuxers that do not parse metadata return
    /// <see cref="MediaMetadata.Empty"/>.
    /// </summary>
    MediaMetadata Metadata => MediaMetadata.Empty;

    /// <summary>
    /// Total container duration. <see cref="TimeSpan.Zero"/> if unknown.
    /// </summary>
    TimeSpan Duration { get; }

    /// <summary>
    /// Enumerate samples in interleaved playback order. Samples are produced lazily.
    /// </summary>
    IAsyncEnumerable<MediaSample> ReadSamplesAsync(CancellationToken cancellationToken = default);

    /// <summary>
    /// Seek the demuxer so that subsequent <see cref="ReadSamplesAsync"/> calls
    /// begin at-or-before <paramref name="time"/>. For video tracks the position
    /// is snapped to the nearest preceding keyframe so playback can resume
    /// without a reference frame from earlier in the stream.
    /// </summary>
    /// <remarks>
    /// The default implementation throws <see cref="NotSupportedException"/>.
    /// Demuxers that can locate samples by timestamp override it.
    /// </remarks>
    ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
        => throw new NotSupportedException(
            $"Demuxer '{FormatName}' does not support seeking.");
}
