using System.Buffers.Binary;

namespace Mediar.Containers.Matroska;

/// <summary>
/// Streaming Matroska / WebM muxer. Writes EBML header + Segment with
/// Info / Tracks / Cluster / SimpleBlock elements.
/// </summary>
/// <remarks>
/// <para>
/// The muxer accepts one or more tracks declared via <see cref="AddTrack"/>
/// before <see cref="StartAsync"/>, and one sample at a time via
/// <see cref="WriteSampleAsync"/>. Samples are grouped into Clusters
/// no larger than 32 seconds each (SimpleBlock relative timestamps are
/// signed 16-bit milliseconds).
/// </para>
/// <para>
/// Limitations: no SeekHead, Cues or Tags are written, so seeking after
/// muxing requires a full demuxer scan. Codec strings follow the Matroska
/// codec mapping registry. For codecs requiring per-sample side data
/// (e.g. AVC NAL length prefix conversion) the caller must pre-format
/// sample data appropriately.
/// </para>
/// </remarks>
public sealed class MatroskaMuxer : IMediaMuxer
{
    /// <summary>Matroska timecode scale: 1 ms in nanoseconds.</summary>
    private const ulong TimecodeScaleNs = 1_000_000;

    /// <summary>Max cluster span in ms (signed 16-bit relative block timestamps).</summary>
    private const int MaxClusterSpanMs = 30_000;

    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private readonly bool _webm;
    private readonly List<MediaTrack> _tracks = new();

    private bool _started;
    private bool _finished;

    // Current cluster state.
    private EbmlWriter? _clusterWriter;
    private long _clusterTimecodeMs = -1;

    public MatroskaMuxer(Stream output, bool webm = false, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Output stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
        _webm = webm;
    }

