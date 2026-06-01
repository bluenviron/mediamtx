using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Mid/Side (M/S) joint-stereo decoder for an AAC
/// <c>channel_pair_element()</c> per ISO/IEC 14496-3 §4.6.8.1.
/// Operates on the dequantized MDCT spectra of the first (left /
/// "mid") and second (right / "side") channels and rewrites the
/// per-band coefficients in-place as
/// <c>L'[k] = M[k] + S[k]</c> and <c>R'[k] = M[k] - S[k]</c>.
/// </summary>
/// <remarks>
/// <para>
/// MS coding is only signalled inside a CPE with
/// <c>common_window = 1</c>. Whether a given scale-factor band is
/// MS-coded is selected by the <c>ms_mask_present</c> field and
/// (for the per-band mask case) the <c>ms_used[g][sfb]</c> bit array
/// parsed alongside it (see
/// <see cref="AacChannelPairElement.MsMaskPresent"/> /
/// <see cref="AacChannelPairElement.MsUsed"/>).
/// </para>
/// <para>
/// Intensity-stereo bands (right-channel codebook 14 / 15) are
/// skipped: the second-channel coefficients carry the IS position
/// rather than independent samples, so the sum/difference rewrite
/// would corrupt them. PNS bands (right-channel codebook 13) are
/// likewise skipped: the spec gives ms_used a different meaning
/// there (per-band noise correlation) which belongs to the PNS
/// synthesis stage rather than this transform.
/// </para>
/// </remarks>
public static class AacMsStereoDecoder
{
    /// <summary>
    /// Apply MS-stereo decoding to <paramref name="leftCoefs"/> and
    /// <paramref name="rightCoefs"/> in-place per ISO/IEC 14496-3
    /// §4.6.8.1.2. Both spans must be
    /// <see cref="AacSpectralData.TransformLength"/> elements long.
    /// </summary>
    /// <param name="leftCoefs">
    /// First (mid) channel dequantized coefficients; rewritten with
    /// the MS sum on bands where MS applies.
    /// </param>
    /// <param name="rightCoefs">
    /// Second (side) channel dequantized coefficients; rewritten
    /// with the MS difference on bands where MS applies.
    /// </param>
    /// <param name="sharedIcsInfo">
    /// Shared <c>ics_info()</c> for the CPE.
    /// </param>
    /// <param name="msMaskPresent">
    /// Value of <c>ms_mask_present</c>.
    /// </param>
    /// <param name="msUsed">
    /// Per-band <c>ms_used[g][sfb]</c> bit array; consulted only
    /// when <paramref name="msMaskPresent"/> is
    /// <see cref="AacMsMaskPresent.PerBand"/>. May be empty for
    /// the other mask values.
    /// </param>
    /// <param name="rightChannelSections">
    /// Section data for the second (right) channel — used to skip
    /// intensity-stereo and PNS bands.
    /// </param>
    /// <param name="sampleRate">
    /// Source sample rate (the AAC-LC core rate; halve the
    /// AudioSpecificConfig rate for HE-AAC streams).
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// Any of <paramref name="sharedIcsInfo"/>, <paramref name="msUsed"/>
    /// or <paramref name="rightChannelSections"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// Either span has the wrong length,
    /// <paramref name="sampleRate"/> has no SWB offset table, or
    /// <paramref name="msMaskPresent"/> is
    /// <see cref="AacMsMaskPresent.Reserved"/>.
    /// </exception>
    public static void Decode(
        Span<float> leftCoefs,
        Span<float> rightCoefs,
        AacIcsInfo sharedIcsInfo,
        AacMsMaskPresent msMaskPresent,
        IReadOnlyList<IReadOnlyList<bool>> msUsed,
        AacSectionData rightChannelSections,
        int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(sharedIcsInfo);
        ArgumentNullException.ThrowIfNull(msUsed);
        ArgumentNullException.ThrowIfNull(rightChannelSections);

        if (leftCoefs.Length != AacSpectralData.TransformLength)
        {
            throw new ArgumentException(
                $"Left coefficient span must be {AacSpectralData.TransformLength} elements.",
                nameof(leftCoefs));
        }
        if (rightCoefs.Length != AacSpectralData.TransformLength)
        {
            throw new ArgumentException(
                $"Right coefficient span must be {AacSpectralData.TransformLength} elements.",
                nameof(rightCoefs));
        }
        if (msMaskPresent == AacMsMaskPresent.Reserved)
        {
            throw new ArgumentException(
                "ms_mask_present = 3 is reserved by the AAC specification.",
                nameof(msMaskPresent));
        }
        if (msMaskPresent == AacMsMaskPresent.None) return;

        bool isShort = sharedIcsInfo.WindowSequence == AacWindowSequence.EightShort;
        ReadOnlySpan<int> swbOffsets = isShort
            ? AacSwbOffsets.GetShortOffsets(sampleRate)
            : AacSwbOffsets.GetLongOffsets(sampleRate);
        if (swbOffsets.IsEmpty)
        {
            throw new ArgumentException(
                $"Sample rate {sampleRate} Hz has no SWB offset table.",
                nameof(sampleRate));
        }

        int maxGroup = sharedIcsInfo.WindowGroupCount;
        int maxSfb = sharedIcsInfo.MaxSfb;
        if (maxSfb == 0 || maxGroup == 0) return;

        if (msMaskPresent == AacMsMaskPresent.PerBand && msUsed.Count < maxGroup)
        {
            throw new ArgumentException(
                $"ms_used has {msUsed.Count} group rows; expected at least {maxGroup}.",
                nameof(msUsed));
        }

        // Build a per-band lookup of the right-channel codebook so we
        // can skip intensity-stereo and PNS bands.
        var rightCbLookup = new int[maxGroup, maxSfb];
        for (int g = 0; g < maxGroup; g++)
        {
            for (int s = 0; s < maxSfb; s++)
            {
                rightCbLookup[g, s] = -1;
            }
        }
        foreach (var section in rightChannelSections.Sections)
        {
            int g = section.Group;
            if ((uint)g >= (uint)maxGroup) continue;
            int start = section.StartSfb;
            int end = section.EndSfb;
            if (end > maxSfb) end = maxSfb;
            for (int sfb = start; sfb < end; sfb++)
            {
                if ((uint)sfb >= (uint)maxSfb) break;
                rightCbLookup[g, sfb] = section.CodebookNumber;
            }
        }

        ReadOnlySpan<byte> windowsPerGroup = sharedIcsInfo.WindowsPerGroup.Span;
        Span<int> groupBases = stackalloc int[maxGroup];
        int runningBase = 0;
        for (int g = 0; g < maxGroup; g++)
        {
            groupBases[g] = runningBase;
            int groupSize = isShort
                ? AacSwbOffsets.ShortTransformLength * windowsPerGroup[g]
                : AacSpectralData.TransformLength;
            runningBase += groupSize;
        }

        int swbLast = swbOffsets.Length - 1;
        if (maxSfb > swbLast) maxSfb = swbLast;

        for (int g = 0; g < maxGroup; g++)
        {
            int windowsInGroup = isShort ? windowsPerGroup[g] : 1;
            IReadOnlyList<bool>? groupFlags = null;
            if (msMaskPresent == AacMsMaskPresent.PerBand)
            {
                groupFlags = msUsed[g];
                if (groupFlags is null)
                {
                    throw new ArgumentException(
                        $"ms_used[{g}] is null.",
                        nameof(msUsed));
                }
            }

            for (int sfb = 0; sfb < maxSfb; sfb++)
            {
                bool apply = msMaskPresent switch
                {
                    AacMsMaskPresent.AllBands => true,
                    AacMsMaskPresent.PerBand => sfb < groupFlags!.Count && groupFlags[sfb],
                    _ => false,
                };
                if (!apply) continue;

                int rightCb = rightCbLookup[g, sfb];
                if (rightCb == AacSpectralCodebookSentinels.NoiseHcb
                    || rightCb == AacSpectralCodebookSentinels.IntensityHcb
                    || rightCb == AacSpectralCodebookSentinels.IntensityHcb2)
                {
                    continue;
                }

                int bandStart = groupBases[g] + swbOffsets[sfb] * windowsInGroup;
                int bandWidth = (swbOffsets[sfb + 1] - swbOffsets[sfb]) * windowsInGroup;
                if (bandStart < 0
                    || bandWidth <= 0
                    || bandStart + bandWidth > leftCoefs.Length)
                {
                    continue;
                }

                ApplyMs(
                    leftCoefs.Slice(bandStart, bandWidth),
                    rightCoefs.Slice(bandStart, bandWidth));
            }
        }
    }

