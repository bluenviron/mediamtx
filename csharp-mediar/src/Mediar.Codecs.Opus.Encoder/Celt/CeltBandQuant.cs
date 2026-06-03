using Mediar.Codecs.Opus.Decoder.Celt;

namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// Encoder-side per-band PVQ quantiser — mirror of the decoder's
/// <see cref="CeltBands.QuantBand"/>. Phase B2 v1 ships the
/// no-split / no-Haar / no-stereo-mid-side path that drives a single
/// <see cref="CeltPvqSearch.AlgQuant"/> call. The TF reshape, partition
/// split, stereo mid/side encode, and folding paths are left for the
/// follow-up B2.1 task — they're isolated behind the
/// <see cref="QuantBandSimple"/> entry point so the rest of the encoder
/// can be wired up against the simpler signature.
/// </summary>
internal static class CeltBandQuant
{
    /// <summary>
    /// Quantise a single mono band whose normalised spectrum lives in
    /// <paramref name="X"/>. Encodes the codeword through
    /// <paramref name="enc"/> and writes the reconstructed unit-norm
    /// shape back into <paramref name="X"/> (so that the encoder's
    /// running synthesis state stays in sync with what the decoder
    /// will reproduce).
    /// </summary>
    /// <param name="enc">Range encoder.</param>
    /// <param name="X">Normalised input shape (length ≥ N), overwritten
    /// with the reconstructed shape on return.</param>
    /// <param name="N">Band size in MDCT bins.</param>
    /// <param name="pulses">PVQ pulse budget K (&gt; 0).</param>
    /// <param name="spread">CELT spread mode (0..3).</param>
    /// <param name="blocks">MDCT block count for the band.</param>
    /// <param name="gain">Synthesis gain.</param>
    /// <param name="complexity">Encoder complexity 0..10.</param>
    /// <returns>Collapse mask for the band (same definition as the decoder).</returns>
    public static uint QuantBandSimple(
        ref OpusRangeEncoder enc,
        Span<float> X,
        int N,
        int pulses,
        int spread,
        int blocks,
        float gain,
        int complexity)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(pulses, 0);
        if (X.Length < N) throw new ArgumentException("X must hold at least N samples.", nameof(X));

        if (N == 1)
        {
            // libopus quant_band_n1 fast path: one sign bit + unit magnitude.
            uint sign = X[0] < 0 ? 1u : 0u;
            enc.EncodeBitLogP((int)sign, 1);
            X[0] = sign != 0 ? -gain : gain;
            return 1u;
        }

        return CeltPvqSearch.AlgQuant(X, N, pulses, spread, blocks, ref enc, gain, complexity);
    }
}
