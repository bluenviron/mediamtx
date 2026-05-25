using System.Buffers;
using Mediar.IO;

namespace Mediar.Containers.Adts;

/// <summary>
/// ADTS demuxer for raw <c>.aac</c> files. Walks the stream by ADTS sync code
/// and emits each AAC access unit as a <see cref="MediaSample"/> with the ADTS
/// header stripped — consumers expect raw AAC payloads, and the muxer can
/// regenerate the ADTS header on the way out.
/// </summary>
public sealed class AdtsDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _firstFrameOffset;
    private readonly int _samplesPerFrame;
    private readonly int _sampleRate;
    private bool _disposed;

    private AdtsDemuxer(IRandomAccessSource source, bool ownsSource, MediaTrack track,
        long firstFrameOffset, int samplesPerFrame, int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _firstFrameOffset = firstFrameOffset;
        _samplesPerFrame = samplesPerFrame;
        _sampleRate = sampleRate;
    }

    /// <summary>Open from disk.</summary>
    public static AdtsDemuxer Open(string path)
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

    /// <summary>Open over an existing random-access source.</summary>
    public static AdtsDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        // Find the first sync, skipping any leading ID3v2 tag.
        long start = SkipId3v2(source);
        Span<byte> header = stackalloc byte[9];
        if (source.Read(start, header) < 7 || !AdtsHeader.TryParse(header, out var h))
        {
            throw new InvalidDataException("No ADTS sync at start of stream.");
        }

        int samplesPerFrame = 1024 * (h.NumberOfRawDataBlocks + 1);
        var codec = new AudioCodecParameters
        {
            Codec = CodecId.Aac,
            SampleRate = h.SampleRate,
            Channels = h.ChannelConfig == 7 ? 8 : h.ChannelConfig,
        };
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, h.SampleRate),
            Codec = codec,
            DurationTicks = 0,
        };

        return new AdtsDemuxer(source, ownsSource, track, start, samplesPerFrame, h.SampleRate);
    }

    /// <inheritdoc/>
    public string FormatName => "aac";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => new[] { _track };

    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long offset = _firstFrameOffset;
        long end = _source.Length;
        long pts = 0;
        byte[] hdr = new byte[9];

        while (offset + 7 <= end)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (_source.Read(offset, hdr.AsSpan(0, 7)) != 7) yield break;
            if (!AdtsHeader.TryParse(hdr, out var h)) yield break;

            int payloadOffset = h.HeaderSize;
            int payloadLen = h.FrameSize - payloadOffset;
            if (payloadLen <= 0 || offset + h.FrameSize > end) yield break;

            var owner = MemoryPool<byte>.Shared.Rent(payloadLen);
            var mem = owner.Memory[..payloadLen];
            int got = await _source.ReadAsync(offset + payloadOffset, mem, cancellationToken).ConfigureAwait(false);
            if (got != payloadLen)
            {
                owner.Dispose();
                yield break;
            }

            yield return new MediaSample
            {
                TrackIndex = 0,
                Pts = pts,
                Dts = pts,
                Duration = _samplesPerFrame,
                IsKeyFrame = true,
                Data = mem,
                Owner = owner,
            };

            pts += _samplesPerFrame;
            offset += h.FrameSize;
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

    private static long SkipId3v2(IRandomAccessSource source)
    {
        Span<byte> head = stackalloc byte[10];
        if (source.Read(0, head) != 10) return 0;
        if (head[0] != 'I' || head[1] != 'D' || head[2] != '3') return 0;
        // Synchsafe 28-bit size.
        int size = ((head[6] & 0x7F) << 21) |
                   ((head[7] & 0x7F) << 14) |
                   ((head[8] & 0x7F) << 7) |
                   (head[9] & 0x7F);
        return 10L + size;
    }
}
