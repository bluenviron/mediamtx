using System.Buffers.Binary;
using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Parses the <c>moov</c> box of an ISO BMFF file and produces a fully-resolved
/// <see cref="Mp4MovieData"/> with per-track sample tables.
/// </summary>
internal static class MovieParser
{
    /// <summary>
    /// Locate the moov box in <paramref name="source"/>, load it into memory and parse it.
    /// </summary>
    public static Mp4MovieData Parse(IRandomAccessSource source)
    {
        long fileEnd = source.Length;
        BoxHeader? moovHeader = null;

        foreach (var box in BoxScanner.Scan(source, 0, fileEnd))
        {
            if (box.Type.Value == BoxTypes.Moov.Value)
            {
                moovHeader = box;
                break;
            }
        }

        if (moovHeader is null)
        {
            throw new InvalidDataException("Not an ISO BMFF file: no 'moov' box found.");
        }

        var moov = moovHeader.Value;
        if (moov.PayloadLength > int.MaxValue)
        {
            throw new InvalidDataException("moov box larger than 2 GiB is not supported.");
        }

        int moovLen = (int)moov.PayloadLength;
        byte[] backing = new byte[moovLen];
        int read = 0;
        while (read < moovLen)
        {
            int n = source.Read(moov.PayloadOffset + read, backing.AsSpan(read));
            if (n <= 0) throw new EndOfStreamException("Truncated moov box.");
            read += n;
        }

        return ParseMoov(new ReadOnlyMemory<byte>(backing));
    }

    // ---------------------------------------------------------------------
    // Top-level moov walk.
    // ---------------------------------------------------------------------

    private static Mp4MovieData ParseMoov(ReadOnlyMemory<byte> moov)
    {
        uint movieTimeScale = 0;
        ulong movieDuration = 0;
        var tracks = new List<Mp4TrackData>();
        var meta = new MediaMetadataBuilder();

        foreach (var (childType, childPayload) in IterateChildren(moov))
        {
            if (childType.Value == BoxTypes.Mvhd.Value)
            {
                ParseMvhd(childPayload.Span, out movieTimeScale, out movieDuration);
            }
            else if (childType.Value == BoxTypes.Trak.Value)
            {
                var track = ParseTrak(childPayload);
                if (track is not null) tracks.Add(track);
            }
            else if (childType.Value == BoxTypes.Udta.Value)
            {
                Mp4MetadataParser.ParseUdta(childPayload, meta);
            }
            else if (childType.Value == BoxTypes.Meta.Value)
            {
                // Top-level meta (rare; movie-level meta usually lives under udta).
                Mp4MetadataParser.ParseMeta(SkipVersionFlags(childPayload), meta);
            }
        }

        return new Mp4MovieData
        {
            MovieTimeScale = movieTimeScale == 0 ? 1000u : movieTimeScale,
            DurationInMovieTimeScale = movieDuration,
            Tracks = tracks,
            Metadata = meta.Build(),
        };
    }

    private static ReadOnlyMemory<byte> SkipVersionFlags(ReadOnlyMemory<byte> payload)
    {
        return payload.Length >= 4 ? payload[4..] : payload;
    }

    private static void ParseMvhd(ReadOnlySpan<byte> payload, out uint timeScale, out ulong duration)
    {
        var r = new BigEndianSpanReader(payload);
        byte version = r.ReadUInt8();
        r.Skip(3);
        if (version == 1)
        {
            r.Skip(8 + 8);
            timeScale = r.ReadUInt32();
            duration = r.ReadUInt64();
        }
        else
        {
            r.Skip(4 + 4);
            timeScale = r.ReadUInt32();
            duration = r.ReadUInt32();
        }
    }

    // ---------------------------------------------------------------------
    // trak / mdia walk.
    // ---------------------------------------------------------------------

