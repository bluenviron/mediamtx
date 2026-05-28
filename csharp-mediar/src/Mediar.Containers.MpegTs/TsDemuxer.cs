using System.Buffers;
using System.Collections.Immutable;
using System.Runtime.CompilerServices;
using Mediar.IO;

namespace Mediar.Containers.MpegTs;

/// <summary>
/// Demuxer for MPEG-TS (ISO/IEC 13818-1) transport streams.
/// </summary>
/// <remarks>
/// <para>
/// Bounded scope for this initial ship: <list type="bullet">
/// <item>188-byte packets only (no 192-byte M2TS or 204-byte forward-error-correction wrap).</item>
/// <item>Single-program TS — the first program advertised by the PAT is selected.</item>
/// <item>PAT and PMT sections must fit in a single TS packet payload.</item>
/// <item>Scrambled packets are rejected.</item>
/// <item>Streaming only: <see cref="Duration"/> reports <see cref="TimeSpan.Zero"/> and
/// seeking is unsupported.</item>
/// <item>Per-PID continuity counters are validated; gaps drop the in-flight PES so the
/// next PUSI = 1 packet starts a fresh access unit.</item>
/// </list></para>
/// <para>
/// Track time-base is the MPEG-TS 90 kHz system clock. Samples carry the 33-bit PTS
/// (and DTS when present in the PES header) verbatim; B-frame ordering is preserved.
/// </para>
/// </remarks>
public sealed class TsDemuxer : IMediaDemuxer
{
    /// <summary>PID reserved for the Program Association Table.</summary>
    public const ushort PatPid = 0x0000;

    /// <summary>Null packet PID — to be discarded.</summary>
    public const ushort NullPid = 0x1FFF;

    /// <summary>MPEG-TS system clock frequency in Hz (90 kHz).</summary>
    public const int SystemClockHz = 90_000;

    private const int DiscoveryByteBudget = 5 * 1024 * 1024;

    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly MediaTrack[] _tracks;
    private readonly ImmutableDictionary<ushort, int> _pidToTrackIndex;
    private readonly long _streamLength;
    private bool _disposed;

    private TsDemuxer(
        IRandomAccessSource source, bool ownsSource,
        MediaTrack[] tracks, ImmutableDictionary<ushort, int> pidMap, long streamLength)
    {
        _source = source;
        _ownsSource = ownsSource;
        _tracks = tracks;
        _pidToTrackIndex = pidMap;
        _streamLength = streamLength;
    }

    /// <summary>Open an MPEG-TS file from disk.</summary>
    public static TsDemuxer Open(string path)
    {
        var src = new FileRandomAccessSource(path);
        try { return Open(src, ownsSource: true); }
        catch { src.Dispose(); throw; }
    }

