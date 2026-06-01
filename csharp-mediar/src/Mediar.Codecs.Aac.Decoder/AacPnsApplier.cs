using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// PNS coefficient synthesiser for AAC per ISO/IEC 14496-3 §4.6.12.
/// Walks the sections of an <see cref="AacChannelFrame"/>, finds every
/// scale-factor band with section codebook <c>cb = 13</c>
/// (<see cref="AacSpectralCodebookSentinels.NoiseHcb"/>), and writes
/// pseudo-random noise samples whose total band energy matches
/// <c>2^(noiseSf / 2)</c> using <see cref="AacPnsNoiseGenerator"/>.
/// </summary>
/// <remarks>
/// <para>
/// PNS bands are emitted in the same group-major / SFB-major order
/// that <see cref="AacDequantizedSpectrum.FromFrame"/> uses to walk
/// non-PNS sections, so the PRNG sequence is deterministic for a
/// given seed + frame.
/// </para>
/// <para>
/// The applier is a pure extension to the dequantization pipeline:
/// after <see cref="AacDequantizedSpectrum.FromFrame"/> has produced
/// a spectrum with PNS bands left at zero, calling
/// <see cref="Apply(AacDequantizedSpectrum, AacChannelFrame, int, AacPnsRandom)"/>
/// returns a new spectrum with PNS bands populated. Bands that hold
/// intensity-stereo codes (<c>cb = 14</c> / <c>15</c>) remain at zero
/// for now; they require <see cref="AacChannelPairElement"/> input
/// and ship as their own synthesis stage.
/// </para>
/// </remarks>
public static class AacPnsApplier
{
    /// <summary>
    /// Fill the PNS bands of <paramref name="spectrum"/> in place.
    /// </summary>
    /// <param name="spectrum">
    /// Mutable spectrum buffer (length
    /// <see cref="AacDequantizedSpectrum.TransformLength"/>). PNS
    /// bands are overwritten; non-PNS coefficients are left as-is.
    /// </param>
    /// <param name="frame">Parsed channel frame.</param>
    /// <param name="sampleRate">Source sample rate in Hz.</param>
    /// <param name="prng">
    /// Per-frame PRNG; advanced once per coefficient in every PNS
    /// band that is written.
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="frame"/> or <paramref name="prng"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="spectrum"/> is the wrong length, or the sample
    /// rate has no SWB table.
    /// </exception>
    public static void ApplyInPlace(
        Span<float> spectrum,
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng)
    {
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(prng);

        if (spectrum.Length != AacDequantizedSpectrum.TransformLength)
        {
            throw new ArgumentException(
                $"Spectrum span must be {AacDequantizedSpectrum.TransformLength} long, was {spectrum.Length}.",
                nameof(spectrum));
        }

        var ics = frame.Stream.IcsInfo;
        var sections = frame.Stream.SectionData.Sections;
        var sfData = frame.Stream.ScaleFactorData;
        int globalGain = frame.Stream.GlobalGain;

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

        var absSfs = AacAbsoluteScaleFactors.FromDelta(sfData, globalGain);
        var noiseSfLookup = new int?[maxGroup, maxSfb];
        foreach (var entry in absSfs.Entries)
        {
            if (entry.Kind != AacScaleFactorKind.NoiseEnergy) continue;
            if ((uint)entry.Group >= (uint)maxGroup) continue;
            if ((uint)entry.Sfb >= (uint)maxSfb) continue;
            noiseSfLookup[entry.Group, entry.Sfb] = entry.Value;
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
            if (section.CodebookNumber != AacSpectralCodebookSentinels.NoiseHcb)
            {
                continue;
            }

            int g = section.Group;
            if ((uint)g >= (uint)maxGroup) continue;
            int windowsInGroup = isShort ? windowsPerGroup[g] : 1;

            for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
            {
                if ((uint)sfb >= (uint)maxSfb) break;
                int? sf = noiseSfLookup[g, sfb];
                if (sf is not int noiseSf) continue;

                int bandStart = groupBases[g] + swbOffsets[sfb] * windowsInGroup;
                int bandWidth = (swbOffsets[sfb + 1] - swbOffsets[sfb]) * windowsInGroup;
                if (bandStart < 0 || bandStart + bandWidth > spectrum.Length) continue;
                if (bandWidth == 0) continue;

                AacPnsNoiseGenerator.FillBand(
                    spectrum.Slice(bandStart, bandWidth),
                    noiseSf,
                    prng);
            }
        }
    }

    /// <summary>
    /// Return a copy of <paramref name="input"/> with all PNS bands
    /// filled by noise generated from <paramref name="prng"/>.
    /// </summary>
    /// <exception cref="ArgumentNullException">
    /// Any argument is <see langword="null"/>.
    /// </exception>
    public static AacDequantizedSpectrum Apply(
        AacDequantizedSpectrum input,
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng)
    {
        ArgumentNullException.ThrowIfNull(input);
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(prng);

        var copy = input.Coefficients.ToArray();
        ApplyInPlace(copy.AsSpan(), frame, sampleRate, prng);

        return new AacDequantizedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(copy),
        };
    }
}