    private static Mp4TrackData? ParseTrak(ReadOnlyMemory<byte> trak)
    {
        uint trackId = 0;
        uint timeScale = 0;
        ulong duration = 0;
        string handler = "";
        string language = "und";
        ReadOnlyMemory<byte> stblPayload = default;

        foreach (var (childType, childPayload) in IterateChildren(trak))
        {
            if (childType.Value == BoxTypes.Tkhd.Value)
            {
                ParseTkhd(childPayload.Span, out trackId);
            }
            else if (childType.Value == BoxTypes.Mdia.Value)
            {
                foreach (var (mediaChildType, mediaPayload) in IterateChildren(childPayload))
                {
                    if (mediaChildType.Value == BoxTypes.Mdhd.Value)
                    {
                        ParseMdhd(mediaPayload.Span, out timeScale, out duration, out language);
                    }
                    else if (mediaChildType.Value == BoxTypes.Hdlr.Value)
                    {
                        handler = ParseHdlr(mediaPayload.Span);
                    }
                    else if (mediaChildType.Value == BoxTypes.Minf.Value)
                    {
                        foreach (var (minfChildType, minfPayload) in IterateChildren(mediaPayload))
                        {
                            if (minfChildType.Value == BoxTypes.Stbl.Value)
                            {
                                stblPayload = minfPayload;
                            }
                        }
                    }
                }
            }
        }

        if (trackId == 0 || timeScale == 0 || stblPayload.IsEmpty)
        {
            return null;
        }

        var (codec, extraData) = ParseStsdCodec(stblPayload, handler);
        var samples = ParseSampleTable(stblPayload);

        var track = new Mp4TrackData
        {
            TrackId = trackId,
            TimeScale = timeScale,
            DurationInTimeScale = duration,
            Handler = handler,
            Language = language,
            Codec = codec,
        };
        track.CodecParameters = BuildCodecParameters(stblPayload, codec, extraData, handler, language);
        track.Samples = samples;
        return track;
    }

    private static void ParseTkhd(ReadOnlySpan<byte> payload, out uint trackId)
    {
        var r = new BigEndianSpanReader(payload);
        byte version = r.ReadUInt8();
        r.Skip(3);
        if (version == 1)
        {
            r.Skip(8 + 8);
            trackId = r.ReadUInt32();
        }
        else
        {
            r.Skip(4 + 4);
            trackId = r.ReadUInt32();
        }
    }

    private static void ParseMdhd(ReadOnlySpan<byte> payload, out uint timeScale, out ulong duration, out string language)
    {
        var r = new BigEndianSpanReader(payload);
        byte version = r.ReadUInt8();
        r.Skip(3);
        if (version == 1)
        {
            r.Skip(8 + 8);
            timeScale = r.ReadUInt32();
            duration = r.ReadUInt64();
        }
        else
        {
            r.Skip(4 + 4);
            timeScale = r.ReadUInt32();
            duration = r.ReadUInt32();
        }

        ushort packedLang = r.ReadUInt16();
        char a = (char)(((packedLang >> 10) & 0x1F) + 0x60);
        char b = (char)(((packedLang >> 5) & 0x1F) + 0x60);
        char c = (char)((packedLang & 0x1F) + 0x60);
        language = a is < 'a' or > 'z' ? "und" : new string([a, b, c]);
    }

    private static string ParseHdlr(ReadOnlySpan<byte> payload)
    {
        var r = new BigEndianSpanReader(payload);
        r.Skip(4);
        r.Skip(4);
        uint handlerType = r.ReadUInt32();
        return new FourCc(handlerType).ToString();
    }

    // ---------------------------------------------------------------------
    // stsd codec id + extradata
    // ---------------------------------------------------------------------

