namespace Mediar.Containers.Amr;

/// <summary>
/// Writes 3GPP AMR-NB or AMR-WB Storage-Format streams (single mono channel).
/// </summary>
public sealed class AmrMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;

    /// <summary>Create an AMR muxer.</summary>
    public AmrMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName =>
        _track?.Codec is AudioCodecParameters a && a.Codec == CodecId.AmrWb ? "amr-wb" : "amr-nb";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_track is not null) throw new InvalidOperationException("AMR supports a single track.");
        if (track.Codec is not AudioCodecParameters a || (a.Codec != CodecId.AmrNb && a.Codec != CodecId.AmrWb))
            throw new ArgumentException("AMR muxer requires an AmrNb or AmrWb audio track.", nameof(track));
        _track = track;
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("AddTrack must be called first.");
        if (_started) return;
        _started = true;
        var a = (AudioCodecParameters)_track.Codec;
        byte[] magic = a.Codec == CodecId.AmrWb ? "#!AMR-WB\n"u8.ToArray() : "#!AMR\n"u8.ToArray();
        await _output.WriteAsync(magic, cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) await StartAsync(cancellationToken).ConfigureAwait(false);
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
