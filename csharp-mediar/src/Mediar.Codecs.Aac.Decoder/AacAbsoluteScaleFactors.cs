namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One absolute (post-accumulator) scale-factor band value.
/// <see cref="Value"/> is the result of applying the appropriate
/// per-kind accumulator (initialised from <c>global_gain</c> for
/// spectral bands, from <c>global_gain - 90</c> for PNS bands, from
/// <c>0</c> for intensity bands) over the running stream of
/// <see cref="AacScaleFactorEntry.Differential"/> values up to this
/// band.
/// </summary>
public sealed record AacAbsoluteScaleFactorEntry
{
    /// <summary>Zero-based window-group index this band belongs to.</summary>
    public required int Group { get; init; }

    /// <summary>Scale-factor band index inside the group's <c>max_sfb</c> range.</summary>
    public required int Sfb { get; init; }

    /// <summary>Classification driven by the band's section codebook.</summary>
    public required AacScaleFactorKind Kind { get; init; }

    /// <summary>
    /// Absolute scale-factor value for this band. For
    /// <see cref="AacScaleFactorKind.None"/> bands this is always
    /// <c>0</c> and downstream stages should treat the band as silent
    /// (ZERO_HCB) or reserved.
    /// </summary>
    public required int Value { get; init; }
}

/// <summary>
/// Absolute scale-factor band values reconstructed from
/// <see cref="AacScaleFactorData"/> deltas and <c>global_gain</c>
/// (ISO/IEC 14496-3 §4.6.2.3.2).
/// </summary>
/// <remarks>
/// <para>
/// AAC carries scale factors as <i>differentials</i> against per-kind
/// accumulators, all initialised from the per-channel
/// <c>global_gain</c> field that prefixes
/// <c>individual_channel_stream()</c> (or, for PNS, from
/// <c>global_gain - <see cref="NoiseOffset"/></c>; or, for intensity
/// stereo, from <c>0</c>). The dequantization stage needs the
/// absolute values to compute the per-band gain
/// <c>2^((sf - 100) / 4)</c>.
/// </para>
/// <para>
/// Per §4.6.2.3 the first PNS band of a scale_factor_data() call
/// carries a 9-bit PCM value <c>dpcm_noise_nrg</c>. The reader in
/// <see cref="AacScaleFactorData"/> normalises that to a signed
/// differential in <c>[-256, +255]</c>; this accumulator adds it to
/// <c>global_gain - 90</c> to obtain the initial noise energy.
/// </para>
/// </remarks>
public sealed record AacAbsoluteScaleFactors
{
    /// <summary>
    /// Constant offset subtracted from <c>global_gain</c> when
    /// initialising the PNS noise-energy accumulator per
    /// ISO/IEC 14496-3 §4.6.2.3.
    /// </summary>
    public const int NoiseOffset = 90;

    /// <summary>Absolute scale-factor entries in the same stream order as the source deltas.</summary>
    public required IReadOnlyList<AacAbsoluteScaleFactorEntry> Entries { get; init; }

    /// <summary>
    /// Reconstruct absolute scale-factor values from
    /// <paramref name="deltas"/> using the channel's <paramref name="globalGain"/>
    /// (the 8-bit field that prefixes <c>individual_channel_stream()</c>).
    /// </summary>
    /// <param name="deltas">
    /// Differentials decoded by <see cref="AacScaleFactorData.TryRead(ref BitReader, AacSectionData, AacHuffmanCodebook, out AacScaleFactorData)"/>.
    /// </param>
    /// <param name="globalGain">
    /// The 8-bit <c>global_gain</c> value (0..255). The caller should
    /// already have validated this is in range.
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="deltas"/> is <see langword="null"/>.
    /// </exception>
    public static AacAbsoluteScaleFactors FromDelta(
        AacScaleFactorData deltas,
        int globalGain)
    {
        ArgumentNullException.ThrowIfNull(deltas);

        int spectralAcc = globalGain;
        int noiseAcc = 0;
        int intensityAcc = 0;
        bool noisePcmPending = true;

        var output = new List<AacAbsoluteScaleFactorEntry>(deltas.Entries.Count);
        foreach (var entry in deltas.Entries)
        {
            int value;
            switch (entry.Kind)
            {
                case AacScaleFactorKind.SpectralGain:
                    spectralAcc += entry.Differential;
                    value = spectralAcc;
                    break;

                case AacScaleFactorKind.NoiseEnergy:
                    if (noisePcmPending)
                    {
                        noiseAcc = globalGain - NoiseOffset + entry.Differential;
                        noisePcmPending = false;
                    }
                    else
                    {
                        noiseAcc += entry.Differential;
                    }
                    value = noiseAcc;
                    break;

                case AacScaleFactorKind.IntensityPosition:
                    intensityAcc += entry.Differential;
                    value = intensityAcc;
                    break;

                case AacScaleFactorKind.None:
                default:
                    value = 0;
                    break;
            }

            output.Add(new AacAbsoluteScaleFactorEntry
            {
                Group = entry.Group,
                Sfb = entry.Sfb,
                Kind = entry.Kind,
                Value = value,
            });
        }

        return new AacAbsoluteScaleFactors { Entries = output };
    }
}
