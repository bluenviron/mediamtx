using System.Collections.Immutable;
using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Codebook selectors that carry no <c>spectral_data()</c> payload
/// (ISO/IEC 14496-3 §4.4.2.4). Sections tagged with one of these are
/// silently skipped by the spectral walker.
/// </summary>
internal static class AacSpectralCodebookSentinels
{
    public const int ZeroHcb = 0;
    public const int NoiseHcb = 13;
    public const int IntensityHcb2 = 14;
    public const int IntensityHcb = 15;
}

/// <summary>
/// Walks an AAC <c>spectral_data()</c> body per ISO/IEC 14496-3
/// §4.4.2.4 and emits the 1024 quantised spectral coefficients of
/// one channel's MDCT frame. Drives the section list captured by
/// <see cref="AacSectionData"/> against the SWB offsets from
/// <see cref="AacSwbOffsets"/> and the per-codebook Huffman tables
/// supplied by the caller.
/// </summary>
/// <remarks>
/// <para>
/// Codebook numbers 0 (ZERO_HCB), 13 (NOISE_HCB) and 14/15
/// (INTENSITY_HCB[2]) do not carry spectral coefficients and are
/// silently skipped; the corresponding coefficient slots stay at
/// zero. Codebook 12 is reserved in AAC-LC and rejected (returns
/// <see langword="false"/>). Codebooks 1..11 are decoded tuple by
/// tuple via <see cref="AacSpectralValueDecoder"/>.
/// </para>
/// <para>
/// The walker honours the window-grouping structure stored on
/// <see cref="AacIcsInfo.WindowsPerGroup"/>. For long sequences the
/// 1024 coefficients map 1:1 to the long SWB table. For EIGHT_SHORT
/// sequences the coefficients are laid out group-by-group, with
/// each group occupying <c>128 * windows_in_group</c> consecutive
/// slots; inside a group, the SWB <i>sfb</i> band occupies
/// <c>swb_offset_short[sfb+1] * N - swb_offset_short[sfb] * N</c>
/// coefficients (interleaved across the group's windows). Total
/// emitted coefficients are always 1024 regardless of the window
/// sequence.
/// </para>
/// <para>
/// This walker only supports the canonical AAC-LC 1024 / 128
/// transform pair. The 960 / 120 LC-LD variant and the LTP profile
/// are not handled.
/// </para>
/// </remarks>
public sealed record AacSpectralData
{
    /// <summary>Transform length emitted by this walker (always 1024).</summary>
    public const int TransformLength = 1024;

    /// <summary>
    /// 1024 quantised spectral coefficients in spec order. SWB cells
    /// covered by ZERO_HCB / NOISE_HCB / INTENSITY_HCB sections stay
    /// at zero.
    /// </summary>
    public required ImmutableArray<int> Coefficients { get; init; }

