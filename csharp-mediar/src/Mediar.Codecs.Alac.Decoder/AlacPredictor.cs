namespace Mediar.Codecs.Alac.Decoder;

/// <summary>
/// ALAC adaptive FIR predictor (Apple's <c>unpc_block</c>). Reconstructs
/// PCM samples from a residual stream using an adaptive linear-prediction
/// scheme where predictions are made in the "delta from base sample"
/// domain (the base is the sample <c>numCoeffs+1</c> positions back). FIR
/// coefficients are updated after each sample by a sign-and-1 rule driven
/// by the residual sign and the per-tap delta magnitude.
/// </summary>
/// <remarks>
/// Clean-room implementation of the algorithm in Apple's ALAC Apache-2.0
/// reference (codec/dp_dec.c, <c>unpc_block</c>). The implementation
/// keeps the general (non-specialised) code path; SIMD / specialised
/// numactive==4 / numactive==8 paths are left as TODOs (see README).
/// </remarks>
internal static class AlacPredictor
{
    /// <summary>
    /// Reconstruct <paramref name="numSamples"/> samples from the residual
    /// stream <paramref name="residuals"/> into <paramref name="output"/>.
    /// </summary>
    /// <param name="residuals">Input residuals (length &gt;= <paramref name="numSamples"/>).</param>
    /// <param name="output">Output samples (length &gt;= <paramref name="numSamples"/>).
    /// <paramref name="residuals"/> and <paramref name="output"/> may alias.</param>
    /// <param name="numSamples">Number of samples to produce.</param>
    /// <param name="coeffs">Adaptive FIR coefficients (mutated in place).</param>
    /// <param name="numCoeffs">Number of active taps. <c>0</c> means "no
    /// predictor — residuals are samples". <c>31</c> means "identity
    /// cumulative sum" (special marker used for the first pass when
    /// <c>mode != 0</c>).</param>
    /// <param name="chanBits">Working bit width for sign extension.</param>
    /// <param name="denShift">Right-shift applied to the FIR sum to recover
    /// the prediction (Apple's <c>denshift</c>).</param>
    public static void Unpc(
        ReadOnlySpan<int> residuals,
        Span<int> output,
        int numSamples,
        Span<short> coeffs,
        int numCoeffs,
        int chanBits,
        int denShift)
    {
        if (residuals.Length < numSamples)
            throw new ArgumentException("residuals buffer too small.", nameof(residuals));
        if (output.Length < numSamples)
            throw new ArgumentException("output buffer too small.", nameof(output));

        int chanShift = 32 - chanBits;

        output[0] = residuals[0];

        if (numCoeffs == 0)
        {
            // No predictor at all — residuals are samples verbatim.
            for (int j = 1; j < numSamples; j++) output[j] = residuals[j];
            return;
        }

        if (numCoeffs == 31)
        {
            // "31" is Apple's identity-sum marker used by the first pass when
            // mode != 0. Cumulative sum with per-step sign extension.
            int prev = output[0];
            for (int j = 1; j < numSamples; j++)
            {
                int del = residuals[j] + prev;
                prev = (del << chanShift) >> chanShift;
                output[j] = prev;
            }
            return;
        }

        // Warm-up: the first numCoeffs samples after sample 0 are decoded as
        // straight cumulative differences from the previous sample.
        for (int j = 1; j <= numCoeffs; j++)
        {
            int del = residuals[j] + output[j - 1];
            output[j] = (del << chanShift) >> chanShift;
        }

        int denHalf = 1 << (denShift - 1);
        int lim = numCoeffs + 1;

        // Adaptive FIR loop. Apple stores the most recent sample at coeffs[0]
        // and the oldest active sample at coeffs[numCoeffs-1] (i.e., the
        // dot product is over out[j-1], out[j-2], ..., out[j-numCoeffs]).
        for (int j = lim; j < numSamples; j++)
        {
            int top = output[j - lim];
            int sum1 = 0;
            for (int k = 0; k < numCoeffs; k++)
            {
                sum1 += coeffs[k] * (output[j - 1 - k] - top);
            }
            int del = residuals[j];
            int del0 = del;
            int sg = Sign(del);
            int prediction = top + ((sum1 + denHalf) >> denShift);
            int outVal = del + prediction;
            output[j] = (outVal << chanShift) >> chanShift;

            // Adaptive coefficient update. Walk the taps newest-first; stop
            // as soon as the running error count crosses zero.
            if (sg > 0)
            {
                for (int k = numCoeffs - 1; k >= 0; k--)
                {
                    int dd = top - output[j - 1 - k];
                    int sgn = Sign(dd);
                    coeffs[k] = (short)(coeffs[k] - sgn);
                    del0 -= (numCoeffs - k) * ((sgn * dd) >> denShift);
                    if (del0 <= 0) break;
                }
            }
            else if (sg < 0)
            {
                for (int k = numCoeffs - 1; k >= 0; k--)
                {
                    int dd = top - output[j - 1 - k];
                    int sgn = Sign(dd);
                    coeffs[k] = (short)(coeffs[k] + sgn);
                    del0 -= (numCoeffs - k) * (((-sgn) * dd) >> denShift);
                    if (del0 >= 0) break;
                }
            }
        }
    }

    // Matches Apple's `sign_of_int`: -1, 0, +1.
    private static int Sign(int x) => (x >> 31) | (int)((uint)-x >> 31);
}
