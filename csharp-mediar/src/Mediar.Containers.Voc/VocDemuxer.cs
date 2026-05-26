using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.Voc;

/// <summary>
/// Demuxer for Creative Labs <c>.voc</c> files (Creative Voice File).
/// </summary>
/// <remarks>
/// Recognized block types: 0 (terminator), 1 (sound data v1.x), 2 (continuation),
/// 3 (silence), 6 (repeat), 7 (end-repeat) and 9 (sound data v1.20+).
/// PCM µ-law / A-law / unsigned 8-bit / signed 16-bit LE codecs are mapped to
/// the corresponding <see cref="CodecId"/> values. Compressed Creative ADPCM
/// blocks are passed through as <see cref="CodecId.CreativeAdpcm"/>.
/// </remarks>
public sealed class VocDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly Block[] _blocks;
    private readonly int _sampleRate;
    private readonly int _bytesPerFrame;
    private int _startBlock;
    private bool _disposed;

    private VocDemuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track, Block[] blocks,
        int sampleRate, int bytesPerFrame)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _blocks = blocks;
        _sampleRate = sampleRate;
        _bytesPerFrame = bytesPerFrame;
    }

    /// <summary>Open a VOC file from disk.</summary>
    public static VocDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open a VOC stream from a random-access source.</summary>
    public static VocDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);
        Span<byte> sig = stackalloc byte[26];
        if (source.Read(0, sig) != 26) throw new InvalidDataException("File too small to be VOC.");
        // 19-byte ASCII signature + 0x1A
        ReadOnlySpan<byte> magic = "Creative Voice File\x1A"u8;
        if (!sig[..20].SequenceEqual(magic))
            throw new InvalidDataException("Missing 'Creative Voice File' signature.");
        ushort dataStart = BinaryPrimitives.ReadUInt16LittleEndian(sig.Slice(20, 2));

        long pos = dataStart;
        long len = source.Length;
        var blocks = new List<Block>();
        int sampleRate = 0;
        int channels = 1;
        int bitsPerSample = 8;
        CodecId codec = CodecId.Unknown;

        Span<byte> blockHdr = stackalloc byte[4];
        Span<byte> meta2 = stackalloc byte[2];
        Span<byte> sil = stackalloc byte[3];
        Span<byte> h12 = stackalloc byte[12];
        while (pos < len)
        {
            if (source.Read(pos, blockHdr[..1]) != 1) break;
            byte blockType = blockHdr[0];
            if (blockType == 0) break; // terminator
            if (pos + 4 > len) break;
            if (source.Read(pos + 1, blockHdr[..3]) != 3) break;
            uint size = (uint)(blockHdr[0] | (blockHdr[1] << 8) | (blockHdr[2] << 16));
            long payload = pos + 4;
            long next = payload + size;

            switch (blockType)
            {
                case 1: // sound data v1.x
                    if (size >= 2 && source.Read(payload, meta2) == 2)
                    {
                        byte divisor = meta2[0];
                        byte cid = meta2[1];
                        sampleRate = 1_000_000 / Math.Max(1, 256 - divisor);
                        (codec, bitsPerSample, channels) = MapV1Codec(cid);
                        blocks.Add(new Block(payload + 2, size - 2));
                    }
                    break;
                case 2: // continuation
                    blocks.Add(new Block(payload, size));
                    break;
                case 3: // silence — emitted as a zero-PCM block
                    if (size >= 3 && source.Read(payload, sil) == 3)
                    {
                        ushort frames = BinaryPrimitives.ReadUInt16LittleEndian(sil[..2]);
                        blocks.Add(new Block(-1, frames)); // negative offset = silence
                    }
                    break;
                case 9: // sound data v1.20+
                    if (size >= 12 && source.Read(payload, h12) == 12)
                    {
                        sampleRate = (int)BinaryPrimitives.ReadUInt32LittleEndian(h12[..4]);
                        bitsPerSample = h12[4];
                        channels = h12[5];
                        ushort codecId = BinaryPrimitives.ReadUInt16LittleEndian(h12.Slice(6, 2));
                        codec = MapV9Codec(codecId, bitsPerSample);
                        blocks.Add(new Block(payload + 12, size - 12));
                    }
                    break;
                default:
                    // ignore: 4 (marker), 5 (text), 6/7 (loop), 8 (extended)
                    break;
            }
            pos = next;
        }

        if (codec == CodecId.Unknown && bitsPerSample == 8) codec = CodecId.PcmU8;

        int bytesPerFrame = ((bitsPerSample + 7) / 8) * Math.Max(1, channels);
        long totalFrames = 0;
        foreach (var b in blocks)
        {
            if (b.Offset < 0) { totalFrames += b.Length; continue; }
            totalFrames += b.Length / Math.Max(1, bytesPerFrame);
        }

        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, Math.Max(1, sampleRate)),
            Codec = new AudioCodecParameters
            {
                Codec = codec, SampleRate = sampleRate,
                Channels = Math.Max(1, channels), BitsPerSample = bitsPerSample,
            },
            DurationTicks = totalFrames,
        };
        return new VocDemuxer(source, ownsSource, track, [.. blocks], sampleRate, bytesPerFrame);
    }

    /// <inheritdoc/>
    public string FormatName => "voc";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => MediaMetadata.Empty;
    /// <inheritdoc/>
    public TimeSpan Duration => _sampleRate > 0
        ? TimeSpan.FromSeconds((double)_track.DurationTicks / _sampleRate)
        : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long pts = 0;
        for (int i = _startBlock; i < _blocks.Length; i++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var b = _blocks[i];
            if (b.Offset < 0)
            {
                // synthesize a silence block (zero bytes of duration `b.Length` samples)
                int sz = (int)Math.Max(1, b.Length * _bytesPerFrame);
                var owner = MemoryPool<byte>.Shared.Rent(sz);
                owner.Memory[..sz].Span.Clear();
                yield return new MediaSample
                {
                    TrackIndex = 0, Pts = pts, Dts = pts, Duration = (int)b.Length,
                    IsKeyFrame = true, Data = owner.Memory[..sz], Owner = owner,
                };
                pts += b.Length;
                continue;
            }
            int packet = Math.Max(_bytesPerFrame, _sampleRate / 100 * _bytesPerFrame);
            long off = b.Offset;
            long end = b.Offset + b.Length;
            while (off < end)
            {
                int toRead = (int)Math.Min(packet, end - off);
                var owner = MemoryPool<byte>.Shared.Rent(toRead);
                var mem = owner.Memory[..toRead];
                int read = await _source.ReadAsync(off, mem, cancellationToken).ConfigureAwait(false);
                if (read != toRead) { owner.Dispose(); yield break; }
                int frames = toRead / Math.Max(1, _bytesPerFrame);
                yield return new MediaSample
                {
                    TrackIndex = 0, Pts = pts, Dts = pts, Duration = frames,
                    IsKeyFrame = true, Data = mem, Owner = owner,
                };
                off += toRead;
                pts += frames;
            }
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        // Coarse seek: snap to nearest block start.
        long target = (long)Math.Round(time.TotalSeconds * _sampleRate);
        long cum = 0;
        _startBlock = 0;
        for (int i = 0; i < _blocks.Length; i++)
        {
            var b = _blocks[i];
            long frames = b.Offset < 0 ? b.Length : b.Length / Math.Max(1, _bytesPerFrame);
            if (cum + frames >= target) { _startBlock = i; return ValueTask.CompletedTask; }
            cum += frames;
        }
        _startBlock = _blocks.Length;
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

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static (CodecId codec, int bits, int channels) MapV1Codec(byte id) => id switch
    {
        0 => (CodecId.PcmU8, 8, 1),
        1 or 2 or 3 => (CodecId.CreativeAdpcm, 8, 1),
        4 => (CodecId.PcmS16Le, 16, 1),
        6 => (CodecId.G711ALaw, 8, 1),
        7 => (CodecId.G711MuLaw, 8, 1),
        _ => (CodecId.Unknown, 8, 1),
    };

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static CodecId MapV9Codec(ushort id, int bits) => id switch
    {
        0 => CodecId.PcmU8,
        1 or 2 or 3 => CodecId.CreativeAdpcm,
        4 => CodecId.PcmS16Le,
        6 => CodecId.G711ALaw,
        7 => CodecId.G711MuLaw,
        _ => CodecId.Unknown,
    };

    private readonly record struct Block(long Offset, long Length);
}
