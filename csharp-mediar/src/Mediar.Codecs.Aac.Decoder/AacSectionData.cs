namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One contiguous run of scale-factor bands sharing a single Huffman
/// codebook, as emitted by AAC <c>section_data()</c>
/// (ISO/IEC 14496-3 §4.4.2.4, Table 4.75). The
/// <c>[StartSfb, EndSfb)</c> half-open interval is local to a
/// window group; sections from different groups are independent.
/// </summary>
public sealed record AacSection
{
    /// <summary>Zero-based window-group index this section belongs to.</summary>
    public required int Group { get; init; }

    /// <summary>
    /// Codebook selector (0..15): 0 = ZERO_HCB, 1..11 = spectral
    /// codebooks, 12 = intensity stereo placeholder, 13 = noise
    /// (PNS), 14/15 = intensity. Phase-1 surfaces the raw value
    /// without further validation.
    /// </summary>
    public required int CodebookNumber { get; init; }

    /// <summary>First scale-factor band in this section (inclusive).</summary>
    public required int StartSfb { get; init; }

    /// <summary>One past the last scale-factor band in this section.</summary>
    public required int EndSfb { get; init; }
}

/// <summary>
/// Decoded view of an AAC <c>section_data()</c> block (ISO/IEC
/// 14496-3 §4.4.2.4). Each window group of <c>max_sfb</c> bands is
/// partitioned into contiguous runs of bands sharing one Huffman
/// codebook; sections are emitted in stream order.
/// </summary>
/// <remarks>
/// <para>
/// <c>sect_len_incr</c> is 3 bits for EIGHT_SHORT sequences and 5
/// bits for any long sequence (per Table 4.75). The escape value
/// <c>(1 &lt;&lt; sect_len_incr) - 1</c> chains additional
/// <c>sect_len_incr</c>-bit reads onto the section length until a
/// non-escape value is read.
/// </para>
/// <para>
/// <c>sect_cb</c> is 4 bits wide for the LC/Main/SSR profiles
/// covered by phase-1; the 5-bit variant used by the ER (error
/// resilient) profile is a future extension.
/// </para>
/// </remarks>
public sealed record AacSectionData
{
    /// <summary>Sections in stream order, partitioned by window group.</summary>
    public required IReadOnlyList<AacSection> Sections { get; init; }

    /// <summary>
    /// Parse <c>section_data()</c> using the window-group structure
    /// already captured in <paramref name="icsInfo"/>. Returns
    /// <see langword="false"/> when the stream underflows or when a
    /// section overruns <see cref="AacIcsInfo.MaxSfb"/>.
    /// </summary>
    internal static bool TryParse(scoped ref BitReader reader, AacIcsInfo icsInfo, out AacSectionData? data)
    {
        data = null;
        ArgumentNullException.ThrowIfNull(icsInfo);
        if (icsInfo.MaxSfb < 0) return false;
        if (icsInfo.MaxSfb == 0)
        {
            data = new AacSectionData { Sections = Array.Empty<AacSection>() };
            return true;
        }

        int sectLenIncr = icsInfo.WindowSequence == AacWindowSequence.EightShort ? 3 : 5;
        int sectEscVal = (1 << sectLenIncr) - 1;

        try
        {
            var sections = new List<AacSection>();
            for (int g = 0; g < icsInfo.WindowGroupCount; g++)
            {
                int i = 0;
                while (i < icsInfo.MaxSfb)
                {
                    if (reader.Remaining < 4) return false;
                    int sectCb = (int)reader.ReadBits(4);

                    int sectLen = 0;
                    int chunk;
                    do
                    {
                        if (reader.Remaining < sectLenIncr) return false;
                        chunk = (int)reader.ReadBits(sectLenIncr);
                        sectLen += chunk;
                    } while (chunk == sectEscVal);

                    int sectEnd = i + sectLen;
                    if (sectEnd > icsInfo.MaxSfb) return false;
                    if (sectLen == 0) return false; // empty section is malformed

                    sections.Add(new AacSection
                    {
                        Group = g,
                        CodebookNumber = sectCb,
                        StartSfb = i,
                        EndSfb = sectEnd,
                    });
                    i = sectEnd;
                }
            }

            data = new AacSectionData { Sections = sections };
            return true;
        }
        catch (EndOfStreamException)
        {
            data = null;
            return false;
        }
    }
}
