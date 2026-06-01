using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Intensity-stereo synthesiser for AAC per ISO/IEC 14496-3 §4.6.8.2.3.
/// Walks the right channel's section data for codebooks <c>cb = 14</c>
/// (<see cref="AacSpectralCodebookSentinels.IntensityHcb2"/>, positive
/// polarity) and <c>cb = 15</c>
/// (<see cref="AacSpectralCodebookSentinels.IntensityHcb"/>, negative
/// polarity) and synthesises the right channel's spectral coefficients
/// from the left channel's by scaling
/// <c>right[k] = left[k] · sign · 0.5^(is_position / 4)</c>, where the
/// sign comes from the polarity (<c>+1</c> for cb=14, <c>-1</c> for
/// cb=15) XORed with the optional MS-mask flag for that band.
/// </summary>
/// <remarks>
/// <para>
/// Intensity-stereo bands are mutually exclusive with MS-stereo for
/// any given band: the MS decoder
/// (<see cref="AacMsStereoDecoder.Decode"/>) skips bands whose right
/// codebook is 14 or 15. For those same bands, the MS-mask flag (when
/// present) acts as a sign inversion rather than an MS sum/difference
/// rotation.
/// </para>
/// <para>
/// Like <see cref="AacPnsApplier"/>, this is a pure extension to the
/// dequantization pipeline: run it AFTER
/// <see cref="AacDequantizedSpectrum.FromFrame"/> on the right channel
/// (which leaves IS bands at zero) and AFTER MS-stereo decoding.
/// </para>
/// </remarks>
public static class AacIntensityStereoApplier
{
    /// <summary>
    /// Fill the intensity-stereo bands of <paramref name="right"/> in
    /// place from <paramref name="left"/>.
    /// </summary>
    /// <param name="left">
    /// Left channel's dequantized spectrum (read-only, used as IS
    /// source).
    /// </param>
    /// <param name="right">
    /// Right channel's dequantized spectrum (mutable, IS bands are
    /// overwritten).
    /// </param>
    /// <param name="rightFrame">
    /// Right channel's parsed frame, providing the section data,
    /// scale-factor data, global gain, and ICS info.
    /// </param>
    /// <param name="msMaskPresent">
    /// MS mask mode shared by the CPE; only
    /// <see cref="AacMsMaskPresent.PerBand"/> and
    /// <see cref="AacMsMaskPresent.AllBands"/> can flip the IS sign.
    /// </param>
    /// <param name="msUsed">
    /// Per-band ms_used flags from the CPE. Ignored when
    /// <paramref name="msMaskPresent"/> is
    /// <see cref="AacMsMaskPresent.None"/>.
    /// </param>
    /// <param name="sampleRate">Source sample rate in Hz.</param>
    public static void ApplyInPlace(
        ReadOnlySpan<float> left,
        Span<float> right,
        AacChannelFrame rightFrame,
        AacMsMaskPresent msMaskPresent,
        IReadOnlyList<IReadOnlyList<bool>> msUsed,
        int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(rightFrame);
        ArgumentNullException.ThrowIfNull(msUsed);

        if (left.Length != AacDequantizedSpectrum.TransformLength)
        {
            throw new ArgumentException(
                $"Left coefficient span must be {AacDequantizedSpectrum.TransformLength} elements.",
                nameof(left));
        }
        if (right.Length != AacDequantizedSpectrum.TransformLength)
        {
            throw new ArgumentException(
                $"Right coefficient span must be {AacDequantizedSpectrum.TransformLength} elements.",
                nameof(right));
        }
        if (msMaskPresent == AacMsMaskPresent.Reserved)
        {
            throw new ArgumentException(
                "ms_mask_present = 3 is reserved by the AAC specification.",
                nameof(msMaskPresent));
        }

        var ics = rightFrame.Stream.IcsInfo;
        var sections = rightFrame.Stream.SectionData.Sections;
        var sfData = rightFrame.Stream.ScaleFactorData;
        int globalGain = rightFrame.Stream.GlobalGain;

        bool isShort = ics.WindowSequence == AacWindowSequence.EightShort;
        ReadOnlySpan<int> swbOffsets = isShort
            ? AacSwbOffsets.GetShortOffsets(sampleRate)
            : AacSwbOffsets.GetLongOffsets(sampleRate);
        if (swbOffsets.IsEmpty)
        {
            throw new ArgumentException(
                $"Sample rate {sampleRate} Hz has no SWB offset table.",
                nameof(sampleRate));
        }

        int maxGroup = ics.WindowGroupCount;
        int maxSfb = swbOffsets.Length - 1;
        if (maxGroup == 0 || maxSfb == 0)
        {
            return;
        }

        if (msMaskPresent == AacMsMaskPresent.PerBand && msUsed.Count < maxGroup)
        {
            throw new ArgumentException(
                $"ms_used has {msUsed.Count} group rows; expected at least {maxGroup}.",
                nameof(msUsed));
        }

        var absSfs = AacAbsoluteScaleFactors.FromDelta(sfData, globalGain);
        var isPosLookup = new int?[maxGroup, maxSfb];
        foreach (var entry in absSfs.Entries)
        {
            if (entry.Kind != AacScaleFactorKind.IntensityPosition) continue;
            if ((uint)entry.Group >= (uint)maxGroup) continue;
            if ((uint)entry.Sfb >= (uint)maxSfb) continue;
            isPosLookup[entry.Group, entry.Sfb] = entry.Value;
        }

        ReadOnlySpan<byte> windowsPerGroup = ics.WindowsPerGroup.Span;
        Span<int> groupBases = stackalloc int[maxGroup];
        int runningBase = 0;
        for (int g = 0; g < maxGroup; g++)
        {
            groupBases[g] = runningBase;
            int groupSize = isShort
                ? AacSwbOffsets.ShortTransformLength * windowsPerGroup[g]
                : AacDequantizedSpectrum.TransformLength;
            runningBase += groupSize;
        }

        foreach (var section in sections)
        {
            int cb = section.CodebookNumber;
            bool isPositive = cb == AacSpectralCodebookSentinels.IntensityHcb2;  // cb = 14
            bool isNegative = cb == AacSpectralCodebookSentinels.IntensityHcb;   // cb = 15
            if (!isPositive && !isNegative) continue;

            int g = section.Group;
            if ((uint)g >= (uint)maxGroup) continue;
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

            for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
            {
                if ((uint)sfb >= (uint)maxSfb) break;

                int? sf = isPosLookup[g, sfb];
                if (sf is not int isPosition) continue;

                bool msFlipsSign = msMaskPresent switch
                {
                    AacMsMaskPresent.AllBands => true,
                    AacMsMaskPresent.PerBand => sfb < groupFlags!.Count && groupFlags[sfb],
                    _ => false,
                };

                // Polarity: cb=14 positive, cb=15 negative; MS flag flips.
                int sign = isNegative ? -1 : +1;
                if (msFlipsSign) sign = -sign;

                double scale = sign * Math.Pow(0.5, isPosition * 0.25);

                int bandStart = groupBases[g] + swbOffsets[sfb] * windowsInGroup;
                int bandWidth = (swbOffsets[sfb + 1] - swbOffsets[sfb]) * windowsInGroup;
                if (bandStart < 0 || bandStart + bandWidth > right.Length) continue;
                if (bandWidth == 0) continue;

                for (int k = 0; k < bandWidth; k++)
                {
                    right[bandStart + k] = (float)(left[bandStart + k] * scale);
                }
            }
        }
    }

    /// <summary>
    /// Return a copy of the right channel spectrum with intensity-
    /// stereo bands synthesised from <paramref name="left"/>.
    /// </summary>
    public static AacDequantizedSpectrum Apply(
        AacDequantizedSpectrum left,
        AacDequantizedSpectrum right,
        AacChannelFrame rightFrame,
        AacMsMaskPresent msMaskPresent,
        IReadOnlyList<IReadOnlyList<bool>> msUsed,
        int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(left);
        ArgumentNullException.ThrowIfNull(right);
        ArgumentNullException.ThrowIfNull(rightFrame);
        ArgumentNullException.ThrowIfNull(msUsed);

        var copy = right.Coefficients.ToArray();
        var leftArr = left.Coefficients.ToArray();
        ApplyInPlace(leftArr, copy.AsSpan(), rightFrame, msMaskPresent, msUsed, sampleRate);

        return new AacDequantizedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(copy),
        };
    }
}
