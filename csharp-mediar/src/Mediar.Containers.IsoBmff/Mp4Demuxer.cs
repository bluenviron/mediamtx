using System.Buffers;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Demuxer for ISO Base Media File Format (MP4, MOV, M4A, M4V, 3GP).
/// </summary>
/// <remarks>
/// <para>
/// The demuxer parses the <c>moov</c> box up-front and lazily reads each sample's
/// bytes from the <see cref="IRandomAccessSource"/> at enumeration time. Samples
/// are produced in DTS-interleaved order (oldest-DTS-across-all-tracks first), which
/// matches how players consume a multi-track file and yields good locality when the
/// file is interleaved (the common case).
/// </para>
/// <para>
/// All buffers backing emitted <see cref="MediaSample"/>s come from
/// <see cref="MemoryPool{Byte}.Shared"/>; callers must dispose
/// <see cref="MediaSample.Owner"/> once they have finished with each sample.
/// </para>
/// </remarks>
public sealed class Mp4Demuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly Mp4MovieData _movie;
    private readonly MediaTrack[] _tracks;
    private bool _disposed;

    /// <summary>Open an MP4/MOV file for demuxing.</summary>
    public static Mp4Demuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try
        {
            return new Mp4Demuxer(src, ownsSource: true);
        }
        catch
        {
            src.Dispose();
            throw;
        }
    }

    /// <summary>Open an MP4 from a caller-owned random-access source.</summary>
    public Mp4Demuxer(IRandomAccessSource source)
        : this(source, ownsSource: false)
    {
    }

    private Mp4Demuxer(IRandomAccessSource source, bool ownsSource)
    {
        _source = source;
        _ownsSource = ownsSource;
        _movie = MovieParser.Parse(source);
        _tracks = new MediaTrack[_movie.Tracks.Count];
        for (int i = 0; i < _tracks.Length; i++)
        {
            var t = _movie.Tracks[i];
            _tracks[i] = new MediaTrack
            {
                Index = i,
                Id = t.TrackId,
                Codec = t.CodecParameters ?? new SubtitleCodecParameters
                {
                    Codec = t.Codec,
                    Language = t.Language,
                },
                TimeBase = new Rational(1, (int)t.TimeScale),
                DurationTicks = (long)t.DurationInTimeScale,
                Language = t.Language,
                IsDefault = i == 0,
            };
        }
    }

    /// <inheritdoc/>
    public string FormatName => "mp4";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => _tracks;

    /// <inheritdoc/>
    public TimeSpan Duration
    {
        get
        {
            if (_movie.MovieTimeScale == 0) return TimeSpan.Zero;
            double seconds = (double)_movie.DurationInMovieTimeScale / _movie.MovieTimeScale;
            return TimeSpan.FromSeconds(seconds);
        }
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);

        // Per-track cursors.
        int trackCount = _movie.Tracks.Count;
        var cursors = new int[trackCount];

        while (true)
        {
            cancellationToken.ThrowIfCancellationRequested();

            // Pick the track whose next sample has the smallest DTS in seconds.
            int picked = -1;
            double pickedSeconds = double.PositiveInfinity;
            for (int t = 0; t < trackCount; t++)
            {
                var data = _movie.Tracks[t];
                int c = cursors[t];
                if (c >= data.Samples.Length) continue;
                double s = (double)data.Samples[c].Dts / data.TimeScale;
                if (s < pickedSeconds)
                {
                    pickedSeconds = s;
                    picked = t;
                }
            }
            if (picked < 0) yield break;

            var tdata = _movie.Tracks[picked];
            var rec = tdata.Samples[cursors[picked]++];

            // Read sample bytes from the source into a pooled buffer.
            var owner = MemoryPool<byte>.Shared.Rent(rec.Size);
            var mem = owner.Memory[..rec.Size];
            int read = 0;
            while (read < rec.Size)
            {
                int n = await _source.ReadAsync(rec.Offset + read, mem[read..], cancellationToken).ConfigureAwait(false);
                if (n <= 0)
                {
                    owner.Dispose();
                    throw new EndOfStreamException($"Unexpected EOF reading sample at offset {rec.Offset}.");
                }
                read += n;
            }

            yield return new MediaSample
            {
                TrackIndex = picked,
                Pts = rec.Dts + rec.CtsOffset,
                Dts = rec.Dts,
                Duration = rec.Duration,
                IsKeyFrame = rec.IsKey,
                Data = mem,
                Owner = owner,
            };
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsSource) _source.Dispose();
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync()
    {
        Dispose();
        return ValueTask.CompletedTask;
    }
}
