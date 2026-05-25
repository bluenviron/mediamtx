using Mediar.Containers.IsoBmff;
using Mediar.Containers.Wav;
using Mediar.Containers.Mp3;
using Mediar.Containers.Flac;
using Mediar.Containers.Adts;
using Mediar.Containers.Ogg;
using Mediar.Containers.Matroska;
using Mediar.Subtitles.Srt;

namespace Mediar;

/// <summary>
/// High-level operations that compose Mediar's container and subtitle modules.
/// All routines are pure passthrough: bytes go in, bytes come out — Mediar never
/// re-encodes audio or video samples and therefore never needs a codec
/// implementation for the user's stated workflows.
/// </summary>
public static class MediarOperations
{
    /// <summary>
    /// Detect the container by file extension and return a demuxer.
    /// Currently recognized: <c>.mp4</c>, <c>.m4a</c>, <c>.m4v</c>, <c>.mov</c>,
    /// <c>.3gp</c>, <c>.wav</c>, <c>.mp3</c>, <c>.flac</c>, <c>.aac</c>, <c>.ogg</c>,
    /// <c>.opus</c>, <c>.mkv</c>, <c>.webm</c>.
    /// </summary>
    public static IMediaDemuxer Open(string path)
    {
        ArgumentNullException.ThrowIfNull(path);
        var ext = Path.GetExtension(path).ToLowerInvariant();
        return ext switch
        {
            ".mp4" or ".m4a" or ".m4v" or ".mov" or ".3gp" => Mp4Demuxer.Open(path),
            ".wav" => WavDemuxer.Open(path),
            ".mp3" => Mp3Demuxer.Open(path),
            ".flac" => FlacDemuxer.Open(path),
            ".aac" => AdtsDemuxer.Open(path),
            ".ogg" or ".opus" or ".oga" or ".ogv" => OggDemuxer.Open(path),
            ".mkv" or ".webm" or ".mka" => MatroskaDemuxer.Open(path),
            _ => throw new NotSupportedException($"Unrecognized container extension '{ext}'."),
        };
    }