    /// <summary>
    /// Apply MS-stereo decoding to the dequantized spectra of a
    /// <see cref="AacChannelPairElement"/> with shared window and
    /// produce the resulting stereo pair. The original
    /// <paramref name="leftSpectrum"/> and
    /// <paramref name="rightSpectrum"/> are not mutated; new
    /// <see cref="AacDequantizedSpectrum"/> instances are returned.
    /// </summary>
    /// <param name="cpe">Source channel-pair element.</param>
    /// <param name="leftSpectrum">First-channel dequantized spectrum.</param>
    /// <param name="rightSpectrum">Second-channel dequantized spectrum.</param>
    /// <param name="sampleRate">Source AAC-LC core sample rate.</param>
    /// <returns>
    /// New dequantized spectra. When
    /// <see cref="AacChannelPairElement.MsMaskPresent"/> is
    /// <see cref="AacMsMaskPresent.None"/> the input spectra are
    /// returned unchanged.
    /// </returns>
    /// <exception cref="ArgumentNullException">Any required argument is <see langword="null"/>.</exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="cpe"/> has no common window / shared ICS, or
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static (AacDequantizedSpectrum Left, AacDequantizedSpectrum Right) DecodeFromCpe(
        AacChannelPairElement cpe,
        AacDequantizedSpectrum leftSpectrum,
        AacDequantizedSpectrum rightSpectrum,
        int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(cpe);
        ArgumentNullException.ThrowIfNull(leftSpectrum);
        ArgumentNullException.ThrowIfNull(rightSpectrum);

        if (!cpe.CommonWindow)
        {
            throw new ArgumentException(
                "MS stereo decoding requires common_window = 1.",
                nameof(cpe));
        }
        if (cpe.SharedIcsInfo is null)
        {
            throw new ArgumentException(
                "MS stereo decoding requires a shared ICS info.",
                nameof(cpe));
        }
        if (cpe.MsMaskPresent == AacMsMaskPresent.None)
        {
            return (leftSpectrum, rightSpectrum);
        }

        var leftBuf = leftSpectrum.Coefficients.ToArray();
        var rightBuf = rightSpectrum.Coefficients.ToArray();

        Decode(
            leftBuf,
            rightBuf,
            cpe.SharedIcsInfo,
            cpe.MsMaskPresent,
            cpe.MsUsed,
            cpe.SecondStream.SectionData,
            sampleRate);

        return (
            new AacDequantizedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(leftBuf),
            },
            new AacDequantizedSpectrum
            {
                Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(rightBuf),
            });
    }

    private static void ApplyMs(Span<float> left, Span<float> right)
    {
        for (int i = 0; i < left.Length; i++)
        {
            float m = left[i];
            float s = right[i];
            left[i] = m + s;
            right[i] = m - s;
        }
    }
}
