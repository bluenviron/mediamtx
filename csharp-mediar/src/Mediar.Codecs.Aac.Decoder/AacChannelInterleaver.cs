namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Converts the per-speaker (planar) PCM output of
/// <see cref="AacRawDataBlockDecoder"/> /
/// <see cref="AacPceRawDataBlockDecoder"/> /
/// <see cref="AacFrameDecoder"/> into the interleaved-float layout
/// most audio APIs and downstream codecs (e.g.,
/// <c>Mediar.Codecs.Pcm.PcmConverter</c> writers) expect.
/// </summary>
/// <remarks>
/// <para>
/// "Interleaved" here means the standard
/// <c>[s0_ch0, s0_ch1, ..., s0_chN-1, s1_ch0, s1_ch1, ...]</c>
/// frame layout. Channel order matches the input list's order; if
/// you obtained the channels from
/// <see cref="AacRawDataBlockDecoder.DecodeToSamples(AacRawDataBlock, int, int, Func{AacPnsRandom}, IReadOnlyDictionary{AacSpeaker, AacSynthesisFilterbank})"/>
/// that order is the speaker-mapping canonical order; for
/// <see cref="AacPceRawDataBlockDecoder"/> it is the PCE slot
/// expansion order.
/// </para>
/// <para>
/// All conversions are allocation-free on the
/// <see cref="Span{T}"/> overloads; the convenience overloads
/// allocate one output array.
/// </para>
/// </remarks>
public static class AacChannelInterleaver
{
    /// <summary>
    /// Interleave a speaker-bound channel list into
    /// <paramref name="destination"/>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="channels"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="channels"/> is empty, contains a
    /// <see langword="null"/> entry, has channel arrays of
    /// inconsistent length, or <paramref name="destination"/> is
    /// shorter than <c>channels.Count * samplesPerChannel</c>.
    /// </exception>
    public static void Interleave(
        IReadOnlyList<AacChannelOutput> channels,
        Span<float> destination)
    {
        ArgumentNullException.ThrowIfNull(channels);
        if (channels.Count == 0)
        {
            throw new ArgumentException("channels is empty.", nameof(channels));
        }

        int channelCount = channels.Count;
        int samplesPerChannel = ValidateAndGetLength(channels);

        int required = channelCount * samplesPerChannel;
        if (destination.Length < required)
        {
            throw new ArgumentException(
                $"destination length {destination.Length} is shorter than required {required} (channels={channelCount} * samples={samplesPerChannel}).",
                nameof(destination));
        }

        for (int s = 0; s < samplesPerChannel; s++)
        {
            int baseIdx = s * channelCount;
            for (int c = 0; c < channelCount; c++)
            {
                destination[baseIdx + c] = channels[c].Samples[s];
            }
        }
    }

    /// <summary>
    /// Convenience overload that allocates a fresh
    /// <c>float[channels.Count * samplesPerChannel]</c>.
    /// </summary>
    public static float[] Interleave(IReadOnlyList<AacChannelOutput> channels)
    {
        ArgumentNullException.ThrowIfNull(channels);
        if (channels.Count == 0)
        {
            throw new ArgumentException("channels is empty.", nameof(channels));
        }

        int samplesPerChannel = ValidateAndGetLength(channels);
        var dest = new float[channels.Count * samplesPerChannel];
        Interleave(channels, dest);
        return dest;
    }

    /// <summary>
    /// Convenience overload that interleaves the
    /// <see cref="AacDecodedRawDataBlock.Channels"/> list directly
    /// into <paramref name="destination"/>.
    /// </summary>
    public static void Interleave(AacDecodedRawDataBlock block, Span<float> destination)
    {
        ArgumentNullException.ThrowIfNull(block);
        Interleave(block.Channels, destination);
    }

    /// <summary>Convenience overload allocating the destination.</summary>
    public static float[] Interleave(AacDecodedRawDataBlock block)
    {
        ArgumentNullException.ThrowIfNull(block);
        return Interleave(block.Channels);
    }

