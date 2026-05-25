using System.Buffers;
using Mediar.IO;

namespace Mediar.Containers.Flac;

/// <summary>
/// Demuxer for the native FLAC stream format (RFC 9639). Parses the
/// <c>STREAMINFO</c> metadata block, skips the rest of the metadata, then walks
/// frames by scanning for the FLAC frame sync code (<c>0xFFF8</c>..<c>0xFFFB</c>).
/// Frames are emitted as opaque packets — decoding is the consumer's
/// responsibility (codec implementations are out of scope for this layer).
/// </summary>
public sealed class FlacDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _firstFrameOffset;
    private readonly long _streamEnd;
    private readonly long _totalSamples;
    private readonly int _sampleRate;
    private readonly int _blockSize;
    private bool _disposed;

    private FlacDemuxer(
        IRandomAccessSource source,
        bool ownsSource,
        MediaTrack track,
        long firstFrameOffset,
        long streamEnd,
        long totalSamples,
        int sampleRate,
        int blockSize)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _firstFrameOffset = firstFrameOffset;
        _streamEnd = streamEnd;
        _totalSamples = totalSamples;
        _sampleRate = sampleRate;
        _blockSize = blockSize;
    }

    /// <summary>Open a FLAC file from disk.</summary>
    public static FlacDemuxer Open(string path)
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

    /// <summary>Open a FLAC stream over an arbitrary <see cref="IRandomAccessSource"/>.</summary>
    public static FlacDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> marker = stackalloc byte[4];
        if (source.Read(0, marker) != 4 ||
            marker[0] != 'f' || marker[1] != 'L' || marker[2] != 'a' || marker[3] != 'C')
        {
            throw new InvalidDataException("Missing fLaC marker.");
        }

        long pos = 4;
        StreamInfo info = default;
        bool gotStreamInfo = false;
        Span<byte> hdr = stackalloc byte[4];
        while (true)
        {
            if (source.Read(pos, hdr) != 4) throw new EndOfStreamException("Truncated metadata.");
            bool isLast = (hdr[0] & 0x80) != 0;
            int blockType = hdr[0] & 0x7F;
            int blockLen = (hdr[1] << 16) | (hdr[2] << 8) | hdr[3];
            pos += 4;

            if (blockType == 0)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent(blockLen);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, blockLen)) != blockLen)
                        throw new EndOfStreamException("Truncated STREAMINFO.");
                    info = ParseStreamInfo(buf.AsSpan(0, blockLen));
                    gotStreamInfo = true;
                }
                finally
                {
                    ArrayPool<byte>.Shared.Return(buf);
                }
            }

            pos += blockLen;
            if (isLast) break;
        }

        if (!gotStreamInfo) throw new InvalidDataException("Missing STREAMINFO.");

        // Build extradata = the whole 34-byte STREAMINFO so muxers can re-embed it.
        byte[] streamInfoBytes = new byte[34];
        source.Read(8, streamInfoBytes); // after fLaC marker + 4-byte metadata header

        var codec = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = info.SampleRate,
            Channels = info.Channels,
            BitsPerSample = info.BitsPerSample,
            ExtraData = streamInfoBytes,
        };
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, info.SampleRate),
            Codec = codec,
            DurationTicks = (long)info.TotalSamples,
        };

        return new FlacDemuxer(source, ownsSource, track, pos, source.Length,
            (long)info.TotalSamples, info.SampleRate, info.MaxBlockSize);
    }

    /// <inheritdoc/>
    public string FormatName => "flac";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => new[] { _track };

    /// <inheritdoc/>
    public TimeSpan Duration => _totalSamples > 0
        ? TimeSpan.FromSeconds((double)_totalSamples / _sampleRate)
        : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        // Frame boundary detection by scanning for sync (16 bits: 1111 1111 1111 100x).
        const int WindowSize = 64 * 1024;
        byte[] scratch = ArrayPool<byte>.Shared.Rent(WindowSize);
        try
        {
            long offset = _firstFrameOffset;
            long pts = 0;

            while (offset < _streamEnd)
            {
                cancellationToken.ThrowIfCancellationRequested();

                int wantRead = (int)Math.Min(WindowSize, _streamEnd - offset);
                int n = _source.Read(offset, scratch.AsSpan(0, wantRead));
                if (n <= 0) yield break;

                // Find first sync code in this window.
                int sync = FindSync(scratch.AsSpan(0, n), 0);
                if (sync < 0)
                {
                    // Window contained no sync. Advance and retry, keeping last byte in case
                    // a sync straddles the boundary.
                    offset += Math.Max(1, n - 1);
                    continue;
                }
                long frameStart = offset + sync;
                int searchFrom = sync + 2;
                int next = searchFrom < n ? FindSync(scratch.AsSpan(0, n), searchFrom) : -1;
                int frameLen;

                if (next > 0)
                {
                    frameLen = (int)(offset + next - frameStart);
                }
                else
                {
                    // Need to keep scanning past WindowSize. Slow path: scan until end-of-stream.
                    frameLen = await ScanToNextFrameAsync(frameStart, _streamEnd, scratch, cancellationToken).ConfigureAwait(false);
                    if (frameLen <= 0)
                    {
                        frameLen = (int)(_streamEnd - frameStart);
                    }
                }
                if (frameLen <= 0) yield break;

                var owner = MemoryPool<byte>.Shared.Rent(frameLen);
                var mem = owner.Memory[..frameLen];
                int got = await _source.ReadAsync(frameStart, mem, cancellationToken).ConfigureAwait(false);
                if (got != frameLen)
                {
                    owner.Dispose();
                    yield break;
                }

                yield return new MediaSample
                {
                    TrackIndex = 0,
                    Pts = pts,
                    Dts = pts,
                    Duration = _blockSize, // best-effort; actual block size encoded in frame header
                    IsKeyFrame = true,
                    Data = mem,
                    Owner = owner,
                };

                pts += _blockSize;
                offset = frameStart + frameLen;
            }
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(scratch);
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

    private static int FindSync(ReadOnlySpan<byte> buf, int from)
    {
        for (int i = from; i + 1 < buf.Length; i++)
        {
            if (buf[i] == 0xFF && (buf[i + 1] & 0xF8) == 0xF8 && (buf[i + 1] & 0x02) == 0)
            {
                return i;
            }
        }
        return -1;
    }

    private async Task<int> ScanToNextFrameAsync(
        long frameStart, long end, byte[] scratch, CancellationToken cancellationToken)
    {
        long pos = frameStart + 2;
        while (pos < end)
        {
            int want = (int)Math.Min(scratch.Length, end - pos);
            int n = await _source.ReadAsync(pos, scratch.AsMemory(0, want), cancellationToken).ConfigureAwait(false);
            if (n <= 0) break;
            int sync = FindSync(scratch.AsSpan(0, n), 0);
            if (sync >= 0)
            {
                return (int)(pos + sync - frameStart);
            }
            pos += Math.Max(1, n - 1);
        }
        return -1;
    }

    private static StreamInfo ParseStreamInfo(ReadOnlySpan<byte> data)
    {
        if (data.Length < 34) throw new InvalidDataException("STREAMINFO too short.");
        var br = new BitReader(data);
        int minBlock = (int)br.ReadBits(16);
        int maxBlock = (int)br.ReadBits(16);
        int minFrame = (int)br.ReadBits(24);
        int maxFrame = (int)br.ReadBits(24);
        int sampleRate = (int)br.ReadBits(20);
        int channels = (int)br.ReadBits(3) + 1;
        int bps = (int)br.ReadBits(5) + 1;
        ulong total = br.ReadBits64(36);

        _ = minBlock; _ = minFrame; _ = maxFrame;

        return new StreamInfo
        {
            MaxBlockSize = maxBlock,
            SampleRate = sampleRate,
            Channels = channels,
            BitsPerSample = bps,
            TotalSamples = total,
        };
    }

    private struct StreamInfo
    {
        public int MaxBlockSize;
        public int SampleRate;
        public int Channels;
        public int BitsPerSample;
        public ulong TotalSamples;
    }
}
