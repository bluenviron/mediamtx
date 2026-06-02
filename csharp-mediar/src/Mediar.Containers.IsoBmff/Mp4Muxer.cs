using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Streaming muxer for ISO Base Media File Format (MP4, MOV, M4A).
/// Writes <c>ftyp</c> + <c>mdat</c> first while collecting per-sample tables, then
/// emits <c>moov</c> at the end. Suitable for offline remuxing operations such as
/// extract-audio and embed-subtitles. The destination <see cref="Stream"/> must support
/// seeking so the muxer can patch the <c>mdat</c> size on close.
/// </summary>
public sealed class Mp4Muxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private readonly List<TrackEntry> _tracks = new();
    private long _mdatStart;
    private long _mdatPayloadStart;
    private bool _started;
    private bool _finished;
    private bool _disposed;
    private uint _movieTimeScale = 1000;

    /// <summary>Create a new muxer writing to <paramref name="output"/>.</summary>
    public Mp4Muxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite || !output.CanSeek)
        {
            throw new ArgumentException("Stream must be writable and seekable.", nameof(output));
        }
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "mp4";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        if (_started) throw new InvalidOperationException("Cannot add tracks after StartAsync.");
        _tracks.Add(new TrackEntry(track));
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_started) throw new InvalidOperationException("Muxer already started.");
        if (_tracks.Count == 0) throw new InvalidOperationException("No tracks added.");

        _started = true;
        _movieTimeScale = ChooseMovieTimeScale();

        // ftyp
        var ftyp = new BoxBuilder(64);
        ftyp.StartBox(BoxTypes.Ftyp);
        ftyp.WriteAscii("isom");
        ftyp.WriteUInt32(512);
        ftyp.WriteAscii("isomiso2avc1mp41");
        ftyp.EndBox();
        await _output.WriteAsync(ftyp.WrittenSpan.ToArray(), cancellationToken).ConfigureAwait(false);

        // mdat header with 64-bit largesize so we never need to truncate
        _mdatStart = _output.Position;
        byte[] mdatHeader = new byte[16];
        // size = 1 (use 64-bit largesize)
        mdatHeader[0] = 0; mdatHeader[1] = 0; mdatHeader[2] = 0; mdatHeader[3] = 1;
        mdatHeader[4] = (byte)'m'; mdatHeader[5] = (byte)'d';
        mdatHeader[6] = (byte)'a'; mdatHeader[7] = (byte)'t';
        // 8..15 = 64-bit largesize (placeholder, patched in Finish)
        await _output.WriteAsync(mdatHeader, cancellationToken).ConfigureAwait(false);
        _mdatPayloadStart = _output.Position;
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) throw new InvalidOperationException("Muxer already finished.");
        if ((uint)sample.TrackIndex >= (uint)_tracks.Count)
        {
            throw new ArgumentOutOfRangeException(nameof(sample), "Unknown TrackIndex.");
        }

        var entry = _tracks[sample.TrackIndex];
        long offset = _output.Position;
        await _output.WriteAsync(sample.Data, cancellationToken).ConfigureAwait(false);

        entry.Samples.Add(new MuxSample
        {
            Offset = offset,
            Size = sample.Data.Length,
            Dts = sample.Dts,
            CtsOffset = (int)(sample.Pts - sample.Dts),
            Duration = sample.Duration,
            IsKey = sample.IsKeyFrame,
        });
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) return;
        _finished = true;

        long mdatPayloadEnd = _output.Position;
        long mdatPayloadLen = mdatPayloadEnd - _mdatPayloadStart;
        long mdatTotalLen = mdatPayloadEnd - _mdatStart;

        // Patch mdat largesize (offset _mdatStart + 8, 8 bytes BE)
        _output.Position = _mdatStart + 8;
        Span<byte> largesize = stackalloc byte[8];
        ulong size = (ulong)mdatTotalLen;
        for (int i = 0; i < 8; i++) largesize[i] = (byte)(size >> (56 - i * 8));
        await _output.WriteAsync(largesize.ToArray(), cancellationToken).ConfigureAwait(false);

        // Append moov.
        _output.Position = mdatPayloadEnd;
        var moov = BuildMoov();
        await _output.WriteAsync(moov, cancellationToken).ConfigureAwait(false);
        await _output.FlushAsync(cancellationToken).ConfigureAwait(false);

        _ = mdatPayloadLen; // silence unused (used for diagnostics)
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished)
        {
            try { FinishAsync().AsTask().GetAwaiter().GetResult(); } catch { /* best effort */ }
        }
        if (!_leaveOpen) _output.Dispose();
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished)
        {
            try { await FinishAsync().ConfigureAwait(false); } catch { /* best effort */ }
        }
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }

    // ---------------------------------------------------------------------
    // moov assembly
    // ---------------------------------------------------------------------

    private uint ChooseMovieTimeScale()
    {
        // Use the LCM-friendly choice: largest track timescale, or 1000 as a sane default.
        uint best = 1000;
        foreach (var t in _tracks)
        {
            if (t.Track.TimeBase.Denominator > best && t.Track.TimeBase.Numerator == 1)
            {
                best = (uint)t.Track.TimeBase.Denominator;
            }
        }
        return best;
    }

    private byte[] BuildMoov()
    {
        var b = new BoxBuilder(64 * 1024);

        // moov
        b.StartBox(BoxTypes.Moov);

        // mvhd (v0)
        b.StartFullBox(BoxTypes.Mvhd, 0, 0);
        b.WriteUInt32(0); // creation
        b.WriteUInt32(0); // modification
        b.WriteUInt32(_movieTimeScale);
        b.WriteUInt32((uint)ComputeMovieDurationInMovieTimescale());
        b.WriteUInt32(0x00010000); // rate 1.0
        b.WriteUInt16(0x0100);     // volume 1.0
        b.WriteZeros(2 + 8);       // reserved
        WriteUnityMatrix(b);
        b.WriteZeros(24);          // pre_defined
        b.WriteUInt32((uint)(_tracks.Count + 1));
        b.EndBox();

        // Each track
        for (int i = 0; i < _tracks.Count; i++)
        {
            BuildTrak(b, _tracks[i], (uint)(i + 1));
        }

        b.EndBox(); // moov
        return b.ToArray();
    }

    private long ComputeMovieDurationInMovieTimescale()
    {
        long maxDur = 0;
        foreach (var t in _tracks)
        {
            long durTicks = SumDuration(t);
            // Rescale durTicks from track timebase to movie timescale.
            long rescaled = t.Track.TimeBase.Rescale(durTicks, new Rational(1, (int)_movieTimeScale));
            if (rescaled > maxDur) maxDur = rescaled;
        }
        return maxDur;
    }

    private static long SumDuration(TrackEntry entry)
    {
        long sum = 0;
        foreach (var s in entry.Samples) sum += s.Duration;
        return sum;
    }

    private void BuildTrak(BoxBuilder b, TrackEntry entry, uint trackId)
    {
        long durTicks = SumDuration(entry);
        long movieDur = entry.Track.TimeBase.Rescale(durTicks, new Rational(1, (int)_movieTimeScale));

        b.StartBox(BoxTypes.Trak);

        // tkhd (v0, flags=0x000007: Track_enabled|in_movie|in_preview)
        b.StartFullBox(BoxTypes.Tkhd, 0, 0x000007);
        b.WriteUInt32(0); // creation
        b.WriteUInt32(0); // modification
        b.WriteUInt32(trackId);
        b.WriteUInt32(0); // reserved
        b.WriteUInt32((uint)movieDur);
        b.WriteZeros(8);          // reserved
        b.WriteUInt16(0);         // layer
        b.WriteUInt16(0);         // alternate_group
        b.WriteUInt16(entry.Track.Kind == StreamKind.Audio ? (ushort)0x0100 : (ushort)0); // volume
        b.WriteUInt16(0);         // reserved
        WriteUnityMatrix(b);
        if (entry.Track.Codec is VideoCodecParameters vp)
        {
            b.WriteUInt32((uint)(vp.Width << 16));
            b.WriteUInt32((uint)(vp.Height << 16));
        }
        else
        {
            b.WriteUInt32(0);
            b.WriteUInt32(0);
        }
        b.EndBox();

        // mdia
        b.StartBox(BoxTypes.Mdia);

        // mdhd (v0)
        b.StartFullBox(BoxTypes.Mdhd, 0, 0);
        b.WriteUInt32(0);
        b.WriteUInt32(0);
        b.WriteUInt32((uint)entry.Track.TimeBase.Denominator);
        b.WriteUInt32((uint)durTicks);
        b.WriteLanguage(entry.Track.Language);
        b.WriteUInt16(0); // pre_defined
        b.EndBox();

        // hdlr
        b.StartFullBox(BoxTypes.Hdlr, 0, 0);
        b.WriteUInt32(0); // pre_defined
        b.WriteUInt32(HandlerFor(entry.Track.Kind));
        b.WriteZeros(12);
        b.WriteAscii(entry.Track.Name ?? "Mediar");
        b.WriteUInt8(0);  // null-terminator
        b.EndBox();

        // minf
        b.StartBox(BoxTypes.Minf);
        if (entry.Track.Kind == StreamKind.Video)
        {
            b.StartFullBox(BoxTypes.Vmhd, 0, 1);
            b.WriteUInt16(0); // graphicsmode
            b.WriteUInt16(0); b.WriteUInt16(0); b.WriteUInt16(0); // opcolor
            b.EndBox();
        }
        else if (entry.Track.Kind == StreamKind.Audio)
        {
            b.StartFullBox(BoxTypes.Smhd, 0, 0);
            b.WriteUInt16(0); // balance
            b.WriteUInt16(0); // reserved
            b.EndBox();
        }
        else
        {
            b.StartFullBox(BoxTypes.Nmhd, 0, 0);
            b.EndBox();
        }

        // dinf > dref > url
        b.StartBox(BoxTypes.Dinf);
        b.StartFullBox(BoxTypes.Dref, 0, 0);
        b.WriteUInt32(1);
        b.StartFullBox(new FourCc("url "), 0, 1); // self-contained flag
        b.EndBox();
        b.EndBox();
        b.EndBox();

        // stbl
        b.StartBox(BoxTypes.Stbl);
        BuildStsd(b, entry);
        BuildStts(b, entry);
        BuildCtts(b, entry);
        BuildStsc(b, entry);
        BuildStsz(b, entry);
        BuildCo64(b, entry);
        BuildStss(b, entry);
        b.EndBox(); // stbl

        b.EndBox(); // minf
        b.EndBox(); // mdia
        b.EndBox(); // trak
    }

    private static uint HandlerFor(StreamKind kind) => kind switch
    {
        StreamKind.Video => BoxTypes.Vide.Value,
        StreamKind.Audio => BoxTypes.Soun.Value,
        StreamKind.Subtitle => BoxTypes.Sbtl.Value,
        _ => BoxTypes.Meta.Value,
    };

    private static FourCc SampleEntryFor(MediaTrack t) => t.Codec.Codec switch
    {
        CodecId.H264 => BoxTypes.Avc1,
        CodecId.H265 => BoxTypes.Hvc1,
        CodecId.Av1 => BoxTypes.Av01,
        CodecId.Av2 => BoxTypes.Av02,
        CodecId.Vp9 => BoxTypes.Vp09,
        CodecId.Mpeg4 => BoxTypes.Mp4v,
        CodecId.Aac => BoxTypes.Mp4a,
        CodecId.Opus => BoxTypes.Opus,
        CodecId.Flac => BoxTypes.FlacEntry,
        CodecId.Alac => BoxTypes.Alac,
        CodecId.Tx3g => BoxTypes.Tx3g,
        CodecId.WebVtt => BoxTypes.Wvtt,
        _ => new FourCc("unkn"),
    };

    private static void BuildStsd(BoxBuilder b, TrackEntry entry)
    {
        b.StartFullBox(BoxTypes.Stsd, 0, 0);
        b.WriteUInt32(1); // entry count

        var sampleEntry = SampleEntryFor(entry.Track);
        b.StartBox(sampleEntry);
        b.WriteZeros(6);
        b.WriteUInt16(1); // data_reference_index

        if (entry.Track.Codec is VideoCodecParameters v)
        {
            b.WriteZeros(16);
            b.WriteUInt16((ushort)v.Width);
            b.WriteUInt16((ushort)v.Height);
            b.WriteUInt32(0x00480000); // 72 dpi horiz
            b.WriteUInt32(0x00480000); // 72 dpi vert
            b.WriteUInt32(0);
            b.WriteUInt16(1); // frame_count
            b.WriteZeros(32); // compressorname
            b.WriteUInt16(0x0018); // depth
            b.WriteUInt16(0xFFFF); // pre_defined
            if (!v.ExtraData.IsEmpty)
            {
                FourCc cfg = entry.Track.Codec.Codec switch
                {
                    CodecId.H264 => BoxTypes.AvcC,
                    CodecId.H265 => BoxTypes.HvcC,
                    CodecId.Av1 => BoxTypes.Av1C,
                    _ => new FourCc("conf"),
                };
                b.StartBox(cfg);
                b.WriteBytes(v.ExtraData.Span);
                b.EndBox();
            }
        }
        else if (entry.Track.Codec is AudioCodecParameters a)
        {
            b.WriteZeros(8);
            b.WriteUInt16((ushort)a.Channels);
            b.WriteUInt16(a.BitsPerSample > 0 ? (ushort)a.BitsPerSample : (ushort)16);
            b.WriteUInt16(0); b.WriteUInt16(0);
            // Opus-in-ISOBMFF specifies the SampleEntry sample rate as
            // 48000 << 16 regardless of the OpusHead.InputSampleRate field.
            int sampleEntryRate = entry.Track.Codec.Codec == CodecId.Opus ? 48000 : a.SampleRate;
            b.WriteUInt32((uint)(sampleEntryRate << 16));
            if (!a.ExtraData.IsEmpty && entry.Track.Codec.Codec == CodecId.Aac)
            {
                b.StartBox(BoxTypes.EsDs);
                b.WriteBytes(a.ExtraData.Span);
                b.EndBox();
            }
            else if (!a.ExtraData.IsEmpty && entry.Track.Codec.Codec == CodecId.Alac)
            {
                // alac child box: 4-byte FullBox header (version 0, flags 0)
                // followed by the 24-byte ALACSpecificConfig body.
                b.StartBox(BoxTypes.Alac);
                b.WriteZeros(4);
                b.WriteBytes(a.ExtraData.Span);
                b.EndBox();
            }
            else if (entry.Track.Codec.Codec == CodecId.Opus)
            {
                // Opus in ISOBMFF REQUIRES an OpusSpecificBox (dOps) child of
                // the sample entry — without it the track is not conformant.
                // ExtraData is expected to hold the Ogg-form OpusHead bytes
                // (matches OggDemuxer's output convention).
                if (a.ExtraData.IsEmpty || !OpusHead.TryReadOgg(a.ExtraData.Span, out var head))
                {
                    throw new InvalidOperationException(
                        "Opus track ExtraData must hold a valid Ogg-form OpusHead so the muxer can emit a conformant 'dOps' box.");
                }
                b.StartBox(BoxTypes.Dops);
                b.WriteBytes(OpusHead.WriteIsobmff(head));
                b.EndBox();
            }
        }
        else if (entry.Track.Kind == StreamKind.Subtitle)
        {
            // tx3g TextSampleEntry (ETSI TS 126 245 / 3GPP).
            b.WriteUInt32(0);    // displayFlags
            b.WriteUInt8(1);     // horizontal-justification
            b.WriteUInt8(0xFF);  // vertical-justification (-1)
            b.WriteZeros(4);     // background-color-rgba
            // BoxRecord
            b.WriteUInt16(0); b.WriteUInt16(0); b.WriteUInt16(0); b.WriteUInt16(0);
            // StyleRecord
            b.WriteUInt16(0); b.WriteUInt16(0); b.WriteUInt16(0);
            b.WriteUInt8(0x01);
            b.WriteUInt8(0x10);
            b.WriteUInt32(0xFFFFFFFF); // text color rgba (white)
        }

        b.EndBox(); // sample entry
        b.EndBox(); // stsd
    }

    private static void BuildStts(BoxBuilder b, TrackEntry entry)
    {
        b.StartFullBox(BoxTypes.Stts, 0, 0);
        // Run-length encode (duration deltas)
        var samples = entry.Samples;
        var runs = new List<(uint count, uint delta)>();
        uint runCount = 0; uint runDelta = 0;
        foreach (var s in samples)
        {
            uint d = (uint)s.Duration;
            if (runCount == 0) { runCount = 1; runDelta = d; continue; }
            if (d == runDelta) runCount++;
            else { runs.Add((runCount, runDelta)); runCount = 1; runDelta = d; }
        }
        if (runCount > 0) runs.Add((runCount, runDelta));

        b.WriteUInt32((uint)runs.Count);
        foreach (var (count, delta) in runs)
        {
            b.WriteUInt32(count);
            b.WriteUInt32(delta);
        }
        b.EndBox();
    }

    private static void BuildCtts(BoxBuilder b, TrackEntry entry)
    {
        bool anyNonZero = false;
        foreach (var s in entry.Samples)
        {
            if (s.CtsOffset != 0) { anyNonZero = true; break; }
        }
        if (!anyNonZero) return;

        b.StartFullBox(BoxTypes.Ctts, 1, 0); // v1 supports signed offsets
        var runs = new List<(uint count, int offset)>();
        uint runCount = 0; int runOffset = 0;
        foreach (var s in entry.Samples)
        {
            if (runCount == 0) { runCount = 1; runOffset = s.CtsOffset; continue; }
            if (s.CtsOffset == runOffset) runCount++;
            else { runs.Add((runCount, runOffset)); runCount = 1; runOffset = s.CtsOffset; }
        }
        if (runCount > 0) runs.Add((runCount, runOffset));

        b.WriteUInt32((uint)runs.Count);
        foreach (var (count, offset) in runs)
        {
            b.WriteUInt32(count);
            b.WriteInt32(offset);
        }
        b.EndBox();
    }

    private static void BuildStsc(BoxBuilder b, TrackEntry entry)
    {
        // We mux each sample into its own "chunk" — keeps things simple and lets us
        // use a single (1, 1, 1) entry. Tools like ffprobe accept this; it's slightly
        // less compact than interleaved chunks but matches the per-sample offset table
        // we emit via co64.
        b.StartFullBox(BoxTypes.Stsc, 0, 0);
        b.WriteUInt32(1);
        b.WriteUInt32(1); // first_chunk
        b.WriteUInt32(1); // samples_per_chunk
        b.WriteUInt32(1); // sample_description_index
        b.EndBox();
    }

    private static void BuildStsz(BoxBuilder b, TrackEntry entry)
    {
        b.StartFullBox(BoxTypes.Stsz, 0, 0);

        // Check for uniform size.
        bool uniform = true;
        int firstSize = entry.Samples.Count > 0 ? entry.Samples[0].Size : 0;
        for (int i = 1; i < entry.Samples.Count; i++)
        {
            if (entry.Samples[i].Size != firstSize) { uniform = false; break; }
        }

        b.WriteUInt32(uniform ? (uint)firstSize : 0);
        b.WriteUInt32((uint)entry.Samples.Count);
        if (!uniform)
        {
            for (int i = 0; i < entry.Samples.Count; i++) b.WriteUInt32((uint)entry.Samples[i].Size);
        }
        b.EndBox();
    }

    private static void BuildCo64(BoxBuilder b, TrackEntry entry)
    {
        b.StartFullBox(BoxTypes.Co64, 0, 0);
        b.WriteUInt32((uint)entry.Samples.Count);
        for (int i = 0; i < entry.Samples.Count; i++) b.WriteUInt64((ulong)entry.Samples[i].Offset);
        b.EndBox();
    }

    private static void BuildStss(BoxBuilder b, TrackEntry entry)
    {
        // Only emit stss for video tracks. Audio/subtitle: all samples are sync.
        if (entry.Track.Kind != StreamKind.Video) return;

        var keys = new List<uint>();
        for (int i = 0; i < entry.Samples.Count; i++)
        {
            if (entry.Samples[i].IsKey) keys.Add((uint)(i + 1));
        }
        if (keys.Count == 0) return;

        b.StartFullBox(BoxTypes.Stss, 0, 0);
        b.WriteUInt32((uint)keys.Count);
        foreach (var k in keys) b.WriteUInt32(k);
        b.EndBox();
    }

    private static void WriteUnityMatrix(BoxBuilder b)
    {
        // 3x3 fixed-point matrix [[1,0,0],[0,1,0],[0,0,1<<30]].
        b.WriteUInt32(0x00010000); b.WriteUInt32(0); b.WriteUInt32(0);
        b.WriteUInt32(0); b.WriteUInt32(0x00010000); b.WriteUInt32(0);
        b.WriteUInt32(0); b.WriteUInt32(0); b.WriteUInt32(0x40000000);
    }

    // ---------------------------------------------------------------------
    // Internal state
    // ---------------------------------------------------------------------

    private sealed class TrackEntry
    {
        public TrackEntry(MediaTrack track) { Track = track; }
        public MediaTrack Track { get; }
        public List<MuxSample> Samples { get; } = new();
    }

    private struct MuxSample
    {
        public long Offset;
        public int Size;
        public long Dts;
        public int CtsOffset;
        public int Duration;
        public bool IsKey;
    }
}
