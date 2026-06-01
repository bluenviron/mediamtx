using System.Runtime.InteropServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Per-frame composer that chains the AAC mono / SCE / LFE channel
/// pipeline up to PNS synthesis:
/// <see cref="AacDequantizedSpectrum.FromFrame"/> ➜
/// <see cref="AacPnsApplier.ApplyInPlace"/>.
/// </summary>
/// <remarks>
/// <para>
/// This is the smallest useful composer in the AAC decoder: it owns
/// the buffer that flows out of dequantization, runs PNS synthesis
/// in place against it, and exposes the result as an immutable
/// <see cref="AacDecodedSpectrum"/>. The pipeline is correct for any
/// SCE / LFE / DSE channel; intensity-stereo (cb=14/15) and M/S
/// stereo apply only to a paired CPE and are NOT performed here.
/// </para>
/// <para>
/// TNS inverse filtering and the final IMDCT are intentionally left
/// to follow-up composers (long-window TNS needs a per-window-layout
/// spectrum, short-window TNS needs an additional deinterleave step,
/// and IMDCT is its own large piece).
/// </para>
/// </remarks>
public static class AacChannelDecoder
{
    /// <summary>
    /// Dequantize <paramref name="frame"/> for source rate
    /// <paramref name="sampleRate"/>, then fill PNS bands using
    /// <paramref name="prng"/>.
    /// </summary>
    /// <param name="frame">Parsed mono channel frame (SCE / LFE / CPE-half).</param>
    /// <param name="sampleRate">Source sample rate (Hz).</param>
    /// <param name="prng">
    /// Per-frame PRNG, advanced once per noise-band coefficient.
    /// </param>
    /// <returns>
    /// An <see cref="AacDecodedSpectrum"/> carrying the post-PNS MDCT
    /// coefficients in the same group-major / SFB-interleaved layout
    /// produced by <see cref="AacDequantizedSpectrum.FromFrame"/>.
    /// </returns>
    /// <exception cref="ArgumentNullException">
    /// <paramref name="frame"/> or <paramref name="prng"/> is
    /// <see langword="null"/>.
    /// </exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="sampleRate"/> has no SWB offset table.
    /// </exception>
    public static AacDecodedSpectrum DecodeMono(
        AacChannelFrame frame,
        int sampleRate,
        AacPnsRandom prng)
    {
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(prng);

        var dequant = AacDequantizedSpectrum.FromFrame(frame, sampleRate);
        var buffer = new float[AacDequantizedSpectrum.TransformLength];
        dequant.Coefficients.CopyTo(buffer);

        AacPnsApplier.ApplyInPlace(buffer, frame, sampleRate, prng);

        return new AacDecodedSpectrum
        {
            Coefficients = ImmutableCollectionsMarshal.AsImmutableArray(buffer),
            WindowSequence = frame.Stream.IcsInfo.WindowSequence,
        };
    }
}
