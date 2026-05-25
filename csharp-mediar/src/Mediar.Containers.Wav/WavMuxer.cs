using Mediar.IO;

namespace Mediar.Containers.Wav;

/// <summary>
/// Muxer for RIFF/WAVE PCM audio files. Supports <c>WAVE_FORMAT_PCM</c> for
/// 16/24/32-bit integer and <c>WAVE_FORMAT_IEEE_FLOAT</c> for 32-bit float.
/// The output stream must be writable and seekable so the RIFF and data chunk
/// sizes can be patched once <see cref="FinishAsync"/> is called.
/// </summary>
public sealed class WavMuxer : IMediaMuxer
{
    private const ushort WaveFormatPcm = 0x0001;
    private const ushort WaveFormatIeeeFloat = 0x0003;

    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private long _riffSizePos;
    private long _dataSizePos;
    private long _dataPayloadStart;
    private bool _started;
    private bool _finished;
    private bool _disposed;

    /// <summary>Create a new muxer writing to <paramref name="output"/>.</summary>
    public WavMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite || !output.CanSeek)
        {
            throw new ArgumentException("Stream must be writable and seekable.", nameof(output));
        }
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "wav";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        if (_track is not null) throw new InvalidOperationException("WAV supports exactly one audio track.");
        if (track.Codec is not AudioCodecParameters)
        {
            throw new ArgumentException("WAV only supports audio tracks.", nameof(track));
        }
        _track = track;
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_started) throw new InvalidOperationException("Already started.");
        if (_track is null) throw new InvalidOperationException("Add a track first.");
        var audio = (AudioCodecParameters)_track.Codec;

        _started = true;

        ushort formatTag = audio.Codec switch
        {
            CodecId.PcmS16Le or CodecId.PcmS24Le or CodecId.PcmS32Le => WaveFormatPcm,
            CodecId.PcmF32Le => WaveFormatIeeeFloat,
            _ => throw new NotSupportedException($"WAV cannot encode {audio.Codec}."),
        };

        int bitsPerSample = audio.BitsPerSample > 0 ? audio.BitsPerSample : audio.Codec switch
        {
            CodecId.PcmS16Le => 16,
            CodecId.PcmS24Le => 24,
            CodecId.PcmS32Le or CodecId.PcmF32Le => 32,
            _ => 16,
        };

        int blockAlign = (bitsPerSample / 8) * audio.Channels;
        int avgBytesPerSec = blockAlign * audio.SampleRate;

        byte[] header = new byte[12 + 8 + 16];
        var w = new LittleEndianSpanWriter(header);

        // RIFF chunk header
        w.WriteUInt8((byte)'R'); w.WriteUInt8((byte)'I');
        w.WriteUInt8((byte)'F'); w.WriteUInt8((byte)'F');
        _riffSizePos = w.Position;
        w.WriteUInt32(0); // placeholder, patched in Finish
        w.WriteUInt8((byte)'W'); w.WriteUInt8((byte)'A');
        w.WriteUInt8((byte)'V'); w.WriteUInt8((byte)'E');

        // fmt chunk
        w.WriteUInt8((byte)'f'); w.WriteUInt8((byte)'m');
        w.WriteUInt8((byte)'t'); w.WriteUInt8((byte)' ');
        w.WriteUInt32(16);
        w.WriteUInt16(formatTag);
        w.WriteUInt16((ushort)audio.Channels);
        w.WriteUInt32((uint)audio.SampleRate);
        w.WriteUInt32((uint)avgBytesPerSec);
        w.WriteUInt16((ushort)blockAlign);
        w.WriteUInt16((ushort)bitsPerSample);

        await _output.WriteAsync(header, cancellationToken).ConfigureAwait(false);

        // data chunk header
        byte[] dataHdr = new byte[8];
        dataHdr[0] = (byte)'d'; dataHdr[1] = (byte)'a';
        dataHdr[2] = (byte)'t'; dataHdr[3] = (byte)'a';
        _dataSizePos = _output.Position + 4;
        await _output.WriteAsync(dataHdr, cancellationToken).ConfigureAwait(false);
        _dataPayloadStart = _output.Position;
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) throw new InvalidOperationException("Muxer already finished.");
        await _output.WriteAsync(sample.Data, cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) return;
        _finished = true;

        long end = _output.Position;
        long dataSize = end - _dataPayloadStart;
        long riffSize = end - 8;

        // Patch data chunk size.
        _output.Position = _dataSizePos;
        byte[] tmp = new byte[4];
        WriteUInt32Le(tmp, (uint)Math.Min(uint.MaxValue, dataSize));
        await _output.WriteAsync(tmp, cancellationToken).ConfigureAwait(false);

        // Patch RIFF chunk size.
        _output.Position = _riffSizePos;
        WriteUInt32Le(tmp, (uint)Math.Min(uint.MaxValue, riffSize));
        await _output.WriteAsync(tmp, cancellationToken).ConfigureAwait(false);

        // Pad to even length per the RIFF spec.
        _output.Position = end;
        if ((dataSize & 1) == 1)
        {
            byte[] pad = new byte[1];
            await _output.WriteAsync(pad, cancellationToken).ConfigureAwait(false);
        }
        await _output.FlushAsync(cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished)
        {
            try { FinishAsync().AsTask().GetAwaiter().GetResult(); } catch { /* best effort */ }
        }
        if (!_leaveOpen) _output.Dispose();
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished)
        {
            try { await FinishAsync().ConfigureAwait(false); } catch { /* best effort */ }
        }
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }

    private static void WriteUInt32Le(Span<byte> dst, uint v)
    {
        dst[0] = (byte)v;
        dst[1] = (byte)(v >> 8);
        dst[2] = (byte)(v >> 16);
        dst[3] = (byte)(v >> 24);
    }
}