    private static (CodecId Codec, ReadOnlyMemory<byte> ExtraData) ParseStsdCodec(
        ReadOnlyMemory<byte> stbl,
        string handler)
    {
        foreach (var (childType, payload) in IterateChildren(stbl))
        {
            if (childType.Value != BoxTypes.Stsd.Value) continue;

            var span = payload.Span;
            var r = new BigEndianSpanReader(span);
            r.Skip(4);
            uint count = r.ReadUInt32();
            if (count == 0) break;

            int entryStart = r.Position;
            uint entrySize = r.ReadUInt32();
            uint entryType = r.ReadUInt32();
            r.Skip(6 + 2);

            if (entryStart + entrySize > span.Length || entrySize < 16)
            {
                return (CodecId.Unknown, ReadOnlyMemory<byte>.Empty);
            }
            int afterHeader = r.Position;
            int entryEnd = entryStart + (int)entrySize;

            CodecId codec = MapSampleEntry(new FourCc(entryType));
            int fixedHeaderLen = handler == "soun" ? 20 : handler == "vide" ? 70 : 0;
            int childStart = Math.Min(entryEnd, afterHeader + fixedHeaderLen);

            ReadOnlyMemory<byte> extra = ReadOnlyMemory<byte>.Empty;
            var childRegion = payload[childStart..entryEnd];
            foreach (var (cType, cPayload) in IterateChildren(childRegion))
            {
                if (cType.Value == BoxTypes.AvcC.Value ||
                    cType.Value == BoxTypes.HvcC.Value ||
                    cType.Value == BoxTypes.Av1C.Value ||
                    cType.Value == BoxTypes.EsDs.Value)
                {
                    extra = cPayload.ToArray();
                    break;
                }
            }

            return (codec, extra);
        }

        return (CodecId.Unknown, ReadOnlyMemory<byte>.Empty);
    }

    private static CodecId MapSampleEntry(FourCc t)
    {
        if (t.Value == BoxTypes.Avc1.Value || t.Value == BoxTypes.Avc3.Value) return CodecId.H264;
        if (t.Value == BoxTypes.Hvc1.Value || t.Value == BoxTypes.Hev1.Value) return CodecId.H265;
        if (t.Value == BoxTypes.Av01.Value) return CodecId.Av1;
        if (t.Value == BoxTypes.Av02.Value) return CodecId.Av2;
        if (t.Value == BoxTypes.Vp09.Value) return CodecId.Vp9;
        if (t.Value == BoxTypes.Mp4v.Value) return CodecId.Mpeg4;
        if (t.Value == BoxTypes.Mp4a.Value) return CodecId.Aac;
        if (t.Value == BoxTypes.Alac.Value) return CodecId.Alac;
        if (t.Value == BoxTypes.Opus.Value) return CodecId.Opus;
        if (t.Value == BoxTypes.FlacEntry.Value) return CodecId.Flac;
        if (t.Value == BoxTypes.Tx3g.Value) return CodecId.Tx3g;
        if (t.Value == BoxTypes.Wvtt.Value) return CodecId.WebVtt;
        return CodecId.Unknown;
    }

    private static CodecParameters BuildCodecParameters(
        ReadOnlyMemory<byte> stbl,
        CodecId codec,
        ReadOnlyMemory<byte> extraData,
        string handler,
        string language)
    {
        foreach (var (childType, payload) in IterateChildren(stbl))
        {
            if (childType.Value != BoxTypes.Stsd.Value) continue;

            var span = payload.Span;
            var r = new BigEndianSpanReader(span);
            r.Skip(4); r.ReadUInt32();
            uint entrySize = r.ReadUInt32();
            r.ReadUInt32();
            r.Skip(6 + 2);
            if (entrySize < 16) break;

            if (handler == "vide")
            {
                r.Skip(16);
                int width = r.ReadUInt16();
                int height = r.ReadUInt16();
                return new VideoCodecParameters
                {
                    Codec = codec,
                    Width = width,
                    Height = height,
                    SampleAspectRatio = Rational.One,
                    FrameRate = default,
                    ExtraData = extraData,
                };
            }
            if (handler == "soun")
            {
                r.Skip(8);
                int channels = r.ReadUInt16();
                int bps = r.ReadUInt16();
                r.Skip(4);
                int sampleRate = (int)(r.ReadUInt32() >> 16);
                return new AudioCodecParameters
                {
                    Codec = codec,
                    Channels = channels,
                    BitsPerSample = bps,
                    SampleRate = sampleRate,
                    ExtraData = extraData,
                };
            }
            break;
        }

        return new SubtitleCodecParameters
        {
            Codec = codec == CodecId.Unknown && (handler == "subt" || handler == "text" || handler == "sbtl")
                ? CodecId.Tx3g : codec,
            Language = language,
            ExtraData = extraData,
        };
    }

