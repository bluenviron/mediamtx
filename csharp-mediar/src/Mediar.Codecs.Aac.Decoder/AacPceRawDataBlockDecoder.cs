namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One decoded output channel from a PCE-described AAC
/// raw_data_block: the PCE positional metadata that selected the
/// element plus 1024 PCM samples.
/// </summary>
/// <remarks>
/// Unlike <see cref="AacChannelOutput"/> this carries no speaker
/// label because PCE-described layouts have no universally-agreed
/// canonical speaker mapping (see
/// <see cref="AacPceResolvedEntry"/>). Callers downstream must
/// interpret <see cref="Region"/> and <see cref="RegionIndex"/>
/// according to their own policy (e.g., assume "L / R" for the
/// first two front slots, etc.).
/// </remarks>
public sealed record AacPceChannelOutput
{
    /// <summary>Which positional region the source PCE slot belongs to.</summary>
    public required AacPceChannelRegion Region { get; init; }

    /// <summary>
    /// Zero-based index of the source PCE slot within
    /// <see cref="Region"/>.
    /// </summary>
    public required int RegionIndex { get; init; }

    /// <summary>
    /// For Channel Pair Elements, 0 = first channel and
    /// 1 = second channel; <see langword="null"/> for SCE and LFE
    /// slots which always produce a single output channel.
    /// </summary>
    public required int? PairIndex { get; init; }

    /// <summary>
    /// PCM samples for this frame, exactly
    /// <see cref="AacSynthesisFilterbank.LongFrameLength"/> long
    /// (1024).
    /// </summary>
    public required float[] Samples { get; init; }
}

/// <summary>
/// Per-frame PCE-driven decoder output: one
/// <see cref="AacPceChannelOutput"/> per audio output channel in
/// the PCE's slot order (front, then side, then back, then LFE,
/// with CPE slots expanded into two outputs in source order).
/// Coupling Channel Elements traversed by the resolver are NOT
/// included.
/// </summary>
public sealed record AacPceDecodedRawDataBlock
{
    /// <summary>Decoded output channels in PCE slot order.</summary>
    public required IReadOnlyList<AacPceChannelOutput> Channels { get; init; }
}

/// <summary>
/// PCE-driven frame-level dispatcher: walks a parsed
/// <see cref="AacRawDataBlock"/> using a parsed
/// <see cref="AacProgramConfigurationElement"/> to drive channel
/// routing, and produces one PCM stream per PCE-selected output
/// channel.
/// </summary>
/// <remarks>
/// <para>
/// This is the explicit-PCE equivalent of
/// <see cref="AacRawDataBlockDecoder"/>. Use it for
/// channelConfiguration == 0 streams (or any stream whose PCE
/// authoritatively overrides the standard 1..7 mappings).
/// </para>
/// <para>
/// The dispatcher is stateless across frames. Filterbank state is
/// carried by the caller via an ordered filterbank list whose
/// length must equal
/// <see cref="GetExpectedChannelCount(AacProgramConfigurationElement)"/>;
/// reusing the same list across consecutive frames preserves the
/// spec-mandated 50 % MDCT overlap-add.
/// </para>
/// <para>
/// The PNS PRNG is acquired via a caller-supplied factory because
/// CPE elements need two independent PRNG streams (one per channel
/// half) — sharing state would couple PNS noise across channels.
/// </para>
/// <para>
/// Coupling Channel Elements are traversed by
/// <see cref="AacPceLayoutResolver"/> but the auxiliary CCE
/// contribution is NOT yet mixed into target channels (that
/// requires the coupling gain table — open ship).
/// </para>
/// </remarks>
public static class AacPceRawDataBlockDecoder
{
    /// <summary>
    /// Get the total number of audio output channels produced by
    /// <paramref name="pce"/>, expanding each CPE slot to 2 and
    /// each SCE / LFE slot to 1. Coupling slots contribute 0.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="pce"/> is <see langword="null"/>.
    /// </exception>
    public static int GetExpectedChannelCount(AacProgramConfigurationElement pce)
    {
        ArgumentNullException.ThrowIfNull(pce);

        int total = 0;
        total += CountChannelSlots(pce.FrontElements);
        total += CountChannelSlots(pce.SideElements);
        total += CountChannelSlots(pce.BackElements);
        total += pce.LfeElements.Count;
        return total;
    }