    /// <summary>
    /// Interleave a PCE-described channel list. Identical semantics
    /// to the speaker-bound overload; channel order matches the
    /// PCE slot expansion (front, side, back, LFE).
    /// </summary>
    public static void Interleave(
        IReadOnlyList<AacPceChannelOutput> channels,
        Span<float> destination)
    {
        ArgumentNullException.ThrowIfNull(channels);
        if (channels.Count == 0)
        {
            throw new ArgumentException("channels is empty.", nameof(channels));
        }

        int channelCount = channels.Count;
        int samplesPerChannel = ValidateAndGetLength(channels);

        int required = channelCount * samplesPerChannel;
        if (destination.Length < required)
        {
            throw new ArgumentException(
                $"destination length {destination.Length} is shorter than required {required} (channels={channelCount} * samples={samplesPerChannel}).",
                nameof(destination));
        }

        for (int s = 0; s < samplesPerChannel; s++)
        {
            int baseIdx = s * channelCount;
            for (int c = 0; c < channelCount; c++)
            {
                destination[baseIdx + c] = channels[c].Samples[s];
            }
        }
    }

    /// <summary>Convenience overload allocating the destination.</summary>
    public static float[] Interleave(IReadOnlyList<AacPceChannelOutput> channels)
    {
        ArgumentNullException.ThrowIfNull(channels);
        if (channels.Count == 0)
        {
            throw new ArgumentException("channels is empty.", nameof(channels));
        }

        int samplesPerChannel = ValidateAndGetLength(channels);
        var dest = new float[channels.Count * samplesPerChannel];
        Interleave(channels, dest);
        return dest;
    }

    /// <summary>Convenience overload for PCE-decoded blocks.</summary>
    public static void Interleave(AacPceDecodedRawDataBlock block, Span<float> destination)
    {
        ArgumentNullException.ThrowIfNull(block);
        Interleave(block.Channels, destination);
    }

    /// <summary>Convenience overload allocating the destination.</summary>
    public static float[] Interleave(AacPceDecodedRawDataBlock block)
    {
        ArgumentNullException.ThrowIfNull(block);
        return Interleave(block.Channels);
    }

    private static int ValidateAndGetLength(IReadOnlyList<AacChannelOutput> channels)
    {
        var first = channels[0]
            ?? throw new ArgumentException("channels[0] is null.", nameof(channels));
        if (first.Samples is null)
        {
            throw new ArgumentException("channels[0].Samples is null.", nameof(channels));
        }
        int length = first.Samples.Length;
        for (int i = 1; i < channels.Count; i++)
        {
            var ch = channels[i]
                ?? throw new ArgumentException($"channels[{i}] is null.", nameof(channels));
            if (ch.Samples is null)
            {
                throw new ArgumentException($"channels[{i}].Samples is null.", nameof(channels));
            }
            if (ch.Samples.Length != length)
            {
                throw new ArgumentException(
                    $"channels[{i}].Samples.Length={ch.Samples.Length} differs from channels[0].Samples.Length={length}; all per-channel arrays must be the same length.",
                    nameof(channels));
            }
        }
        return length;
    }

    private static int ValidateAndGetLength(IReadOnlyList<AacPceChannelOutput> channels)
    {
        var first = channels[0]
            ?? throw new ArgumentException("channels[0] is null.", nameof(channels));
        if (first.Samples is null)
        {
            throw new ArgumentException("channels[0].Samples is null.", nameof(channels));
        }
        int length = first.Samples.Length;
        for (int i = 1; i < channels.Count; i++)
        {
            var ch = channels[i]
                ?? throw new ArgumentException($"channels[{i}] is null.", nameof(channels));
            if (ch.Samples is null)
            {
                throw new ArgumentException($"channels[{i}].Samples is null.", nameof(channels));
            }
            if (ch.Samples.Length != length)
            {
                throw new ArgumentException(
                    $"channels[{i}].Samples.Length={ch.Samples.Length} differs from channels[0].Samples.Length={length}; all per-channel arrays must be the same length.",
                    nameof(channels));
            }
        }
        return length;
    }
}
