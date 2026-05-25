using System.Buffers;
using System.Runtime.CompilerServices;
using System.Text;
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
    private readonly MediaMetadata _metadata;
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
        MediaMetadata metadata,
        long dataOffset,
        long dataLength,
        int bytesPerFrame,
        int sampleRate)
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

        // Scan chunks for fmt + data, collecting metadata along the way.
        long pos = 12;
        long len = source.Length;
        WavFormat? fmt = null;
        long dataOffset = -1;
        long dataLength = 0;
        var meta = new MediaMetadataBuilder();

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
                // Continue scanning so LIST chunks placed after data still register
                // — many WAV writers do this. We only break once we have both fmt
                // and data and there's no more content to scan.
                pos += size + (size & 1);
                continue;
            }
            else if (id == 0x5453494C) // "LIST"
            {
                ParseListChunk(source, pos, size, meta);
            }
            else if (id == 0x74786562) // "bext"
            {
                ParseBextChunk(source, pos, size, meta);
            }
            else if (id == 0x4C4D5869) // "iXML"
            {
                ParseIxmlChunk(source, pos, size, meta);
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

        return new WavDemuxer(source, ownsSource, track, meta.Build(), dataOffset, dataLength, bytesPerFrame, fmt.Value.SampleRate);
    }

    /// <inheritdoc/>
    public string FormatName => "wav";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => new[] { _track };

    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata;

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

    // -----------------------------------------------------------------------
    // Metadata extraction. WAV stores tags in three commonly-seen places:
    //   * LIST INFO subchunks (INAM=title, IART=artist, ICRD=date, …)
    //   * bext  — Broadcast WAV extension (originator, description, time)
    //   * iXML  — embedded XML metadata for production workflows
    // We parse all three to give callers the richest view possible.
    // -----------------------------------------------------------------------

    private static void ParseListChunk(IRandomAccessSource source, long offset, uint size, MediaMetadataBuilder meta)
    {
        if (size < 4) return;
        Span<byte> kind = stackalloc byte[4];
        if (source.Read(offset, kind) != 4) return;
        // Only the "INFO" list contains text tags. The "adtl" list carries
        // labels/notes for sample cues which are not file-level metadata.
        if (kind[0] != (byte)'I' || kind[1] != (byte)'N' || kind[2] != (byte)'F' || kind[3] != (byte)'O') return;

        long end = offset + size;
        long pos = offset + 4;
        Span<byte> hdr = stackalloc byte[8];
        while (pos + 8 <= end)
        {
            if (source.Read(pos, hdr) != 8) return;
            uint id =
                (uint)hdr[0] |
                ((uint)hdr[1] << 8) |
                ((uint)hdr[2] << 16) |
                ((uint)hdr[3] << 24);
            uint subSize =
                (uint)hdr[4] |
                ((uint)hdr[5] << 8) |
                ((uint)hdr[6] << 16) |
                ((uint)hdr[7] << 24);
            pos += 8;
            if (pos + subSize > end) return;

            byte[] buf = ArrayPool<byte>.Shared.Rent((int)subSize);
            try
            {
                int n = source.Read(pos, buf.AsSpan(0, (int)subSize));
                if (n == (int)subSize)
                {
                    string canonical = MapInfoId(id);
                    if (canonical.Length > 0)
                    {
                        string value = DecodeNullTerminatedLatin1(buf.AsSpan(0, (int)subSize));
                        meta.Set(canonical, value);
                    }
                }
            }
            finally
            {
                ArrayPool<byte>.Shared.Return(buf);
            }

            pos += subSize + (subSize & 1);
        }
    }

    private static void ParseBextChunk(IRandomAccessSource source, long offset, uint size, MediaMetadataBuilder meta)
    {
        // BWF v0 layout: char Description[256]; char Originator[32];
        // char OriginatorReference[32]; char OriginationDate[10];
        // char OriginationTime[8]; ...
        if (size < 256 + 32 + 32 + 10 + 8) return;
        byte[] buf = ArrayPool<byte>.Shared.Rent(256 + 32 + 32 + 10 + 8);
        try
        {
            if (source.Read(offset, buf.AsSpan(0, buf.Length)) != buf.Length) return;
            string desc = DecodeNullTerminatedLatin1(buf.AsSpan(0, 256));
            string originator = DecodeNullTerminatedLatin1(buf.AsSpan(256, 32));
            string originatorRef = DecodeNullTerminatedLatin1(buf.AsSpan(288, 32));
            string date = DecodeNullTerminatedLatin1(buf.AsSpan(320, 10));
            meta.Set("DESCRIPTION", desc);
            meta.Set("ENCODED_BY", originator);
            meta.Set("ISRC", originatorRef);
            meta.Set("DATE", date);
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(buf);
        }
    }

    private static void ParseIxmlChunk(IRandomAccessSource source, long offset, uint size, MediaMetadataBuilder meta)
    {
        // Best-effort extraction of obvious top-level elements. iXML is an
        // open XML schema used by production audio devices; we look for the
        // most common scalar fields and store the whole payload under iXML
        // so consumers can do richer parsing if they want to.
        if (size == 0 || size > 1_000_000) return;
        byte[] buf = ArrayPool<byte>.Shared.Rent((int)size);
        try
        {
            int n = source.Read(offset, buf.AsSpan(0, (int)size));
            if (n != (int)size) return;
            string xml = Encoding.UTF8.GetString(buf.AsSpan(0, n));
            meta.Set("IXML", xml);
            TryExtractXmlElement(xml, "PROJECT", out var project);
            TryExtractXmlElement(xml, "NOTE", out var note);
            if (project is not null) meta.Set("ALBUM", project);
            if (note is not null) meta.Set("COMMENT", note);
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(buf);
        }
    }

    private static string MapInfoId(uint id) => id switch
    {
        0x4D414E49 => "TITLE",           // INAM
        0x54524149 => "ARTIST",          // IART
        0x44524349 => "DATE",            // ICRD
        0x544D4349 => "COMMENT",         // ICMT
        0x524E4749 => "GENRE",           // IGNR
        0x44525049 => "ALBUM",           // IPRD
        0x4B525449 => "TRACKNUMBER",     // ITRK
        0x504F4349 => "COPYRIGHT",       // ICOP
        0x54465349 => "ENCODER",         // ISFT
        0x474E4549 => "ENCODED_BY",      // IENG
        0x474E4C49 => "LANGUAGE",        // ILNG
        0x534D4349 => "COMMENT",         // ICMS = commissioned by → mapped to comment
        _ => "",
    };

    private static string DecodeNullTerminatedLatin1(ReadOnlySpan<byte> bytes)
    {
        int end = bytes.IndexOf((byte)0);
        if (end < 0) end = bytes.Length;
        // Trim trailing whitespace as some writers null-pad with spaces.
        while (end > 0 && (bytes[end - 1] == (byte)' ' || bytes[end - 1] == (byte)'\0')) end--;
        if (end == 0) return string.Empty;
        return Encoding.Latin1.GetString(bytes[..end]);
    }

    private static bool TryExtractXmlElement(string xml, string name, out string? value)
    {
        value = null;
        string openTag = "<" + name + ">";
        string closeTag = "</" + name + ">";
        int s = xml.IndexOf(openTag, StringComparison.OrdinalIgnoreCase);
        if (s < 0) return false;
        s += openTag.Length;
        int e = xml.IndexOf(closeTag, s, StringComparison.OrdinalIgnoreCase);
        if (e < 0) return false;
        value = xml[s..e].Trim();
        return value.Length > 0;
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