    /// <summary>Total bits consumed by <c>spectral_data()</c>.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Decode <c>spectral_data()</c> directly from <paramref name="reader"/>
    /// using the section list captured for one
    /// <c>individual_channel_stream()</c>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at the first spectral tuple.</param>
    /// <param name="icsInfo">Active ics_info() (window sequence + grouping).</param>
    /// <param name="sectionData">Parsed section_data() for this ICS body.</param>
    /// <param name="sampleRate">
    /// The source sample rate (Hz) used to dispatch the SWB offset table.
    /// For SBR-doubled AOT 5 streams this is the underlying (non-doubled)
    /// rate that drives the AAC core decoder.
    /// </param>
    /// <param name="spectralCodebooks">
    /// Codebook lookup indexed by codebook number; element <c>i</c> holds
    /// the Huffman codebook used by <c>sect_cb == i</c>. Slots that the
    /// caller knows will never be referenced may be <see langword="null"/>;
    /// the walker only dereferences slots it actually visits.
    /// </param>
    /// <param name="data">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns>
    /// <see langword="true"/> on a successful walk; <see langword="false"/>
    /// when the stream underflows, an unsupported codebook (12) is
    /// encountered, the caller didn't supply the codebook a section
    /// references, the sample rate has no SWB table, a SWB lookup is
    /// out of range, or a section's coefficient span is not a multiple
    /// of its codebook's dimension.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacIcsInfo icsInfo,
        AacSectionData sectionData,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacSpectralData? data)
    {
        data = null;
        ArgumentNullException.ThrowIfNull(icsInfo);
        ArgumentNullException.ThrowIfNull(sectionData);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);

        bool isShort = icsInfo.WindowSequence == AacWindowSequence.EightShort;
        ReadOnlySpan<int> swbOffsets = isShort
            ? AacSwbOffsets.GetShortOffsets(sampleRate)
            : AacSwbOffsets.GetLongOffsets(sampleRate);
        if (swbOffsets.IsEmpty) return false;

        int startBits = reader.Position;
        int[] coefficients = new int[TransformLength];

        ReadOnlySpan<byte> windowsPerGroup = icsInfo.WindowsPerGroup.Span;
        Span<int> groupCoeffBase = stackalloc int[icsInfo.WindowGroupCount];
        int runningBase = 0;
        for (int g = 0; g < icsInfo.WindowGroupCount; g++)
        {
            groupCoeffBase[g] = runningBase;
            int groupCoeffSize = isShort
                ? AacSwbOffsets.ShortTransformLength * windowsPerGroup[g]
                : TransformLength;
            runningBase += groupCoeffSize;
            if (runningBase > TransformLength) return false;
        }
        if (runningBase != TransformLength) return false;

        Span<int> tuple = stackalloc int[4];

        foreach (var section in sectionData.Sections)
        {
            int cb = section.CodebookNumber;
            if (cb == AacSpectralCodebookSentinels.ZeroHcb
                || cb == AacSpectralCodebookSentinels.NoiseHcb
                || cb == AacSpectralCodebookSentinels.IntensityHcb2
                || cb == AacSpectralCodebookSentinels.IntensityHcb)
            {
                continue;
            }

            var geometry = AacSpectralCodebookGeometry.Get(cb);
            if (geometry is null) return false; // cb 12 + reserved values
            if ((uint)cb >= (uint)spectralCodebooks.Count) return false;
            var codebook = spectralCodebooks[cb];
            if (codebook is null) return false;

            int g = section.Group;
            if ((uint)g >= (uint)icsInfo.WindowGroupCount) return false;

            int windowsInGroup = isShort ? windowsPerGroup[g] : 1;

            if ((uint)section.StartSfb >= (uint)swbOffsets.Length
                || (uint)section.EndSfb >= (uint)swbOffsets.Length
                || section.EndSfb <= section.StartSfb)
            {
                return false;
            }

            int sectionWidth = (swbOffsets[section.EndSfb] - swbOffsets[section.StartSfb]) * windowsInGroup;
            if (sectionWidth <= 0) return false;
            if (sectionWidth % geometry.Dimension != 0) return false;

            int destBase = groupCoeffBase[g] + swbOffsets[section.StartSfb] * windowsInGroup;
            if (destBase < 0 || destBase + sectionWidth > TransformLength) return false;

            int dim = geometry.Dimension;
            for (int k = 0; k < sectionWidth; k += dim)
            {
                if (!AacSpectralValueDecoder.TryRead(ref reader, geometry, codebook, tuple))
                {
                    return false;
                }
                for (int d = 0; d < dim; d++)
                {
                    coefficients[destBase + k + d] = tuple[d];
                }
            }
        }

        data = new AacSpectralData
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(coefficients),
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Decode <c>spectral_data()</c> from a byte buffer starting at the
    /// first bit. See <see cref="TryRead"/> for parameter semantics.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacIcsInfo icsInfo,
        AacSectionData sectionData,
        int sampleRate,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        out AacSpectralData? data)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, icsInfo, sectionData, sampleRate, spectralCodebooks, out data);
    }
}
