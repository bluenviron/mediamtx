using System.Buffers;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.Gsm;

/// <summary>
/// Demuxer for raw GSM 06.10 (Full-Rate) audio streams as produced by the
/// classic <c>gsm</c>/<c>toast</c> tools and used widely in voicemail systems.
/// </summary>
/// <remarks>
/// The stream has no header. A frame is exactly 33 bytes representing
/// 160 PCM samples (20 ms at 8 kHz mono). Because the file format carries no
/// sample-rate metadata, callers may override the default 8 kHz by passing
/// the appropriate <c>sampleRate</c> argument when the file is known to use a
/// different rate (e.g. half-rate "GSM HR" packs different framing).
/// </remarks>
public sealed class GsmDemuxer : IMediaDemuxer
{
    /// <summary>Bytes per GSM 06.10 frame.</summary>
    public const int FrameBytes = 33;
    /// <summary>PCM samples per GSM 06.10 frame.</summary>
    public const int FrameSamples = 160;
    /// <summary>Default sample rate (8 kHz mono).</summary>
    public const int DefaultSampleRate = 8000;

    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _length;
    private readonly int _sampleRate;
    private long _startFrame;
    private bool _disposed;

    private GsmDemuxer(IRandomAccessSource source, bool ownsSource, MediaTrack track, long length, int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _length = length;
        _sampleRate = sampleRate;
    }

    /// <summary>Open a GSM 06.10 file from disk.</summary>
    public static GsmDemuxer Open(string path, int sampleRate = DefaultSampleRate)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true, sampleRate); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open a GSM 06.10 stream from a random-access source.</summary>
    public static GsmDemuxer Open(IRandomAccessSource source, bool ownsSource = false, int sampleRate = DefaultSampleRate)
    {
        ArgumentNullException.ThrowIfNull(source);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(sampleRate);
        long len = source.Length;
        if (len > 0 && len % FrameBytes != 0)
        {
            // tolerated but unusual — some tools append a trailing newline; round down.
            len -= len % FrameBytes;
        }
        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, sampleRate),
            Codec = new AudioCodecParameters { Codec = CodecId.Gsm610, SampleRate = sampleRate, Channels = 1 },
            DurationTicks = len / FrameBytes * FrameSamples,
        };
        return new GsmDemuxer(source, ownsSource, track, len, sampleRate);
    }

    /// <inheritdoc/>
    public string FormatName => "gsm";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => MediaMetadata.Empty;
    /// <inheritdoc/>
    public TimeSpan Duration =>
        TimeSpan.FromSeconds((double)(_length / FrameBytes * FrameSamples) / _sampleRate);

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long off = _startFrame * FrameBytes;
        long pts = _startFrame * FrameSamples;
        while (off + FrameBytes <= _length)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var owner = MemoryPool<byte>.Shared.Rent(FrameBytes);
            var mem = owner.Memory[..FrameBytes];
            int n = await _source.ReadAsync(off, mem, cancellationToken).ConfigureAwait(false);
            if (n != FrameBytes) { owner.Dispose(); yield break; }
            yield return new MediaSample
            {
                TrackIndex = 0, Pts = pts, Dts = pts, Duration = FrameSamples,
                IsKeyFrame = true, Data = mem, Owner = owner,
            };
            off += FrameBytes;
            pts += FrameSamples;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        long sample = (long)Math.Round(time.TotalSeconds * _sampleRate);
        _startFrame = Math.Clamp(sample / FrameSamples, 0, _length / FrameBytes);
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsSource) _source.Dispose();
    }
    /// <inheritdoc/>
    public ValueTask DisposeAsync() { Dispose(); return ValueTask.CompletedTask; }
}
