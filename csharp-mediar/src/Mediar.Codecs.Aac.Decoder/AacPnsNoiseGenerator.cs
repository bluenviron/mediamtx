namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC perceptual-noise-substitution (PNS) coefficient synthesiser
/// per ISO/IEC 14496-3 §4.6.12. For a scale-factor band whose
/// section codebook is <c>cb = 13 (NOISE_HCB)</c>, the encoder has
/// stored a single <em>noise scale factor</em> (in the same Huffman
/// stream as ordinary scale factors) instead of <c>band_width</c>
/// quantised spectral coefficients. The decoder fabricates the
/// coefficients from a pseudo-random sequence and rescales them so
/// the band's total energy matches the target encoded in the noise
/// SF.
/// </summary>
/// <remarks>
/// <para>
/// Algorithm (matches the reference flow in ISO/IEC 14496-3 §4.6.12
/// and is byte-identical to libfaad's <c>gen_rand_vector</c>):
/// </para>
/// <list type="number">
///   <item>For each sample in the band, advance the PRNG and store
///         the signed-int32 sample reinterpreted as a float in
///         <c>[-1, 1)</c> (see <see cref="AacPnsRandom.NextFloat"/>).</item>
///   <item>Accumulate <c>E = Σ samples[i]²</c>.</item>
///   <item>If <c>E = 0</c> (theoretically impossible for the LCG
///         used, but defensively handled), leave the samples
///         untouched. Otherwise compute
///         <c>scale = 2^(noiseSf / 4) / √E</c>.</item>
///   <item>Multiply every sample by <c>scale</c>.</item>
/// </list>
/// <para>
/// The resulting band has total energy <c>2^(noiseSf / 2)</c>, which
/// is the energy convention used by every downstream stage of the
/// AAC decoder.
/// </para>
/// <para>
/// The <see cref="AacScaleFactorGain.SfOffset"/> constant of 100
/// does <strong>not</strong> apply here. PNS scale factors are
/// stored as their absolute (post-<c>global_gain</c>,
/// post-<c>noise_offset = 90</c>) values directly in the
/// scale-factor stream, and the energy formula uses them verbatim.
/// See <see cref="AacAbsoluteScaleFactors"/> for the parser side.
/// </para>
/// <para>
/// Stereo intensity-coupled PNS bands (where both channels share a
/// single PNS band but with opposite signs) can be handled by
/// generating the noise once and passing <c>negate: true</c> when
/// writing the coupled channel.
/// </para>
/// </remarks>
public static class AacPnsNoiseGenerator
{
    private const double Quarter = 0.25;

    /// <summary>
    /// Fill <paramref name="band"/> with pseudo-random samples
    /// normalised so that the band's total energy is
    /// <c>2^(noiseScaleFactor / 2)</c>, advancing
    /// <paramref name="prng"/> once per coefficient.
    /// </summary>
    /// <param name="band">
    /// Spectral coefficients of a single scale-factor band that has
    /// section codebook <c>cb = 13</c>. The contents on entry are
    /// ignored and overwritten.
    /// </param>
    /// <param name="noiseScaleFactor">
    /// The absolute noise scale factor produced by
    /// <see cref="AacAbsoluteScaleFactors"/> for this band (already
    /// includes the <c>noise_offset = 90</c> bias).
    /// </param>
    /// <param name="prng">
    /// Per-frame PNS pseudo-random generator. Advanced exactly
    /// <c>band.Length</c> times.
    /// </param>
    /// <param name="negate">
    /// If <c>true</c>, negates each generated coefficient. Used by
    /// intensity-coupled stereo PNS where the right channel mirrors
    /// the left channel's noise with opposite sign.
    /// </param>
    public static void FillBand(
        Span<float> band,
        int noiseScaleFactor,
        AacPnsRandom prng,
        bool negate = false)
    {
        ArgumentNullException.ThrowIfNull(prng);

        if (band.IsEmpty)
        {
            return;
        }

        double energy = 0.0;
        for (int i = 0; i < band.Length; i++)
        {
            float sample = prng.NextFloat();
            band[i] = sample;
            energy += (double)sample * sample;
        }

        if (energy <= 0.0)
        {
            return;
        }

        double amplitudeScale = Math.Pow(2.0, Quarter * noiseScaleFactor) / Math.Sqrt(energy);
        float scale = (float)(negate ? -amplitudeScale : amplitudeScale);

        for (int i = 0; i < band.Length; i++)
        {
            band[i] *= scale;
        }
    }

    /// <summary>
    /// Compute the target total energy
    /// (<c>Σ band[i]²</c>) that a PNS band of the given noise scale
    /// factor must achieve.
    /// </summary>
    public static double TargetBandEnergy(int noiseScaleFactor)
    {
        return Math.Pow(2.0, 0.5 * noiseScaleFactor);
    }
}
