namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One decoded speaker output from a single AAC raw_data_block:
/// the assigned speaker position plus 1024 PCM samples.
/// </summary>
public sealed record AacChannelOutput
{
    /// <summary>The speaker position this PCM stream feeds.</summary>
    public required AacSpeaker Speaker { get; init; }

    /// <summary>
    /// PCM samples for this frame, exactly
    /// <see cref="AacSynthesisFilterbank.LongFrameLength"/> long
    /// (1024). Range is roughly [-1, +1] for normal content but
    /// may exceed it for transient peaks.
    /// </summary>
    public required float[] Samples { get; init; }
}

/// <summary>
/// Per-frame decoder output: one <see cref="AacChannelOutput"/>
/// per speaker in the channel configuration's mapping, in the
/// canonical order defined by
/// <see cref="AacChannelMapping.GetForConfiguration"/>.
/// </summary>
public sealed record AacDecodedRawDataBlock
{
    /// <summary>Decoded speaker outputs, in mapping order.</summary>
    public required IReadOnlyList<AacChannelOutput> Channels { get; init; }
}

/// <summary>
/// Frame-level dispatcher that walks a parsed
/// <see cref="AacRawDataBlock"/> and produces one PCM stream per
/// speaker, applying the spec channel mapping for
/// channel_configuration 1..7.
/// </summary>
/// <remarks>
/// <para>
/// The dispatcher is stateless across frames. Filterbank state
/// must be carried by the caller via the per-speaker dictionary
/// passed to <see cref="DecodeToSamples(AacRawDataBlock, int, int, Func{AacPnsRandom}, IReadOnlyDictionary{AacSpeaker, AacSynthesisFilterbank})"/>.
/// Reusing the same filterbank instances across consecutive
/// frames preserves the spec-mandated 50 % MDCT overlap-add; a
/// fresh dictionary at the start of a stream / after seek produces
/// the correct half-frame silent ramp-up.
/// </para>
/// <para>
/// The PNS PRNG is acquired via a caller-supplied factory because
/// CPE elements need two independent PRNG streams (one per
/// channel half) — sharing state would couple the PNS noise of
/// the left and right channels. Typical callers pass
/// <c>() =&gt; new AacPnsRandom(seed: ...)</c> or wire in a
/// per-channel-position PRNG bank.
/// </para>
/// <para>
/// Coupling Channel Elements are currently traversed but their
/// auxiliary gain contribution is NOT mixed into target SCE /
/// CPE channels — that requires the coupling gain table (open
/// ship). CCE channels do not appear in the returned channel
/// list.
/// </para>
/// </remarks>
public static class AacRawDataBlockDecoder
{
    /// <summary>
    /// Get the ordered list of speakers produced by
    /// <paramref name="channelConfiguration"/>'s mapping. The
    /// returned order matches the
    /// <see cref="AacDecodedRawDataBlock.Channels"/> order.
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="channelConfiguration"/> is outside [1, 7].
    /// </exception>
    public static IReadOnlyList<AacSpeaker> GetExpectedSpeakers(int channelConfiguration)
    {
        if (channelConfiguration is < 1 or > 7)
        {
            throw new ArgumentOutOfRangeException(
                nameof(channelConfiguration),
                channelConfiguration,
                "channelConfiguration must be in [1, 7].");
        }

        var mapping = AacChannelMapping.GetForConfiguration(channelConfiguration);
        var speakers = new List<AacSpeaker>(mapping.Count * 2);
        foreach (var entry in mapping)
        {
            if (entry.FirstSpeaker != AacSpeaker.None)
            {
                speakers.Add(entry.FirstSpeaker);
            }
            if (entry.SecondSpeaker != AacSpeaker.None)
            {
                speakers.Add(entry.SecondSpeaker);
            }
        }
        return speakers;
    }

    /// <summary>
    /// Convenience: build a fresh per-speaker filterbank dictionary
    /// for <paramref name="channelConfiguration"/>'s expected
    /// speakers. Use this to seed the
    /// <see cref="DecodeToSamples(AacRawDataBlock, int, int, Func{AacPnsRandom}, IReadOnlyDictionary{AacSpeaker, AacSynthesisFilterbank})"/>
    /// argument at stream start; carry the SAME dictionary across
    /// subsequent frames so overlap-add state survives.
    /// </summary>
    public static Dictionary<AacSpeaker, AacSynthesisFilterbank> CreateFilterbanks(int channelConfiguration)
    {
        var speakers = GetExpectedSpeakers(channelConfiguration);
        var dict = new Dictionary<AacSpeaker, AacSynthesisFilterbank>(speakers.Count);
        foreach (var s in speakers)
        {
            dict[s] = new AacSynthesisFilterbank();
        }
        return dict;
    }

