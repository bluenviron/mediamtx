namespace Mediar.Containers.Gsm;

/// <summary>Writes raw GSM 06.10 frames (33-byte packets) to a stream.</summary>
public sealed class GsmMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;

    /// <summary>Create a GSM muxer.</summary>
    public GsmMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "gsm";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_track is not null) throw new InvalidOperationException("GSM supports a single track.");
        if (track.Codec is not AudioCodecParameters a || a.Codec != CodecId.Gsm610)
            throw new ArgumentException("GSM muxer requires a Gsm610 audio track.", nameof(track));
        _track = track;
    }

    /// <inheritdoc/>
    public ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("AddTrack must be called first.");
        _started = true;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) await StartAsync(cancellationToken).ConfigureAwait(false);
        if (sample.Data.Length != GsmDemuxer.FrameBytes)
            throw new InvalidDataException(
                $"GSM frames must be {GsmDemuxer.FrameBytes} bytes, got {sample.Data.Length}.");
        await _output.WriteAsync(sample.Data, cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        _finished = true;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (!_finished) await FinishAsync().ConfigureAwait(false);
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }
    /// <inheritdoc/>
    public void Dispose()
    {
        if (!_finished) FinishAsync().AsTask().GetAwaiter().GetResult();
        if (!_leaveOpen) _output.Dispose();
    }
}