    /// <summary>
    /// Convenience: build a fresh filterbank list of length
    /// <see cref="GetExpectedChannelCount(AacProgramConfigurationElement)"/>,
    /// ready to be passed to
    /// <see cref="DecodeToSamples(AacRawDataBlock, AacProgramConfigurationElement, int, Func{AacPnsRandom}, IReadOnlyList{AacSynthesisFilterbank})"/>.
    /// Carry the SAME list across subsequent frames so overlap-add
    /// state survives.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="pce"/> is <see langword="null"/>.
    /// </exception>
    public static AacSynthesisFilterbank[] CreateFilterbanks(AacProgramConfigurationElement pce)
    {
        int n = GetExpectedChannelCount(pce);
        var arr = new AacSynthesisFilterbank[n];
        for (int i = 0; i < n; i++)
        {
            arr[i] = new AacSynthesisFilterbank();
        }
        return arr;
    }

    /// <summary>
    /// Decode <paramref name="block"/> through the supplied
    /// <paramref name="pce"/> to per-channel PCM. The non-AOT
    /// overload skips TNS inverse filtering; use the AOT overload
    /// to apply TNS for AOTs that emit it (AAC-LC, …).
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// Any argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="filterbanks"/> length does not match the
    /// expected channel count for <paramref name="pce"/>, or any
    /// filterbank entry is <see langword="null"/>.
    /// </exception>
    public static AacPceDecodedRawDataBlock DecodeToSamples(
        AacRawDataBlock block,
        AacProgramConfigurationElement pce,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        IReadOnlyList<AacSynthesisFilterbank> filterbanks)
    {
        return DecodeCore(block, pce, sampleRate, prngFactory, filterbanks, applyTns: false, objectType: default);
    }

    /// <summary>
    /// AOT-aware decode: also applies TNS inverse filtering for the
    /// supplied <paramref name="objectType"/>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// Any reference argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="filterbanks"/> length does not match the
    /// expected channel count for <paramref name="pce"/>, or any
    /// filterbank entry is <see langword="null"/>.
    /// </exception>
    public static AacPceDecodedRawDataBlock DecodeToSamples(
        AacRawDataBlock block,
        AacProgramConfigurationElement pce,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        AacAudioObjectType objectType,
        IReadOnlyList<AacSynthesisFilterbank> filterbanks)
    {
        return DecodeCore(block, pce, sampleRate, prngFactory, filterbanks, applyTns: true, objectType);
    }

