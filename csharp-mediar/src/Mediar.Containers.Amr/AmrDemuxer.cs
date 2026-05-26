using System.Buffers;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.Amr;

/// <summary>
/// Demuxer for 3GPP TS 26.101 (AMR-NB) and TS 26.201 (AMR-WB) Storage Format
/// streams ("#!AMR\n" / "#!AMR-WB\n" magic), single channel only.
/// </summary>
public sealed class AmrDemuxer : IMediaDemuxer
{
    private static readonly byte[] MagicNb  = "#!AMR\n"u8.ToArray();
    private static readonly byte[] MagicWb  = "#!AMR-WB\n"u8.ToArray();

    // AMR-NB frame sizes by mode (excluding type-octet); index 15 = NO_DATA.
    private static ReadOnlySpan<byte> NbSize => [12, 13, 15, 17, 19, 20, 26, 31, 5, 0, 0, 0, 0, 0, 0, 0];
    // AMR-WB frame sizes by mode (excluding type-octet); index 15 = NO_DATA.
    private static ReadOnlySpan<byte> WbSize => [17, 23, 32, 36, 40, 46, 50, 58, 60, 5, 0, 0, 0, 0, 0, 0];

    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack _track;
    private readonly long _dataOffset;
    private readonly long _dataEnd;
    private readonly bool _wb;
    private readonly int _sampleRate;
    private long _startOffset;
    private long _startPts;
    private bool _disposed;

    private AmrDemuxer(
        IRandomAccessSource source, bool ownsSource, MediaTrack track,
        long dataOffset, long dataEnd, bool wb, int sampleRate)
    {
        _source = source;
        _ownsSource = ownsSource;
        _track = track;
        _dataOffset = dataOffset;
        _dataEnd = dataEnd;
        _wb = wb;
        _sampleRate = sampleRate;
        _startOffset = dataOffset;
    }

    /// <summary>Open an AMR file from disk.</summary>
    public static AmrDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an AMR stream from a random-access source.</summary>
    public static AmrDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);
        Span<byte> peek = stackalloc byte[9];
        int n = source.Read(0, peek);
        bool wb = false;
        long offset;
        if (n >= MagicWb.Length && peek[..MagicWb.Length].SequenceEqual(MagicWb))
        {
            wb = true;
            offset = MagicWb.Length;
        }
        else if (n >= MagicNb.Length && peek[..MagicNb.Length].SequenceEqual(MagicNb))
        {
            offset = MagicNb.Length;
        }
        else
        {
            throw new InvalidDataException("Missing AMR / AMR-WB magic header.");
        }

        int sr = wb ? 16000 : 8000;
        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, sr),
            Codec = new AudioCodecParameters
            {
                Codec = wb ? CodecId.AmrWb : CodecId.AmrNb,
                SampleRate = sr, Channels = 1,
            },
        };
        return new AmrDemuxer(source, ownsSource, track, offset, source.Length, wb, sr);
    }

    /// <inheritdoc/>
    public string FormatName => _wb ? "amr-wb" : "amr-nb";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => [_track];
    /// <inheritdoc/>
    public MediaMetadata Metadata => MediaMetadata.Empty;
    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        long off = _startOffset;
        long pts = _startPts;
        int samplesPerFrame = _wb ? 320 : 160; // 20 ms at 16 kHz / 8 kHz
        byte[] one = new byte[1];
        byte[] sizeTable = _wb ? WbSize.ToArray() : NbSize.ToArray();
        while (off < _dataEnd)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (_source.Read(off, one) != 1) yield break;
            byte typeOctet = one[0];
            int mode = (typeOctet >> 3) & 0x0F;
            int payloadBytes = sizeTable[mode];
            int frameSize = 1 + payloadBytes;
            if (off + frameSize > _dataEnd) yield break;

            var owner = MemoryPool<byte>.Shared.Rent(frameSize);
            var mem = owner.Memory[..frameSize];
            int read = await _source.ReadAsync(off, mem, cancellationToken).ConfigureAwait(false);
            if (read != frameSize) { owner.Dispose(); yield break; }
            yield return new MediaSample
            {
                TrackIndex = 0, Pts = pts, Dts = pts, Duration = samplesPerFrame,
                IsKeyFrame = true, Data = mem, Owner = owner,
            };
            off += frameSize;
            pts += samplesPerFrame;
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        // Coarse: scan from start and advance frame-by-frame until pts >= target.
        long target = (long)Math.Round(time.TotalSeconds * _sampleRate);
        int samplesPerFrame = _wb ? 320 : 160;
        ReadOnlySpan<byte> sizeTable = _wb ? WbSize : NbSize;
        long off = _dataOffset;
        long pts = 0;
        Span<byte> one = stackalloc byte[1];
        while (off < _dataEnd && pts < target)
        {
            if (_source.Read(off, one) != 1) break;
            int mode = (one[0] >> 3) & 0x0F;
            int frameSize = 1 + sizeTable[mode];
            if (off + frameSize > _dataEnd) break;
            off += frameSize;
            pts += samplesPerFrame;
        }
        _startOffset = off;
        _startPts = pts;
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
}
