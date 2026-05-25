namespace Mediar;

/// <summary>
/// Builds a container around supplied tracks and samples.
/// </summary>
public interface IMediaMuxer : IAsyncDisposable, IDisposable
{
    /// <summary>The container format name (e.g. <c>mp4</c>, <c>wav</c>).</summary>
    string FormatName { get; }

    /// <summary>
    /// Declare a track. Must be called for every track before <see cref="WriteSampleAsync"/>.
    /// </summary>
    void AddTrack(MediaTrack track);

    /// <summary>Finalize track metadata and prepare for sample writes.</summary>
    ValueTask StartAsync(CancellationToken cancellationToken = default);

    /// <summary>Write one sample.</summary>
    ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default);

    /// <summary>Finalize the container (write indexes / footer).</summary>
    ValueTask FinishAsync(CancellationToken cancellationToken = default);
}
