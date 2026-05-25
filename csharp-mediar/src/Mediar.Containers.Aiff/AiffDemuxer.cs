using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.IO;

namespace Mediar.Containers.Aiff;

/// <summary>
/// Demuxer for Apple's AIFF and AIFC (compressed AIFF) containers, defined by
/// EA IFF 85. Reads the <c>COMM</c> common chunk for format parameters, the
/// <c>SSND</c> sound-data chunk for audio bytes, and the textual
/// <c>NAME</c> / <c>AUTH</c> / <c>ANNO</c> / <c>COPY</c> chunks for metadata.
/// </summary>
public sealed class AiffDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly MediaMetadata _metadata;
    private readonly long _dataOffset;
    private readonly long _dataLength;
    private readonly int _bytesPerFrame;
    private readonly int _sampleRate;
    private long _startFrame;
    private bool _disposed;

    private AiffDemuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track, MediaMetadata metadata,
        long dataOffset, long dataLength, int bytesPerFrame, int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _metadata = metadata;
        _dataOffset = dataOffset;
        _dataLength = dataLength;
        _bytesPerFrame = bytesPerFrame;
        _sampleRate = sampleRate;
    }

    /// <summary>Open an AIFF/AIFC file from disk.</summary>
    public static AiffDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an AIFF/AIFC stream from an arbitrary <see cref="IRandomAccessSource"/>.</summary>
    public static AiffDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> hdr = stackalloc byte[12];
        if (source.Read(0, hdr) != 12)
            throw new InvalidDataException("File too small to be AIFF.");
        if (hdr[0] != 'F' || hdr[1] != 'O' || hdr[2] != 'R' || hdr[3] != 'M')
            throw new InvalidDataException("Missing FORM marker.");
        bool isAifc;
        if (hdr[8] == 'A' && hdr[9] == 'I' && hdr[10] == 'F' && hdr[11] == 'F') isAifc = false;
        else if (hdr[8] == 'A' && hdr[9] == 'I' && hdr[10] == 'F' && hdr[11] == 'C') isAifc = true;
        else throw new InvalidDataException("Missing AIFF/AIFC marker.");

        uint formSize = BinaryPrimitives.ReadUInt32BigEndian(hdr[4..8]);
        long formEnd = Math.Min(source.Length, 8L + formSize);

        var meta = new MediaMetadataBuilder();
        CommChunk? comm = null;
        long ssndOffset = -1;
        uint ssndSize = 0;
        uint ssndDataOffset = 0;

        long pos = 12;
        Span<byte> chunkHdr = stackalloc byte[8];
        Span<byte> ssndHdr = stackalloc byte[8];
        while (pos + 8 <= formEnd)
        {
            if (source.Read(pos, chunkHdr) != 8) break;
            uint id = BinaryPrimitives.ReadUInt32BigEndian(chunkHdr[..4]);
            uint size = BinaryPrimitives.ReadUInt32BigEndian(chunkHdr[4..]);
            pos += 8;

            if (id == FourCcs.COMM && size >= 18)
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)size)) == (int)size)
                    {
                        comm = ParseComm(buf.AsSpan(0, (int)size), isAifc);
                    }
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }
            else if (id == FourCcs.SSND && size >= 8)
            {
                if (source.Read(pos, ssndHdr) == 8)
                {
                    ssndDataOffset = BinaryPrimitives.ReadUInt32BigEndian(ssndHdr[..4]);
                    ssndOffset = pos + 8 + ssndDataOffset;
                    ssndSize = size - 8 - ssndDataOffset;
                }
            }
            else if (size > 0 && size < 1 << 24
                && (id == FourCcs.NAME || id == FourCcs.AUTH || id == FourCcs.ANNO || id == FourCcs.COPY))
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    if (source.Read(pos, buf.AsSpan(0, (int)size)) == (int)size)
                    {
                        string s = Encoding.UTF8.GetString(buf, 0, (int)size).TrimEnd('\0', ' ');
                        switch (id)
                        {
                            case FourCcs.NAME: meta.Set("TITLE", s); break;
                            case FourCcs.AUTH: meta.Set("ARTIST", s); break;
                            case FourCcs.ANNO: meta.Set("COMMENT", s); break;
                            case FourCcs.COPY: meta.Set("COPYRIGHT", s); break;
                        }
                    }
                }
                finally { ArrayPool<byte>.Shared.Return(buf); }
            }

            pos += size + (size & 1);
        }

        if (comm is null) throw new InvalidDataException("Missing COMM chunk.");
        if (ssndOffset < 0) throw new InvalidDataException("Missing SSND chunk.");

        var c = comm.Value;
        int bytesPerFrame = ((c.BitsPerSample + 7) / 8) * c.Channels;
        if (bytesPerFrame <= 0) throw new InvalidDataException("Invalid bytes-per-frame.");

        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, c.SampleRate),
            Codec = new AudioCodecParameters
            {
                Codec = c.Codec,
                SampleRate = c.SampleRate,
                Channels = c.Channels,
                BitsPerSample = c.BitsPerSample,
            },
            DurationTicks = c.SampleFrames,
        };

        return new AiffDemuxer(
            source, ownsSource, track, meta.Build(),
            ssndOffset, ssndSize,
            bytesPerFrame, c.SampleRate);
    }

    /// <inheritdoc/>
    public string FormatName => "aiff";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];

    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public TimeSpan Duration => _bytesPerFrame > 0
        ? TimeSpan.FromSeconds((double)(_dataLength / _bytesPerFrame) / _sampleRate)
        : TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        int framesPerPacket = Math.Max(1, _sampleRate / 100);
        int bytesPerPacket = framesPerPacket * _bytesPerFrame;
        long offset = _dataOffset + _startFrame * _bytesPerFrame;
        long end = _dataOffset + _dataLength;
        long ptsFrames = _startFrame;

        while (offset < end)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int toRead = (int)Math.Min(bytesPerPacket, end - offset);
            int frames = toRead / _bytesPerFrame;
            if (frames == 0) yield break;

            var owner = MemoryPool<byte>.Shared.Rent(frames * _bytesPerFrame);
            var mem = owner.Memory[..(frames * _bytesPerFrame)];
            int read = await _source.ReadAsync(offset, mem, cancellationToken).ConfigureAwait(false);
            if (read != mem.Length) { owner.Dispose(); yield break; }

            yield return new MediaSample
            {
                TrackIndex = 0,
                Pts = ptsFrames,
                Dts = ptsFrames,
                Duration = frames,
                IsKeyFrame = true,
                Data = mem,
                Owner = owner,
            };

            offset += frames * _bytesPerFrame;
            ptsFrames += frames;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        if (time < TimeSpan.Zero) time = TimeSpan.Zero;
        long target = (long)Math.Round(time.TotalSeconds * _sampleRate);
        long maxFrames = _dataLength / _bytesPerFrame;
        if (target > maxFrames) target = maxFrames;
        _startFrame = target;
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

    private static CommChunk ParseComm(ReadOnlySpan<byte> data, bool isAifc)
    {
        ushort channels = BinaryPrimitives.ReadUInt16BigEndian(data[..2]);
        uint sampleFrames = BinaryPrimitives.ReadUInt32BigEndian(data[2..6]);
        ushort bitsPerSample = BinaryPrimitives.ReadUInt16BigEndian(data[6..8]);
        // sampleRate is 80-bit IEEE extended precision (10 bytes).
        int sampleRate = (int)Math.Round(ReadExtendedFloat(data[8..18]));

        CodecId codec = bitsPerSample switch
        {
            16 => CodecId.PcmS16Be,
            _  => CodecId.Unknown,
        };

        if (isAifc && data.Length >= 22)
        {
            // After the sampleRate there is a 4-byte compression type fourcc
            // followed by a Pascal string name.
            uint comp = BinaryPrimitives.ReadUInt32BigEndian(data[18..22]);
            codec = MapCompressionId(comp, bitsPerSample, codec);
        }

        return new CommChunk
        {
            Channels = channels,
            SampleFrames = sampleFrames,
            BitsPerSample = bitsPerSample,
            SampleRate = sampleRate,
            Codec = codec,
        };
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static double ReadExtendedFloat(ReadOnlySpan<byte> b)
    {
        // 80-bit IEEE 754 extended-precision big-endian "long double".
        ushort sExp = BinaryPrimitives.ReadUInt16BigEndian(b[..2]);
        ulong mantissa = BinaryPrimitives.ReadUInt64BigEndian(b[2..10]);

        int sign = (sExp & 0x8000) != 0 ? -1 : 1;
        int exponent = (sExp & 0x7FFF) - 16383;
        if (exponent == -16383 && mantissa == 0) return 0;
        double value = (double)mantissa / (1UL << 63);
        return sign * value * Math.Pow(2, exponent);
    }

    private static CodecId MapCompressionId(uint comp, int bits, CodecId fallback)
    {
        return comp switch
        {
            FourCcs.NONE => bits == 16 ? CodecId.PcmS16Be : fallback,
            FourCcs.twos => bits == 16 ? CodecId.PcmS16Be : fallback,
            FourCcs.sowt => bits == 16 ? CodecId.PcmS16Le : fallback,
            FourCcs.fl32 or FourCcs.FL32 => CodecId.PcmF32Le, // 32-bit IEEE float
            FourCcs.ulaw or FourCcs.ULAW => CodecId.G711MuLaw,
            FourCcs.alaw or FourCcs.ALAW => CodecId.G711ALaw,
            _ => fallback,
        };
    }

    private readonly struct CommChunk
    {
        public ushort Channels { get; init; }
        public uint SampleFrames { get; init; }
        public ushort BitsPerSample { get; init; }
        public int SampleRate { get; init; }
        public CodecId Codec { get; init; }
    }

    private static class FourCcs
    {
        public const uint COMM = 0x434F4D4D;
        public const uint SSND = 0x53534E44;
        public const uint NAME = 0x4E414D45;
        public const uint AUTH = 0x41555448;
        public const uint ANNO = 0x414E4E4F;
        public const uint COPY = 0x434F5059;
        // AIFC compression-type fourccs (stored big-endian).
        public const uint NONE = 0x4E4F4E45;
        public const uint twos = 0x74776F73;
        public const uint sowt = 0x736F7774;
        public const uint fl32 = 0x666C3332;
        public const uint FL32 = 0x464C3332;
        public const uint ulaw = 0x756C6177;
        public const uint ULAW = 0x554C4157;
        public const uint alaw = 0x616C6177;
        public const uint ALAW = 0x414C4157;
    }
}
