namespace Mediar.Containers.Mp3;

/// <summary>
/// Frame-concatenation MP3 muxer. Writes raw MPEG audio frames back-to-back
/// (no ID3 tags, no Xing header). Suitable for losslessly re-packaging MP3
/// data extracted from an MP4/MKV/etc. container.
/// </summary>
public sealed class Mp3Muxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;

    /// <summary>Create a muxer that writes to <paramref name="output"/>.</summary>
    public Mp3Muxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Output stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "mp3";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_started) throw new InvalidOperationException("Cannot add tracks after Start.");
        if (_track is not null) throw new InvalidOperationException("MP3 supports a single track.");
        if (track.Codec is not AudioCodecParameters { Codec: CodecId.Mp3 })
        {
            throw new ArgumentException("MP3 muxer accepts only CodecId.Mp3 audio tracks.", nameof(track));
        }
        _track = track;
    }

    /// <inheritdoc/>
    public ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("Add a track before starting.");
        _started = true;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(sample);
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) throw new InvalidOperationException("Muxer already finalized.");
        if (sample.TrackIndex != _track!.Index)
        {
            throw new ArgumentException("Sample track index does not match the registered track.", nameof(sample));
        }

        // Reject samples that obviously aren't MP3 frames. A valid MP3 frame
        // starts with an 11-bit sync (0xFFE).
        var span = sample.Data.Span;
        if (span.Length < 4 || span[0] != 0xFF || (span[1] & 0xE0) != 0xE0)
        {
            throw new InvalidDataException("Sample does not start with an MP3 frame sync.");
        }

        await _output.WriteAsync(sample.Data, cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return;
        await _output.FlushAsync(cancellationToken).ConfigureAwait(false);
        _finished = true;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (!_finished)
        {
            try { _output.Flush(); } catch { /* swallow on dispose */ }
            _finished = true;
        }
        if (!_leaveOpen) _output.Dispose();
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (!_finished)
        {
            try { await _output.FlushAsync().ConfigureAwait(false); } catch { /* swallow */ }
            _finished = true;
        }
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }
}
