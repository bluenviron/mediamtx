using Mediar.IO;

namespace Mediar.Containers.Matroska;

/// <summary>
/// Matroska / WebM container demuxer. Parses the EBML header, the Segment's
/// Info and Tracks elements, then walks Clusters and emits SimpleBlock and
/// BlockGroup samples as <see cref="MediaSample"/>. Xiph, Fixed and EBML
/// lacing are decoded — laced blocks emit one <see cref="MediaSample"/> per
/// frame with PTS values derived from <c>BlockDuration / N</c> (BlockGroup)
/// or the track's <c>DefaultDuration</c> (SimpleBlock).
/// </summary>
public sealed class MatroskaDemuxer : IMediaDemuxer
{
    private readonly IRandomAccessSource _source;
    private readonly bool _ownsSource;
    private readonly List<MediaTrack> _tracks = new();
    private readonly Dictionary<int, int> _trackNumberToIndex = new();
    /// <summary>DefaultDuration per track-index, in cluster-tick units (TimecodeScaleNs).</summary>
    private readonly Dictionary<int, long> _defaultDurationTicks = new();
    private readonly MediaMetadataBuilder _metaBuilder = new();
    private MediaMetadata? _metadata;
    private long _segmentStart;
    private long _segmentEnd;
    private ulong _timecodeScaleNs = 1_000_000; // 1 ms default
    private double _segmentDurationTicks;
    private double _seekSeconds;
    private bool _disposed;

    private MatroskaDemuxer(IRandomAccessSource source, bool ownsSource)
    {
        _source = source;
        _ownsSource = ownsSource;
    }

    /// <summary>Open a Matroska file from disk.</summary>
    public static MatroskaDemuxer Open(string path)
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
    public static MatroskaDemuxer Open(IRandomAccessSource source, bool ownsSource = false)
    {
        ArgumentNullException.ThrowIfNull(source);
        var d = new MatroskaDemuxer(source, ownsSource);
        d.ParseHeader();
        return d;
    }

    /// <inheritdoc/>
    public string FormatName => "matroska";

    /// <inheritdoc/>
    public IReadOnlyList<MediaTrack> Tracks => _tracks;

    /// <inheritdoc/>
    public MediaMetadata Metadata => _metadata ??= _metaBuilder.Build();

    /// <inheritdoc/>
    public TimeSpan Duration => _segmentDurationTicks > 0
        ? TimeSpan.FromTicks((long)(_segmentDurationTicks * _timecodeScaleNs / 100.0))
        : TimeSpan.Zero;

    private void ParseHeader()
    {
        var r = new EbmlReader(_source, 0);

        // EBML root element
        ulong id = r.ReadElementId(out _);
        if (id != MatroskaIds.Ebml) throw new InvalidDataException("Not an EBML stream.");
        ulong size = r.ReadVarInt(out _, out _);
        r.Skip((long)size);

        // Segment
        id = r.ReadElementId(out _);
        if (id != MatroskaIds.Segment) throw new InvalidDataException("Missing Segment element.");
        ulong segSize = r.ReadVarInt(out _, out bool unknownSize);
        _segmentStart = r.Position;
        _segmentEnd = unknownSize ? _source.Length : r.Position + (long)segSize;

        // Walk top-level segment children, stopping at the first Cluster (data section).
        while (r.Position < _segmentEnd)
        {
            ulong elemId = r.ReadElementId(out int idBytes);
            ulong elemSize = r.ReadVarInt(out int sizeBytes, out bool elemUnknown);
            long elemEnd = elemUnknown ? _segmentEnd : r.Position + (long)elemSize;
            switch (elemId)
            {
                case MatroskaIds.Info:
                    ParseInfo(r, elemEnd);
                    break;
                case MatroskaIds.Tracks:
                    ParseTracks(r, elemEnd);
                    break;
                case MatroskaIds.Tags:
                    ParseTags(r, elemEnd);
                    break;
                case MatroskaIds.Cluster:
                    // Data section begins here. Rewind to the Cluster header so
                    // ReadSamplesAsync can find it.
                    r.Position = r.Position - sizeBytes - idBytes;
                    _clustersStart = r.Position;
                    return;
                default:
                    r.Skip(elemEnd - r.Position);
                    break;
            }
        }
        _clustersStart = r.Position;
    }

