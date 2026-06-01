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

        var preparedFrame = ApplyPulsesIfPresent(frame, sampleRate);
        var dequant = AacDequantizedSpectrum.FromFrame(preparedFrame, sampleRate);
        var buffer = new float[AacDequantizedSpectrum.TransformLength];
        dequant.Coefficients.CopyTo(buffer);

        AacPnsApplier.ApplyInPlace(buffer, preparedFrame, sampleRate, prng);

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = preparedFrame.Stream.IcsInfo.WindowSequence,
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

        var preparedFrame = ApplyPulsesIfPresent(frame, sampleRate);
        var dequant = AacDequantizedSpectrum.FromFrame(preparedFrame, sampleRate);
        var buffer = new float[AacDequantizedSpectrum.TransformLength];
        dequant.Coefficients.CopyTo(buffer);

        AacPnsApplier.ApplyInPlace(buffer, preparedFrame, sampleRate, prng);

        ApplyLongWindowTnsIfPresent(preparedFrame, sampleRate, objectType, buffer);

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = preparedFrame.Stream.IcsInfo.WindowSequence,
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

        leftFrame = ApplyPulsesIfPresent(leftFrame, sampleRate);
        rightFrame = ApplyPulsesIfPresent(rightFrame, sampleRate);

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

    /// <summary>
    /// CCE (coupling channel element) per-channel composer: dequantize
    /// the coupling channel's spectrum, then fill PNS bands. Coupling
    /// channels are mono auxiliary streams and never use M/S or
    /// intensity stereo, so the pipeline is the same shape as
    /// <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom)"/>
    /// but operates against the CCE's
    /// <see cref="AacCouplingChannelElement.Stream"/> +
    /// <see cref="AacCouplingChannelElement.SpectralData"/>.
    /// </summary>
    /// <remarks>
    /// The result is the coupling channel's MDCT spectrum in the
    /// group-major / SFB-interleaved layout produced by
    /// <see cref="AacDequantizedSpectrum.FromFrame"/>. <strong>The
    /// per-target coupling-gain application that mixes this spectrum
    /// into the downstream SCE/CPE targets is a separate ship and is
    /// NOT performed here.</strong> Callers receive the prepared
    /// auxiliary spectrum and run gain application as a subsequent
    /// step.
    /// </remarks>
    /// <param name="cce">Parsed CCE including spectral data.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">Per-frame PRNG for PNS synthesis.</param>
    /// <returns>An <see cref="AacDecodedSpectrum"/> after Dequantize + PNS.</returns>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="cce"/> or <paramref name="prng"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="cce"/> is missing spectral data (parsed with
    /// the boundary-stopping overload) or
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static AacDecodedSpectrum DecodeCce(
        AacCouplingChannelElement cce,
        int sampleRate,
        AacPnsRandom prng)
    {
        ArgumentNullException.ThrowIfNull(cce);
        ArgumentNullException.ThrowIfNull(prng);

        var frame = CouplingChannelFrame(cce);
        return DecodeMono(frame, sampleRate, prng);
    }

    /// <summary>
    /// AOT-aware CCE composer: same as
    /// <see cref="DecodeCce(AacCouplingChannelElement, int, AacPnsRandom)"/>
    /// but additionally applies TNS inverse filtering when the
    /// coupling channel carries TNS data AND uses a long window
    /// sequence. Short-window TNS is intentionally skipped (see
    /// <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom, AacAudioObjectType)"/>
    /// for rationale).
    /// </summary>
    /// <param name="cce">Parsed CCE including spectral data.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">Per-frame PRNG for PNS synthesis.</param>
    /// <param name="objectType">AAC audio object type for TNS limits.</param>
    public static AacDecodedSpectrum DecodeCce(
        AacCouplingChannelElement cce,
        int sampleRate,
        AacPnsRandom prng,
        AacAudioObjectType objectType)
    {
        ArgumentNullException.ThrowIfNull(cce);
        ArgumentNullException.ThrowIfNull(prng);

        var frame = CouplingChannelFrame(cce);
        return DecodeMono(frame, sampleRate, prng, objectType);
    }

    /// <summary>
    /// End-to-end SCE / LFE / CPE-half decoder that produces 1024 PCM
    /// samples per frame: runs the
    /// <see cref="DecodeMono(AacChannelFrame, int, AacPnsRandom)"/>
    /// composer to obtain post-PNS MDCT coefficients, then drives
    /// <paramref name="filterbank"/> to apply the IMDCT + window +
    /// overlap-add.
    /// </summary>
    /// <remarks>
    /// <para>
    /// <paramref name="filterbank"/> is stateful: it owns the overlap
    /// buffer that carries between consecutive frames. The caller
    /// must construct one filterbank per channel and feed every frame
    /// of that channel through the same instance in stream order. A
    /// fresh filterbank (or one whose
    /// <see cref="AacSynthesisFilterbank.Reset"/> has been called)
    /// produces a half-frame of silent ramp-up on the first frame,
    /// which is the spec-compliant behaviour at stream start / seek.
    /// </para>
    /// <para>
    /// TNS is not applied by this overload. Use the AOT-aware
    /// overload when long-window TNS inverse filtering is required.
    /// </para>
    /// </remarks>
    /// <param name="frame">Parsed mono channel frame.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">Per-frame PRNG for PNS synthesis.</param>
    /// <param name="filterbank">
    /// Per-channel synthesis filterbank carrying the overlap state.
    /// </param>
    /// <param name="output">
    /// Receives exactly <see cref="AacSynthesisFilterbank.LongFrameLength"/>
    /// PCM samples (1024).
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// Any required argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="output"/> is not 1024 samples long, or
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static void DecodeMonoToSamples(
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng,
        AacSynthesisFilterbank filterbank,
        Span<float> output)
    {
        ArgumentNullException.ThrowIfNull(filterbank);
        var decoded = DecodeMono(frame, sampleRate, prng);
        RunFilterbank(decoded, frame.Stream.IcsInfo.WindowShape, filterbank, output);
    }

    /// <summary>
    /// AOT-aware end-to-end mono decoder: same as
    /// <see cref="DecodeMonoToSamples(AacChannelFrame, int, AacPnsRandom, AacSynthesisFilterbank, Span{float})"/>
    /// but additionally applies long-window TNS inverse filtering
    /// when the frame carries TNS data.
    /// </summary>
    /// <param name="frame">Parsed mono channel frame.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">Per-frame PRNG for PNS synthesis.</param>
    /// <param name="objectType">AAC audio object type for TNS limits.</param>
    /// <param name="filterbank">
    /// Per-channel synthesis filterbank carrying the overlap state.
    /// </param>
    /// <param name="output">Receives 1024 PCM samples.</param>
    public static void DecodeMonoToSamples(
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng,
        AacAudioObjectType objectType,
        AacSynthesisFilterbank filterbank,
        Span<float> output)
    {
        ArgumentNullException.ThrowIfNull(filterbank);
        var decoded = DecodeMono(frame, sampleRate, prng, objectType);
        RunFilterbank(decoded, frame.Stream.IcsInfo.WindowShape, filterbank, output);
    }

    private static void RunFilterbank(
        AacDecodedSpectrum decoded,
        AacWindowShape currentWindowShape,
        AacSynthesisFilterbank filterbank,
        Span<float> output)
    {
        var coefs = decoded.Coefficients.AsSpan();
        if (decoded.WindowSequence == AacWindowSequence.EightShort)
        {
            filterbank.ProcessEightShortBlock(coefs, currentWindowShape, output);
        }
        else
        {
            filterbank.ProcessLongBlock(coefs, decoded.WindowSequence, currentWindowShape, output);
        }
    }

    /// <summary>
    /// CPE end-to-end composer that produces 1024 PCM samples per
    /// channel: runs the
    /// <see cref="DecodePair(AacChannelPairElement, int, AacPnsRandom, AacPnsRandom)"/>
    /// composer to obtain post-PNS MDCT spectra for both channels,
    /// then drives one
    /// <see cref="AacSynthesisFilterbank"/> per channel for IMDCT +
    /// window + overlap-add.
    /// </summary>
    /// <remarks>
    /// The two filterbanks are independent and stateful - each must
    /// be the same instance reused across consecutive frames of its
    /// respective channel.
    /// </remarks>
    /// <param name="cpe">Parsed channel-pair element including spectral data.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="leftPrng">PRNG for first channel's PNS synthesis.</param>
    /// <param name="rightPrng">PRNG for second channel's PNS synthesis.</param>
    /// <param name="leftFilterbank">Filterbank for the first channel.</param>
    /// <param name="rightFilterbank">Filterbank for the second channel.</param>
    /// <param name="leftOutput">1024 samples for the first channel.</param>
    /// <param name="rightOutput">1024 samples for the second channel.</param>
    public static void DecodePairToSamples(
        AacChannelPairElement cpe,
        int sampleRate,
        AacPnsRandom leftPrng,
        AacPnsRandom rightPrng,
        AacSynthesisFilterbank leftFilterbank,
        AacSynthesisFilterbank rightFilterbank,
        Span<float> leftOutput,
        Span<float> rightOutput)
    {
        ArgumentNullException.ThrowIfNull(leftFilterbank);
        ArgumentNullException.ThrowIfNull(rightFilterbank);

        var (left, right) = DecodePair(cpe, sampleRate, leftPrng, rightPrng);

        var leftShape = (cpe.SharedIcsInfo ?? cpe.FirstStream.IcsInfo).WindowShape;
        var rightShape = (cpe.SharedIcsInfo ?? cpe.SecondStream.IcsInfo).WindowShape;

        RunFilterbank(left, leftShape, leftFilterbank, leftOutput);
        RunFilterbank(right, rightShape, rightFilterbank, rightOutput);
    }

    /// <summary>
    /// AOT-aware CPE end-to-end composer: same as
    /// <see cref="DecodePairToSamples(AacChannelPairElement, int, AacPnsRandom, AacPnsRandom, AacSynthesisFilterbank, AacSynthesisFilterbank, Span{float}, Span{float})"/>
    /// but also runs long-window TNS inverse filtering on each
    /// channel.
    /// </summary>
    public static void DecodePairToSamples(
        AacChannelPairElement cpe,
        int sampleRate,
        AacPnsRandom leftPrng,
        AacPnsRandom rightPrng,
        AacAudioObjectType objectType,
        AacSynthesisFilterbank leftFilterbank,
        AacSynthesisFilterbank rightFilterbank,
        Span<float> leftOutput,
        Span<float> rightOutput)
    {
        ArgumentNullException.ThrowIfNull(leftFilterbank);
        ArgumentNullException.ThrowIfNull(rightFilterbank);

        var (left, right) = DecodePair(cpe, sampleRate, leftPrng, rightPrng, objectType);

        var leftShape = (cpe.SharedIcsInfo ?? cpe.FirstStream.IcsInfo).WindowShape;
        var rightShape = (cpe.SharedIcsInfo ?? cpe.SecondStream.IcsInfo).WindowShape;

        RunFilterbank(left, leftShape, leftFilterbank, leftOutput);
        RunFilterbank(right, rightShape, rightFilterbank, rightOutput);
    }

    /// <summary>
    /// CCE auxiliary-channel end-to-end composer: produces 1024 PCM
    /// samples of the coupling channel by running
    /// <see cref="DecodeCce(AacCouplingChannelElement, int, AacPnsRandom)"/>
    /// followed by the per-channel synthesis filterbank pass. <strong>This
    /// is the auxiliary coupling channel only;</strong> the
    /// per-target gain application that mixes this signal into the
    /// downstream SCE / CPE targets is a separate composer.
    /// </summary>
    /// <param name="cce">Parsed CCE including spectral data.</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">PRNG for PNS synthesis.</param>
    /// <param name="filterbank">Filterbank carrying the overlap state.</param>
    /// <param name="output">Receives 1024 PCM samples.</param>
    public static void DecodeCceToSamples(
        AacCouplingChannelElement cce,
        int sampleRate,
        AacPnsRandom prng,
        AacSynthesisFilterbank filterbank,
        Span<float> output)
    {
        ArgumentNullException.ThrowIfNull(filterbank);
        var decoded = DecodeCce(cce, sampleRate, prng);
        RunFilterbank(decoded, cce.Stream.IcsInfo.WindowShape, filterbank, output);
    }

    /// <summary>
    /// AOT-aware CCE auxiliary-channel end-to-end composer: same as
    /// <see cref="DecodeCceToSamples(AacCouplingChannelElement, int, AacPnsRandom, AacSynthesisFilterbank, Span{float})"/>
    /// but also runs long-window TNS inverse filtering.
    /// </summary>
    public static void DecodeCceToSamples(
        AacCouplingChannelElement cce,
        int sampleRate,
        AacPnsRandom prng,
        AacAudioObjectType objectType,
        AacSynthesisFilterbank filterbank,
        Span<float> output)
    {
        ArgumentNullException.ThrowIfNull(filterbank);
        var decoded = DecodeCce(cce, sampleRate, prng, objectType);
        RunFilterbank(decoded, cce.Stream.IcsInfo.WindowShape, filterbank, output);
    }

    private static AacChannelFrame CouplingChannelFrame(AacCouplingChannelElement cce)
    {
        if (cce.SpectralData is null)
        {
            throw new ArgumentException(
                "CCE is missing spectral_data (boundary-stopping parse); use the 'full' TryRead/TryParse overload before decoding.",
                nameof(cce));
        }

        return new AacChannelFrame
        {
            Stream = cce.Stream,
            SpectralData = cce.SpectralData,
            BitsConsumed = 0,
        };
    }

    private static void ApplyTnsIfPresent(
        AacChannelFrame frame,
        int sampleRate,
        AacAudioObjectType objectType,
        float[] spectrum)
    {
        var ics = frame.Stream.IcsInfo;
        var ws = ics.WindowSequence;
        if (!frame.Stream.TnsDataPresent
            || frame.Stream.TnsData is not { } tnsData)
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

        bool isShort = ws == AacWindowSequence.EightShort;
        ReadOnlySpan<int> swbOffsets = isShort
            ? AacSwbOffsets.GetShortOffsets(sampleRate)
            : AacSwbOffsets.GetLongOffsets(sampleRate);
        int tnsMaxSfb = AacTnsSpecLimits.GetMaxBands(objectType, sfIndex, ws);
        int tnsMaxOrder = AacTnsSpecLimits.GetMaxOrder(objectType, ws);

        int numSwb = swbOffsets.Length - 1;
        if (tnsMaxSfb > numSwb) tnsMaxSfb = numSwb;
        if (tnsMaxOrder > AacTnsLpcStepUp.MaxOrder) tnsMaxOrder = AacTnsLpcStepUp.MaxOrder;

        if (!isShort)
        {
            AacTnsSpectrumApplier.Apply(
                tnsData, ics, spectrum, swbOffsets, tnsMaxSfb, tnsMaxOrder);
            return;
        }

        // Short-window TNS: deinterleave the group-major /
        // SFB-window-interleaved layout to window-major (8 contiguous
        // 128-sample windows) so AacTnsSpectrumApplier can act per
        // window, then re-interleave back to group-major.
        var windowMajor = new float[AacShortWindowDeinterleaver.TotalLength];
        AacShortWindowDeinterleaver.ToWindowMajor(spectrum, ics, swbOffsets, windowMajor);
        AacTnsSpectrumApplier.Apply(
            tnsData, ics, windowMajor, swbOffsets, tnsMaxSfb, tnsMaxOrder);
        AacShortWindowDeinterleaver.ToGroupMajor(windowMajor, ics, swbOffsets, spectrum);
    }

    private static void ApplyLongWindowTnsIfPresent(
        AacChannelFrame frame,
        int sampleRate,
        AacAudioObjectType objectType,
        float[] spectrum)
    {
        ApplyTnsIfPresent(frame, sampleRate, objectType, spectrum);
    }

    /// <summary>
    /// If the frame carries pulse_data, returns a new
    /// <see cref="AacChannelFrame"/> whose <see cref="AacSpectralData.Coefficients"/>
    /// have been updated per spec §4.6.2.1 BEFORE inverse quantisation.
    /// Returns the input frame unchanged when no pulse data is present.
    /// </summary>
    /// <remarks>
    /// Pulse data is illegal in EIGHT_SHORT_SEQUENCE windows (the
    /// parser already enforces this); the helper additionally returns
    /// the input frame unchanged for short-window frames as a defence
    /// in depth. The pulse_data() bitstream layout addresses positions
    /// via the long-window SWB offsets only, so applying it to a short
    /// frame would corrupt the spectrum even if the parser ever let one
    /// through.
    /// </remarks>
    private static AacChannelFrame ApplyPulsesIfPresent(
        AacChannelFrame frame,
        int sampleRate)
    {
        if (!frame.Stream.PulseDataPresent
            || frame.Stream.PulseData is not { } pulses)
        {
            return frame;
        }

        var ics = frame.Stream.IcsInfo;
        if (ics.WindowSequence == AacWindowSequence.EightShort)
        {
            return frame;
        }

        ReadOnlySpan<int> longSwbOffsets = AacSwbOffsets.GetLongOffsets(sampleRate);
        if (longSwbOffsets.IsEmpty)
        {
            throw new ArgumentException(
                $"Sample rate {sampleRate} Hz has no SWB offset table.",
                nameof(sampleRate));
        }

        var modified = frame.SpectralData.Coefficients.ToArray();
        AacPulseApplier.ApplyToQuantised(modified, pulses, longSwbOffsets);

        var newSpectral = new AacSpectralData
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(modified),
            BitsConsumed = frame.SpectralData.BitsConsumed,
        };

        return frame with { SpectralData = newSpectral };
    }
}
