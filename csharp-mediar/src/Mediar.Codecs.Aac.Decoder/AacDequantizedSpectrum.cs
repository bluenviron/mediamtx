using System.Collections.Immutable;
using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Dequantized AAC channel spectrum: 1024 floating-point MDCT
/// coefficients after inverse-quantization
/// (<see cref="AacInverseQuantization"/>) and per-band
/// scale-factor gain (<see cref="AacScaleFactorGain"/>) have been
/// applied to a single <see cref="AacChannelFrame"/>.
/// </summary>
/// <remarks>
/// <para>
/// This integrates the structural side of the decoder: it takes a
/// frame as parsed by the raw_data_block dispatcher (or any of the
/// "full" element wrappers) and produces the spectrum that the
/// joint-stereo / PNS / intensity / TNS / MDCT chain operates on.
/// </para>
/// <para>
/// PNS bands (codebook 13) and intensity-stereo bands (codebooks 14
/// and 15) leave the corresponding coefficient slots at zero - they
/// will be filled in by their respective synthesis stages once those
/// land.
/// </para>
/// </remarks>
public sealed record AacDequantizedSpectrum
{
    /// <summary>Length of the dequantized spectrum (always 1024).</summary>
    public const int TransformLength = AacSpectralData.TransformLength;

    /// <summary>
    /// 1024 dequantized MDCT coefficients in spec layout: for long
    /// windows, samples 0..1023 of the single window; for grouped
    /// EIGHT_SHORT, group-major / SFB-window-interleaved per the
    /// AAC §4.6.2 ordering inherited from <see cref="AacSpectralData"/>.
    /// </summary>
    public required ImmutableArray<float> Coefficients { get; init; }

    /// <summary>
    /// Dequantize <paramref name="frame"/> using the source
    /// <paramref name="sampleRate"/> to select the SWB offset table.
    /// </summary>
    /// <param name="frame">
    /// Parsed channel frame (<see cref="AacChannelFrame"/>).
    /// </param>
    /// <param name="sampleRate">
    /// Source sample rate (Hz) - the AAC-LC core rate, even for HE-AAC
    /// streams where the SBR layer reports a doubled rate.
    /// </param>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="frame"/> is <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static AacDequantizedSpectrum FromFrame(AacChannelFrame frame, int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(frame);

        var ics = frame.Stream.IcsInfo;
        var sections = frame.Stream.SectionData.Sections;
        var sfData = frame.Stream.ScaleFactorData;
        int globalGain = frame.Stream.GlobalGain;
        var sourceCoefs = frame.SpectralData.Coefficients;

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

        var output = new float[TransformLength];
        AacInverseQuantization.Dequantize(sourceCoefs.AsSpan(), output);

        var absSfs = AacAbsoluteScaleFactors.FromDelta(sfData, globalGain);

        int maxGroup = ics.WindowGroupCount;
        int maxSfb = swbOffsets.Length - 1;
        var sfLookup = new int?[maxGroup, maxSfb];
        foreach (var entry in absSfs.Entries)
        {
            if (entry.Kind != AacScaleFactorKind.SpectralGain) continue;
            if ((uint)entry.Group >= (uint)maxGroup) continue;
            if ((uint)entry.Sfb >= (uint)maxSfb) continue;
            sfLookup[entry.Group, entry.Sfb] = entry.Value;
        }

        ReadOnlySpan<byte> windowsPerGroup = ics.WindowsPerGroup.Span;
        Span<int> groupBases = stackalloc int[maxGroup];
        int runningBase = 0;
        for (int g = 0; g < maxGroup; g++)
        {
            groupBases[g] = runningBase;
            int groupSize = isShort
                ? AacSwbOffsets.ShortTransformLength * windowsPerGroup[g]
                : TransformLength;
            runningBase += groupSize;
        }

        foreach (var section in sections)
        {
            int cb = section.CodebookNumber;
            if (cb == AacSpectralCodebookSentinels.ZeroHcb
                || cb == AacSpectralCodebookSentinels.NoiseHcb
                || cb == AacSpectralCodebookSentinels.IntensityHcb2
                || cb == AacSpectralCodebookSentinels.IntensityHcb)
            {
                continue;
            }

            int g = section.Group;
            if ((uint)g >= (uint)maxGroup) continue;

            int windowsInGroup = isShort ? windowsPerGroup[g] : 1;

            for (int sfb = section.StartSfb; sfb < section.EndSfb; sfb++)
            {
                if ((uint)sfb >= (uint)maxSfb) break;
                int? sf = sfLookup[g, sfb];
                if (sf is not int value) continue;

                int bandStart = groupBases[g] + swbOffsets[sfb] * windowsInGroup;
                int bandWidth = (swbOffsets[sfb + 1] - swbOffsets[sfb]) * windowsInGroup;
                if (bandStart < 0 || bandStart + bandWidth > TransformLength) continue;

                AacScaleFactorGain.ApplyTo(output.AsSpan(bandStart, bandWidth), value);
            }
        }

        return new AacDequantizedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(output),
        };
    }
}
