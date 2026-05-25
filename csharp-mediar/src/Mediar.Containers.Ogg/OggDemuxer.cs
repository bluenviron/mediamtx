using System.Buffers;
using System.Buffers.Binary;
using Mediar.IO;

namespace Mediar.Containers.Ogg;

/// <summary>
/// Ogg container demuxer. Reassembles packets that span multiple pages and
/// emits each codec packet as a <see cref="MediaSample"/>. Detects the codec
/// of each logical stream from the first packet (Opus, Vorbis, FLAC). Other
/// codecs are exposed as <see cref="CodecId.Unknown"/> tracks so the raw
/// bitstream can still be routed downstream.
/// </summary>
public sealed class OggDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly List<LogicalStream> _streams = new();
    private double _seekSeconds;
    private bool _disposed;

    private sealed class LogicalStream
    {
        public uint Serial;
        public int TrackIndex;
        public MediaTrack Track = null!;
        public List<byte[]> PendingPacketParts = new();
        public int PendingLength;
        public long SamplesEmitted;
        public int SamplesPerPacket;
        public int SampleRate;
    }

    private OggDemuxer(IRandomAccessSource source, bool ownsSource)
    {
        _source = source;
        _ownsSource = ownsSource;
    }

    /// <summary>Open an Ogg file from disk.</summary>
    public static OggDemuxer Open(string path)
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

    /// <summary>Open over an existing source.</summary>
    public static OggDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);
        var d = new OggDemuxer(source, ownsSource);
        d.ParseHeaderPages();
        return d;
    }

    /// <inheritdoc/>
    public string FormatName => "ogg";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => _streams.Select(s => s.Track).ToArray();

    /// <inheritdoc/>
    public TimeSpan Duration => TimeSpan.Zero;

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.Yield();
        var reader = new OggPageReader(_source);
        // Skip past header pages we already consumed.
        // Strategy: rewind to start and re-walk pages, but skip "header packets"
        // we already accounted for using a per-stream counter.
        var headerPacketsRemaining = new Dictionary<uint, int>();
        foreach (var s in _streams)
        {
            headerPacketsRemaining[s.Serial] = HeaderPacketCount(s.Track.Codec.Codec);
        }

        byte[] lacingScratch = new byte[255];

        while (reader.Position < reader.Length)
        {
            cancellationToken.ThrowIfCancellationRequested();
            long pageStart = reader.Position;

            if (!reader.TryReadPage(out var hdr, out var owner, out int payloadLen)) yield break;
            using var ownerScope = owner;

            var s = FindStream(hdr.SerialNumber);
            if (s is null) continue;

            ReadOnlyMemory<byte> payload = owner!.Memory[..payloadLen];
            int segmentCount = ReadSegments(pageStart, lacingScratch);

            int offset = 0;
            int segIdx = 0;
            while (segIdx < segmentCount)
            {
                int packetLen = 0;
                bool packetComplete = false;
                while (segIdx < segmentCount)
                {
                    int seg = lacingScratch[segIdx++];
                    packetLen += seg;
                    if (seg < 255)
                    {
                        packetComplete = true;
                        break;
                    }
                }

                ReadOnlyMemory<byte> slice = payload.Slice(offset, packetLen);
                offset += packetLen;

                if (s.PendingPacketParts.Count > 0 || !packetComplete)
                {
                    // Accumulating multi-page packet.
                    byte[] part = new byte[packetLen];
                    slice.Span.CopyTo(part);
                    s.PendingPacketParts.Add(part);
                    s.PendingLength += packetLen;
                    if (!packetComplete) continue;
                    byte[] full = new byte[s.PendingLength];
                    int pos = 0;
                    foreach (var p in s.PendingPacketParts)
                    {
                        p.CopyTo(full, pos);
                        pos += p.Length;
                    }
                    s.PendingPacketParts.Clear();
                    s.PendingLength = 0;

                    if (headerPacketsRemaining[s.Serial] > 0)
                    {
                        headerPacketsRemaining[s.Serial]--;
                        continue;
                    }

                    if (ShouldSkipForSeek(s))
                    {
                        // Advance per-packet PTS counter without yielding.
                        s.SamplesEmitted += s.SamplesPerPacket > 0 ? s.SamplesPerPacket : 0;
                        continue;
                    }

                    yield return MakeSample(s, full);
                }
                else
                {
                    if (headerPacketsRemaining[s.Serial] > 0)
                    {
                        headerPacketsRemaining[s.Serial]--;
                        continue;
                    }
                    if (ShouldSkipForSeek(s))
                    {
                        s.SamplesEmitted += s.SamplesPerPacket > 0 ? s.SamplesPerPacket : 0;
                        continue;
                    }
                    byte[] full = new byte[packetLen];
                    slice.Span.CopyTo(full);
                    yield return MakeSample(s, full);
                }
            }
        }
    }

    private MediaSample MakeSample(LogicalStream s, byte[] data)
    {
        long pts = s.SamplesEmitted;
        s.SamplesEmitted += s.SamplesPerPacket > 0 ? s.SamplesPerPacket : 0;
        return new MediaSample
        {
            TrackIndex = s.TrackIndex,
            Pts = pts,
            Dts = pts,
            Duration = s.SamplesPerPacket,
            IsKeyFrame = true,
            Data = data,
        };
    }

    private bool ShouldSkipForSeek(LogicalStream s)
    {
        if (_seekSeconds <= 0) return false;
        if (s.SampleRate <= 0 || s.SamplesPerPacket <= 0) return false;
        double endSeconds = (double)(s.SamplesEmitted + s.SamplesPerPacket) / s.SampleRate;
        return endSeconds <= _seekSeconds;
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        _seekSeconds = time < TimeSpan.Zero ? 0 : time.TotalSeconds;
        return ValueTask.CompletedTask;
    }

    private LogicalStream? FindStream(uint serial)
    {
        foreach (var s in _streams)
        {
            if (s.Serial == serial) return s;
        }
        return null;
    }

    private int ReadSegments(long pageStart, Span<byte> dest)
    {
        Span<byte> hdr = stackalloc byte[27];
        _source.Read(pageStart, hdr);
        int n = hdr[26];
        _source.Read(pageStart + 27, dest[..n]);
        return n;
    }

    private void ParseHeaderPages()
    {
        var reader = new OggPageReader(_source);
        var packetBuilders = new Dictionary<uint, List<byte[]>>();
        var packetLens = new Dictionary<uint, int>();
        var collectedPackets = new Dictionary<uint, List<byte[]>>();
        var requiredPackets = new Dictionary<uint, int>();
        var registered = new HashSet<uint>();

        Span<byte> lacingScratch = new byte[255];

        while (reader.Position < reader.Length && _streams.Count < 8)
        {
            long pageStart = reader.Position;
            if (!reader.TryReadPage(out var hdr, out var owner, out int payloadLen)) break;
            using var ownerScope = owner;

            int segmentCount = ReadSegments(pageStart, lacingScratch);
            ReadOnlyMemory<byte> payload = owner!.Memory[..payloadLen];
            int offset = 0;
            int segIdx = 0;
            bool isFirstPage = hdr.IsBeginningOfStream;

            if (isFirstPage && !collectedPackets.ContainsKey(hdr.SerialNumber))
            {
                packetBuilders[hdr.SerialNumber] = new List<byte[]>();
                packetLens[hdr.SerialNumber] = 0;
                collectedPackets[hdr.SerialNumber] = new List<byte[]>();
                requiredPackets[hdr.SerialNumber] = 1;
            }

            if (!collectedPackets.ContainsKey(hdr.SerialNumber)) continue;
            if (registered.Contains(hdr.SerialNumber)) continue;

            while (segIdx < segmentCount && !registered.Contains(hdr.SerialNumber))
            {
                int packetLen = 0;
                bool packetComplete = false;
                while (segIdx < segmentCount)
                {
                    int seg = lacingScratch[segIdx++];
                    packetLen += seg;
                    if (seg < 255)
                    {
                        packetComplete = true;
                        break;
                    }
                }
                byte[] part = new byte[packetLen];
                payload.Slice(offset, packetLen).Span.CopyTo(part);
                offset += packetLen;
                packetBuilders[hdr.SerialNumber].Add(part);
                packetLens[hdr.SerialNumber] += packetLen;

                if (packetComplete)
                {
                    var assembled = new byte[packetLens[hdr.SerialNumber]];
                    int pos = 0;
                    foreach (var p in packetBuilders[hdr.SerialNumber])
                    {
                        p.CopyTo(assembled, pos);
                        pos += p.Length;
                    }
                    collectedPackets[hdr.SerialNumber].Add(assembled);
                    packetBuilders[hdr.SerialNumber].Clear();
                    packetLens[hdr.SerialNumber] = 0;

                    if (collectedPackets[hdr.SerialNumber].Count == 1)
                    {
                        var (codec, _, _, _) = IdentifyStream(assembled);
                        requiredPackets[hdr.SerialNumber] = ExtraDataPacketCount(codec);
                    }

                    if (collectedPackets[hdr.SerialNumber].Count >= requiredPackets[hdr.SerialNumber])
                    {
                        registered.Add(hdr.SerialNumber);
                        RegisterStream(hdr.SerialNumber, collectedPackets[hdr.SerialNumber]);
                    }
                }
            }

            if (_streams.Count > 0 && registered.Count == collectedPackets.Count)
            {
                break;
            }
        }
    }

    private void RegisterStream(uint serial, List<byte[]> headerPackets)
    {
        var firstPacket = headerPackets[0];
        var (codec, sampleRate, channels, samplesPerPacket) = IdentifyStream(firstPacket);

        byte[] extraData;
        if (headerPackets.Count > 1)
        {
            // Multi-packet codecs (Vorbis, Opus) — pack the priming packets
            // using Xiph lacing, matching Matroska/WebM CodecPrivate.
            extraData = PackXiphLaced(headerPackets);
        }
        else
        {
            extraData = (byte[])firstPacket.Clone();
        }

        var codecParams = new AudioCodecParameters
        {
            Codec = codec,
            SampleRate = sampleRate,
            Channels = channels,
            ExtraData = extraData,
        };
        var track = new MediaTrack
        {
            Index = _streams.Count,
            Id = serial,
            TimeBase = sampleRate > 0 ? new Rational(1, sampleRate) : new Rational(1, 1000),
            Codec = codecParams,
            DurationTicks = 0,
        };
        _streams.Add(new LogicalStream
        {
            Serial = serial,
            TrackIndex = _streams.Count,
            Track = track,
            SamplesPerPacket = samplesPerPacket,
            SampleRate = sampleRate,
        });
    }

    private static byte[] PackXiphLaced(List<byte[]> packets)
    {
        if (packets.Count == 0) return Array.Empty<byte>();
        int headerSize = 1;
        for (int i = 0; i < packets.Count - 1; i++) headerSize += packets[i].Length / 255 + 1;
        int total = headerSize;
        for (int i = 0; i < packets.Count; i++) total += packets[i].Length;

        var buf = new byte[total];
        int o = 0;
        buf[o++] = (byte)(packets.Count - 1);
        for (int i = 0; i < packets.Count - 1; i++)
        {
            int len = packets[i].Length;
            while (len >= 255) { buf[o++] = 0xFF; len -= 255; }
            buf[o++] = (byte)len;
        }
        for (int i = 0; i < packets.Count; i++)
        {
            packets[i].CopyTo(buf, o);
            o += packets[i].Length;
        }
        return buf;
    }

    private static (CodecId Codec, int SampleRate, int Channels, int SamplesPerPacket) IdentifyStream(ReadOnlySpan<byte> head)
    {
        if (head.Length >= 19 && head.StartsWith("OpusHead"u8))
        {
            int channels = head[9];
            int inputSampleRate = (int)BinaryPrimitives.ReadUInt32LittleEndian(head[12..16]);
            // Opus packets carry their own duration but always at 48 kHz.
            return (CodecId.Opus, inputSampleRate == 0 ? 48000 : inputSampleRate, channels, 0);
        }
        if (head.Length >= 30 && head[0] == 0x01 && head[1] == (byte)'v' && head[2] == (byte)'o' && head[3] == (byte)'r' &&
            head[4] == (byte)'b' && head[5] == (byte)'i' && head[6] == (byte)'s')
        {
            int channels = head[11];
            int sampleRate = (int)BinaryPrimitives.ReadUInt32LittleEndian(head[12..16]);
            return (CodecId.Vorbis, sampleRate, channels, 0);
        }
        if (head.Length >= 9 && head[0] == 0x7F &&
            head[1] == (byte)'F' && head[2] == (byte)'L' && head[3] == (byte)'A' && head[4] == (byte)'C')
        {
            return (CodecId.Flac, 0, 0, 0);
        }
        return (CodecId.Unknown, 0, 0, 0);
    }

    private static int HeaderPacketCount(CodecId codec) => codec switch
    {
        CodecId.Opus => 2,
        CodecId.Vorbis => 3,
        CodecId.Flac => 1,
        _ => 1,
    };

    /// <summary>
    /// Number of header packets that contribute to <c>ExtraData</c>. Vorbis
    /// requires all three priming packets to be carried as Xiph-laced
    /// <c>ExtraData</c> so the decoder can parse the setup header. Opus's
    /// second header (OpusTags) is informational only, so we keep
    /// <c>ExtraData = OpusHead</c> for compatibility with existing tooling.
    /// </summary>
    private static int ExtraDataPacketCount(CodecId codec) => codec switch
    {
        CodecId.Vorbis => 3,
        _ => 1,
    };

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
}
