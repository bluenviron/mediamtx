namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Expands a parsed <see cref="AacCouplingGainList"/> into absolute
/// per-(window group, scale-factor band) gain indices ready for
/// downstream <see cref="AacCouplingChannelElement"/> gain
/// application (ISO/IEC 14496-3 §4.6.8.3.3).
/// </summary>
/// <remarks>
/// <para>
/// This is the data-structural half of the coupling-gain decode: it
/// turns the bitstream representation (either a single common
/// Huffman codeword or a DPCM-encoded per-band stream) into a flat
/// <c>int[group, sfb]</c> table of absolute gain indices in the
/// range <c>[-60, +60]</c>. The floating-point gain reconstruction
/// (<c>gain_factor = 2^(index × step)</c> where step depends on
/// <c>gain_element_scale</c> and <c>gain_element_sign</c>) is a
/// separate composer because the exact spec formula needs further
/// PDF cross-check.
/// </para>
/// <para>
/// For <see cref="AacCouplingGainList.CommonGainElementPresent"/> =
/// <see langword="true"/>, every band receives
/// <see cref="AacCouplingGainList.CommonGainDifferential"/>.
/// </para>
/// <para>
/// For DPCM the spec scan order is
/// <c>for (g) for (sfb) if (sect_cb[g][sfb] != ZERO_HCB)</c>. The
/// running cumulative sum is updated per non-ZERO_HCB band only;
/// ZERO_HCB bands inherit the current running value (no entry is
/// transmitted for them). The helper validates that the DPCM
/// stream matches the section data's non-ZERO_HCB scan order
/// exactly.
/// </para>
/// </remarks>
public static class AacCouplingGainExpansion
{
    /// <summary>
    /// Expand <paramref name="gainList"/> into an absolute-index
    /// table indexed by (group, sfb) for the coupling channel
    /// described by <paramref name="couplingSectionData"/>.
    /// </summary>
    /// <param name="gainList">Per-target gain list from the CCE.</param>
    /// <param name="couplingSectionData">
    /// The coupling channel's own <see cref="AacSectionData"/> (NOT
    /// the target channel's). The DPCM entries were encoded against
    /// this scan order during parsing.
    /// </param>
    /// <param name="windowGroupCount">
    /// Number of window groups in the coupling channel's frame
    /// (1 for long windows; 1..8 for EIGHT_SHORT).
    /// </param>
    /// <param name="maxSfb">
    /// Maximum scale-factor band index used by the coupling
    /// channel's <c>ics_info</c>.
    /// </param>
    /// <returns>
    /// A <see langword="new"/> <c>int[windowGroupCount, maxSfb]</c>
    /// matrix of absolute gain indices.
    /// </returns>
    /// <exception cref="ArgumentNullException">
    /// Any required argument is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="windowGroupCount"/> &lt; 1 or
    /// <paramref name="maxSfb"/> &lt; 0.
    /// </exception>
    /// <exception cref="InvalidOperationException">
    /// The gain list's DPCM entry count or scan order does not
    /// match <paramref name="couplingSectionData"/>'s non-ZERO_HCB
    /// scan order.
    /// </exception>
    public static int[,] ExpandToIndices(
        AacCouplingGainList gainList,
        AacSectionData couplingSectionData,
        int windowGroupCount,
        int maxSfb)
    {
        ArgumentNullException.ThrowIfNull(gainList);
        ArgumentNullException.ThrowIfNull(couplingSectionData);
        ArgumentOutOfRangeException.ThrowIfLessThan(windowGroupCount, 1);
        ArgumentOutOfRangeException.ThrowIfNegative(maxSfb);

        var indices = new int[windowGroupCount, maxSfb];

        if (gainList.CommonGainElementPresent)
        {
            int common = gainList.CommonGainDifferential
                ?? throw new InvalidOperationException(
                    "Gain list has CommonGainElementPresent = true but no CommonGainDifferential value.");

            for (int g = 0; g < windowGroupCount; g++)
            {
                for (int s = 0; s < maxSfb; s++)
                {
                    indices[g, s] = common;
                }
            }
            return indices;
        }

        int running = 0;
        int dpcmIdx = 0;
        var dpcm = gainList.DpcmGains;

        foreach (var section in couplingSectionData.Sections)
        {
            for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
            {
                if (section.Group >= windowGroupCount || sfb >= maxSfb)
                {
                    throw new InvalidOperationException(
                        $"Section ({section.Group}, [{section.StartSfb}, {section.EndSfb})) overruns the (windowGroupCount, maxSfb) = ({windowGroupCount}, {maxSfb}) grid.");
                }

                if (section.CodebookNumber == 0)
                {
                    indices[section.Group, sfb] = running;
                    continue;
                }

                if (dpcmIdx >= dpcm.Count)
                {
                    throw new InvalidOperationException(
                        $"Coupling gain list has only {dpcm.Count} DPCM entries; target section data requires more.");
                }

                var entry = dpcm[dpcmIdx++];
                if (entry.Group != section.Group || entry.Sfb != sfb)
                {
                    throw new InvalidOperationException(
                        $"Coupling gain DPCM entry #{dpcmIdx - 1} mismatch: expected (g={section.Group}, sfb={sfb}), got (g={entry.Group}, sfb={entry.Sfb}).");
                }

                running += entry.Differential;
                indices[section.Group, sfb] = running;
            }
        }

        if (dpcmIdx != dpcm.Count)
        {
            throw new InvalidOperationException(
                $"Coupling gain list has {dpcm.Count} DPCM entries but only {dpcmIdx} matched the section scan order.");
        }

        return indices;
    }
}
