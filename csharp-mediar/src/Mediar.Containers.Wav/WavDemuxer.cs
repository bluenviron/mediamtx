using System.Buffers;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.Wav;

/// <summary>
/// Read-only demuxer for the RIFF/WAVE container.
/// Supports <c>WAVE_FORMAT_PCM</c> (1), <c>WAVE_FORMAT_IEEE_FLOAT</c> (3) and
/// <c>WAVE_FORMAT_EXTENSIBLE</c> (0xFFFE) variants with PCM or float sub-format
/// GUIDs. Samples are read as fixed-size packets matching ~10 ms of audio so
/// downstream consumers don't have to decide on chunking.
/// </summary>
public sealed class WavDemuxer : IMediaDemuxer
{
    // Common Microsoft format-tag codes.
    private const ushort WaveFormatPcm = 0x0001;
    private const ushort WaveFormatIeeeFloat = 0x0003;
    private const ushort WaveFormatExtensible = 0xFFFE;

    private static readonly Guid KsdataSubtypePcm = new(
        "00000001-0000-0010-8000-00aa00389b71");
    private static readonly Guid KsdataSubtypeIeeeFloat = new(
        "00000003-0000-0010-8000-00aa00389b71");

    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _dataOffset;
    private readonly long _dataLength;
    private readonly int _bytesPerFrame;
    private readonly int _sampleRate;
    private long _startFrame;
    private bool _disposed;

    private WavDemuxer(
        IRandomAccessSource source,
        bool ownsSource,
        MediaTrack track,
        long dataOffset,
        long dataLength,
        int bytesPerFrame,
        int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _dataOffset = dataOffset;
        _dataLength = dataLength;
        _bytesPerFrame = bytesPerFrame;
        _sampleRate = sampleRate;
    }

    /// <summary>Open a WAV file from disk.</summary>
    public static WavDemuxer Open(string path)
    {
        var source = new FileRandomAccessSource(path);
        try
        {
            return Open(source, ownsSource: true);
        }
        catch
        {
            source.Dispose();
            throw;
        }
    }

    /// <summary>Open a WAV stream from an arbitrary <see cref="IRandomAccessSource"/>.</summary>
    public static WavDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        Span<byte> hdr = stackalloc byte[12];
        if (source.Read(0, hdr) != 12)
        {
            throw new InvalidDataException("File too small to be a RIFF/WAVE container.");
        }
        if (!(hdr[0] == 'R' && hdr[1] == 'I' && hdr[2] == 'F' && hdr[3] == 'F'))
        {
            throw new InvalidDataException("Missing RIFF marker.");
        }
        if (!(hdr[8] == 'W' && hdr[9] == 'A' && hdr[10] == 'V' && hdr[11] == 'E'))
        {
            throw new InvalidDataException("Missing WAVE marker.");
        }

        // Scan chunks for fmt + data.
        long pos = 12;
        long len = source.Length;
        WavFormat? fmt = null;
        long dataOffset = -1;
        long dataLength = 0;

        Span<byte> chunkHdr = stackalloc byte[8];
        while (pos + 8 <= len)
        {
            if (source.Read(pos, chunkHdr) != 8) break;
            uint id =
                (uint)chunkHdr[0] |
                ((uint)chunkHdr[1] << 8) |
                ((uint)chunkHdr[2] << 16) |
                ((uint)chunkHdr[3] << 24);
            uint size =
                (uint)chunkHdr[4] |
                ((uint)chunkHdr[5] << 8) |
                ((uint)chunkHdr[6] << 16) |
                ((uint)chunkHdr[7] << 24);
            pos += 8;

            if (id == 0x20746D66) // "fmt "
            {
                byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
                try
                {
                    int read = source.Read(pos, buf.AsSpan(0, (int)size));
                    if (read != (int)size) throw new InvalidDataException("Truncated fmt chunk.");
                    fmt = ParseFmt(buf.AsSpan(0, (int)size));
                }
                finally
                {
                    ArrayPool<byte>.Shared.Return(buf);
                }
            }
            else if (id == 0x61746164) // "data"
            {
                dataOffset = pos;
                dataLength = size;
                break;
            }

            pos += size + (size & 1); // chunks are word-aligned
        }

