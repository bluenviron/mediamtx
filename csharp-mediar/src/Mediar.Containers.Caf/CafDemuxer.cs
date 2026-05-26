using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.Caf;

/// <summary>
/// Demuxer for Apple's Core Audio Format (<c>.caf</c>). CAF is a chunked,
/// 64-bit-clean container that can hold most Apple audio codecs. This
/// implementation parses the Audio Description (<c>desc</c>), Audio Data
/// (<c>data</c>), Packet Table (<c>pakt</c>) and Information
/// (<c>info</c>) chunks and exposes the audio as passthrough samples.
/// </summary>
/// <remarks>
/// Recognized codecs: <c>lpcm</c> (signed/unsigned, big/little-endian, 8/16/24/32-bit
/// integer + 32-bit float), <c>alac</c>, <c>aac </c>, <c>ulaw</c>, <c>alaw</c>, <c>opus</c>.
/// Variable-bitrate codecs (ALAC, AAC, Opus) require the <c>pakt</c> chunk to
/// recover packet boundaries.
/// </remarks>
public sealed class CafDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly MediaMetadata _metadata;
    private readonly long _dataOffset;
    private readonly long _dataLength;
    private readonly int _bytesPerFrame;
    private readonly int _sampleRate;
    private readonly long[]? _variablePacketTable;
    private readonly int _variablePacketFrames;
    private long _startFrame;
    private bool _disposed;

    private CafDemuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track, MediaMetadata metadata,
        long dataOffset, long dataLength,
        int bytesPerFrame, int sampleRate,
        long[]? variablePacketTable, int variablePacketFrames)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _metadata = metadata;
        _dataOffset = dataOffset;
        _dataLength = dataLength;
        _bytesPerFrame = bytesPerFrame;
        _sampleRate = sampleRate;
        _variablePacketTable = variablePacketTable;
        _variablePacketFrames = variablePacketFrames;
    }

    /// <summary>Open a CAF file from disk.</summary>
    public static CafDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open a CAF stream from a random-access source.</summary>
    public static CafDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> hdr = stackalloc byte[8];
        if (source.Read(0, hdr) != 8)
            throw new InvalidDataException("File too small to be CAF.");
        if (hdr[0] != (byte)'c' || hdr[1] != (byte)'a' || hdr[2] != (byte)'f' || hdr[3] != (byte)'f')
            throw new InvalidDataException("Missing 'caff' marker.");

        var meta = new MediaMetadataBuilder();
        AudioDesc? desc = null;
        long dataStart = -1;
        long dataLength = 0;
        long[]? packetTable = null;
        int packetFrames = 0;

        long pos = 8;
        Span<byte> chunkHdr = stackalloc byte[12];
        long len = source.Length;
        while (pos + 12 <= len)
        {
            if (source.Read(pos, chunkHdr) != 12) break;
            uint id = BinaryPrimitives.ReadUInt32BigEndian(chunkHdr[..4]);
            long size = BinaryPrimitives.ReadInt64BigEndian(chunkHdr[4..12]);
            pos += 12;
            if (size < 0) size = len - pos;

            if (id == ChunkIds.Desc && size >= 32)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)size)) == (int)size)
                        desc = ParseDesc(buf.AsSpan(0, (int)size));
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }
            else if (id == ChunkIds.Data)
            {
                dataStart = pos + 4; // skip edit-count uint32
                dataLength = size - 4;
            }
            else if (id == ChunkIds.Info && size > 4 && size < 1 << 24)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)size)) == (int)size)
                        ParseInfo(buf.AsSpan(4, (int)size - 4), meta);
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }
            else if (id == ChunkIds.Pakt && size >= 24 && size < 1 << 24)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)size)) == (int)size)
                        packetTable = ParsePacketTable(buf.AsSpan(0, (int)size), out packetFrames);
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }

            pos += size;
        }

        if (desc is null) throw new InvalidDataException("Missing 'desc' chunk.");
        if (dataStart < 0) throw new InvalidDataException("Missing 'data' chunk.");

        var d = desc.Value;
        int bytesPerFrame = d.BytesPerPacket > 0 && d.FramesPerPacket > 0
            ? (int)(d.BytesPerPacket / d.FramesPerPacket)
            : 0;

        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, d.SampleRate),
            Codec = new AudioCodecParameters
            {
                Codec = d.Codec,
                SampleRate = d.SampleRate,
                Channels = (int)d.ChannelsPerFrame,
                BitsPerSample = (int)d.BitsPerChannel,
            },
            DurationTicks = packetFrames > 0 ? packetFrames :
                (bytesPerFrame > 0 ? dataLength / bytesPerFrame : 0),
        };

        return new CafDemuxer(
            source, ownsSource, track, meta.Build(),
            dataStart, dataLength,
            bytesPerFrame, d.SampleRate,
            packetTable, (int)d.FramesPerPacket);
    }

    /// <inheritdoc/>
    public string FormatName => "caf";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;
    /// <inheritdoc/>
    public TimeSpan Duration =>
        _bytesPerFrame > 0 && _sampleRate > 0
            ? TimeSpan.FromSeconds((double)(_dataLength / _bytesPerFrame) / _sampleRate)
            : _track.DurationTicks > 0
                ? TimeSpan.FromSeconds((double)_track.DurationTicks / _sampleRate)
                : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (_variablePacketTable is { } table)
        {
            long offset = _dataOffset;
            long pts = 0;
            foreach (long sz in table)
            {
                cancellationToken.ThrowIfCancellationRequested();
                if (sz <= 0) continue;
                var owner = MemoryPool<byte>.Shared.Rent((int)sz);
                var mem = owner.Memory[..(int)sz];
                if (await _source.ReadAsync(offset, mem, cancellationToken).ConfigureAwait(false) != sz)
                {
                    owner.Dispose();
                    yield break;
                }
                yield return new MediaSample
                {
                    TrackIndex = 0, Pts = pts, Dts = pts,
                    Duration = _variablePacketFrames > 0 ? _variablePacketFrames : 1,
                    IsKeyFrame = true, Data = mem, Owner = owner,
                };
                offset += sz;
                pts += _variablePacketFrames > 0 ? _variablePacketFrames : 1;
            }
            yield break;
        }

        if (_bytesPerFrame <= 0) yield break;
        int framesPerPacket = Math.Max(1, _sampleRate / 100);
        int bytesPerPacket = framesPerPacket * _bytesPerFrame;
        long off = _dataOffset + _startFrame * _bytesPerFrame;
        long end = _dataOffset + _dataLength;
        long ptsFrames = _startFrame;
        while (off < end)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int toRead = (int)Math.Min(bytesPerPacket, end - off);
            int frames = toRead / _bytesPerFrame;
            if (frames == 0) yield break;
            var owner = MemoryPool<byte>.Shared.Rent(frames * _bytesPerFrame);
            var mem = owner.Memory[..(frames * _bytesPerFrame)];
            int read = await _source.ReadAsync(off, mem, cancellationToken).ConfigureAwait(false);
            if (read != mem.Length) { owner.Dispose(); yield break; }
            yield return new MediaSample
            {
                TrackIndex = 0, Pts = ptsFrames, Dts = ptsFrames,
                Duration = frames, IsKeyFrame = true, Data = mem, Owner = owner,
            };
            off += frames * _bytesPerFrame;
            ptsFrames += frames;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        if (_bytesPerFrame > 0)
            _startFrame = Math.Max(0, (long)Math.Round(time.TotalSeconds * _sampleRate));
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

    private static AudioDesc ParseDesc(ReadOnlySpan<byte> data)
    {
        double sampleRate = BitConverter.Int64BitsToDouble(
            BinaryPrimitives.ReadInt64BigEndian(data[..8]));
        uint formatId = BinaryPrimitives.ReadUInt32BigEndian(data[8..12]);
        uint flags = BinaryPrimitives.ReadUInt32BigEndian(data[12..16]);
        uint bytesPerPacket = BinaryPrimitives.ReadUInt32BigEndian(data[16..20]);
        uint framesPerPacket = BinaryPrimitives.ReadUInt32BigEndian(data[20..24]);
        uint channels = BinaryPrimitives.ReadUInt32BigEndian(data[24..28]);
        uint bits = BinaryPrimitives.ReadUInt32BigEndian(data[28..32]);
        return new AudioDesc
        {
            SampleRate = (int)Math.Round(sampleRate),
            FormatId = formatId, Flags = flags,
            BytesPerPacket = bytesPerPacket,
            FramesPerPacket = framesPerPacket,
            ChannelsPerFrame = channels,
            BitsPerChannel = bits,
            Codec = MapCodec(formatId, flags, bits),
        };
    }

    private static CodecId MapCodec(uint id, uint flags, uint bits) => id switch
    {
        0x6C70636D => bits switch                                  // 'lpcm'
        {
            8  => (flags & 0x04) != 0 ? CodecId.PcmS8 : CodecId.PcmU8,
            16 => (flags & 0x02) != 0 ? CodecId.PcmS16Le : CodecId.PcmS16Be,
            24 => CodecId.PcmS24Le,
            32 => (flags & 0x01) != 0 ? CodecId.PcmF32Le : CodecId.PcmS32Le,
            _  => CodecId.Unknown,
        },
        0x616C6163 => CodecId.Alac,        // 'alac'
        0x61616320 => CodecId.Aac,         // 'aac '
        0x756C6177 => CodecId.G711MuLaw,   // 'ulaw'
        0x616C6177 => CodecId.G711ALaw,    // 'alaw'
        0x6F707573 => CodecId.Opus,        // 'opus'
        _          => CodecId.Unknown,
    };

    private static long[]? ParsePacketTable(ReadOnlySpan<byte> data, out int framesPerPacket)
    {
        long numberPackets = BinaryPrimitives.ReadInt64BigEndian(data[..8]);
        long numberFrames  = BinaryPrimitives.ReadInt64BigEndian(data[8..16]);
        framesPerPacket = numberPackets > 0 ? (int)(numberFrames / numberPackets) : 0;
        if (numberPackets <= 0 || numberPackets > int.MaxValue) return null;
        var table = new long[numberPackets];
        int p = 24;
        for (int i = 0; i < numberPackets && p < data.Length; i++)
        {
            long v = 0;
            while (p < data.Length)
            {
                byte b = data[p++];
                v = (v << 7) | (uint)(b & 0x7F);
                if ((b & 0x80) == 0) break;
            }
            table[i] = v;
        }
        return table;
    }

    private static void ParseInfo(ReadOnlySpan<byte> data, MediaMetadataBuilder meta)
    {
        int i = 0;
        while (i < data.Length)
        {
            int keyStart = i;
            while (i < data.Length && data[i] != 0) i++;
            if (i >= data.Length) break;
            string key = Encoding.UTF8.GetString(data[keyStart..i]);
            i++;
            int valStart = i;
            while (i < data.Length && data[i] != 0) i++;
            string value = Encoding.UTF8.GetString(data[valStart..i]);
            if (i < data.Length) i++;
            meta.Set(key, value);
        }
    }

    private readonly struct AudioDesc
    {
        public int SampleRate { get; init; }
        public uint FormatId { get; init; }
        public uint Flags { get; init; }
        public uint BytesPerPacket { get; init; }
        public uint FramesPerPacket { get; init; }
        public uint ChannelsPerFrame { get; init; }
        public uint BitsPerChannel { get; init; }
        public CodecId Codec { get; init; }
    }

    private static class ChunkIds
    {
        public const uint Desc = 0x64657363; // 'desc'
        public const uint Data = 0x64617461; // 'data'
        public const uint Info = 0x696E666F; // 'info'
        public const uint Pakt = 0x70616B74; // 'pakt'
    }
}
