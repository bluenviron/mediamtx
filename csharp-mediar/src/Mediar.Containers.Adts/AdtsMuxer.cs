namespace Mediar.Containers.Adts;

/// <summary>
/// ADTS muxer: wraps each AAC access unit in a 7-byte ADTS header derived from
/// the track's <see cref="AudioCodecParameters"/>. Produces a raw <c>.aac</c>
/// stream compatible with virtually every AAC tool in existence.
/// </summary>
public sealed class AdtsMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;
    private int _sampleRateIndex;
    private int _channelConfig;
    private int _profile;

    /// <summary>Create a muxer writing to <paramref name="output"/>.</summary>
    public AdtsMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Output stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "aac";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_started) throw new InvalidOperationException("Cannot add tracks after Start.");
        if (_track is not null) throw new InvalidOperationException("ADTS supports a single track.");
        if (track.Codec is not AudioCodecParameters audio || audio.Codec != CodecId.Aac)
        {
            throw new ArgumentException("ADTS muxer accepts only CodecId.Aac audio tracks.", nameof(track));
        }
        int sri = AdtsHeader.IndexForSampleRate(audio.SampleRate);
        if (sri < 0) throw new ArgumentException($"Sample rate {audio.SampleRate} is not representable in ADTS.", nameof(track));
        if (audio.Channels is < 1 or > 7)
        {
            throw new ArgumentException("ADTS channel config must be 1..7.", nameof(track));
        }
        _sampleRateIndex = sri;
        _channelConfig = audio.Channels;
        // Default to AAC-LC (profile = AOT-1 = 1).
        _profile = 1;
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

        int payloadLen = sample.Data.Length;
        int frameLen = 7 + payloadLen;
        if (frameLen >= 1 << 13) throw new InvalidDataException("ADTS frame exceeds 8191 bytes.");

        byte[] header = new byte[7];
        // syncword
        header[0] = 0xFF;
        // sync (4) | MPEG-4 (1 bit = 0) | layer (2 bits = 0) | protection_absent (1)
        header[1] = (byte)(0xF0 | 0x01);
        // profile (2) | sample_freq_idx (4) | private (1) | channel_config high bit (1)
        header[2] = (byte)((_profile << 6) | (_sampleRateIndex << 2) | ((_channelConfig >> 2) & 0x01));
        // channel_config low 2 bits (2) | original_copy (1) | home (1) | copyright bits (2) | frame_length high 2 bits (2)
        header[3] = (byte)(((_channelConfig & 0x03) << 6) | ((frameLen >> 11) & 0x03));
        header[4] = (byte)((frameLen >> 3) & 0xFF);
        header[5] = (byte)(((frameLen & 0x07) << 5) | 0x1F);
        header[6] = 0xFC; // buffer fullness = 0x7FF, raw_data_blocks = 0

        await _output.WriteAsync(header, cancellationToken).ConfigureAwait(false);
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