    /// <summary>
    /// Extract the first audio track from <paramref name="sourcePath"/> and write it to
    /// <paramref name="destinationPath"/> as an M4A (ISO BMFF audio-only) file. Samples
    /// are copied verbatim — no re-encoding.
    /// </summary>
    public static async Task ExtractAudioAsync(
        string sourcePath, string destinationPath, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(sourcePath);
        ArgumentNullException.ThrowIfNull(destinationPath);

        await using var demuxer = Open(sourcePath);
        var audioTrack = FindFirstAudioTrack(demuxer)
            ?? throw new InvalidOperationException("No audio track found in source.");

        await using var outStream = File.Create(destinationPath);
        await using var muxer = new Mp4Muxer(outStream);
        var newTrack = CloneTrackAtIndex(audioTrack, 0, 1);
        muxer.AddTrack(newTrack);
        await muxer.StartAsync(cancellationToken).ConfigureAwait(false);

        await foreach (var sample in demuxer.ReadSamplesAsync(cancellationToken).ConfigureAwait(false))
        {
            if (sample.TrackIndex != audioTrack.Index)
            {
                sample.Owner?.Dispose();
                continue;
            }
            try
            {
                await muxer.WriteSampleAsync(sample with { TrackIndex = 0 }, cancellationToken).ConfigureAwait(false);
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }

        await muxer.FinishAsync(cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Combine the first audio track from <paramref name="audioSourcePath"/> with the
    /// first video track from <paramref name="videoSourcePath"/> into a new MP4 file at
    /// <paramref name="destinationPath"/>. Samples are passthrough; the operation
    /// requires that the two sources contain codecs an MP4 muxer can carry.
    /// </summary>
    public static async Task MuxAudioWithVideoAsync(
        string videoSourcePath,
        string audioSourcePath,
        string destinationPath,
        CancellationToken cancellationToken = default)
    {
        await using var videoSrc = Open(videoSourcePath);
        await using var audioSrc = Open(audioSourcePath);
        var videoTrack = FindFirstVideoTrack(videoSrc)
            ?? throw new InvalidOperationException("No video track found in video source.");
        var audioTrack = FindFirstAudioTrack(audioSrc)
            ?? throw new InvalidOperationException("No audio track found in audio source.");

        await using var outStream = File.Create(destinationPath);
        await using var muxer = new Mp4Muxer(outStream);

        var v = CloneTrackAtIndex(videoTrack, 0, 1);
        var a = CloneTrackAtIndex(audioTrack, 1, 2);
        muxer.AddTrack(v);
        muxer.AddTrack(a);
        await muxer.StartAsync(cancellationToken).ConfigureAwait(false);

        // Pump both demuxers concurrently and write video first then audio for each
        // tick — simpler than a real interleaved merge and good enough for offline mux.
        var videoTask = PumpAsync(videoSrc, videoTrack.Index, 0, muxer, cancellationToken);
        var audioTask = PumpAsync(audioSrc, audioTrack.Index, 1, muxer, cancellationToken);
        await Task.WhenAll(videoTask, audioTask).ConfigureAwait(false);

        await muxer.FinishAsync(cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Embed an SRT subtitle file as a <c>tx3g</c> subtitle track inside an MP4.
    /// The original tracks are copied verbatim; the resulting file has one extra track.
    /// </summary>
    public static async Task EmbedSrtAsync(
        string mp4SourcePath,
        string srtSourcePath,
        string destinationPath,
        string language = "und",
        CancellationToken cancellationToken = default)
    {
        await using var demuxer = Mp4Demuxer.Open(mp4SourcePath);
        var subtitleSamples = BuildTx3gSamples(SrtReader.ReadFile(srtSourcePath));

        await using var outStream = File.Create(destinationPath);
        await using var muxer = new Mp4Muxer(outStream);

        // Re-add the existing tracks with new indexes/ids.
        var indexMap = new Dictionary<int, int>();
        for (int i = 0; i < demuxer.Tracks.Count; i++)
        {
            var original = demuxer.Tracks[i];
            var cloned = CloneTrackAtIndex(original, i, (uint)(i + 1));
            muxer.AddTrack(cloned);
            indexMap[original.Index] = i;
        }

        int subTrackIndex = demuxer.Tracks.Count;
        var subTrack = new MediaTrack
        {
            Index = subTrackIndex,
            Id = (uint)(subTrackIndex + 1),
            Codec = new SubtitleCodecParameters
            {
                Codec = CodecId.Tx3g,
                Language = language,
            },
            TimeBase = new Rational(1, 1000),
            Language = language,
        };
        muxer.AddTrack(subTrack);
        await muxer.StartAsync(cancellationToken).ConfigureAwait(false);

        await foreach (var sample in demuxer.ReadSamplesAsync(cancellationToken).ConfigureAwait(false))
        {
            try
            {
                int target = indexMap[sample.TrackIndex];
                await muxer.WriteSampleAsync(sample with { TrackIndex = target }, cancellationToken).ConfigureAwait(false);
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }

        foreach (var (data, pts, duration) in subtitleSamples)
        {
            var sample = new MediaSample
            {
                TrackIndex = subTrackIndex,
                Pts = pts,
                Dts = pts,
                Duration = duration,
                IsKeyFrame = true,
                Data = data,
            };
            await muxer.WriteSampleAsync(sample, cancellationToken).ConfigureAwait(false);
        }

        await muxer.FinishAsync(cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Extract the first <c>tx3g</c> subtitle track from an MP4 and write it as SRT.
    /// </summary>
    public static async Task ExtractSrtAsync(
        string mp4SourcePath,
        string destinationPath,
        CancellationToken cancellationToken = default)
    {
        await using var demuxer = Mp4Demuxer.Open(mp4SourcePath);
        var subTrack = demuxer.Tracks.FirstOrDefault(t => t.Codec.Codec == CodecId.Tx3g)
            ?? throw new InvalidOperationException("Source has no tx3g subtitle track.");

        var cues = new List<SrtCue>();
        int index = 1;
        await foreach (var sample in demuxer.ReadSamplesAsync(cancellationToken).ConfigureAwait(false))
        {
            try
            {
                if (sample.TrackIndex != subTrack.Index) continue;
                if (sample.Data.Length < 2) continue;

                var span = sample.Data.Span;
                int textLen = (span[0] << 8) | span[1];
                if (2 + textLen > span.Length) continue;
                string text = System.Text.Encoding.UTF8.GetString(span.Slice(2, textLen));
                if (string.IsNullOrEmpty(text)) continue;

                var start = TimeSpan.FromMilliseconds(
                    subTrack.TimeBase.Rescale(sample.Pts, new Rational(1, 1000)));
                var duration = TimeSpan.FromMilliseconds(
                    subTrack.TimeBase.Rescale(sample.Duration, new Rational(1, 1000)));
                cues.Add(new SrtCue(index++, start, start + duration, text));
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }

        SrtWriter.WriteFile(destinationPath, cues);
    }

    /// <summary>
    /// Probe a media file and return human-readable metadata strings — useful for the
    /// CLI's <c>info</c> subcommand.
    /// </summary>
    public static async Task<MediaInfo> ProbeAsync(string path, CancellationToken cancellationToken = default)
    {
        _ = cancellationToken;
        await using var demuxer = Open(path);
        var tracks = new List<TrackInfo>(demuxer.Tracks.Count);
        foreach (var t in demuxer.Tracks)
        {
            tracks.Add(new TrackInfo(
                t.Index, t.Id, t.Kind, t.Codec.Codec, t.Language, t.TimeBase,
                t.DurationTicks > 0
                    ? TimeSpan.FromSeconds((double)t.DurationTicks * t.TimeBase.Numerator / t.TimeBase.Denominator)
                    : (TimeSpan?)null));
        }
        return new MediaInfo(path, demuxer.FormatName, demuxer.Duration, tracks);
    }

    // ---------------------------------------------------------------------
    // Internals
    // ---------------------------------------------------------------------

    private static MediaTrack? FindFirstAudioTrack(IMediaDemuxer demuxer)
    {
        foreach (var t in demuxer.Tracks) if (t.Kind == StreamKind.Audio) return t;
        return null;
    }

    private static MediaTrack? FindFirstVideoTrack(IMediaDemuxer demuxer)
    {
        foreach (var t in demuxer.Tracks) if (t.Kind == StreamKind.Video) return t;
        return null;
    }

    private static MediaTrack CloneTrackAtIndex(MediaTrack source, int newIndex, uint newId)
    {
        return new MediaTrack
        {
            Index = newIndex,
            Id = newId,
            Codec = source.Codec,
            TimeBase = source.TimeBase,
            DurationTicks = source.DurationTicks,
            Language = source.Language,
            IsDefault = source.IsDefault,
            Name = source.Name,
        };
    }

    private static async Task PumpAsync(
        IMediaDemuxer source, int sourceTrackIndex, int destTrackIndex,
        Mp4Muxer muxer, CancellationToken cancellationToken)
    {
        await foreach (var sample in source.ReadSamplesAsync(cancellationToken).ConfigureAwait(false))
        {
            try
            {
                if (sample.TrackIndex != sourceTrackIndex) continue;
                await muxer.WriteSampleAsync(sample with { TrackIndex = destTrackIndex }, cancellationToken).ConfigureAwait(false);
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }
    }

    private static List<(byte[] Data, long Pts, int Duration)> BuildTx3gSamples(IEnumerable<SrtCue> cues)
    {
        var result = new List<(byte[], long, int)>();
        foreach (var cue in cues)
        {
            long startMs = (long)cue.Start.TotalMilliseconds;
            long endMs = (long)cue.End.TotalMilliseconds;
            int duration = (int)Math.Max(1, endMs - startMs);

            byte[] utf8 = System.Text.Encoding.UTF8.GetBytes(cue.Text);
            byte[] sample = new byte[2 + utf8.Length];
            sample[0] = (byte)(utf8.Length >> 8);
            sample[1] = (byte)utf8.Length;
            utf8.CopyTo(sample.AsSpan(2));
            result.Add((sample, startMs, duration));
        }
        return result;
    }
}

/// <summary>Container-level metadata returned by <see cref="MediarOperations.ProbeAsync"/>.</summary>
public sealed record MediaInfo(
    string Path,
    string Format,
    TimeSpan Duration,
    IReadOnlyList<TrackInfo> Tracks);

/// <summary>Track-level metadata returned by <see cref="MediarOperations.ProbeAsync"/>.</summary>
public sealed record TrackInfo(
    int Index,
    uint Id,
    StreamKind Kind,
    CodecId Codec,
    string Language,
    Rational TimeBase,
    TimeSpan? Duration);