    /// <summary>Open an MPEG-TS stream from a random-access source.</summary>
    public static TsDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);

        long streamLength = source.Length;
        if (streamLength < TsPacket.PacketSize)
            throw new InvalidDataException("Stream is shorter than one MPEG-TS packet.");

        long startOffset = LocateSync(source, streamLength);
        if (startOffset < 0)
            throw new InvalidDataException("MPEG-TS sync byte (0x47) not found.");

        Pat? pat = null;
        Pmt? pmt = null;
        ushort? pmtPid = null;
        Span<byte> packetBuf = stackalloc byte[TsPacket.PacketSize];

        long offset = startOffset;
        long endOfScan = Math.Min(streamLength, startOffset + DiscoveryByteBudget);
        while (offset + TsPacket.PacketSize <= endOfScan)
        {
            if (source.Read(offset, packetBuf) != TsPacket.PacketSize) break;
            if (!TsPacket.TryParse(packetBuf, out var pkt))
            {
                offset++;
                continue;
            }
            if (pkt.TransportScramblingControl != 0)
            {
                offset += TsPacket.PacketSize;
                continue;
            }
            if (pkt.HasPayload && pkt.PayloadUnitStartIndicator)
            {
                var payload = packetBuf.Slice(pkt.PayloadOffset, pkt.PayloadLength);
                if (pat is null && pkt.Pid == PatPid)
                {
                    if (Pat.TryParse(payload, out var parsed) && parsed!.Entries.Length > 0)
                    {
                        pat = parsed;
                        pmtPid = pat.Entries[0].PmtPid;
                    }
                }
                else if (pmt is null && pmtPid is ushort wanted && pkt.Pid == wanted)
                {
                    if (Pmt.TryParse(payload, out var parsedPmt))
                        pmt = parsedPmt;
                }
            }
            offset += TsPacket.PacketSize;
            if (pat is not null && pmt is not null) break;
        }

        if (pat is null) throw new InvalidDataException("No PAT section observed.");
        if (pmt is null) throw new InvalidDataException("No PMT section observed for the selected program.");

        var tracks = new List<MediaTrack>();
        var pidMapBuilder = ImmutableDictionary.CreateBuilder<ushort, int>();
        int trackIndex = 0;
        foreach (var es in pmt.Streams)
        {
            var codec = StreamTypes.ToCodecId(es.StreamType);
            if (codec == CodecId.Unknown) continue;

            CodecParameters parameters = StreamTypes.IsVideo(codec)
                ? new VideoCodecParameters { Codec = codec }
                : new AudioCodecParameters { Codec = codec };

            tracks.Add(new MediaTrack
            {
                Index = trackIndex,
                Id = es.ElementaryPid,
                Codec = parameters,
                TimeBase = new Rational(1, SystemClockHz),
                IsDefault = trackIndex == 0,
            });
            pidMapBuilder.Add(es.ElementaryPid, trackIndex);
            trackIndex++;
        }
        if (tracks.Count == 0)
            throw new InvalidDataException("PMT advertised no streams with a recognized codec.");

        return new TsDemuxer(source, ownsSource, tracks.ToArray(), pidMapBuilder.ToImmutable(), streamLength);
    }

    /// <inheritdoc/>
    public string FormatName => "mpegts";
    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => _tracks;
    /// <inheritdoc/>
    public MediaMetadata Metadata => MediaMetadata.Empty;
    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (_streamLength < TsPacket.PacketSize) yield break;

        long syncOffset = LocateSync(_source, _streamLength);
        if (syncOffset < 0) yield break;

        var assemblers = new Dictionary<ushort, PesAssembler>();
        foreach (var pid in _pidToTrackIndex.Keys)
            assemblers[pid] = new PesAssembler();

        byte[] packetBuf = new byte[TsPacket.PacketSize];
        long offset = syncOffset;
        while (offset + TsPacket.PacketSize <= _streamLength)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int read = await _source.ReadAsync(
                offset, packetBuf.AsMemory(0, TsPacket.PacketSize), cancellationToken).ConfigureAwait(false);
            if (read != TsPacket.PacketSize) break;

            if (!TsPacket.TryParse(packetBuf, out var pkt))
            {
                offset++;
                continue;
            }
            offset += TsPacket.PacketSize;

            if (pkt.Pid == NullPid) continue;
            if (pkt.TransportScramblingControl != 0) continue;
            if (!_pidToTrackIndex.TryGetValue(pkt.Pid, out int trackIdx)) continue;
            if (!pkt.HasPayload) continue;

            var asm = assemblers[pkt.Pid];

            if (pkt.PayloadUnitStartIndicator)
            {
                if (asm.TryComplete(out var completed))
                    yield return Materialize(trackIdx, completed);
                asm.Begin(pkt.ContinuityCounter, packetBuf.AsSpan(pkt.PayloadOffset, pkt.PayloadLength));
            }
            else
            {
                asm.Append(pkt.ContinuityCounter, packetBuf.AsSpan(pkt.PayloadOffset, pkt.PayloadLength));
            }
        }

        foreach (var (pid, asm) in assemblers)
        {
            if (asm.TryComplete(out var completed))
                yield return Materialize(_pidToTrackIndex[pid], completed);
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
    public ValueTask DisposeAsync() { Dispose(); return ValueTask.CompletedTask; }

    private static MediaSample Materialize(int trackIndex, PesAssembler.Completed completed)
    {
        long pts = completed.Pts ?? 0;
        long dts = completed.Dts ?? pts;
        return new MediaSample
        {
            TrackIndex = trackIndex,
            Pts = pts,
            Dts = dts,
            Duration = 0,
            IsKeyFrame = false,
            Data = completed.Owner.Memory[..completed.Length],
            Owner = completed.Owner,
        };
    }

    private static long LocateSync(IRandomAccessSource source, long streamLength)
    {
        Span<byte> probe = stackalloc byte[TsPacket.PacketSize * 2];
        long limit = Math.Min(streamLength - TsPacket.PacketSize, TsPacket.PacketSize * 32L);
        for (long start = 0; start <= limit; start++)
        {
            int want = (int)Math.Min((long)probe.Length, streamLength - start);
            if (want < TsPacket.PacketSize + 1) break;
            int got = source.Read(start, probe[..want]);
            if (got < TsPacket.PacketSize + 1) break;
            if (probe[0] == TsPacket.SyncByte && probe[TsPacket.PacketSize] == TsPacket.SyncByte)
                return start;
        }
        return streamLength > 0 && source.Read(0, probe[..1]) == 1 && probe[0] == TsPacket.SyncByte
            ? 0
            : -1;
    }

    /// <summary>
    /// Per-PID PES-packet assembler. Buffers TS payloads until the next PUSI = 1
    /// packet marks the previous PES complete, then surfaces the assembled payload
    /// with its decoded PTS/DTS. Continuity-counter gaps invalidate the in-flight
    /// PES so partial frames are not emitted.
    /// </summary>
    private sealed class PesAssembler
    {
        private IMemoryOwner<byte>? _buffer;
        private int _length;
        private byte _lastCc;
        private bool _ccPrimed;
        private bool _active;
        private long? _pts;
        private long? _dts;
        private int _pesPayloadOffsetInBuffer;

        public void Begin(byte cc, ReadOnlySpan<byte> payload)
        {
            ReleaseBuffer();
            _length = 0;
            _ccPrimed = true;
            _lastCc = cc;
            _active = true;
            _pts = null;
            _dts = null;
            _pesPayloadOffsetInBuffer = 0;

            if (payload.Length == 0)
            {
                _active = false;
                return;
            }

            EnsureCapacity(payload.Length);
            payload.CopyTo(_buffer!.Memory.Span);
            _length = payload.Length;

            if (PesHeader.TryParse(_buffer.Memory.Span[.._length], out var hdr))
            {
                _pts = hdr.Pts;
                _dts = hdr.Dts;
                _pesPayloadOffsetInBuffer = hdr.PayloadOffset;
            }
            else
            {
                _active = false;
            }
        }

        public void Append(byte cc, ReadOnlySpan<byte> payload)
        {
            if (!_active) return;
            byte expected = (byte)((_lastCc + 1) & 0xF);
            if (_ccPrimed && cc != expected)
            {
                _active = false;
                ReleaseBuffer();
                _length = 0;
                return;
            }
            _lastCc = cc;
            if (payload.IsEmpty) return;
            EnsureCapacity(_length + payload.Length);
            payload.CopyTo(_buffer!.Memory.Span[_length..]);
            _length += payload.Length;
        }

        public bool TryComplete(out Completed completed)
        {
            completed = default;
            if (!_active || _buffer is null) return false;

            int payloadOff = _pesPayloadOffsetInBuffer;
            int payloadLen = _length - payloadOff;
            if (payloadLen <= 0)
            {
                ReleaseBuffer();
                _active = false;
                _length = 0;
                return false;
            }

            var owner = MemoryPool<byte>.Shared.Rent(payloadLen);
            _buffer.Memory.Span.Slice(payloadOff, payloadLen).CopyTo(owner.Memory.Span);
            completed = new Completed(owner, payloadLen, _pts, _dts);

            ReleaseBuffer();
            _length = 0;
            _active = false;
            _pts = null;
            _dts = null;
            _pesPayloadOffsetInBuffer = 0;
            return true;
        }

        private void EnsureCapacity(int required)
        {
            if (_buffer is null)
            {
                _buffer = MemoryPool<byte>.Shared.Rent(Math.Max(required, 4096));
                return;
            }
            if (_buffer.Memory.Length >= required) return;
            int newSize = Math.Max(_buffer.Memory.Length * 2, required);
            var next = MemoryPool<byte>.Shared.Rent(newSize);
            _buffer.Memory.Span[.._length].CopyTo(next.Memory.Span);
            _buffer.Dispose();
            _buffer = next;
        }

        private void ReleaseBuffer()
        {
            _buffer?.Dispose();
            _buffer = null;
        }

        public readonly record struct Completed(IMemoryOwner<byte> Owner, int Length, long? Pts, long? Dts);
    }
}
