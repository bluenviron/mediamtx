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
    private readonly Dictionary<int, LacingConfig> _lacing = new();
    private readonly Dictionary<int, List<MediaSample>> _pendingLace = new();
    private readonly Dictionary<int, TimeSpan> _defaultDurations = new();

    private bool _started;
    private bool _finished;

    // Current cluster state.
    private EbmlWriter? _clusterWriter;
    private long _clusterTimecodeMs = -1;

    private readonly struct LacingConfig
    {
        public LacingConfig(MatroskaLacing mode, int maxFrames)
        {
            Mode = mode;
            MaxFrames = maxFrames;
        }
        public MatroskaLacing Mode { get; }
        public int MaxFrames { get; }
    }

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

    /// <summary>
    /// Enable Block / SimpleBlock lacing for the given track. Must be called
    /// after <see cref="AddTrack"/> and before <see cref="StartAsync"/>.
    /// </summary>
    /// <param name="trackIndex">Index returned by <see cref="AddTrack"/> order.</param>
    /// <param name="mode">Lacing dialect to use. <see cref="MatroskaLacing.None"/> disables lacing.</param>
    /// <param name="maxFramesPerBlock">
    /// Soft cap on the number of frames per laced block. Matroska's lacing
    /// header stores <c>frame_count - 1</c> in one byte, so the hard ceiling
    /// is 256. Practical values are typically 4-16 for audio.
    /// </param>
    /// <remarks>
    /// <para>
    /// Laced SimpleBlocks only record the timestamp of the first frame. To
    /// recover per-frame PTS, downstream demuxers rely on the track's
    /// <c>DefaultDuration</c>. Configure that via
    /// <see cref="SetDefaultDuration(int, TimeSpan)"/> for any laced track —
    /// otherwise the demuxer will assign identical PTS to every frame in the
    /// lace, which usually breaks playback.
    /// </para>
    /// </remarks>
    public void SetLacing(int trackIndex, MatroskaLacing mode, int maxFramesPerBlock = 8)
    {
        if (_started) throw new InvalidOperationException("Cannot configure lacing after Start.");
        if (trackIndex < 0 || trackIndex >= _tracks.Count)
            throw new ArgumentOutOfRangeException(nameof(trackIndex));
        if (maxFramesPerBlock < 1 || maxFramesPerBlock > MatroskaLacingCodec.MaxFrames)
            throw new ArgumentOutOfRangeException(nameof(maxFramesPerBlock),
                $"Must be 1..{MatroskaLacingCodec.MaxFrames}.");
        if (mode == MatroskaLacing.None)
        {
            _lacing.Remove(trackIndex);
        }
        else
        {
            _lacing[trackIndex] = new LacingConfig(mode, maxFramesPerBlock);
        }
    }

    /// <summary>
    /// Set the <c>DefaultDuration</c> element for a track. Required by the
    /// Matroska spec for any track that uses lacing on SimpleBlock; otherwise
    /// downstream demuxers cannot derive per-frame PTS within a lace.
    /// </summary>
    public void SetDefaultDuration(int trackIndex, TimeSpan duration)
    {
        if (_started) throw new InvalidOperationException("Cannot configure default duration after Start.");
        if (trackIndex < 0 || trackIndex >= _tracks.Count)
            throw new ArgumentOutOfRangeException(nameof(trackIndex));
        if (duration <= TimeSpan.Zero)
            throw new ArgumentOutOfRangeException(nameof(duration), "Must be > 0.");
        _defaultDurations[trackIndex] = duration;
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
            for (int i = 0; i < _tracks.Count; i++)
            {
                var t = _tracks[i];
                int idx = i;
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
                    bool tracksLaced = _lacing.ContainsKey(idx);
                    te.WriteUInt(0x9C, tracksLaced ? 1UL : 0UL); // FlagLacing
                    te.WriteString(MatroskaIds.CodecId, MapCodecId(t.Codec.Codec)!);
                    if (t.Codec.ExtraData.Length > 0)
                    {
                        te.WriteBinary(MatroskaIds.CodecPrivate, t.Codec.ExtraData.Span);
                    }
                    if (!string.IsNullOrEmpty(t.Name)) te.WriteString(0x536E, t.Name); // Name
                    if (!string.IsNullOrEmpty(t.Language)) te.WriteString(0x22B59C, t.Language); // Language
                    if (_defaultDurations.TryGetValue(idx, out var dd))
                    {
                        // DefaultDuration is in nanoseconds and the spec disallows zero.
                        ulong ns = (ulong)Math.Max(1, dd.Ticks * 100L);
                        te.WriteUInt(MatroskaIds.DefaultDuration, ns);
                    }

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
        long ptsMs = checked(sample.Pts * 1000L * track.TimeBase.Numerator / track.TimeBase.Denominator);

        // EnsureClusterForPtsAsync is the ONLY place that can flush pending
        // laces and roll a cluster. It flushes pending laces into the OLD
        // cluster before opening the new one, so by the time we return any
        // pending lace whose first PTS belongs to a now-closed cluster has
        // already been written. FlushLace itself never recurses here — it
        // simply writes into the currently open cluster.
        await EnsureClusterForPtsAsync(ptsMs, cancellationToken).ConfigureAwait(false);

        if (!_lacing.TryGetValue(sample.TrackIndex, out var cfg))
        {
            EmitSingleBlock(sample, ptsMs);
            return;
        }

        if (!_pendingLace.TryGetValue(sample.TrackIndex, out var buf))
        {
            buf = new List<MediaSample>(cfg.MaxFrames);
            _pendingLace[sample.TrackIndex] = buf;
        }
        if (buf.Count > 0 && !CanLaceWithExisting(buf, sample, cfg.Mode))
        {
            FlushLace(sample.TrackIndex);
        }
        buf.Add(sample);
        if (buf.Count >= cfg.MaxFrames)
        {
            FlushLace(sample.TrackIndex);
        }
    }

    private static bool CanLaceWithExisting(List<MediaSample> buf, MediaSample candidate, MatroskaLacing mode)
    {
        var head = buf[0];
        if (head.TrackIndex != candidate.TrackIndex) return false;
        // All frames in a lace must share the keyframe / discardable flag of
        // the block — otherwise the demuxer cannot reconstruct per-frame
        // metadata. Conservatively require IsKeyFrame parity.
        if (head.IsKeyFrame != candidate.IsKeyFrame) return false;
        // PTS must be monotonically increasing within a lace.
        if (candidate.Pts < buf[^1].Pts) return false;
        if (mode == MatroskaLacing.Fixed && candidate.Data.Length != head.Data.Length) return false;
        return true;
    }

    private void EmitSingleBlock(MediaSample sample, long ptsMs)
    {
        var track = _tracks[sample.TrackIndex];

        short relative = (short)(ptsMs - _clusterTimecodeMs);

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

    private async ValueTask EnsureClusterForPtsAsync(long ptsMs, CancellationToken cancellationToken)
    {
        if (_clusterWriter is null ||
            ptsMs - _clusterTimecodeMs > MaxClusterSpanMs ||
            ptsMs - _clusterTimecodeMs < 0)
        {
            // Flush every pending lace whose first PTS belongs to the cluster
            // we're about to close, in chronological order. This prevents a
            // pending audio lace from leaking into the next cluster (which
            // would corrupt block ordering). FlushAllPendingLaces is purely
            // synchronous in-memory work — it cannot recurse here.
            FlushAllPendingLaces();
            await FlushClusterAsync(cancellationToken).ConfigureAwait(false);
            _clusterTimecodeMs = ptsMs;
            _clusterWriter = new EbmlWriter(8192);
            _clusterWriter.WriteUInt(MatroskaIds.Timecode, (ulong)ptsMs);
        }
    }

    private void FlushAllPendingLaces()
    {
        if (_pendingLace.Count == 0) return;
        // Drain in chronological order of first sample to keep block order sane.
        var ordered = _pendingLace
            .Where(kv => kv.Value.Count > 0)
            .OrderBy(kv => kv.Value[0].Pts * 1000L * _tracks[kv.Key].TimeBase.Numerator / _tracks[kv.Key].TimeBase.Denominator)
            .Select(kv => kv.Key)
            .ToArray();
        foreach (var trackIndex in ordered)
        {
            FlushLace(trackIndex);
        }
    }

    private void FlushLace(int trackIndex)
    {
        if (!_pendingLace.TryGetValue(trackIndex, out var buf) || buf.Count == 0) return;
        // INVARIANT: _clusterWriter is non-null. Established by the caller
        // (either WriteSampleAsync after EnsureClusterForPtsAsync, or
        // FlushAllPendingLaces during a roll where the OLD cluster is still
        // open). The lace's first PTS must therefore yield a non-negative
        // signed 16-bit relative timestamp within that cluster.
        var first = buf[0];
        var track = _tracks[trackIndex];
        long firstPtsMs = checked(first.Pts * 1000L * track.TimeBase.Numerator / track.TimeBase.Denominator);
        short relative = (short)(firstPtsMs - _clusterTimecodeMs);

        var cfg = _lacing[trackIndex];
        var sbw = new EbmlWriter(SumBytes(buf) + 32);
        sbw.WriteVintLength(track.Id);
        Span<byte> tsFlags = stackalloc byte[3];
        BinaryPrimitives.WriteInt16BigEndian(tsFlags, relative);
        byte flags = 0;
        if (first.IsKeyFrame) flags |= 0x80;
        if (buf.Count > 1)
        {
            // Set lacing bits 1-2 only if we have something to lace.
            flags |= (byte)(((int)cfg.Mode & 0x03) << 1);
        }
        tsFlags[2] = flags;
        sbw.WriteRaw(tsFlags);

        if (buf.Count == 1)
        {
            sbw.WriteRaw(first.Data.Span);
        }
        else
        {
            int[] sizes = new int[buf.Count];
            for (int i = 0; i < buf.Count; i++) sizes[i] = buf[i].Data.Length;
            MatroskaLacingCodec.EncodeSizes(cfg.Mode, sizes, out byte[] header);
            sbw.WriteRaw(header);
            for (int i = 0; i < buf.Count; i++) sbw.WriteRaw(buf[i].Data.Span);
        }

        _clusterWriter!.WriteBinary(MatroskaIds.SimpleBlock, sbw.Written);
        buf.Clear();
    }

    private static int SumBytes(List<MediaSample> buf)
    {
        int total = 0;
        foreach (var s in buf) total += s.Data.Length;
        return total;
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return;
        _finished = true;
        FlushAllPendingLaces();
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
