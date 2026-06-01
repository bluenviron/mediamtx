namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Classifies how a scale-factor band's value should be interpreted
/// based on its section's codebook number
/// (ISO/IEC 14496-3 §4.4.2.5, §4.6.3.3 Table 4.76, and Annex 4.A.2).
/// </summary>
public enum AacScaleFactorKind : byte
{
    /// <summary>
    /// No value read for this band - either because the section uses
    /// the ZERO_HCB sentinel (codebook 0, spectral coefficients are
    /// all zero) or because the codebook is reserved (12).
    /// </summary>
    None = 0,

    /// <summary>
    /// Spectral-gain scale factor (codebooks 1..11). The differential
    /// is encoded against the previous spectral-gain scale factor
    /// (initially global_gain).
    /// </summary>
    SpectralGain = 1,

    /// <summary>
    /// Perceptual-noise-substitution (PNS) noise energy
    /// (codebook 13). One Huffman codeword per band, decoded the
    /// same way as a spectral-gain differential (idx - 60). The
    /// caller is responsible for cumulative reconstruction against
    /// the PNS noise-energy state machine.
    /// </summary>
    NoiseEnergy = 2,

    /// <summary>
    /// Intensity-stereo position (codebooks 14 and 15). One Huffman
    /// codeword per band, decoded the same way as a spectral-gain
    /// differential (idx - 60). The caller is responsible for
    /// cumulative reconstruction against the intensity-position
    /// accumulator (codebook 15 indicates inverted polarity).
    /// </summary>
    IntensityPosition = 3,
}

/// <summary>
/// One decoded scale-factor band value: which group, which band,
/// what classification, and the signed differential as it appears
/// in the bitstream (before cumulative reconstruction).
/// </summary>
public sealed record AacScaleFactorEntry
{
    /// <summary>Zero-based window group index this band belongs to.</summary>
    public required int Group { get; init; }

    /// <summary>Scale-factor band index inside the group's <c>max_sfb</c> range.</summary>
    public required int Sfb { get; init; }

    /// <summary>Classification driven by the band's section codebook.</summary>
    public required AacScaleFactorKind Kind { get; init; }

    /// <summary>
    /// Signed differential as read from the bitstream. <c>0</c> when
    /// <see cref="Kind"/> is <see cref="AacScaleFactorKind.None"/>; in
    /// the range <c>[-60, +60]</c> for
    /// <see cref="AacScaleFactorKind.SpectralGain"/> and
    /// <see cref="AacScaleFactorKind.IntensityPosition"/>; in the
    /// range <c>[-256, +255]</c> for the first
    /// <see cref="AacScaleFactorKind.NoiseEnergy"/> band of a
    /// scale_factor_data() call (which uses a 9-bit unsigned PCM
    /// value <c>dpcm_noise_nrg</c> with offset 256, per ISO/IEC
    /// 14496-3 §4.6.2.3), and <c>[-60, +60]</c> for every
    /// subsequent noise band.
    /// </summary>
    public required int Differential { get; init; }
}

/// <summary>
/// Decoded view of an AAC <c>scale_factor_data()</c> block
/// (ISO/IEC 14496-3 §4.4.2.5). Walks the sections produced by
/// <see cref="AacSectionData"/> and, for every spectral-gain band,
/// reads one Huffman codeword from the scale-factor codebook plus
/// the canonical <c>idx - 60</c> diff conversion.
/// </summary>
/// <remarks>
/// The scale-factor codebook is passed in so this reader stays
/// decoupled from the (large) static length table in
/// ISO/IEC 14496-3 Annex 4.A.2.1; the codebook can therefore be
/// either the standard 121-entry table or a synthetic codebook in
/// tests. The codebook must have exactly <c>121</c> symbol slots
/// mapping <c>[0, 120]</c> to scale-factor differentials
/// <c>[-60, +60]</c>.
/// </remarks>
public sealed record AacScaleFactorData
{
    /// <summary>Decoded entries in stream order (one per scale-factor band of every section).</summary>
    public required IReadOnlyList<AacScaleFactorEntry> Entries { get; init; }

    /// <summary>Number of bits consumed from the bit reader.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Walk a section list and read scale-factor differentials.
    /// Returns <see langword="false"/> on stream underflow or on a
    /// decoded symbol outside <c>[0, 120]</c>.
    /// </summary>
    /// <remarks>
    /// <para>
    /// Spectral-gain sections (codebooks 1..11) each carry one
    /// Huffman codeword per band, decoded against the scale-factor
    /// codebook (value <c>idx - 60</c>).
    /// </para>
    /// <para>
    /// Intensity-stereo sections (codebooks 14, 15) likewise carry
    /// one Huffman codeword per band, decoded the same way.
    /// </para>
    /// <para>
    /// PNS sections (codebook 13) follow a special two-mode encoding
    /// per ISO/IEC 14496-3 §4.6.2.3: the <i>first</i> PNS band of a
    /// scale_factor_data() call uses a 9-bit unsigned PCM value
    /// (<c>dpcm_noise_nrg</c>, exposed as <c>raw - 256</c> via
    /// <see cref="AacScaleFactorEntry.Differential"/>); every
    /// subsequent PNS band uses the SF Huffman codebook like the
    /// other kinds. Cumulative reconstruction (per-state initial
    /// values, PNS energy offset, intensity-position accumulator) is
    /// the caller's responsibility.
    /// </para>
    /// </remarks>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacSectionData sectionData,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacScaleFactorData? data)
    {
        data = null;
        ArgumentNullException.ThrowIfNull(sectionData);
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        if (scaleFactorCodebook.Capacity != 121) return false;

        int startBits = reader.Position;
        var entries = new List<AacScaleFactorEntry>();
        bool noisePcmPending = true;

        foreach (var section in sectionData.Sections)
        {
            var kind = ClassifyCodebook(section.CodebookNumber);

            for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
            {
                if (kind == AacScaleFactorKind.None)
                {
                    entries.Add(new AacScaleFactorEntry
                    {
                        Group = section.Group,
                        Sfb = sfb,
                        Kind = AacScaleFactorKind.None,
                        Differential = 0,
                    });
                    continue;
                }

                int diff;
                if (kind == AacScaleFactorKind.NoiseEnergy && noisePcmPending)
                {
                    if (reader.Remaining < 9) return false;
                    int raw = (int)reader.ReadBits(9);
                    diff = raw - 256;
                    noisePcmPending = false;
                }
                else
                {
                    if (!scaleFactorCodebook.TryDecode(ref reader, out int symbolIndex))
                        return false;
                    if (symbolIndex < 0 || symbolIndex > 120)
                        return false;
                    diff = symbolIndex - 60;
                }

                entries.Add(new AacScaleFactorEntry
                {
                    Group = section.Group,
                    Sfb = sfb,
                    Kind = kind,
                    Differential = diff,
                });
            }
        }

        data = new AacScaleFactorData
        {
            Entries = entries,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    private static AacScaleFactorKind ClassifyCodebook(int codebookNumber) => codebookNumber switch
    {
        0 => AacScaleFactorKind.None,
        >= 1 and <= 11 => AacScaleFactorKind.SpectralGain,
        12 => AacScaleFactorKind.None,
        13 => AacScaleFactorKind.NoiseEnergy,
        14 or 15 => AacScaleFactorKind.IntensityPosition,
        _ => AacScaleFactorKind.None,
    };
}
