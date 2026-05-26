using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.Iff8Svx;

/// <summary>
/// Demuxer for the classic Amiga 8SVX IFF container (8-bit sampled voice).
/// </summary>
/// <remarks>
/// Parses the <c>VHDR</c> voice header (sample rate, octave count, compression),
/// the <c>BODY</c> sample chunk, and the textual <c>NAME</c> / <c>AUTH</c> /
/// <c>ANNO</c> / <c>(c) </c> metadata chunks. Uncompressed signed 8-bit and
/// Fibonacci-delta (compression=1) variants are recognized. Multi-octave 8SVX
/// files expose only the first octave.
/// </remarks>
public sealed class Iff8SvxDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly MediaMetadata _metadata;
    private readonly long _bodyOffset;
    private readonly long _bodyLength;
    private readonly int _sampleRate;
    private long _startFrame;
    private bool _disposed;

    private Iff8SvxDemuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track, MediaMetadata metadata,
        long bodyOffset, long bodyLength, int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _metadata = metadata;
        _bodyOffset = bodyOffset;
        _bodyLength = bodyLength;
        _sampleRate = sampleRate;
    }

    /// <summary>Open an 8SVX file from disk.</summary>
    public static Iff8SvxDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an 8SVX stream from a random-access source.</summary>
    public static Iff8SvxDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> hdr = stackalloc byte[12];
        if (source.Read(0, hdr) != 12) throw new InvalidDataException("File too small to be 8SVX.");
        if (hdr[0] != 'F' || hdr[1] != 'O' || hdr[2] != 'R' || hdr[3] != 'M')
            throw new InvalidDataException("Missing FORM marker.");
        if (hdr[8] != '8' || hdr[9] != 'S' || hdr[10] != 'V' || hdr[11] != 'X')
            throw new InvalidDataException("Missing 8SVX marker.");

        uint formSize = BinaryPrimitives.ReadUInt32BigEndian(hdr[4..8]);
        long formEnd = Math.Min(source.Length, 8L + formSize);

        var meta = new MediaMetadataBuilder();
        Vhdr? vhdr = null;
        long bodyOffset = -1;
        long bodyLength = 0;

        long pos = 12;
        Span<byte> chunkHdr = stackalloc byte[8];
        Span<byte> vhdrBuf = stackalloc byte[20];
        byte[] textBuf = [];
        while (pos + 8 <= formEnd)
        {
            if (source.Read(pos, chunkHdr) != 8) break;
            uint id = BinaryPrimitives.ReadUInt32BigEndian(chunkHdr[..4]);
            uint size = BinaryPrimitives.ReadUInt32BigEndian(chunkHdr[4..8]);
            pos += 8;

            if (id == ChunkIds.VHDR && size >= 20)
            {
                if (source.Read(pos, vhdrBuf) == 20) vhdr = ParseVhdr(vhdrBuf);
            }
            else if (id == ChunkIds.BODY)
            {
                bodyOffset = pos;
                bodyLength = size;
            }
            else if (size > 0 && size < 1 << 24 && IsTextChunk(id))
            {
                if (textBuf.Length < size) textBuf = new byte[size];
                if (source.Read(pos, textBuf.AsSpan(0, (int)size)) == (int)size)
                {
                    string s = Encoding.UTF8.GetString(textBuf, 0, (int)size).TrimEnd('\0', ' ');
                    switch (id)
                    {
                        case ChunkIds.NAME: meta.Set("TITLE", s); break;
                        case ChunkIds.AUTH: meta.Set("ARTIST", s); break;
                        case ChunkIds.ANNO: meta.Set("COMMENT", s); break;
                        case ChunkIds.COPY: meta.Set("COPYRIGHT", s); break;
                    }
                }
            }

            pos += size + (size & 1); // pad to even
        }

        if (vhdr is null) throw new InvalidDataException("Missing VHDR chunk.");
        if (bodyOffset < 0) throw new InvalidDataException("Missing BODY chunk.");

        var h = vhdr.Value;
        CodecId codec = h.Compression switch
        {
            0 => CodecId.PcmS8,
            1 => CodecId.Fibonacci8Svx,
            _ => CodecId.Unknown,
        };
        // For multi-octave samples only the first octave is exposed.
        long firstOctaveLen = h.OneShotHiSamples + h.RepeatHiSamples;
        if (firstOctaveLen <= 0 || firstOctaveLen > bodyLength) firstOctaveLen = bodyLength;

        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, h.SamplesPerSec),
            Codec = new AudioCodecParameters
            {
                Codec = codec, SampleRate = h.SamplesPerSec, Channels = 1, BitsPerSample = 8,
            },
            DurationTicks = firstOctaveLen,
        };

        return new Iff8SvxDemuxer(
            source, ownsSource, track, meta.Build(),
            bodyOffset, firstOctaveLen, h.SamplesPerSec);
    }

    /// <inheritdoc/>
    public string FormatName => "8svx";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;
    /// <inheritdoc/>
    public TimeSpan Duration => _sampleRate > 0
        ? TimeSpan.FromSeconds((double)_bodyLength / _sampleRate)
        : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        int framesPerPacket = Math.Max(1, _sampleRate / 100);
        long off = _bodyOffset + _startFrame;
        long end = _bodyOffset + _bodyLength;
        long pts = _startFrame;
        while (off < end)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int toRead = (int)Math.Min(framesPerPacket, end - off);
            var owner = MemoryPool<byte>.Shared.Rent(toRead);
            var mem = owner.Memory[..toRead];
            int read = await _source.ReadAsync(off, mem, cancellationToken).ConfigureAwait(false);
            if (read != toRead) { owner.Dispose(); yield break; }
            yield return new MediaSample
            {
                TrackIndex = 0, Pts = pts, Dts = pts, Duration = toRead,
                IsKeyFrame = true, Data = mem, Owner = owner,
            };
            off += toRead;
            pts += toRead;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        _startFrame = Math.Clamp((long)Math.Round(time.TotalSeconds * _sampleRate), 0, _bodyLength);
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
    private static bool IsTextChunk(uint id) =>
        id == ChunkIds.NAME || id == ChunkIds.AUTH ||
        id == ChunkIds.ANNO || id == ChunkIds.COPY;

    private static Vhdr ParseVhdr(ReadOnlySpan<byte> s) => new()
    {
        OneShotHiSamples  = BinaryPrimitives.ReadUInt32BigEndian(s[..4]),
        RepeatHiSamples   = BinaryPrimitives.ReadUInt32BigEndian(s[4..8]),
        SamplesPerHiCycle = BinaryPrimitives.ReadUInt32BigEndian(s[8..12]),
        SamplesPerSec     = BinaryPrimitives.ReadUInt16BigEndian(s[12..14]),
        Octaves           = s[14],
        Compression       = s[15],
        Volume            = BinaryPrimitives.ReadUInt32BigEndian(s[16..20]),
    };

    private readonly struct Vhdr
    {
        public uint OneShotHiSamples { get; init; }
        public uint RepeatHiSamples { get; init; }
        public uint SamplesPerHiCycle { get; init; }
        public ushort SamplesPerSec { get; init; }
        public byte Octaves { get; init; }
        public byte Compression { get; init; }
        public uint Volume { get; init; }
    }

    private static class ChunkIds
    {
        public const uint VHDR = 0x56484452;
        public const uint BODY = 0x424F4459;
        public const uint NAME = 0x4E414D45;
        public const uint AUTH = 0x41555448;
        public const uint ANNO = 0x414E4E4F;
        public const uint COPY = 0x28632920; // '(c) '
    }
}