    // ---------------------------------------------------------------------
    // stbl → SampleRecord[]
    // ---------------------------------------------------------------------

    private static SampleRecord[] ParseSampleTable(ReadOnlyMemory<byte> stbl)
    {
        ReadOnlyMemory<byte> sttsPayload = default, cttsPayload = default;
        ReadOnlyMemory<byte> stscPayload = default, stszPayload = default;
        ReadOnlyMemory<byte> stcoPayload = default, co64Payload = default;
        ReadOnlyMemory<byte> stssPayload = default;
        bool isStz2 = false;

        foreach (var (childType, payload) in IterateChildren(stbl))
        {
            uint v = childType.Value;
            if (v == BoxTypes.Stts.Value) sttsPayload = payload;
            else if (v == BoxTypes.Ctts.Value) cttsPayload = payload;
            else if (v == BoxTypes.Stsc.Value) stscPayload = payload;
            else if (v == BoxTypes.Stsz.Value) stszPayload = payload;
            else if (v == BoxTypes.Stz2.Value) { stszPayload = payload; isStz2 = true; }
            else if (v == BoxTypes.Stco.Value) stcoPayload = payload;
            else if (v == BoxTypes.Co64.Value) co64Payload = payload;
            else if (v == BoxTypes.Stss.Value) stssPayload = payload;
        }

        if (stszPayload.IsEmpty || stscPayload.IsEmpty ||
            (stcoPayload.IsEmpty && co64Payload.IsEmpty))
        {
            return [];
        }

        int sampleCount;
        uint uniformSize;

        if (!isStz2)
        {
            var sr = new BigEndianSpanReader(stszPayload.Span);
            sr.Skip(4);
            uniformSize = sr.ReadUInt32();
            sampleCount = (int)sr.ReadUInt32();

            ReadOnlyMemory<byte> sizesPacked = uniformSize == 0
                ? stszPayload[(4 + 8)..]
                : ReadOnlyMemory<byte>.Empty;

            return ParseSampleTableStsz(
                sampleCount, uniformSize, sizesPacked,
                stscPayload, stcoPayload, co64Payload,
                sttsPayload, cttsPayload, stssPayload);
        }
        else
        {
            var sr = new BigEndianSpanReader(stszPayload.Span);
            sr.Skip(4);
            sr.Skip(3);
            byte fieldSize = sr.ReadUInt8();
            sampleCount = (int)sr.ReadUInt32();
            if (fieldSize != 4 && fieldSize != 8 && fieldSize != 16)
            {
                throw new InvalidDataException($"Unsupported stz2 field_size {fieldSize}.");
            }
            int bytesNeeded = (sampleCount * fieldSize + 7) / 8;
            int sizesStart = 4 + 3 + 1 + 4;
            ReadOnlyMemory<byte> sizesPacked = stszPayload.Slice(sizesStart, Math.Min(bytesNeeded, stszPayload.Length - sizesStart));
            return ParseSampleTableStz2(
                sampleCount, fieldSize, sizesPacked,
                stscPayload, stcoPayload, co64Payload,
                sttsPayload, cttsPayload, stssPayload);
        }
    }

