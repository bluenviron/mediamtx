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
    /// Total container duration. <see cref="TimeSpan.Zero"/> if unknown.
    /// </summary>
    TimeSpan Duration { get; }

    /// <summary>
    /// Enumerate samples in interleaved playback order. Samples are produced lazily.
    /// </summary>
    IAsyncEnumerable<MediaSample> ReadSamplesAsync(CancellationToken cancellationToken = default);
}