        if (fmt is null) throw new InvalidDataException("Missing fmt chunk.");
        if (dataOffset < 0) throw new InvalidDataException("Missing data chunk.");

        var codec = MapCodec(fmt.Value);
        int bytesPerFrame = fmt.Value.BlockAlign != 0
            ? fmt.Value.BlockAlign
            : (fmt.Value.BitsPerSample / 8) * fmt.Value.Channels;
        if (bytesPerFrame <= 0)
        {
            throw new InvalidDataException("Invalid bytes-per-frame.");
        }

        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            TimeBase = new Rational(1, fmt.Value.SampleRate),
            Codec = new AudioCodecParameters
            {
                Codec = codec,
                SampleRate = fmt.Value.SampleRate,
                Channels = fmt.Value.Channels,
                BitsPerSample = fmt.Value.BitsPerSample,
            },
            DurationTicks = dataLength / bytesPerFrame,
        };

        return new WavDemuxer(source, ownsSource, track, dataOffset, dataLength, bytesPerFrame, fmt.Value.SampleRate);
    }

    /// <inheritdoc/>
    public string FormatName => "wav";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => new[] { _track };

    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.FromSeconds((double)(_dataLength / _bytesPerFrame) / _sampleRate);

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        // Emit ~10 ms of audio per packet.
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
            if (read != mem.Length)
            {
                owner.Dispose();
                yield break;
            }

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

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    internal static WavFormat ParseFmt(ReadOnlySpan<byte> data)
    {
        if (data.Length < 16) throw new InvalidDataException("fmt chunk too short.");

        var r = new LittleEndianSpanReader(data);
        ushort tag = r.ReadUInt16();
        ushort channels = r.ReadUInt16();
        uint sampleRate = r.ReadUInt32();
        _ = r.ReadUInt32(); // avg bytes/sec
        ushort blockAlign = r.ReadUInt16();
        ushort bitsPerSample = r.ReadUInt16();

        Guid subFormat = Guid.Empty;
        ushort validBits = bitsPerSample;
        if (tag == WaveFormatExtensible && data.Length >= 40)
        {
            r.Skip(2); // cbSize
            validBits = r.ReadUInt16();
            r.Skip(4); // channel mask
            var guidBytes = r.ReadBytes(16);
            subFormat = new Guid(guidBytes);
        }

        return new WavFormat
        {
            FormatTag = tag,
            Channels = channels,
            SampleRate = (int)sampleRate,
            BlockAlign = blockAlign,
            BitsPerSample = bitsPerSample,
            ValidBitsPerSample = validBits,
            SubFormat = subFormat,
        };
    }

    private static CodecId MapCodec(WavFormat fmt)
    {
        bool isPcm = fmt.FormatTag == WaveFormatPcm
            || (fmt.FormatTag == WaveFormatExtensible && fmt.SubFormat == KsdataSubtypePcm);
        bool isFloat = fmt.FormatTag == WaveFormatIeeeFloat
            || (fmt.FormatTag == WaveFormatExtensible && fmt.SubFormat == KsdataSubtypeIeeeFloat);

        if (isFloat && fmt.BitsPerSample == 32) return CodecId.PcmF32Le;
        if (isPcm)
        {
            return fmt.BitsPerSample switch
            {
                16 => CodecId.PcmS16Le,
                24 => CodecId.PcmS24Le,
                32 => CodecId.PcmS32Le,
                _ => CodecId.Unknown,
            };
        }
        return CodecId.Unknown;
    }
}

internal readonly struct WavFormat
{
    public ushort FormatTag { get; init; }
    public ushort Channels { get; init; }
    public int SampleRate { get; init; }
    public ushort BlockAlign { get; init; }
    public ushort BitsPerSample { get; init; }
    public ushort ValidBitsPerSample { get; init; }
    public Guid SubFormat { get; init; }
}
