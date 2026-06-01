namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One ADTS-decoded PCM frame in interleaved signed-16-bit format,
/// the wire format expected by the majority of audio devices and
/// classic WAV / PCM-S16LE writers.
/// </summary>
/// <remarks>
/// <see cref="Samples"/> is interleaved
/// <c>[s0_ch0, s0_ch1, ..., s0_chN-1, s1_ch0, ...]</c> with each
/// sample clipped to [-32768, 32767] before storage. Channel
/// order matches <see cref="Speakers"/>.
/// </remarks>
public sealed record AacPcmInt16Frame
{
    /// <summary>Interleaved PCM-S16 samples; length = ChannelCount * SamplesPerChannel.</summary>
    public required short[] Samples { get; init; }

    /// <summary>Number of channels in the interleaved layout.</summary>
    public required int ChannelCount { get; init; }

    /// <summary>Number of samples per channel produced for this frame.</summary>
    public required int SamplesPerChannel { get; init; }

    /// <summary>Sample rate in Hz, copied from the source frame.</summary>
    public required int SampleRate { get; init; }

    /// <summary>Speaker order for the interleaved layout.</summary>
    public required IReadOnlyList<AacSpeaker> Speakers { get; init; }
}

/// <summary>
/// Helpers that convert the float-PCM output produced by
/// <see cref="AacAdtsPcmStreamReader"/> into the integer PCM
/// formats commonly used by audio APIs and PCM file writers.
/// </summary>
/// <remarks>
/// <para>
/// All converters follow the convention that <c>+1.0f</c> maps to
/// <c>+32767</c> and <c>-1.0f</c> maps to <c>-32768</c>; values
/// outside the unit range are clipped (saturated) to the
/// respective endpoint rather than wrapping.
/// </para>
/// <para>
/// Rounding is "round half to nearest even"-equivalent via
/// <c>Math.Round(MidpointRounding.ToEven)</c> to avoid the small
/// DC bias that round-half-up introduces over long runs.
/// </para>
/// </remarks>
public static class AacPcmFrameConverter
{
    private const float ScalePos = 32767f;
    private const float ScaleNeg = 32768f;

    /// <summary>
    /// In-place float[N] -> short[N] conversion into a destination
    /// span that must be at least as large as <paramref name="source"/>.
    /// </summary>
    public static void ToInt16Samples(ReadOnlySpan<float> source, Span<short> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException(
                $"destination length {destination.Length} is shorter than source length {source.Length}.",
                nameof(destination));
        }
        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = ToInt16Sample(source[i]);
        }
    }

    /// <summary>Allocating overload that returns a fresh short[].</summary>
    public static short[] ToInt16Samples(ReadOnlySpan<float> source)
    {
        var dst = new short[source.Length];
        ToInt16Samples(source, dst);
        return dst;
    }

    /// <summary>
    /// Convert a float <see cref="AacPcmFrame"/> into an integer
    /// <see cref="AacPcmInt16Frame"/>, preserving channel layout,
    /// sample rate, and speaker ordering.
    /// </summary>
    public static AacPcmInt16Frame ToInt16Frame(AacPcmFrame source)
    {
        ArgumentNullException.ThrowIfNull(source);
        var ints = ToInt16Samples(source.Samples);
        return new AacPcmInt16Frame
        {
            Samples = ints,
            ChannelCount = source.ChannelCount,
            SamplesPerChannel = source.SamplesPerChannel,
            SampleRate = source.SampleRate,
            Speakers = source.Speakers,
        };
    }

    private static short ToInt16Sample(float sample)
    {
        if (float.IsNaN(sample)) return 0;
        float scaled = sample >= 0f ? sample * ScalePos : sample * ScaleNeg;
        if (scaled >= 32767f) return short.MaxValue;
        if (scaled <= -32768f) return short.MinValue;
        return (short)Math.Round(scaled, MidpointRounding.ToEven);
    }
}