    /// <summary>
    /// Decode <paramref name="block"/> to per-speaker PCM. The
    /// non-AOT overload skips TNS inverse filtering; use the AOT
    /// overload to apply TNS for AOTs that emit it (AAC-LC, …).
    /// </summary>
    public static AacDecodedRawDataBlock DecodeToSamples(
        AacRawDataBlock block,
        int channelConfiguration,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        IReadOnlyDictionary<AacSpeaker, AacSynthesisFilterbank> filterbanks)
    {
        return DecodeCore(block, channelConfiguration, sampleRate, prngFactory, filterbanks, applyTns: false, objectType: default);
    }

    /// <summary>
    /// AOT-aware decode: also applies TNS inverse filtering for the
    /// supplied <paramref name="objectType"/>.
    /// </summary>
    public static AacDecodedRawDataBlock DecodeToSamples(
        AacRawDataBlock block,
        int channelConfiguration,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        AacAudioObjectType objectType,
        IReadOnlyDictionary<AacSpeaker, AacSynthesisFilterbank> filterbanks)
    {
        return DecodeCore(block, channelConfiguration, sampleRate, prngFactory, filterbanks, applyTns: true, objectType);
    }

    private static AacDecodedRawDataBlock DecodeCore(
        AacRawDataBlock block,
        int channelConfiguration,
        int sampleRate,
        Func<AacPnsRandom> prngFactory,
        IReadOnlyDictionary<AacSpeaker, AacSynthesisFilterbank> filterbanks,
        bool applyTns,
        AacAudioObjectType objectType)
    {
        ArgumentNullException.ThrowIfNull(block);
        ArgumentNullException.ThrowIfNull(prngFactory);
        ArgumentNullException.ThrowIfNull(filterbanks);

        var resolved = AacChannelLayoutResolver.Resolve(block, channelConfiguration);
        var expectedSpeakers = GetExpectedSpeakers(channelConfiguration);

        foreach (var s in expectedSpeakers)
        {
            if (!filterbanks.TryGetValue(s, out var fb) || fb is null)
            {
                throw new ArgumentException(
                    $"filterbanks is missing an entry for speaker {s} required by channel configuration {channelConfiguration}.",
                    nameof(filterbanks));
            }
        }

        var outputs = new List<AacChannelOutput>(expectedSpeakers.Count);

        foreach (var entry in resolved)
        {
            if (entry.Mapping is null)
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
                    var speaker = entry.Mapping.FirstSpeaker;
                    var fb = filterbanks[speaker];
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
                    outputs.Add(new AacChannelOutput { Speaker = speaker, Samples = pcm });
                    break;
                }

                case AacSyntacticElementType.LfeChannelElement:
                {
                    var lfe = entry.RawEntry.LowFrequency
                        ?? throw new InvalidOperationException(
                            "Resolved LFE entry has null LowFrequency payload; raw_data_block must be parsed via the 'full' overload that populates typed payloads.");
                    var speaker = entry.Mapping.FirstSpeaker;
                    var fb = filterbanks[speaker];
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
                    outputs.Add(new AacChannelOutput { Speaker = speaker, Samples = pcm });
                    break;
                }

                case AacSyntacticElementType.ChannelPairElement:
                {
                    var cpe = entry.RawEntry.ChannelPair
                        ?? throw new InvalidOperationException(
                            "Resolved CPE entry has null ChannelPair payload; raw_data_block must be parsed via the 'full' overload that populates typed payloads.");
                    var leftSpeaker = entry.Mapping.FirstSpeaker;
                    var rightSpeaker = entry.Mapping.SecondSpeaker;
                    var leftFb = filterbanks[leftSpeaker];
                    var rightFb = filterbanks[rightSpeaker];
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
                    outputs.Add(new AacChannelOutput { Speaker = leftSpeaker, Samples = leftPcm });
                    outputs.Add(new AacChannelOutput { Speaker = rightSpeaker, Samples = rightPcm });
                    break;
                }

                default:
                    throw new InvalidOperationException(
                        $"Unexpected speaker-bound element type: {entry.RawEntry.Type}.");
            }
        }

        return new AacDecodedRawDataBlock { Channels = outputs };
    }

    private static AacPnsRandom NextPrng(Func<AacPnsRandom> factory)
    {
        var prng = factory()
            ?? throw new InvalidOperationException(
                "prngFactory returned null; the factory must return a non-null AacPnsRandom on every invocation.");
        return prng;
    }
}
