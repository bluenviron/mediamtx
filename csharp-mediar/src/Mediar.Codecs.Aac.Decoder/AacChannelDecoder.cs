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

        ApplyLongWindowTnsIfPresent(frame, sampleRate, objectType, buffer);

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = frame.Stream.IcsInfo.WindowSequence,
        };
    }

    /// <summary>
    /// CPE composer: dequantize both channels, apply M/S stereo when
    /// active, fill intensity-stereo bands on the right channel, then
    /// run PNS synthesis on each channel. The result is a pair of
    /// post-PNS spectra ready for TNS / IMDCT.
    /// </summary>
    /// <remarks>
    /// <para>
    /// Decoding order follows the AAC spec §4.6 chain:
    /// dequantize ➜ M/S ➜ intensity stereo ➜ PNS. The four operations
    /// write to disjoint band sets (M/S to "real" Huffman bands, IS to
    /// cb 14/15 bands, PNS to cb 13 bands), so the visible output is
    /// invariant to small reorderings - but the spec ordering is kept
    /// for clarity and future-proofing.
    /// </para>
    /// <para>
    /// Intensity stereo and M/S are applied only when the CPE supplies
    /// a shared <c>ics_info()</c> (common_window = 1) because both
    /// stages require the two channels to share window / SFB
    /// partitioning. CPEs with common_window = 0 are passed through
    /// without joint-stereo / intensity processing.
    /// </para>
    /// </remarks>
    /// <param name="cpe">Parsed channel-pair element including spectral data for both channels.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="leftPrng">Per-frame PNS PRNG for the first channel.</param>
    /// <param name="rightPrng">Per-frame PNS PRNG for the second channel.</param>
    /// <returns>Post-PNS decoded spectra for the first and second channels.</returns>
    /// <exception cref="ArgumentNullException">
    /// Any required argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="cpe"/> is missing spectral data for one of the
    /// channels, or <paramref name="sampleRate"/> has no SWB offset
    /// table.
    /// </exception>
    public static (AacDecodedSpectrum Left, AacDecodedSpectrum Right) DecodePair(
        AacChannelPairElement cpe,
        int sampleRate,
        AacPnsRandom leftPrng,
        AacPnsRandom rightPrng)
    {
        ArgumentNullException.ThrowIfNull(cpe);
        ArgumentNullException.ThrowIfNull(leftPrng);
        ArgumentNullException.ThrowIfNull(rightPrng);

        if (cpe.FirstSpectralData is null)
        {
            throw new ArgumentException(
                "CPE is missing spectral data for the first channel.",
                nameof(cpe));
        }
        if (cpe.SecondSpectralData is null)
        {
            throw new ArgumentException(
                "CPE is missing spectral data for the second channel.",
                nameof(cpe));
        }

        var leftFrame = new AacChannelFrame
        {
            Stream = cpe.FirstStream,
            SpectralData = cpe.FirstSpectralData,
            BitsConsumed = 0,
        };
        var rightFrame = new AacChannelFrame
        {
            Stream = cpe.SecondStream,
            SpectralData = cpe.SecondSpectralData,
            BitsConsumed = 0,
        };

        var leftDeq = AacDequantizedSpectrum.FromFrame(leftFrame, sampleRate);
        var rightDeq = AacDequantizedSpectrum.FromFrame(rightFrame, sampleRate);

        var leftBuf = new float[AacDequantizedSpectrum.TransformLength];
        var rightBuf = new float[AacDequantizedSpectrum.TransformLength];
        leftDeq.Coefficients.CopyTo(leftBuf);
        rightDeq.Coefficients.CopyTo(rightBuf);

        if (cpe.CommonWindow
            && cpe.SharedIcsInfo is not null
            && cpe.MsMaskPresent != AacMsMaskPresent.None)
        {
            AacMsStereoDecoder.Decode(
                leftBuf,
                rightBuf,
                cpe.SharedIcsInfo,
                cpe.MsMaskPresent,
                cpe.MsUsed,
                cpe.SecondStream.SectionData,
                sampleRate);
        }

        if (cpe.CommonWindow && cpe.SharedIcsInfo is not null)
        {
            AacIntensityStereoApplier.ApplyInPlace(
                leftBuf,
                rightBuf,
                rightFrame,
                cpe.MsMaskPresent,
                cpe.MsUsed,
                sampleRate);
        }

        AacPnsApplier.ApplyInPlace(leftBuf, leftFrame, sampleRate, leftPrng);
        AacPnsApplier.ApplyInPlace(rightBuf, rightFrame, sampleRate, rightPrng);

        var ws = cpe.SharedIcsInfo?.WindowSequence
            ?? cpe.FirstStream.IcsInfo.WindowSequence;

        return (
            new AacDecodedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(leftBuf),
                WindowSequence = ws,
            },
            new AacDecodedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(rightBuf),
                WindowSequence = cpe.SharedIcsInfo?.WindowSequence
                    ?? cpe.SecondStream.IcsInfo.WindowSequence,
            });
    }

    /// <summary>
    /// Same as <see cref="DecodePair(AacChannelPairElement, int, AacPnsRandom, AacPnsRandom)"/>
    /// but additionally applies long-window TNS inverse filtering to
    /// each channel that carries parsed
    /// <see cref="AacIndividualChannelStream.TnsData"/>.
    /// <see cref="AacWindowSequence.EightShort"/> channels skip the TNS
    /// step (short-window TNS requires deinterleaving the group-major
    /// spectrum back to per-window layout, which is a separate composer
    /// ship).
    /// </summary>
    /// <param name="cpe">Parsed channel-pair element including spectral data for both channels.</param>
    /// <param name="sampleRate">
    /// Source sample rate in Hz; must be one of the 13 indexed AAC
    /// rates so the SWB tables and the
    /// <see cref="AacTnsSpecLimits"/> per-rate band limits resolve.
    /// </param>
    /// <param name="leftPrng">Per-frame PNS PRNG for the first channel.</param>
    /// <param name="rightPrng">Per-frame PNS PRNG for the second channel.</param>
    /// <param name="objectType">
    /// AAC audio object type. See
    /// <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom, AacAudioObjectType)"/>
    /// for the supported set.
    /// </param>
    /// <returns>Post-PNS, post-(long)-TNS decoded spectra for both channels.</returns>
    /// <exception cref="ArgumentNullException">
    /// Any required argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="cpe"/> is missing spectral data for one of the
    /// channels, or <paramref name="sampleRate"/> has no SWB / TNS
    /// table.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="objectType"/> is outside the four AOTs
    /// supported by <see cref="AacTnsSpecLimits"/> AND at least one
    /// channel actually carries TNS data of a non-short window.
    /// </exception>
    public static (AacDecodedSpectrum Left, AacDecodedSpectrum Right) DecodePair(
        AacChannelPairElement cpe,
        int sampleRate,
        AacPnsRandom leftPrng,
        AacPnsRandom rightPrng,
        AacAudioObjectType objectType)
    {
        var (left, right) = DecodePair(cpe, sampleRate, leftPrng, rightPrng);

        var leftBuf = left.Coefficients.ToArray();
        var rightBuf = right.Coefficients.ToArray();

        var leftFrame = new AacChannelFrame
        {
            Stream = cpe.FirstStream,
            SpectralData = cpe.FirstSpectralData!,
            BitsConsumed = 0,
        };
        var rightFrame = new AacChannelFrame
        {
            Stream = cpe.SecondStream,
            SpectralData = cpe.SecondSpectralData!,
            BitsConsumed = 0,
        };

        ApplyLongWindowTnsIfPresent(leftFrame, sampleRate, objectType, leftBuf);
        ApplyLongWindowTnsIfPresent(rightFrame, sampleRate, objectType, rightBuf);

        return (
            new AacDecodedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(leftBuf),
                WindowSequence = left.WindowSequence,
            },
            new AacDecodedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(rightBuf),
                WindowSequence = right.WindowSequence,
            });
    }

    private static void ApplyLongWindowTnsIfPresent(
        AacChannelFrame frame,
        int sampleRate,
        AacAudioObjectType objectType,
        float[] spectrum)
    {
        var ics = frame.Stream.IcsInfo;
        var ws = ics.WindowSequence;
        if (!frame.Stream.TnsDataPresent
            || frame.Stream.TnsData is not { } tnsData
            || ws == AacWindowSequence.EightShort)
        {
            return;
        }

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
            tnsData, ics, spectrum, swbOffsets, tnsMaxSfb, tnsMaxOrder);
    }
}