    private long _clustersStart;

    private void ParseInfo(EbmlReader r, long end)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            switch (id)
            {
                case MatroskaIds.TimecodeScale:
                    _timecodeScaleNs = r.ReadUInt((int)size);
                    break;
                case MatroskaIds.Duration:
                    _segmentDurationTicks = r.ReadFloat((int)size);
                    break;
                default:
                    r.Skip((long)size);
                    break;
            }
        }
    }

    private void ParseTracks(EbmlReader r, long end)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            if (id == MatroskaIds.TrackEntry)
            {
                ParseTrackEntry(r, r.Position + (long)size);
            }
            else
            {
                r.Skip((long)size);
            }
        }
    }

    private void ParseTrackEntry(EbmlReader r, long end)
    {
        int trackNumber = 0;
        int trackType = 0;
        string codecId = string.Empty;
        byte[] codecPrivate = [];
        int sampleRate = 0, channels = 0, bitDepth = 0;
        int width = 0, height = 0;
        ulong defaultDurationNs = 0;

        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            long elemEnd = r.Position + (long)size;
            switch (id)
            {
                case MatroskaIds.TrackNumber: trackNumber = (int)r.ReadUInt((int)size); break;
                case MatroskaIds.TrackType: trackType = (int)r.ReadUInt((int)size); break;
                case MatroskaIds.CodecId: codecId = r.ReadString((long)size); break;
                case MatroskaIds.CodecPrivate: codecPrivate = r.ReadBytes((long)size); break;
                case MatroskaIds.DefaultDuration: defaultDurationNs = r.ReadUInt((int)size); break;
                case MatroskaIds.Audio:
                    ParseAudio(r, elemEnd, ref sampleRate, ref channels, ref bitDepth);
                    break;
                case MatroskaIds.Video:
                    ParseVideo(r, elemEnd, ref width, ref height);
                    break;
                default:
                    r.Skip((long)size);
                    break;
            }
        }

        if (trackNumber == 0 || trackType == 0 || string.IsNullOrEmpty(codecId)) return;

        CodecParameters codecParams;
        if (trackType == 2)
        {
            codecParams = new AudioCodecParameters
            {
                Codec = MapAudioCodec(codecId),
                SampleRate = sampleRate,
                Channels = channels,
                BitsPerSample = bitDepth,
                ExtraData = codecPrivate,
            };
        }
        else if (trackType == 1)
        {
            codecParams = new VideoCodecParameters
            {
                Codec = MapVideoCodec(codecId),
                Width = width,
                Height = height,
                ExtraData = codecPrivate,
            };
        }
        else if (trackType == 17)
        {
            codecParams = new SubtitleCodecParameters
            {
                Codec = codecId == "S_TEXT/UTF8" ? CodecId.SubRip : CodecId.Unknown,
                ExtraData = codecPrivate,
            };
        }
        else
        {
            return;
        }

        int trackIndex = _tracks.Count;
        var track = new MediaTrack
        {
            Index = trackIndex,
            Id = (uint)trackNumber,
            TimeBase = new Rational(1, 1_000_000_000 / (int)_timecodeScaleNs),
            Codec = codecParams,
            DurationTicks = 0,
        };
        _tracks.Add(track);
        _trackNumberToIndex[trackNumber] = trackIndex;
        if (defaultDurationNs > 0 && _timecodeScaleNs > 0)
        {
            // Convert ns → cluster ticks. Rounding to nearest gives best lace
            // PTS spacing for codecs whose frame duration is not an integer
            // number of cluster ticks (e.g. AAC 1024/48000 ≈ 21.333 ms with a
            // 1 ms TimecodeScale).
            long ticks = (long)((defaultDurationNs + (_timecodeScaleNs / 2)) / _timecodeScaleNs);
            if (ticks > 0) _defaultDurationTicks[trackIndex] = ticks;
        }
    }

    private static void ParseAudio(EbmlReader r, long end, ref int sampleRate, ref int channels, ref int bitDepth)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            switch (id)
            {
                case MatroskaIds.SamplingFrequency: sampleRate = (int)r.ReadFloat((int)size); break;
                case MatroskaIds.Channels: channels = (int)r.ReadUInt((int)size); break;
                case MatroskaIds.BitDepth: bitDepth = (int)r.ReadUInt((int)size); break;
                default: r.Skip((long)size); break;
            }
        }
    }

    private static void ParseVideo(EbmlReader r, long end, ref int width, ref int height)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            switch (id)
            {
                case MatroskaIds.PixelWidth: width = (int)r.ReadUInt((int)size); break;
                case MatroskaIds.PixelHeight: height = (int)r.ReadUInt((int)size); break;
                default: r.Skip((long)size); break;
            }
        }
    }

    private void ParseTags(EbmlReader r, long end)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            long elemEnd = r.Position + (long)size;
            if (id == MatroskaIds.Tag)
            {
                ParseTag(r, elemEnd);
            }
            else
            {
                r.Skip((long)size);
            }
        }
    }

    private void ParseTag(EbmlReader r, long end)
    {
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            long elemEnd = r.Position + (long)size;
            if (id == MatroskaIds.SimpleTag)
            {
                ParseSimpleTag(r, elemEnd);
            }
            else
            {
                r.Skip((long)size);
            }
        }
    }

    private void ParseSimpleTag(EbmlReader r, long end)
    {
        string name = string.Empty;
        string value = string.Empty;
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            long elemEnd = r.Position + (long)size;
            switch (id)
            {
                case MatroskaIds.TagName:
                    name = r.ReadString((long)size);
                    break;
                case MatroskaIds.TagString:
                    value = r.ReadString((long)size);
                    break;
                case MatroskaIds.SimpleTag:
                    // Nested SimpleTag — recurse so parent tags don't get lost.
                    ParseSimpleTag(r, elemEnd);
                    break;
                default:
                    r.Skip((long)size);
                    break;
            }
        }
        if (name.Length > 0 && value.Length > 0)
        {
            _metaBuilder.Set(name, value);
        }
    }

    private static CodecId MapAudioCodec(string codecId) => codecId switch
    {
        "A_AAC" => CodecId.Aac,
        "A_MPEG/L3" => CodecId.Mp3,
        "A_FLAC" => CodecId.Flac,
        "A_OPUS" => CodecId.Opus,
        "A_VORBIS" => CodecId.Vorbis,
        "A_AC3" => CodecId.Ac3,
        "A_EAC3" => CodecId.EAc3,
        "A_ALAC" => CodecId.Alac,
        _ when codecId.StartsWith("A_PCM/INT/LIT", StringComparison.Ordinal) => CodecId.PcmS16Le,
        _ => CodecId.Unknown,
    };

    private static CodecId MapVideoCodec(string codecId) => codecId switch
    {
        "V_MPEG4/ISO/AVC" => CodecId.H264,
        "V_MPEGH/ISO/HEVC" => CodecId.H265,
        "V_AV1" => CodecId.Av1,
        // Pending standardisation in the Matroska codec registry; AOM has
        // proposed V_AV2 as the placeholder while the codec spec is being
        // finalised. Mediar treats it as opaque samples for passthrough.
        "V_AV2" => CodecId.Av2,
        "V_VP8" => CodecId.Vp8,
        "V_VP9" => CodecId.Vp9,
        "V_MPEG4/ISO/ASP" => CodecId.Mpeg4,
        _ => CodecId.Unknown,
    };

    /// <inheritdoc/>
    public async IAsyncEnumerable<MediaSample> ReadSamplesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.Yield();
        // Convert seek time to cluster-tick units (1 tick = _timecodeScaleNs nanoseconds).
        double scaleSeconds = _timecodeScaleNs / 1_000_000_000.0;
        long targetClusterTicks = _seekSeconds <= 0
            ? long.MinValue
            : (long)Math.Round(_seekSeconds / scaleSeconds);

        var r = new EbmlReader(_source, _clustersStart);
        while (r.Position < _segmentEnd)
        {
            cancellationToken.ThrowIfCancellationRequested();
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out bool unknownSize);
            long end = unknownSize ? _segmentEnd : r.Position + (long)size;

            if (id != MatroskaIds.Cluster)
            {
                r.Position = end;
                continue;
            }

            long clusterTimecode = 0;
            while (r.Position < end)
            {
                ulong elemId = r.ReadElementId(out _);
                ulong elemSize = r.ReadVarInt(out _, out _);
                long elemEnd = r.Position + (long)elemSize;

                if (elemId == MatroskaIds.Timecode)
                {
                    clusterTimecode = (long)r.ReadUInt((int)elemSize);
                    // If the entire cluster ends before the seek target (clusters are
                    // bounded to 30s by our muxer), skip the rest of this Cluster.
                    if (targetClusterTicks > long.MinValue
                        && clusterTimecode + 30_000 < targetClusterTicks)
                    {
                        r.Position = end;
                        break;
                    }
                }
                else if (elemId == MatroskaIds.SimpleBlock)
                {
                    foreach (var sample in DecodeBlock(r, (int)elemSize, clusterTimecode, isSimple: true, blockDuration: 0))
                    {
                        if (sample.Pts < targetClusterTicks) continue;
                        yield return sample;
                    }
                }
                else if (elemId == MatroskaIds.BlockGroup)
                {
                    foreach (var sample in DecodeBlockGroup(r, elemEnd, clusterTimecode))
                    {
                        if (sample.Pts < targetClusterTicks) continue;
                        yield return sample;
                    }
                }
                else
                {
                    r.Skip((long)elemSize);
                }
            }
        }
    }

    /// <inheritdoc/>
    public ValueTask SeekAsync(TimeSpan time, CancellationToken cancellationToken = default)
    {
        _seekSeconds = time < TimeSpan.Zero ? 0 : time.TotalSeconds;
        return ValueTask.CompletedTask;
    }

    private List<MediaSample> DecodeBlockGroup(EbmlReader r, long end, long clusterTimecode)
    {
        long blockDuration = 0;
        // Buffer the raw Block bytes first so we can apply BlockDuration (which
        // may appear AFTER Block in element order) when computing per-frame
        // PTS spacing for laced blocks.
        byte[]? pendingBlock = null;
        int pendingSize = 0;
        while (r.Position < end)
        {
            ulong id = r.ReadElementId(out _);
            ulong size = r.ReadVarInt(out _, out _);
            switch (id)
            {
                case MatroskaIds.Block:
                    pendingBlock = r.ReadBytes((long)size);
                    pendingSize = (int)size;
                    break;
                case MatroskaIds.BlockDuration:
                    blockDuration = (long)r.ReadUInt((int)size);
                    break;
                default:
                    r.Skip((long)size);
                    break;
            }
        }
        var samples = new List<MediaSample>();
        if (pendingBlock is null) return samples;
        foreach (var s in DecodeBlockBytes(pendingBlock, pendingSize, clusterTimecode, isSimple: false, blockDuration))
        {
            samples.Add(s);
        }
        return samples;
    }

    private IEnumerable<MediaSample> DecodeBlock(
        EbmlReader r, int size, long clusterTimecode, bool isSimple, long blockDuration)
    {
        // SimpleBlock (and Block when no BlockDuration is provided): read the
        // raw payload once and hand to the shared per-byte decoder.
        byte[] data = r.ReadBytes(size);
        return DecodeBlockBytes(data, size, clusterTimecode, isSimple, blockDuration);
    }

    private IEnumerable<MediaSample> DecodeBlockBytes(
        byte[] data, int size, long clusterTimecode, bool isSimple, long blockDuration)
    {
        int offset = 0;
        ulong trackNumber = ReadVarIntFromBuffer(data, offset, out int idLen);
        offset += idLen;
        if (offset + 3 > size)
            throw new InvalidDataException("Block header truncated.");
        short relTimecode = (short)((data[offset] << 8) | data[offset + 1]);
        offset += 2;
        byte flags = data[offset++];
        bool isKeyFrame = isSimple ? (flags & 0x80) != 0 : true;
        int lacingBits = (flags >> 1) & 0x03;
        var lacing = (MatroskaLacing)lacingBits;

        if (!_trackNumberToIndex.TryGetValue((int)trackNumber, out int trackIndex))
            yield break;

        long basePts = clusterTimecode + relTimecode;

        if (lacing == MatroskaLacing.None)
        {
            int payloadLen = size - offset;
            byte[] payload = new byte[payloadLen];
            Array.Copy(data, offset, payload, 0, payloadLen);
            yield return new MediaSample
            {
                TrackIndex = trackIndex,
                Pts = basePts,
                Dts = basePts,
                Duration = (int)blockDuration,
                IsKeyFrame = isKeyFrame,
                Data = payload,
            };
            yield break;
        }

        // Decode the lacing size table; everything after the header is frame bytes.
        var bodyAfterFlags = new ReadOnlySpan<byte>(data, offset, size - offset);
        int payloadStart = MatroskaLacingCodec.DecodeSizes(lacing, bodyAfterFlags, out int[] sizes);
        int frameDataStart = offset + payloadStart;

        // Per-frame PTS spacing: prefer the block's own BlockDuration (split
        // evenly), then fall back to the track's DefaultDuration, finally 0.
        long step;
        if (blockDuration > 0 && sizes.Length > 0)
        {
            step = blockDuration / sizes.Length;
        }
        else if (_defaultDurationTicks.TryGetValue(trackIndex, out long dd))
        {
            step = dd;
        }
        else
        {
            step = 0;
        }

        int cursor = frameDataStart;
        for (int i = 0; i < sizes.Length; i++)
        {
            int len = sizes[i];
            if (cursor + len > size)
                throw new InvalidDataException("Laced frame extends past block payload.");
            byte[] payload = new byte[len];
            Array.Copy(data, cursor, payload, 0, len);
            cursor += len;
            long pts = basePts + i * step;
            int dur = blockDuration > 0 && sizes.Length > 0
                ? (int)(blockDuration / sizes.Length)
                : (int)step;
            yield return new MediaSample
            {
                TrackIndex = trackIndex,
                Pts = pts,
                Dts = pts,
                Duration = dur,
                IsKeyFrame = isKeyFrame,
                Data = payload,
            };
        }
    }

    private static ulong ReadVarIntFromBuffer(byte[] buf, int offset, out int bytesRead)
    {
        byte b0 = buf[offset];
        if (b0 == 0) throw new InvalidDataException("Bad var-int in block header.");
        int len = 1;
        byte mask = 0x80;
        while ((b0 & mask) == 0)
        {
            len++;
            mask >>= 1;
            if (len > 8) throw new InvalidDataException("Var-int too long.");
        }
        ulong value = (ulong)(b0 & (mask - 1));
        for (int i = 1; i < len; i++) value = (value << 8) | buf[offset + i];
        bytesRead = len;
        return value;
    }

    private static int VarIntLength(ulong value, bool withLeadingBit)
    {
        // For an N-byte EBML VINT, the first byte contains (N-1) leading zeros
        // followed by a '1' length marker, then 8N-N=7N data bits.
        // With leading bit: value is in [1<<(7N), (1<<(7N+1))-1].
        // Without leading bit: value is in [0, (1<<(7N))-1] (with (1<<(7N))-1 being "unknown").
        if (withLeadingBit)
        {
            for (int len = 1; len <= 8; len++)
            {
                ulong lo = 1UL << (7 * len);
                ulong hi = (len == 8) ? ulong.MaxValue : ((1UL << (7 * len + 1)) - 1);
                if (value >= lo && value <= hi) return len;
            }
            return 8;
        }
        else
        {
            for (int len = 1; len <= 8; len++)
            {
                ulong max = (1UL << (7 * len)) - 1;
                if (value <= max) return len;
            }
            return 8;
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
    public ValueTask DisposeAsync()
    {
        Dispose();
        return ValueTask.CompletedTask;
    }
}