    private static SampleRecord[] ParseSampleTableStsz(
        int sampleCount,
        uint uniformSize,
        ReadOnlyMemory<byte> sizesPacked,
        ReadOnlyMemory<byte> stscPayload,
        ReadOnlyMemory<byte> stcoPayload,
        ReadOnlyMemory<byte> co64Payload,
        ReadOnlyMemory<byte> sttsPayload,
        ReadOnlyMemory<byte> cttsPayload,
        ReadOnlyMemory<byte> stssPayload)
    {
        var samples = new SampleRecord[sampleCount];
        if (uniformSize > 0)
        {
            int size = (int)uniformSize;
            for (int i = 0; i < sampleCount; i++) samples[i].Size = size;
        }
        else
        {
            var s = sizesPacked.Span;
            for (int i = 0; i < sampleCount; i++)
            {
                samples[i].Size = (int)BinaryPrimitives.ReadUInt32BigEndian(s.Slice(i * 4, 4));
            }
        }
        FillOffsetsTimestampsAndSync(samples, stscPayload, stcoPayload, co64Payload,
            sttsPayload, cttsPayload, stssPayload);
        return samples;
    }

    private static SampleRecord[] ParseSampleTableStz2(
        int sampleCount,
        byte fieldSize,
        ReadOnlyMemory<byte> sizesPacked,
        ReadOnlyMemory<byte> stscPayload,
        ReadOnlyMemory<byte> stcoPayload,
        ReadOnlyMemory<byte> co64Payload,
        ReadOnlyMemory<byte> sttsPayload,
        ReadOnlyMemory<byte> cttsPayload,
        ReadOnlyMemory<byte> stssPayload)
    {
        var samples = new SampleRecord[sampleCount];
        var s = sizesPacked.Span;
        if (fieldSize == 16)
        {
            for (int i = 0; i < sampleCount; i++)
                samples[i].Size = BinaryPrimitives.ReadUInt16BigEndian(s.Slice(i * 2, 2));
        }
        else if (fieldSize == 8)
        {
            for (int i = 0; i < sampleCount; i++)
                samples[i].Size = s[i];
        }
        else
        {
            for (int i = 0; i < sampleCount; i++)
            {
                byte b = s[i / 2];
                samples[i].Size = (i & 1) == 0 ? (b >> 4) : (b & 0x0F);
            }
        }
        FillOffsetsTimestampsAndSync(samples, stscPayload, stcoPayload, co64Payload,
            sttsPayload, cttsPayload, stssPayload);
        return samples;
    }

