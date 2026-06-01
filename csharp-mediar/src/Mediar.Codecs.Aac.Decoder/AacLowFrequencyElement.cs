#pragma warning disable CA1711 // The type name mirrors the ISO/IEC 14496-3 syntactic element low_frequency_element().

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of an AAC <c>lfe_channel_element()</c> (LFE) per
/// ISO/IEC 14496-3 §4.4.2.1 Table 4.6. Composes the 4-bit
/// <c>element_instance_tag</c> prefix and the
/// <see cref="AacIndividualChannelStream"/> body. The body parse stops
/// at the <c>spectral_data()</c> boundary — the spectral coefficients
/// are not consumed here (they require the swb_offset tables from
/// ISO/IEC 14496-3 Annex 4.A).
/// </summary>
/// <remarks>
/// LFE is byte-for-byte identical to <see cref="AacSingleChannelElement"/>
/// at the bitstream level — the only difference is the surrounding
/// raw_data_block dispatcher's <c>id_syn_ele</c> code (LFE = 3 vs
/// SCE = 0; see <see cref="AacSyntacticElementType"/>). The dedicated
/// wrapper type lets callers distinguish the two element families
/// after the dispatch is done. Both share the same scale_flag = false
/// convention and parse their own <c>ics_info()</c> (LFE never appears
/// as part of a <c>common_window</c> pair).
/// </remarks>
public sealed record AacLowFrequencyElement
{
    /// <summary>Maximum value of <c>element_instance_tag</c> (4-bit field).</summary>
    public const int MaxElementInstanceTag = 15;

    /// <summary>4-bit <c>element_instance_tag</c> identifying this LFE within the raw_data_block.</summary>
    public required int ElementInstanceTag { get; init; }

    /// <summary>Parsed <c>individual_channel_stream()</c> body (excluding spectral_data()).</summary>
    public required AacIndividualChannelStream Stream { get; init; }

    /// <summary>
    /// Parsed <c>spectral_data()</c> coefficients when the caller used the
    /// "full" <see cref="TryRead(ref BitReader, AacHuffmanCodebook, int, IReadOnlyList{AacHuffmanCodebook?}, out AacLowFrequencyElement?)"/>
    /// overload; <see langword="null"/> when the caller used the
    /// boundary-stopping overload.
    /// </summary>
    public AacSpectralData? SpectralData { get; init; }

    /// <summary>
    /// Total bits consumed by the <c>element_instance_tag</c> prefix plus the
    /// <see cref="AacIndividualChannelStream.BitsConsumed"/> body. When
    /// <see cref="SpectralData"/> is populated this also includes the bits
    /// consumed by <c>spectral_data()</c>; otherwise the bits that follow
    /// inside the parent raw_data_block belong to <c>spectral_data()</c>
    /// and are not counted here.
    /// </summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Read a <c>lfe_channel_element()</c> from <paramref name="reader"/>
    /// positioned at <c>element_instance_tag</c>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at element_instance_tag.</param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="element">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns><see langword="true"/> when the prefix and body parsed cleanly.</returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacLowFrequencyElement? element)
    {
        element = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);

        int startBits = reader.Position;
        if (reader.Remaining < 4) return false;
        int elementInstanceTag = (int)reader.ReadBits(4);

        // LFE always parses its own ics_info (no common_window machinery) and
        // is always non-scalable (scaleFlag=false).
        if (!AacIndividualChannelStream.TryRead(
                ref reader,
                sharedIcsInfo: null,
                scaleFlag: false,
                scaleFactorCodebook,
                out var stream)
            || stream is null)
        {
            return false;
        }

        element = new AacLowFrequencyElement
        {
            ElementInstanceTag = elementInstanceTag,
            Stream = stream,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>lfe_channel_element()</c> body from
    /// <paramref name="bytes"/> starting at the first bit.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacLowFrequencyElement? element)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, scaleFactorCodebook, out element);
    }

    /// <summary>
    /// Read a "full" <c>lfe_channel_element()</c> that also consumes the
    /// inline <c>spectral_data()</c> coefficient block, populating
    /// <see cref="SpectralData"/>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at element_instance_tag.</param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="sampleRate">
    /// Source sample rate (Hz) used to dispatch the SWB offset table.
    /// </param>
    /// <param name="spectralCodebooks">
    /// Codebook lookup indexed by codebook number; element <c>i</c> holds
    /// the Huffman codebook used by <c>sect_cb == i</c>. Slots known not to
    /// be referenced may be <see langword="null"/>.
    /// </param>
    /// <param name="element">Populated on success; <see langword="null"/> otherwise.</param>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacHuffmanCodebook scaleFactorCodebook,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacLowFrequencyElement? element)
    {
        element = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);

        int startBits = reader.Position;
        if (reader.Remaining < 4) return false;
        int elementInstanceTag = (int)reader.ReadBits(4);

        if (!AacChannelFrame.TryRead(
                ref reader,
                sharedIcsInfo: null,
                scaleFlag: false,
                scaleFactorCodebook,
                sampleRate,
                spectralCodebooks,
                out var frame)
            || frame is null)
        {
            return false;
        }

        element = new AacLowFrequencyElement
        {
            ElementInstanceTag = elementInstanceTag,
            Stream = frame.Stream,
            SpectralData = frame.SpectralData,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous "full" <c>lfe_channel_element()</c> body
    /// (element_instance_tag + ICS body + spectral_data) from
    /// <paramref name="bytes"/> starting at the first bit.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacHuffmanCodebook scaleFactorCodebook,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacLowFrequencyElement? element)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, scaleFactorCodebook, sampleRate, spectralCodebooks, out element);
    }
}
