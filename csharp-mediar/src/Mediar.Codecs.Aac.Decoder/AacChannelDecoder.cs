using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Per-frame composer that chains the AAC mono / SCE / LFE channel
/// pipeline up to PNS synthesis:
/// <see cref="AacDequantizedSpectrum.FromFrame"/> ➜
/// <see cref="AacPnsApplier.ApplyInPlace"/>.
/// </summary>
/// <remarks>
/// <para>
/// This is the smallest useful composer in the AAC decoder: it owns
/// the buffer that flows out of dequantization, runs PNS synthesis
/// in place against it, and exposes the result as an immutable
/// <see cref="AacDecodedSpectrum"/>. The pipeline is correct for any
/// SCE / LFE / DSE channel; intensity-stereo (cb=14/15) and M/S
/// stereo apply only to a paired CPE and are NOT performed here.
/// </para>
/// <para>
/// TNS inverse filtering and the final IMDCT are intentionally left
/// to follow-up composers (long-window TNS needs a per-window-layout
/// spectrum, short-window TNS needs an additional deinterleave step,
/// and IMDCT is its own large piece).
/// </para>
/// </remarks>
public static class AacChannelDecoder
{
    /// <summary>
    /// Dequantize <paramref name="frame"/> for source rate
    /// <paramref name="sampleRate"/>, then fill PNS bands using
    /// <paramref name="prng"/>.
    /// </summary>
    /// <param name="frame">Parsed mono channel frame (SCE / LFE / CPE-half).</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">
    /// Per-frame PRNG, advanced once per noise-band coefficient.
    /// </param>
    /// <returns>
    /// An <see cref="AacDecodedSpectrum"/> carrying the post-PNS MDCT
    /// coefficients in the same group-major / SFB-interleaved layout
    /// produced by <see cref="AacDequantizedSpectrum.FromFrame"/>.
    /// </returns>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="frame"/> or <paramref name="prng"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static AacDecodedSpectrum DecodeMono(
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng)
    {
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(prng);

        var dequant = AacDequantizedSpectrum.FromFrame(frame, sampleRate);
        var buffer = new float[AacDequantizedSpectrum.TransformLength];
        dequant.Coefficients.CopyTo(buffer);

        AacPnsApplier.ApplyInPlace(buffer, frame, sampleRate, prng);

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = frame.Stream.IcsInfo.WindowSequence,
        };
    }

    /// <summary>
    /// Same as <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom)"/>
    /// but additionally applies TNS inverse filtering when the frame
    /// carries a parsed <see cref="AacTnsData"/> AND uses a long
    /// window sequence (Only / LongStart / LongStop). For
    /// <see cref="AacWindowSequence.EightShort"/> frames the TNS step
    /// is skipped (short-window TNS requires deinterleaving the
    /// group-major spectrum back to per-window layout, which is a
    /// separate composer ship).
    /// </summary>
    /// <param name="frame">Parsed mono channel frame (SCE / LFE / CPE-half).</param>
    /// <param name="sampleRate">
    /// Source sample rate in Hz; must be one of the 13 indexed AAC
    /// rates so the SWB tables and the
    /// <see cref="AacTnsSpecLimits"/> per-rate band limits resolve.
    /// </param>
    /// <param name="prng">
    /// Per-frame PRNG for PNS synthesis (also see
    /// <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom)"/>).
    /// </param>
    /// <param name="objectType">
    /// AAC audio object type. Required by
    /// <see cref="AacTnsSpecLimits.GetMaxOrder"/> and
    /// <see cref="AacTnsSpecLimits.GetMaxBands"/>; must be one of
    /// <see cref="AacAudioObjectType.AacMain"/>,
    /// <see cref="AacAudioObjectType.AacLc"/>,
    /// <see cref="AacAudioObjectType.AacLtp"/>, or
    /// <see cref="AacAudioObjectType.ErAacLc"/>.
    /// </param>
    /// <returns>An <see cref="AacDecodedSpectrum"/> after Dequantize + PNS + (long) TNS.</returns>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="frame"/> or <paramref name="prng"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="sampleRate"/> has no SWB / TNS table.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="objectType"/> is outside the four AOTs
    /// supported by <see cref="AacTnsSpecLimits"/>.
    /// </exception>
    public static AacDecodedSpectrum DecodeMono(
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng,
        AacAudioObjectType objectType)
    {
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(prng);

        var dequant = AacDequantizedSpectrum.FromFrame(frame, sampleRate);
        var buffer = new float[AacDequantizedSpectrum.TransformLength];
        dequant.Coefficients.CopyTo(buffer);

        AacPnsApplier.ApplyInPlace(buffer, frame, sampleRate, prng);

        var ics = frame.Stream.IcsInfo;
        var ws = ics.WindowSequence;
        if (frame.Stream.TnsDataPresent
            && frame.Stream.TnsData is { } tnsData
            && ws != AacWindowSequence.EightShort)
        {
            int sfIndex = AacSampleRates.ToIndex(sampleRate);
            if (sfIndex == AacSampleRates.EscapeIndex)
            {
                throw new ArgumentException(
                    $"Sample rate {sampleRate} Hz has no indexed AAC TNS limit table.",
                    nameof(sampleRate));
            }

            ReadOnlySpan<int> swbOffsets = AacSwbOffsets.GetLongOffsets(sampleRate);
            int tnsMaxSfb = AacTnsSpecLimits.GetMaxBands(objectType, sfIndex, ws);
            int tnsMaxOrder = AacTnsSpecLimits.GetMaxOrder(objectType, ws);

            int numSwb = swbOffsets.Length - 1;
            if (tnsMaxSfb > numSwb) tnsMaxSfb = numSwb;
            if (tnsMaxOrder > AacTnsLpcStepUp.MaxOrder) tnsMaxOrder = AacTnsLpcStepUp.MaxOrder;

            AacTnsSpectrumApplier.Apply(
                tnsData, ics, buffer, swbOffsets, tnsMaxSfb, tnsMaxOrder);
        }

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = ws,
        };
    }
}
