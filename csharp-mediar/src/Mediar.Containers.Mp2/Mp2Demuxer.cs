using System.Buffers;
using System.Runtime.CompilerServices;
using Mediar.Containers.Mp3;
using Mediar.IO;

namespace Mediar.Containers.Mp2;

/// <summary>
/// Demuxer for raw MPEG-1/2 Audio Layer II streams (typically <c>.mp2</c>).
/// </summary>
/// <remarks>
/// MP2 shares the 4-byte MPEG audio sync header with MP3; this demuxer reuses
/// <see cref="Mp3FrameHeader"/> for parsing but emits <see cref="CodecId.Mp2"/>
/// (or <see cref="CodecId.Mp1"/> for Layer-I streams) and refuses to open
/// streams whose first frame is Layer III.
/// </remarks>
public sealed class Mp2Demuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _dataOffset;
    private readonly long _dataEnd;
    private readonly int _sampleRate;
    private readonly int _samplesPerFrame;
    private long _seekTargetSamples;
    private bool _disposed;

    private Mp2Demuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track,
        long dataOffset, long dataEnd, int sampleRate, int samplesPerFrame)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _dataOffset = dataOffset;
        _dataEnd = dataEnd;
        _sampleRate = sampleRate;
        _samplesPerFrame = samplesPerFrame;
    }

    /// <summary>Open an MP2 file from disk.</summary>
    public static Mp2Demuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an MP2 stream from a random-access source.</summary>
    public static Mp2Demuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);
        long start = 0;
        long end = source.Length;

        if (!ScanFrame(source, ref start, end, out var hdr))
            throw new InvalidDataException("No MPEG audio frame found.");
        if (hdr.Layer == 3)
            throw new InvalidDataException("Layer-III stream — use Mp3Demuxer instead.");

        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, hdr.SampleRate),
            Codec = new AudioCodecParameters
            {
                Codec = hdr.Layer == 1 ? CodecId.Mp1 : CodecId.Mp2,
                SampleRate = hdr.SampleRate,
                Channels = hdr.Channels,
                BitsPerSample = 0,
            },
        };
        return new Mp2Demuxer(source, ownsSource, track, start, end, hdr.SampleRate, hdr.SamplesPerFrame);
    }

    /// <inheritdoc/>
    public string FormatName => "mp2";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => MediaMetadata.Empty;
    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long offset = _dataOffset;
        long pts = 0;
        byte[] hdrBuf = new byte[4];
        while (offset + 4 <= _dataEnd)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (_source.Read(offset, hdrBuf) < 4) yield break;
            if (!Mp3FrameHeader.TryParse(hdrBuf, out var header)) { offset++; continue; }
            if (header.FrameSize <= 0 || offset + header.FrameSize > _dataEnd) yield break;

            if (pts + header.SamplesPerFrame <= _seekTargetSamples)
            {
                offset += header.FrameSize;
                pts += header.SamplesPerFrame;
                continue;
            }

            var owner = MemoryPool<byte>.Shared.Rent(header.FrameSize);
            var mem = owner.Memory[..header.FrameSize];
            int n = await _source.ReadAsync(offset, mem, cancellationToken).ConfigureAwait(false);
            if (n != header.FrameSize) { owner.Dispose(); yield break; }
            yield return new MediaSample
            {
                TrackIndex = 0, Pts = pts, Dts = pts,
                Duration = header.SamplesPerFrame, IsKeyFrame = true,
                Data = mem, Owner = owner,
            };
            offset += header.FrameSize;
            pts += header.SamplesPerFrame;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        if (time < TimeSpan.Zero) time = TimeSpan.Zero;
        _seekTargetSamples = (long)Math.Round(time.TotalSeconds * _sampleRate);
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

    private static bool ScanFrame(IRandomAccessSource source, ref long offset, long end, out Mp3FrameHeader hdr)
    {
        Span<byte> buf = stackalloc byte[4];
        while (offset + 4 <= end)
        {
            if (source.Read(offset, buf) < 4) break;
            if (Mp3FrameHeader.TryParse(buf, out hdr)) return true;
            offset++;
        }
        hdr = default;
        return false;
    }
}
