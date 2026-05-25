namespace Mediar.Containers.Flac;

/// <summary>
/// Native FLAC stream muxer. Writes the <c>fLaC</c> marker, a single
/// STREAMINFO metadata block (taken from the source track's
/// <see cref="CodecParameters.ExtraData"/>), then each FLAC frame verbatim.
/// </summary>
/// <remarks>
/// The muxer is designed for losslessly re-packaging FLAC frames demuxed from
/// another container (Ogg, Matroska) into a native <c>.flac</c> file.
/// </remarks>
public sealed class FlacMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;

    /// <summary>Create a muxer that writes to <paramref name="output"/>.</summary>
    public FlacMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Output stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "flac";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_started) throw new InvalidOperationException("Cannot add tracks after Start.");
        if (_track is not null) throw new InvalidOperationException("FLAC supports a single track.");
        if (track.Codec is not AudioCodecParameters audio || audio.Codec != CodecId.Flac)
        {
            throw new ArgumentException("FLAC muxer accepts only CodecId.Flac audio tracks.", nameof(track));
        }
        if (audio.ExtraData.Length != 34)
        {
            throw new ArgumentException("FLAC track must carry a 34-byte STREAMINFO in ExtraData.", nameof(track));
        }
        _track = track;
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("Add a track before starting.");
        var audio = (AudioCodecParameters)_track.Codec;

        byte[] header = new byte[4 /* "fLaC" */ + 4 /* metadata block header */ + 34 /* STREAMINFO */];
        header[0] = (byte)'f';
        header[1] = (byte)'L';
        header[2] = (byte)'a';
        header[3] = (byte)'C';
        // Last metadata block (0x80) | type=STREAMINFO (0).
        header[4] = 0x80;
        // 24-bit big-endian length = 34.
        header[5] = 0x00;
        header[6] = 0x00;
        header[7] = 0x22;
        audio.ExtraData.Span.CopyTo(header.AsSpan(8, 34));

        await _output.WriteAsync(header, cancellationToken).ConfigureAwait(false);
        _started = true;
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
        var span = sample.Data.Span;
        // FLAC frame sync code: 14 bits 0b11111111111110 + 1 reserved + 1 blocking-strategy.
        if (span.Length < 4 || span[0] != 0xFF || (span[1] & 0xF8) != 0xF8 || (span[1] & 0x02) != 0)
        {
            throw new InvalidDataException("Sample does not start with a FLAC frame sync.");
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
            try { _output.Flush(); } catch { /* swallow */ }
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