    /// <inheritdoc/>
    public string FormatName => _webm ? "webm" : "matroska";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_started) throw new InvalidOperationException("Cannot add tracks after Start.");
        if (MapCodecId(track.Codec.Codec) is null)
        {
            throw new ArgumentException($"Matroska muxer does not have a codec string for {track.Codec.Codec}.", nameof(track));
        }
        if (_webm && !IsWebmCodec(track.Codec.Codec))
        {
            throw new ArgumentException($"WebM does not support codec {track.Codec.Codec}.", nameof(track));
        }
        _tracks.Add(track);
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_started) return;
        if (_tracks.Count == 0) throw new InvalidOperationException("No tracks added.");
        _started = true;

        // EBML header
        var ebml = new EbmlWriter();
        ebml.WriteMaster(MatroskaIds.Ebml, w =>
        {
            w.WriteUInt(0x4286, 1);       // EBMLVersion
            w.WriteUInt(0x42F7, 1);       // EBMLReadVersion
            w.WriteUInt(0x42F2, 4);       // EBMLMaxIDLength
            w.WriteUInt(0x42F3, 8);       // EBMLMaxSizeLength
            w.WriteString(0x4282, _webm ? "webm" : "matroska"); // DocType
            w.WriteUInt(0x4287, _webm ? 4UL : 4UL);   // DocTypeVersion
            w.WriteUInt(0x4285, 2);       // DocTypeReadVersion
        });
        await _output.WriteAsync(ebml.ToArray(), cancellationToken).ConfigureAwait(false);

        // Segment header with unknown size (streamed muxer).
        var seg = new EbmlWriter();
        seg.WriteId(MatroskaIds.Segment);
        seg.WriteVintUnknown(8);
        await _output.WriteAsync(seg.ToArray(), cancellationToken).ConfigureAwait(false);

        // Info element
        var info = new EbmlWriter();
        info.WriteMaster(MatroskaIds.Info, w =>
        {
            w.WriteUInt(MatroskaIds.TimecodeScale, TimecodeScaleNs);
            w.WriteString(0x4D80, "Mediar");        // MuxingApp
            w.WriteString(0x5741, "Mediar");        // WritingApp
        });
        await _output.WriteAsync(info.ToArray(), cancellationToken).ConfigureAwait(false);

        // Tracks element
        var tracks = new EbmlWriter();
        tracks.WriteMaster(MatroskaIds.Tracks, ww =>
        {
            foreach (var t in _tracks)
            {
                ww.WriteMaster(MatroskaIds.TrackEntry, te =>
                {
                    te.WriteUInt(MatroskaIds.TrackNumber, t.Id);
                    te.WriteUInt(0x73C5, t.Id);     // TrackUID
                    te.WriteUInt(MatroskaIds.TrackType, t.Kind switch
                    {
                        StreamKind.Video => 1UL,
                        StreamKind.Audio => 2UL,
                        StreamKind.Subtitle => 0x11UL,
                        _ => 0x20UL,
                    });
                    te.WriteUInt(0x9C, 1);          // FlagLacing = 1
                    te.WriteString(MatroskaIds.CodecId, MapCodecId(t.Codec.Codec)!);
                    if (t.Codec.ExtraData.Length > 0)
                    {
                        te.WriteBinary(MatroskaIds.CodecPrivate, t.Codec.ExtraData.Span);
                    }
                    if (!string.IsNullOrEmpty(t.Name)) te.WriteString(0x536E, t.Name); // Name
                    if (!string.IsNullOrEmpty(t.Language)) te.WriteString(0x22B59C, t.Language); // Language

                    if (t.Codec is AudioCodecParameters audio)
                    {
                        te.WriteMaster(MatroskaIds.Audio, a =>
                        {
                            a.WriteFloat64(MatroskaIds.SamplingFrequency, audio.SampleRate);
                            a.WriteUInt(MatroskaIds.Channels, (ulong)audio.Channels);
                            if (audio.BitsPerSample > 0) a.WriteUInt(MatroskaIds.BitDepth, (ulong)audio.BitsPerSample);
                        });
                    }
                    else if (t.Codec is VideoCodecParameters video)
                    {
                        te.WriteMaster(MatroskaIds.Video, v =>
                        {
                            v.WriteUInt(MatroskaIds.PixelWidth, (ulong)video.Width);
                            v.WriteUInt(MatroskaIds.PixelHeight, (ulong)video.Height);
                        });
                    }
                });
            }
        });
        await _output.WriteAsync(tracks.ToArray(), cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) throw new InvalidOperationException("StartAsync must be called first.");
        if (_finished) throw new InvalidOperationException("Muxer is finished.");
        if (sample.TrackIndex < 0 || sample.TrackIndex >= _tracks.Count)
            throw new ArgumentOutOfRangeException(nameof(sample), "TrackIndex out of range.");

        var track = _tracks[sample.TrackIndex];

        // Convert PTS in track timebase to milliseconds.
        long ptsMs = checked(sample.Pts * 1000L * track.TimeBase.Numerator / track.TimeBase.Denominator);

        // Start a new cluster if needed.
        if (_clusterWriter is null ||
            ptsMs - _clusterTimecodeMs > MaxClusterSpanMs ||
            ptsMs - _clusterTimecodeMs < 0)
        {
            await FlushClusterAsync(cancellationToken).ConfigureAwait(false);
            _clusterTimecodeMs = ptsMs;
            _clusterWriter = new EbmlWriter(8192);
            _clusterWriter.WriteUInt(MatroskaIds.Timecode, (ulong)ptsMs);
        }

        short relative = (short)(ptsMs - _clusterTimecodeMs);

        // SimpleBlock payload: VINT trackNum + 16-bit BE relative ts + 1 byte flags + data.
        var sbw = new EbmlWriter(sample.Data.Length + 16);
        sbw.WriteVintLength(track.Id);
        Span<byte> tsFlags = stackalloc byte[3];
        BinaryPrimitives.WriteInt16BigEndian(tsFlags, relative);
        byte flags = 0;
        if (sample.IsKeyFrame) flags |= 0x80;
        tsFlags[2] = flags;
        sbw.WriteRaw(tsFlags);
        sbw.WriteRaw(sample.Data.Span);

        _clusterWriter!.WriteBinary(MatroskaIds.SimpleBlock, sbw.Written);
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return;
        _finished = true;
        await FlushClusterAsync(cancellationToken).ConfigureAwait(false);
        await _output.FlushAsync(cancellationToken).ConfigureAwait(false);
    }

    private async ValueTask FlushClusterAsync(CancellationToken cancellationToken)
    {
        if (_clusterWriter is null) return;
        var inner = _clusterWriter;
        _clusterWriter = null;
        var outer = new EbmlWriter(inner.Length + 16);
        outer.WriteId(MatroskaIds.Cluster);
        outer.WriteVintLength(inner.Length);
        outer.WriteRaw(inner.Written);
        await _output.WriteAsync(outer.ToArray(), cancellationToken).ConfigureAwait(false);
    }

    private static string? MapCodecId(CodecId codec) => codec switch
    {
        CodecId.Opus => "A_OPUS",
        CodecId.Vorbis => "A_VORBIS",
        CodecId.Flac => "A_FLAC",
        CodecId.Aac => "A_AAC",
        CodecId.Mp3 => "A_MPEG/L3",
        CodecId.Ac3 => "A_AC3",
        CodecId.EAc3 => "A_EAC3",
        CodecId.Alac => "A_ALAC",
        CodecId.G711MuLaw => "A_MS/ACM",
        CodecId.G711ALaw => "A_MS/ACM",
        CodecId.PcmS16Le => "A_PCM/INT/LIT",
        CodecId.PcmS16Be => "A_PCM/INT/BIG",
        CodecId.PcmS24Le => "A_PCM/INT/LIT",
        CodecId.PcmS32Le => "A_PCM/INT/LIT",
        CodecId.PcmF32Le => "A_PCM/FLOAT/IEEE",
        CodecId.H264 => "V_MPEG4/ISO/AVC",
        CodecId.H265 => "V_MPEGH/ISO/HEVC",
        CodecId.Av1 => "V_AV1",
        CodecId.Av2 => "V_AV2",
        CodecId.Vp8 => "V_VP8",
        CodecId.Vp9 => "V_VP9",
        CodecId.Mpeg4 => "V_MPEG4/ISO/ASP",
        CodecId.SubRip => "S_TEXT/UTF8",
        CodecId.WebVtt => "S_TEXT/WEBVTT",
        CodecId.Ass => "S_TEXT/ASS",
        _ => null,
    };

    private static bool IsWebmCodec(CodecId codec) => codec is
        CodecId.Opus or CodecId.Vorbis or CodecId.Vp8 or CodecId.Vp9 or CodecId.Av1 or CodecId.WebVtt;

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (!_finished && _started) await FinishAsync().ConfigureAwait(false);
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (!_finished && _started) FinishAsync().AsTask().GetAwaiter().GetResult();
        if (!_leaveOpen) _output.Dispose();
    }
}