    private static AacPceDecodedRawDataBlock DecodeCore(
        AacRawDataBlock block,
        AacProgramConfigurationElement pce,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        IReadOnlyList<AacSynthesisFilterbank> filterbanks,
        bool applyTns,
        AacAudioObjectType objectType)
    {
        ArgumentNullException.ThrowIfNull(block);
        ArgumentNullException.ThrowIfNull(pce);
        ArgumentNullException.ThrowIfNull(prngFactory);
        ArgumentNullException.ThrowIfNull(filterbanks);

        int expectedCount = GetExpectedChannelCount(pce);
        if (filterbanks.Count != expectedCount)
        {
            throw new ArgumentException(
                $"filterbanks length {filterbanks.Count} does not match the expected output channel count {expectedCount} for the supplied PCE.",
                nameof(filterbanks));
        }
        for (int i = 0; i < filterbanks.Count; i++)
        {
            if (filterbanks[i] is null)
            {
                throw new ArgumentException(
                    $"filterbanks[{i}] is null; every output channel needs a non-null filterbank instance.",
                    nameof(filterbanks));
            }
        }

        var resolved = AacPceLayoutResolver.Resolve(block, pce);
        var outputs = new List<AacPceChannelOutput>(expectedCount);
        int fbIndex = 0;

        foreach (var entry in resolved)
        {
            if (entry.Region == AacPceChannelRegion.Coupling)
            {
                continue;
            }

            switch (entry.RawEntry.Type)
            {
                case AacSyntacticElementType.SingleChannelElement:
                {
                    var sce = entry.RawEntry.SingleChannel
                        ?? throw new InvalidOperationException(
                            "Resolved SCE entry has null SingleChannel payload; raw_data_block must be parsed via the 'full' overload that populates typed payloads.");
                    var fb = filterbanks[fbIndex++];
                    var pcm = new float[AacSynthesisFilterbank.LongFrameLength];
                    var prng = NextPrng(prngFactory);
                    if (applyTns)
                    {
                        AacChannelDecoder.DecodeSingleChannelToSamples(sce, sampleRate, prng, objectType, fb, pcm);
                    }
                    else
                    {
                        AacChannelDecoder.DecodeSingleChannelToSamples(sce, sampleRate, prng, fb, pcm);
                    }
                    outputs.Add(new AacPceChannelOutput
                    {
                        Region = entry.Region,
                        RegionIndex = entry.RegionIndex,
                        PairIndex = null,
                        Samples = pcm,
                    });
                    break;
                }

                case AacSyntacticElementType.LfeChannelElement:
                {
                    var lfe = entry.RawEntry.LowFrequency
                        ?? throw new InvalidOperationException(
                            "Resolved LFE entry has null LowFrequency payload; raw_data_block must be parsed via the 'full' overload that populates typed payloads.");
                    var fb = filterbanks[fbIndex++];
                    var pcm = new float[AacSynthesisFilterbank.LongFrameLength];
                    var prng = NextPrng(prngFactory);
                    if (applyTns)
                    {
                        AacChannelDecoder.DecodeLfeToSamples(lfe, sampleRate, prng, objectType, fb, pcm);
                    }
                    else
                    {
                        AacChannelDecoder.DecodeLfeToSamples(lfe, sampleRate, prng, fb, pcm);
                    }
                    outputs.Add(new AacPceChannelOutput
                    {
                        Region = entry.Region,
                        RegionIndex = entry.RegionIndex,
                        PairIndex = null,
                        Samples = pcm,
                    });
                    break;
                }

                case AacSyntacticElementType.ChannelPairElement:
                {
                    var cpe = entry.RawEntry.ChannelPair
                        ?? throw new InvalidOperationException(
                            "Resolved CPE entry has null ChannelPair payload; raw_data_block must be parsed via the 'full' overload that populates typed payloads.");
                    var leftFb = filterbanks[fbIndex++];
                    var rightFb = filterbanks[fbIndex++];
                    var leftPcm = new float[AacSynthesisFilterbank.LongFrameLength];
                    var rightPcm = new float[AacSynthesisFilterbank.LongFrameLength];
                    var leftPrng = NextPrng(prngFactory);
                    var rightPrng = NextPrng(prngFactory);
                    if (applyTns)
                    {
                        AacChannelDecoder.DecodePairToSamples(cpe, sampleRate, leftPrng, rightPrng, objectType, leftFb, rightFb, leftPcm, rightPcm);
                    }
                    else
                    {
                        AacChannelDecoder.DecodePairToSamples(cpe, sampleRate, leftPrng, rightPrng, leftFb, rightFb, leftPcm, rightPcm);
                    }
                    outputs.Add(new AacPceChannelOutput
                    {
                        Region = entry.Region,
                        RegionIndex = entry.RegionIndex,
                        PairIndex = 0,
                        Samples = leftPcm,
                    });
                    outputs.Add(new AacPceChannelOutput
                    {
                        Region = entry.Region,
                        RegionIndex = entry.RegionIndex,
                        PairIndex = 1,
                        Samples = rightPcm,
                    });
                    break;
                }

                default:
                    throw new InvalidOperationException(
                        $"Unexpected PCE-resolved element type: {entry.RawEntry.Type}.");
            }
        }

        return new AacPceDecodedRawDataBlock { Channels = outputs };
    }

    private static int CountChannelSlots(IReadOnlyList<AacPceChannelSlot> slots)
    {
        int total = 0;
        for (int i = 0; i < slots.Count; i++)
        {
            total += slots[i].IsCpe ? 2 : 1;
        }
        return total;
    }

    private static AacPnsRandom NextPrng(Func<AacPnsRandom> factory)
    {
        var prng = factory()
            ?? throw new InvalidOperationException(
                "prngFactory returned null; the factory must return a non-null AacPnsRandom on every invocation.");
        return prng;
    }
}
