namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One AAC channel's bitstream-side frame state: an
/// <see cref="AacIndividualChannelStream"/> body (global_gain +
/// ics_info + section_data + scale_factor_data + optional
/// pulse_data / tns_data / gain_control_data) plus the
/// <see cref="AacSpectralData"/> coefficient block that immediately
/// follows in the raw_data_block. Bridges the two existing parsers
/// without modifying either one, so the same readers stay usable
/// in their original "stop at spectral_data" mode for callers that
/// only want the side-information.
/// </summary>
/// <remarks>
/// <para>
/// This aggregator is the building block the raw_data_block walker
/// will eventually use to fully consume <c>single_channel_element()</c>,
/// <c>channel_pair_element()</c>, <c>coupling_channel_element()</c>
/// and <c>lfe_channel_element()</c>. It is intentionally agnostic of
/// the enclosing element class - the caller decides whether to feed
/// a shared <c>ics_info()</c> (CPE common_window = 1) or
/// <see langword="null"/> (own-ics_info path for SCE / LFE / CCE /
/// CPE common_window = 0), and supplies the per-codebook Huffman
/// tables plus the source sample rate.
/// </para>
/// <para>
/// <see cref="BitsConsumed"/> is the sum of the ICS body and the
/// spectral_data() block. When the caller passes
/// <c>scaleFlag = true</c> the ICS body stops after scale_factor_data
/// (no pulse / tns / gain_control flags) and the spectral_data block
/// is still walked; the value still reflects exactly what was read.
/// </para>
/// </remarks>
public sealed record AacChannelFrame
{
    /// <summary>Parsed individual_channel_stream() body (excluding spectral_data).</summary>
    public required AacIndividualChannelStream Stream { get; init; }

    /// <summary>Parsed spectral_data() coefficient block.</summary>
    public required AacSpectralData SpectralData { get; init; }

    /// <summary>Total bits consumed by the ICS body + spectral_data block.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Read a single channel frame: ICS body followed by spectral_data.
    /// </summary>
    /// <param name="reader">Bit reader positioned at <c>global_gain</c>.</param>
    /// <param name="sharedIcsInfo">
    /// Caller-supplied shared ics_info when the enclosing CPE passed
    /// <c>common_window = 1</c>; <see langword="null"/> when the body
    /// should parse its own ics_info.
    /// </param>
    /// <param name="scaleFlag">
    /// Scalable-coding flag from the enclosing scalable_extension_data().
    /// Always <see langword="false"/> for AAC-LC content.
    /// </param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="sampleRate">
    /// Source sample rate (Hz) used to dispatch the SWB offset table.
    /// </param>
    /// <param name="spectralCodebooks">
    /// Codebook lookup indexed by codebook number; element <c>i</c> holds
    /// the Huffman codebook used by <c>sect_cb == i</c>. Slots known
    /// not to be referenced may be <see langword="null"/>.
    /// </param>
    /// <param name="frame">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns>
    /// <see langword="true"/> when both phases parsed cleanly;
    /// <see langword="false"/> when the ICS body rejects (see
    /// <see cref="AacIndividualChannelStream.TryRead"/>) or when the
    /// spectral_data walker rejects (see
    /// <see cref="AacSpectralData.TryRead"/>).
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacIcsInfo? sharedIcsInfo,
        bool scaleFlag,
        AacHuffmanCodebook scaleFactorCodebook,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacChannelFrame? frame)
    {
        frame = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);

        int startBits = reader.Position;

        if (!AacIndividualChannelStream.TryRead(
                ref reader,
                sharedIcsInfo,
                scaleFlag,
                scaleFactorCodebook,
                out var stream)
            || stream is null)
        {
            return false;
        }

        if (!AacSpectralData.TryRead(
                ref reader,
                stream.IcsInfo,
                stream.SectionData,
                sampleRate,
                spectralCodebooks,
                out var spectral)
            || spectral is null)
        {
            return false;
        }

        frame = new AacChannelFrame
        {
            Stream = stream,
            SpectralData = spectral,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous channel frame (ICS body + spectral_data) from
    /// <paramref name="bytes"/> starting at the first bit. See
    /// <see cref="TryRead"/> for the parameter semantics.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacIcsInfo? sharedIcsInfo,
        bool scaleFlag,
        AacHuffmanCodebook scaleFactorCodebook,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacChannelFrame? frame)
    {
        var reader = new BitReader(bytes);
        return TryRead(
            ref reader,
            sharedIcsInfo,
            scaleFlag,
            scaleFactorCodebook,
            sampleRate,
            spectralCodebooks,
            out frame);
    }
}