    private static void FillOffsetsTimestampsAndSync(
        SampleRecord[] samples,
        ReadOnlyMemory<byte> stscPayload,
        ReadOnlyMemory<byte> stcoPayload,
        ReadOnlyMemory<byte> co64Payload,
        ReadOnlyMemory<byte> sttsPayload,
        ReadOnlyMemory<byte> cttsPayload,
        ReadOnlyMemory<byte> stssPayload)
    {
        bool isCo64 = !co64Payload.IsEmpty;
        ReadOnlyMemory<byte> chunkOffsetsPayload = isCo64 ? co64Payload : stcoPayload;
        int chunkCount;
        {
            var r = new BigEndianSpanReader(chunkOffsetsPayload.Span);
            r.Skip(4);
            chunkCount = (int)r.ReadUInt32();
        }

        int stscCount;
        var stscSpan = stscPayload.Span;
        {
            var r = new BigEndianSpanReader(stscSpan);
            r.Skip(4);
            stscCount = (int)r.ReadUInt32();
        }
        if (stscCount == 0) return;

        Span<uint> firstChunks = stscCount <= 256 ? stackalloc uint[stscCount] : new uint[stscCount];
        Span<uint> samplesPerChunkArr = stscCount <= 256 ? stackalloc uint[stscCount] : new uint[stscCount];
        {
            var r = new BigEndianSpanReader(stscSpan);
            r.Skip(8);
            for (int i = 0; i < stscCount; i++)
            {
                firstChunks[i] = r.ReadUInt32();
                samplesPerChunkArr[i] = r.ReadUInt32();
                r.Skip(4);
            }
        }

        int sampleIdx = 0;
        int stscEntry = 0;
        {
            var coR = new BigEndianSpanReader(chunkOffsetsPayload.Span);
            coR.Skip(8);
            for (int chunkIdx = 1; chunkIdx <= chunkCount; chunkIdx++)
            {
                while (stscEntry + 1 < stscCount && firstChunks[stscEntry + 1] <= chunkIdx)
                {
                    stscEntry++;
                }
                uint spc = samplesPerChunkArr[stscEntry];
                long offset = isCo64 ? (long)coR.ReadUInt64() : coR.ReadUInt32();
                for (uint k = 0; k < spc; k++)
                {
                    if (sampleIdx >= samples.Length) break;
                    samples[sampleIdx].Offset = offset;
                    offset += samples[sampleIdx].Size;
                    sampleIdx++;
                }
            }
        }

        if (!sttsPayload.IsEmpty)
        {
            var r = new BigEndianSpanReader(sttsPayload.Span);
            r.Skip(4);
            uint count = r.ReadUInt32();
            long dts = 0;
            int si = 0;
            for (uint i = 0; i < count && si < samples.Length; i++)
            {
                uint sampleCount = r.ReadUInt32();
                int delta = (int)r.ReadUInt32();
                for (uint k = 0; k < sampleCount && si < samples.Length; k++)
                {
                    samples[si].Dts = dts;
                    samples[si].Duration = delta;
                    dts += delta;
                    si++;
                }
            }
        }

        if (!cttsPayload.IsEmpty)
        {
            var r = new BigEndianSpanReader(cttsPayload.Span);
            byte version = r.ReadUInt8();
            r.Skip(3);
            uint count = r.ReadUInt32();
            int si = 0;
            for (uint i = 0; i < count && si < samples.Length; i++)
            {
                uint sampleCount = r.ReadUInt32();
                int offset = version == 0 ? (int)r.ReadUInt32() : r.ReadInt32();
                for (uint k = 0; k < sampleCount && si < samples.Length; k++)
                {
                    samples[si].CtsOffset = offset;
                    si++;
                }
            }
        }

        if (!stssPayload.IsEmpty)
        {
            var r = new BigEndianSpanReader(stssPayload.Span);
            r.Skip(4);
            uint count = r.ReadUInt32();
            for (uint i = 0; i < count; i++)
            {
                uint sn = r.ReadUInt32();
                if (sn >= 1 && sn <= samples.Length)
                {
                    samples[sn - 1].IsKey = true;
                }
            }
        }
        else
        {
            for (int i = 0; i < samples.Length; i++) samples[i].IsKey = true;
        }
    }

    // ---------------------------------------------------------------------
    // Helpers
    // ---------------------------------------------------------------------

    internal static IEnumerable<BoxChild> IterateChildren(ReadOnlyMemory<byte> container)
    {
        int pos = 0;
        while (pos + 8 <= container.Length)
        {
            uint size = BinaryPrimitives.ReadUInt32BigEndian(container.Span.Slice(pos, 4));
            uint type = BinaryPrimitives.ReadUInt32BigEndian(container.Span.Slice(pos + 4, 4));
            int headerLen = 8;
            long payloadLen;

            if (size == 1)
            {
                if (pos + 16 > container.Length) yield break;
                ulong large = BinaryPrimitives.ReadUInt64BigEndian(container.Span.Slice(pos + 8, 8));
                if (large < 16) yield break;
                headerLen = 16;
                payloadLen = (long)large - 16;
            }
            else if (size == 0)
            {
                payloadLen = container.Length - (pos + headerLen);
            }
            else
            {
                if (size < 8) yield break;
                payloadLen = size - 8;
            }

            int payloadStart = pos + headerLen;
            if (payloadStart + payloadLen > container.Length) yield break;
            yield return new BoxChild(new FourCc(type), container.Slice(payloadStart, (int)payloadLen));
            pos = payloadStart + (int)payloadLen;
        }
    }

    internal readonly record struct BoxChild(FourCc Type, ReadOnlyMemory<byte> Payload);
}
