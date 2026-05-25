using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.Avi;

/// <summary>
/// Demuxer for RIFF/AVI files (and OpenDML AVI 2.0 with multiple <c>RIFF AVIX</c>
/// segments). Parses the <c>hdrl</c> stream-header list, the <c>idx1</c> classic
/// index (when present), and the <c>LIST INFO</c> metadata chunk; emits samples
/// from the <c>movi</c> body in stored order.
/// </summary>
/// <remarks>
/// AVI is a RIFF container and is one of the older interleaved A/V formats in
/// active use. We support the common codecs by direct fourcc / wFormatTag
/// mapping but do not implement bitstream decoding here — samples are emitted
/// verbatim. Files without an <c>idx1</c> index are demuxed by linear walk of
/// the <c>movi</c> list.
/// </remarks>
public sealed class AviDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack[] _tracks;
    private readonly StreamState[] _streams;
    private readonly MediaMetadata _metadata;
    private readonly long _moviStart;
    private readonly long _moviEnd;
    private readonly TimeSpan _duration;
    private readonly IndexEntry[]? _index;
    private long _startIndex;
    private bool _disposed;

    private AviDemuxer(
        IRandomAccessSource source,
        bool ownsSource,
        MediaTrack[] tracks,
        StreamState[] streams,
        MediaMetadata metadata,
        long moviStart,
        long moviEnd,
        TimeSpan duration,
        IndexEntry[]? index)
    {
        _source = source;
        _ownsSource = ownsSource;
        _tracks = tracks;
        _streams = streams;
        _metadata = metadata;
        _moviStart = moviStart;
        _moviEnd = moviEnd;
        _duration = duration;
        _index = index;
    }

    /// <summary>Open an AVI file from disk.</summary>
    public static AviDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an AVI stream from an arbitrary <see cref="IRandomAccessSource"/>.</summary>
    public static AviDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> hdr = stackalloc byte[12];
        if (source.Read(0, hdr) != 12) throw new InvalidDataException("File too small to be RIFF/AVI.");
        if (hdr[0] != 'R' || hdr[1] != 'I' || hdr[2] != 'F' || hdr[3] != 'F')
            throw new InvalidDataException("Missing RIFF marker.");
        if (hdr[8] != 'A' || hdr[9] != 'V' || hdr[10] != 'I' || hdr[11] != ' ')
            throw new InvalidDataException("Missing AVI marker.");

        uint riffSize = BinaryPrimitives.ReadUInt32LittleEndian(hdr[4..8]);
        long riffEnd = Math.Min(source.Length, 8L + riffSize);

        var meta = new MediaMetadataBuilder();
        var streams = new List<StreamState>();
        long moviStart = -1, moviEnd = -1;
        long idx1Offset = -1;
        uint idx1Size = 0;
        uint microsecPerFrame = 0;
        uint totalFrames = 0;

        long pos = 12;
        Span<byte> chunkHdr = stackalloc byte[8];
        Span<byte> kindBuf = stackalloc byte[4];
        while (pos + 8 <= riffEnd)
        {
            if (source.Read(pos, chunkHdr) != 8) break;
            uint id = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[..4]);
            uint size = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[4..]);
            pos += 8;

            if (id == FourCcs.LIST)
            {
                if (source.Read(pos, kindBuf) != 4) break;
                uint listKind = BinaryPrimitives.ReadUInt32LittleEndian(kindBuf);

                if (listKind == FourCcs.hdrl)
                {
                    ParseHdrl(source, pos + 4, size - 4, streams, ref microsecPerFrame, ref totalFrames);
                }
                else if (listKind == FourCcs.movi)
                {
                    moviStart = pos + 4;
                    moviEnd = pos + size;
                }
                else if (listKind == FourCcs.INFO)
                {
                    ParseInfo(source, pos + 4, size - 4, meta);
                }
            }
            else if (id == FourCcs.idx1)
            {
                idx1Offset = pos;
                idx1Size = size;
            }

            pos += size + (size & 1);
        }

        if (moviStart < 0) throw new InvalidDataException("Missing movi chunk.");
        if (streams.Count == 0) throw new InvalidDataException("AVI file declares no streams.");

        // Build the public track list.
        var tracks = new MediaTrack[streams.Count];
        var states = streams.ToArray();
        for (int i = 0; i < states.Length; i++)
        {
            var s = states[i];
            tracks[i] = new MediaTrack
            {
                Index = i,
                Id = (uint)i,
                TimeBase = s.TimeBase,
                Codec = s.CodecParameters,
                Language = "und",
                DurationTicks = s.Length > 0 ? s.Length : -1,
            };
        }

        // Parse idx1 if present — gives us per-sample offsets and lengths
        // including keyframe flags.
        IndexEntry[]? index = null;
        if (idx1Offset > 0 && idx1Size > 0)
        {
            index = ParseIdx1(source, idx1Offset, idx1Size, moviStart, states);
        }

        TimeSpan duration = TimeSpan.Zero;
        if (microsecPerFrame > 0 && totalFrames > 0)
        {
            duration = TimeSpan.FromMilliseconds((double)microsecPerFrame * totalFrames / 1000.0);
        }

        return new AviDemuxer(source, ownsSource, tracks, states, meta.Build(), moviStart, moviEnd, duration, index);
    }

    /// <inheritdoc/>
    public string FormatName => "avi";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => _tracks;

    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public TimeSpan Duration => _duration;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (_index is { } idx)
        {
            for (long i = _startIndex; i < idx.LongLength; i++)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var e = idx[i];
                var sample = await ReadSampleAsync(e.StreamIndex, e.Offset, e.Length, e.IsKeyFrame, cancellationToken).ConfigureAwait(false);
                if (sample is not null) yield return sample;
            }
            yield break;
        }

        // Linear walk fallback — used for AVI files without idx1 (rare for
        // well-formed muxers but legal). We treat each ##xx chunk inside movi
        // as one sample for the corresponding stream.
        long pos = _moviStart;
        Span<byte> chunkHdr = stackalloc byte[8];
        long[] sampleCounters = new long[_streams.Length];
        while (pos + 8 <= _moviEnd)
        {
            cancellationToken.ThrowIfCancellationRequested();
            byte[] hdrArr = ArrayPool<byte>.Shared.Rent(8);
            try
            {
                if (_source.Read(pos, hdrArr.AsSpan(0, 8)) != 8) yield break;
                uint id = BinaryPrimitives.ReadUInt32LittleEndian(hdrArr.AsSpan(0, 4));
                uint size = BinaryPrimitives.ReadUInt32LittleEndian(hdrArr.AsSpan(4, 4));
                pos += 8;
                if (id == FourCcs.LIST)
                {
                    pos += 4; // skip list type
                    continue;
                }
                if (TryParseSampleChunkId(id, out int streamIndex))
                {
                    long offset = pos;
                    pos += size + (size & 1);
                    var sample = await ReadSampleAsync(streamIndex, offset, size, isKeyFrame: true, cancellationToken).ConfigureAwait(false);
                    if (sample is not null)
                    {
                        sampleCounters[streamIndex]++;
                        yield return sample;
                    }
                }
                else
                {
                    pos += size + (size & 1);
                }
            }
            finally
            {
                ArrayPool<byte>.Shared.Return(hdrArr);
            }
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        if (_index is null) return ValueTask.CompletedTask;
        if (time < TimeSpan.Zero) time = TimeSpan.Zero;

        // Find first index entry whose stream-clock time is >= target.
        // For video streams we snap back to the nearest preceding keyframe.
        var idx = _index;
        long bestIndex = 0;
        double target = time.TotalSeconds;
        for (long i = 0; i < idx.LongLength; i++)
        {
            var e = idx[i];
            var s = _streams[e.StreamIndex];
            double t = s.TimeOf(e.SampleInStream);
            if (t <= target) bestIndex = i;
            else break;
        }
        // Walk backwards to keyframe.
        for (long i = bestIndex; i >= 0; i--)
        {
            var e = idx[i];
            var s = _streams[e.StreamIndex];
            if (s.IsVideo)
            {
                if (e.IsKeyFrame) { bestIndex = i; break; }
            }
            else { bestIndex = i; break; }
        }
        _startIndex = bestIndex;
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

    // -----------------------------------------------------------------------
    // hdrl / strl parsing
    // -----------------------------------------------------------------------

    private static void ParseHdrl(
        IRandomAccessSource source, long offset, uint size,
        List<StreamState> streams, ref uint microsecPerFrame, ref uint totalFrames)
    {
        long end = offset + size;
        long pos = offset;
        Span<byte> chunkHdr = stackalloc byte[8];
        Span<byte> avih = stackalloc byte[64];
        Span<byte> kindBuf = stackalloc byte[4];
        while (pos + 8 <= end)
        {
            if (source.Read(pos, chunkHdr) != 8) return;
            uint id = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[..4]);
            uint csize = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[4..]);
            pos += 8;

            if (id == FourCcs.avih)
            {
                int avihLen = Math.Min((int)csize, avih.Length);
                int read = source.Read(pos, avih[..avihLen]);
                if (read >= 24)
                {
                    microsecPerFrame = BinaryPrimitives.ReadUInt32LittleEndian(avih[..4]);
                    // bytes 4..7 = MaxBytesPerSec, 8..11 = PaddingGranularity,
                    // 12..15 = Flags, 16..19 = TotalFrames
                    totalFrames = BinaryPrimitives.ReadUInt32LittleEndian(avih[16..20]);
                }
            }
            else if (id == FourCcs.LIST)
            {
                if (source.Read(pos, kindBuf) != 4) return;
                uint listKind = BinaryPrimitives.ReadUInt32LittleEndian(kindBuf);
                if (listKind == FourCcs.strl)
                {
                    var s = ParseStrl(source, pos + 4, csize - 4, (byte)streams.Count);
                    if (s is not null) streams.Add(s);
                }
            }
            pos += csize + (csize & 1);
        }
    }

    private static StreamState? ParseStrl(IRandomAccessSource source, long offset, uint size, byte streamIndex)
    {
        long end = offset + size;
        long pos = offset;
        StreamHeader? strh = null;
        byte[]? strf = null;
        string? streamName = null;
        Span<byte> chunkHdr = stackalloc byte[8];
        while (pos + 8 <= end)
        {
            if (source.Read(pos, chunkHdr) != 8) return null;
            uint id = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[..4]);
            uint csize = BinaryPrimitives.ReadUInt32LittleEndian(chunkHdr[4..]);
            pos += 8;

            if (id == FourCcs.strh && csize >= 32)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)csize);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)csize)) == (int)csize)
                    {
                        strh = ParseStrh(buf.AsSpan(0, (int)csize));
                    }
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }
            else if (id == FourCcs.strf && csize > 0)
            {
                strf = new byte[csize];
                source.Read(pos, strf);
            }
            else if (id == FourCcs.strn && csize > 0 && csize < 256)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)csize);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)csize)) == (int)csize)
                    {
                        int z = Array.IndexOf(buf, (byte)0, 0, (int)csize);
                        if (z < 0) z = (int)csize;
                        streamName = Encoding.Latin1.GetString(buf, 0, z);
                    }
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }
            pos += csize + (csize & 1);
        }

        if (strh is null) return null;
        return BuildStream(strh.Value, strf, streamName, streamIndex);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static StreamHeader ParseStrh(ReadOnlySpan<byte> s)
    {
        return new StreamHeader
        {
            Type = BinaryPrimitives.ReadUInt32LittleEndian(s[..4]),
            Handler = BinaryPrimitives.ReadUInt32LittleEndian(s[4..8]),
            Flags = BinaryPrimitives.ReadUInt32LittleEndian(s[8..12]),
            Priority = BinaryPrimitives.ReadUInt16LittleEndian(s[12..14]),
            Language = BinaryPrimitives.ReadUInt16LittleEndian(s[14..16]),
            InitialFrames = BinaryPrimitives.ReadUInt32LittleEndian(s[16..20]),
            Scale = BinaryPrimitives.ReadUInt32LittleEndian(s[20..24]),
            Rate = BinaryPrimitives.ReadUInt32LittleEndian(s[24..28]),
            Start = BinaryPrimitives.ReadUInt32LittleEndian(s[28..32]),
            Length = s.Length >= 36 ? BinaryPrimitives.ReadUInt32LittleEndian(s[32..36]) : 0,
            SampleSize = s.Length >= 44 ? BinaryPrimitives.ReadUInt32LittleEndian(s[40..44]) : 0,
        };
    }

    private static StreamState? BuildStream(StreamHeader strh, byte[]? strf, string? name, byte index)
    {
        var timeBase = strh.Scale > 0 && strh.Rate > 0
            ? new Rational((int)strh.Scale, (int)strh.Rate)
            : new Rational(1, 1);

        switch (strh.Type)
        {
            case FourCcs.vids:
            {
                int width = 0, height = 0;
                CodecId codec = MapVideoCodec(strh.Handler);
                if (strf is { Length: >= 40 })
                {
                    // BITMAPINFOHEADER
                    width = BinaryPrimitives.ReadInt32LittleEndian(strf.AsSpan(4, 4));
                    height = BinaryPrimitives.ReadInt32LittleEndian(strf.AsSpan(8, 4));
                    uint compression = BinaryPrimitives.ReadUInt32LittleEndian(strf.AsSpan(16, 4));
                    if (codec == CodecId.Unknown) codec = MapVideoCodec(compression);
                }
                var p = new VideoCodecParameters
                {
                    Codec = codec,
                    Width = width,
                    Height = Math.Abs(height),
                    FrameRate = strh.Scale > 0
                        ? new Rational((int)strh.Rate, (int)strh.Scale)
                        : default,
                };
                return new StreamState((byte)index, IsVideo: true, p, timeBase, strh.Length, strh.SampleSize, name);
            }
            case FourCcs.auds:
            {
                CodecId codec = CodecId.Unknown;
                int sampleRate = 0, channels = 0, bits = 0;
                if (strf is { Length: >= 16 })
                {
                    // WAVEFORMATEX
                    ushort wFormatTag = BinaryPrimitives.ReadUInt16LittleEndian(strf.AsSpan(0, 2));
                    channels = BinaryPrimitives.ReadUInt16LittleEndian(strf.AsSpan(2, 2));
                    sampleRate = (int)BinaryPrimitives.ReadUInt32LittleEndian(strf.AsSpan(4, 4));
                    bits = BinaryPrimitives.ReadUInt16LittleEndian(strf.AsSpan(14, 2));
                    codec = MapAudioCodec(wFormatTag, bits);
                }
                var p = new AudioCodecParameters
                {
                    Codec = codec,
                    Channels = channels,
                    SampleRate = sampleRate,
                    BitsPerSample = bits,
                };
                return new StreamState((byte)index, IsVideo: false, p, timeBase, strh.Length, strh.SampleSize, name);
            }
            case FourCcs.txts:
            {
                var p = new SubtitleCodecParameters { Codec = CodecId.SubRip };
                return new StreamState((byte)index, IsVideo: false, p, timeBase, strh.Length, 0, name);
            }
            default:
                return null;
        }
    }

    // -----------------------------------------------------------------------
    // idx1 (classic AVI index)
    // -----------------------------------------------------------------------

    private static IndexEntry[] ParseIdx1(
        IRandomAccessSource source, long offset, uint size, long moviStart, StreamState[] streams)
    {
        const int recordSize = 16;
        int count = (int)(size / recordSize);
        if (count == 0) return [];

        byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
        try
        {
            int read = source.Read(offset, buf.AsSpan(0, (int)size));
            if (read != (int)size) return [];

            // The idx1 records may use either movi-relative offsets (the
            // conventional form) or absolute file offsets. We auto-detect by
            // probing the first entry.
            uint firstOffset = BinaryPrimitives.ReadUInt32LittleEndian(buf.AsSpan(8, 4));
            // 8 bytes of preceding RIFF/LIST headers means movi-relative
            // entries point at +8 of their chunk-header position relative to
            // 'movi' (i.e. moviStart - 4). We just bias against absolute.
            bool absolute = firstOffset >= moviStart;

            var index = new IndexEntry[count];
            var perStreamCount = new uint[streams.Length];
            int written = 0;
            for (int i = 0; i < count; i++)
            {
                int p = i * recordSize;
                uint chunkId = BinaryPrimitives.ReadUInt32LittleEndian(buf.AsSpan(p, 4));
                uint flags = BinaryPrimitives.ReadUInt32LittleEndian(buf.AsSpan(p + 4, 4));
                uint chunkOffset = BinaryPrimitives.ReadUInt32LittleEndian(buf.AsSpan(p + 8, 4));
                uint chunkLength = BinaryPrimitives.ReadUInt32LittleEndian(buf.AsSpan(p + 12, 4));

                if (!TryParseSampleChunkId(chunkId, out int streamIndex)) continue;
                if ((uint)streamIndex >= streams.Length) continue;

                long fileOffset = absolute
                    ? chunkOffset + 8L                          // skip 8-byte chunk header
                    : (moviStart - 4L) + chunkOffset + 8L;      // relative-to-moviStart - 4 (the RIFF list header)

                index[written++] = new IndexEntry(
                    (byte)streamIndex,
                    fileOffset,
                    chunkLength,
                    (flags & 0x10) != 0, // AVIIF_KEYFRAME
                    perStreamCount[streamIndex]++);
            }
            if (written != count) Array.Resize(ref index, written);
            return index;
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(buf);
        }
    }

    // -----------------------------------------------------------------------
    // LIST INFO metadata
    // -----------------------------------------------------------------------

    private static void ParseInfo(IRandomAccessSource source, long offset, uint size, MediaMetadataBuilder meta)
    {
        long end = offset + size;
        long pos = offset;
        Span<byte> hdr = stackalloc byte[8];
        while (pos + 8 <= end)
        {
            if (source.Read(pos, hdr) != 8) return;
            uint id = BinaryPrimitives.ReadUInt32LittleEndian(hdr[..4]);
            uint csize = BinaryPrimitives.ReadUInt32LittleEndian(hdr[4..]);
            pos += 8;
            if (pos + csize > end) return;

            byte[] buf = ArrayPool<byte>.Shared.Rent((int)csize);
            try
            {
                if (source.Read(pos, buf.AsSpan(0, (int)csize)) == (int)csize)
                {
                    string canonical = MapInfoId(id);
                    if (canonical.Length > 0)
                    {
                        string value = DecodeNullTerminatedLatin1(buf.AsSpan(0, (int)csize));
                        meta.Set(canonical, value);
                    }
                }
            }
            finally { ArrayPool<byte>.Shared.Return(buf); }

            pos += csize + (csize & 1);
        }
    }

    // -----------------------------------------------------------------------
    // Sample reading
    // -----------------------------------------------------------------------

    private async ValueTask<MediaSample?> ReadSampleAsync(
        int streamIndex, long offset, uint length, bool isKeyFrame, CancellationToken cancellationToken)
    {
        var s = _streams[streamIndex];
        var owner = MemoryPool<byte>.Shared.Rent((int)length);
        var mem = owner.Memory[..(int)length];
        int read = await _source.ReadAsync(offset, mem, cancellationToken).ConfigureAwait(false);
        if (read != (int)length)
        {
            owner.Dispose();
            return null;
        }

        long pts = s.NextPts;
        int dur = 1;
        if (!s.IsVideo && s.SampleSize > 0)
        {
            // For audio streams with fixed sample size, an AVI "sample" is one
            // block, and the duration in time-base units equals the number of
            // blocks contained in this chunk.
            dur = (int)(length / s.SampleSize);
            if (dur <= 0) dur = 1;
        }
        s.NextPts += dur;

        return new MediaSample
        {
            TrackIndex = streamIndex,
            Pts = pts,
            Dts = pts,
            Duration = dur,
            IsKeyFrame = s.IsVideo ? isKeyFrame : true,
            Data = mem,
            Owner = owner,
        };
    }

    // -----------------------------------------------------------------------
    // Codec mappings
    // -----------------------------------------------------------------------

    private static CodecId MapVideoCodec(uint fourcc) => fourcc switch
    {
        // little-endian packed: e.g. "H264" stored as 'H','2','6','4'
        0x34363248 or 0x31435641 or 0x63766461 => CodecId.H264,  // H264, AVC1, avc1 (lowercase)
        0x35363248 or 0x31435648 => CodecId.H265,                 // H265, HEVC
        0x31305641 => CodecId.Av1,                                // AV01
        0x30385056 => CodecId.Vp8,                                // VP80
        0x30395056 => CodecId.Vp9,                                // VP90
        0x44495658 or 0x44495644 or 0x56344D58 or 0x5634504D => CodecId.Mpeg4, // XVID, DIVX, XM4V, MP4V
        _ => CodecId.Unknown,
    };

    private static CodecId MapAudioCodec(ushort formatTag, int bits) => (formatTag, bits) switch
    {
        (0x0001, 16) => CodecId.PcmS16Le,
        (0x0001, 24) => CodecId.PcmS24Le,
        (0x0001, 32) => CodecId.PcmS32Le,
        (0x0003, 32) => CodecId.PcmF32Le,
        (0x0006, _) => CodecId.G711ALaw,
        (0x0007, _) => CodecId.G711MuLaw,
        (0x0055, _) => CodecId.Mp3,
        (0x00FF, _) => CodecId.Aac,
        (0x2000, _) => CodecId.Ac3,
        (0x2001, _) => CodecId.EAc3,
        (0xF1AC, _) => CodecId.Flac,
        (0x6750, _) => CodecId.Vorbis,
        _ => CodecId.Unknown,
    };

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static bool TryParseSampleChunkId(uint id, out int streamIndex)
    {
        // chunk IDs are ##xx where ## is the stream index in hex (ascii) and
        // xx is the data type code: dc=video, db=video, wb=audio, tx=text.
        byte a = (byte)id;
        byte b = (byte)(id >> 8);
        if (!(IsHexDigit(a) && IsHexDigit(b)))
        {
            streamIndex = -1;
            return false;
        }
        byte c = (byte)(id >> 16);
        byte d = (byte)(id >> 24);
        bool typed = (c == (byte)'d' && (d == (byte)'c' || d == (byte)'b'))
            || (c == (byte)'w' && d == (byte)'b')
            || (c == (byte)'t' && d == (byte)'x');
        if (!typed) { streamIndex = -1; return false; }
        streamIndex = HexValue(a) * 16 + HexValue(b);
        return true;
    }

    private static bool IsHexDigit(byte b) => (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F');
    private static int HexValue(byte b) => b switch
    {
        >= (byte)'0' and <= (byte)'9' => b - '0',
        >= (byte)'a' and <= (byte)'f' => b - 'a' + 10,
        _ => b - 'A' + 10,
    };

    private static string MapInfoId(uint id) => id switch
    {
        0x4D414E49 => "TITLE",       // INAM
        0x54524149 => "ARTIST",      // IART
        0x44524349 => "DATE",        // ICRD
        0x544D4349 => "COMMENT",     // ICMT
        0x524E4749 => "GENRE",       // IGNR
        0x44525049 => "ALBUM",       // IPRD
        0x4B525449 => "TRACKNUMBER", // ITRK
        0x504F4349 => "COPYRIGHT",   // ICOP
        0x54465349 => "ENCODER",     // ISFT
        0x474E4549 => "ENCODED_BY",  // IENG
        0x474E4C49 => "LANGUAGE",    // ILNG
        0x534D4349 => "COMMENT",     // ICMS
        0x42434953 => "ISRC",        // ISBJ -> subject; not ISRC; left for completeness
        _ => "",
    };

    private static string DecodeNullTerminatedLatin1(ReadOnlySpan<byte> bytes)
    {
        int end = bytes.IndexOf((byte)0);
        if (end < 0) end = bytes.Length;
        while (end > 0 && (bytes[end - 1] == (byte)' ' || bytes[end - 1] == (byte)'\0')) end--;
        if (end == 0) return string.Empty;
        return Encoding.Latin1.GetString(bytes[..end]);
    }

    // -----------------------------------------------------------------------
    // Internal data structures
    // -----------------------------------------------------------------------

    private readonly record struct StreamHeader
    {
        public uint Type { get; init; }
        public uint Handler { get; init; }
        public uint Flags { get; init; }
        public ushort Priority { get; init; }
        public ushort Language { get; init; }
        public uint InitialFrames { get; init; }
        public uint Scale { get; init; }
        public uint Rate { get; init; }
        public uint Start { get; init; }
        public uint Length { get; init; }
        public uint SampleSize { get; init; }
    }

    private sealed class StreamState(
        byte index, bool IsVideo,
        CodecParameters codecParameters, Rational timeBase,
        long length, uint sampleSize, string? name)
    {
        public byte Index { get; } = index;
        public bool IsVideo { get; } = IsVideo;
        public CodecParameters CodecParameters { get; } = codecParameters;
        public Rational TimeBase { get; } = timeBase;
        public long Length { get; } = length;
        public uint SampleSize { get; } = sampleSize;
        public string? Name { get; } = name;
        public long NextPts { get; set; }

        public double TimeOf(uint sampleInStream) => TimeBase.Denominator == 0
            ? 0
            : ((double)sampleInStream * TimeBase.Numerator) / TimeBase.Denominator;
    }

    private readonly record struct IndexEntry(
        byte StreamIndex, long Offset, uint Length, bool IsKeyFrame, uint SampleInStream);

    private static class FourCcs
    {
        public const uint RIFF = 0x46464952;
        public const uint AVI  = 0x20495641; // "AVI "
        public const uint LIST = 0x5453494C;
        public const uint hdrl = 0x6C726468;
        public const uint avih = 0x68697661;
        public const uint strl = 0x6C727473;
        public const uint strh = 0x68727473;
        public const uint strf = 0x66727473;
        public const uint strn = 0x6E727473;
        public const uint movi = 0x69766F6D;
        public const uint idx1 = 0x31786469;
        public const uint INFO = 0x4F464E49;
        public const uint vids = 0x73646976;
        public const uint auds = 0x73647561;
        public const uint txts = 0x73747874;
    }
}
