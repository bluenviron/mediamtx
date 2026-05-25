using System.Buffers;
using Mediar.IO;

namespace Mediar.Containers.Mp3;

/// <summary>
/// Demuxer for raw MPEG-1/2/2.5 Audio Layer III streams (typically <c>.mp3</c>).
/// Skips ID3v2 leading tags and ID3v1 trailing tags, parses the first valid frame
/// header to seed track parameters and emits one sample per MPEG frame.
/// </summary>
public sealed class Mp3Demuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly MediaMetadata _metadata;
    private readonly long _dataOffset;
    private readonly long _dataEnd;
    private readonly int _sampleRate;
    private readonly int _samplesPerFrame;
    private readonly long _totalFrames;
    private long _seekTargetSamples;
    private bool _disposed;

    private Mp3Demuxer(
        IRandomAccessSource source,
        bool ownsSource,
        MediaTrack track,
        MediaMetadata metadata,
        long dataOffset,
        long dataEnd,
        int sampleRate,
        int samplesPerFrame,
        long totalFrames)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _metadata = metadata;
        _dataOffset = dataOffset;
        _dataEnd = dataEnd;
        _sampleRate = sampleRate;
        _samplesPerFrame = samplesPerFrame;
        _totalFrames = totalFrames;
    }

    /// <summary>Open an MP3 file from disk.</summary>
    public static Mp3Demuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try
        {
            return Open(src, ownsSource: true);
        }
        catch
        {
            src.Dispose();
            throw;
        }
    }

    /// <summary>Open an MP3 stream backed by an arbitrary <see cref="IRandomAccessSource"/>.</summary>
    public static Mp3Demuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        long length = source.Length;
        long start = 0;
        long end = length;
        var meta = new MediaMetadataBuilder();

        // ID3v2 header at start?
        Span<byte> hdr10 = stackalloc byte[10];
        if (source.Read(0, hdr10) == 10 && hdr10[0] == 'I' && hdr10[1] == 'D' && hdr10[2] == '3')
        {
            int size = (hdr10[6] << 21) | (hdr10[7] << 14) | (hdr10[8] << 7) | hdr10[9];
            int tagEnd = 10 + size;
            if ((hdr10[5] & 0x10) != 0) tagEnd += 10; // footer present
            Id3v2.Parse(source, hdr10[3], hdr10[5], size, meta);
            start = tagEnd;
        }

        // ID3v1 at end?
        if (length >= 128)
        {
            Span<byte> tag = stackalloc byte[3];
            if (source.Read(length - 128, tag) == 3 && tag[0] == 'T' && tag[1] == 'A' && tag[2] == 'G')
            {
                end = length - 128;
                Id3v1.Parse(source, length - 128, meta);
            }
        }

        // Scan for the first valid frame header.
        if (!ScanFrame(source, ref start, end, out var firstHeader))
        {
            throw new InvalidDataException("No MPEG audio frame found.");
        }

        var codec = new AudioCodecParameters
        {
            Codec = CodecId.Mp3,
            SampleRate = firstHeader.SampleRate,
            Channels = firstHeader.Channels,
            BitsPerSample = 0,
        };
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, firstHeader.SampleRate),
            Codec = codec,
        };

        long durationFrames = -1;
        // Best-effort total frame count by integer-dividing data length over the first frame size;
        // VBR streams will be inaccurate but it's good enough for Duration reporting.
        if (firstHeader.FrameSize > 0)
        {
            durationFrames = (end - start) / firstHeader.FrameSize;
        }

        return new Mp3Demuxer(source, ownsSource, track, meta.Build(), start, end, firstHeader.SampleRate, firstHeader.SamplesPerFrame, durationFrames);
    }

    /// <inheritdoc/>
    public string FormatName => "mp3";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => new[] { _track };

    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public TimeSpan Duration => _totalFrames > 0
        ? TimeSpan.FromSeconds((double)(_totalFrames * _samplesPerFrame) / _sampleRate)
        : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long offset = _dataOffset;
        long pts = 0;
        byte[] hdrBuf = new byte[4];

        while (offset + 4 <= _dataEnd)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int read = _source.Read(offset, hdrBuf);
            if (read < 4) yield break;
            if (!Mp3FrameHeader.TryParse(hdrBuf, out var header))
            {
                offset++;
                continue;
            }
            if (header.FrameSize <= 0 || offset + header.FrameSize > _dataEnd) yield break;

            // Skip past frames whose entire duration falls before the seek target;
            // we read the 4-byte header only to know how big the frame is.
            if (pts + header.SamplesPerFrame <= _seekTargetSamples)
            {
                offset += header.FrameSize;
                pts += header.SamplesPerFrame;
                continue;
            }

            var owner = MemoryPool<byte>.Shared.Rent(header.FrameSize);
            var mem = owner.Memory[..header.FrameSize];
            int n = await _source.ReadAsync(offset, mem, cancellationToken).ConfigureAwait(false);
            if (n != header.FrameSize)
            {
                owner.Dispose();
                yield break;
            }

            yield return new MediaSample
            {
                TrackIndex = 0,
                Pts = pts,
                Dts = pts,
                Duration = header.SamplesPerFrame,
                IsKeyFrame = true,
                Data = mem,
                Owner = owner,
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
    public ValueTask DisposeAsync()
    {
        Dispose();
        return ValueTask.CompletedTask;
    }

    private static bool ScanFrame(IRandomAccessSource source, ref long offset, long end, out Mp3FrameHeader header)
    {
        Span<byte> buf = stackalloc byte[4];
        while (offset + 4 <= end)
        {
            int n = source.Read(offset, buf);
            if (n < 4) break;
            if (Mp3FrameHeader.TryParse(buf, out header))
            {
                return true;
            }
            offset++;
        }
        header = default;
        return false;
    }
}
