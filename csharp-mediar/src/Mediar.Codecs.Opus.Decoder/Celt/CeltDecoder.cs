namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// CELT layer decoder. Owned per-stream by <see cref="OpusDecoder"/>.
/// </summary>
/// <remarks>
/// <para>
/// <b>Phased delivery</b>:
/// </para>
/// <list type="bullet">
///   <item><description>Phase 2a (this commit) — foundation: state, mode resolution, decode-frame skeleton that still emits silence.</description></item>
///   <item><description>Phase 2b — silence / transient / post-filter / intra flags + coarse + fine + final energy decode.</description></item>
///   <item><description>Phase 2c — PVQ shape decode + bit allocation + anti-collapse + mid-side stereo.</description></item>
///   <item><description>Phase 2d — IMDCT, post-filter, window overlap-add → first real PCM samples.</description></item>
/// </list>
/// <para>
/// Once Phase 2d lands, this class produces real audio for CELT-only Opus
/// configs (16..31). Hybrid (12..15) waits for the SILK half, which is
/// shipped in Phases 3-4.
/// </para>
/// </remarks>
internal sealed class CeltDecoder
{
    private readonly int _channels;

    /// <summary>The active band layout for this decoder.</summary>
    public CeltMode Mode { get; }

    /// <summary>Number of output channels (1 = mono, 2 = stereo).</summary>
    public int Channels => _channels;

    /// <summary>
    /// Whether the next frame should be treated as the start of a new
    /// stream (no overlap-add history). Set by <see cref="Reset"/> and
    /// cleared after the first <see cref="DecodeFrame"/> call.
    /// </summary>
    public bool IsFirstFrame { get; private set; } = true;

    /// <summary>
    /// Total decoded sample count (per channel) the decoder has produced
    /// since construction or the most recent <see cref="Reset"/>.
    /// </summary>
    public long SamplesProduced { get; private set; }

    /// <summary>
    /// Create a decoder for the given band layout and channel count.
    /// </summary>
    public CeltDecoder(CeltMode mode, int channels)
    {
        if (channels is < 1 or > 2)
            throw new ArgumentOutOfRangeException(nameof(channels), "CELT supports 1 or 2 channels per stream.");
        if (mode.SamplesPerFrame <= 0)
            throw new ArgumentException("CeltMode is uninitialised (SamplesPerFrame == 0).", nameof(mode));

        Mode = mode;
        _channels = channels;
    }

    /// <summary>
    /// Decode one CELT frame from the range decoder into
    /// <paramref name="output"/>. Buffer must be at least
    /// <c>Mode.SamplesPerFrame × Channels</c> floats long; samples are
    /// written interleaved (L R L R …) and bounded to <c>[-1.0, 1.0]</c>.
    /// </summary>
    public int DecodeFrame(ref OpusRangeDecoder rangeDecoder, Span<float> output)
    {
        _ = rangeDecoder; // Phase 2b begins consuming bits from this.
        int needed = Mode.SamplesPerFrame * _channels;
        if (output.Length < needed)
            throw new ArgumentException(
                $"Output buffer is too small: need {needed} floats, got {output.Length}.",
                nameof(output));

        // Phase 2a only — produce silence and bump the sample counter so
        // upstream code can observe forward progress.
        output.Slice(0, needed).Clear();
        IsFirstFrame = false;
        SamplesProduced += Mode.SamplesPerFrame;
        return Mode.SamplesPerFrame;
    }

    /// <summary>Clear all decode history (call after a seek).</summary>
    public void Reset()
    {
        IsFirstFrame = true;
        SamplesProduced = 0;
    }
}
