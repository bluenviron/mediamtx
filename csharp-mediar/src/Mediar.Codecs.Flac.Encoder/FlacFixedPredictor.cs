using Mediar.IO;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Fixed-prediction subframe coder for FLAC (RFC 9639 §10.3.3). Implements
/// orders 0..4 with Rice-coded residuals in a single partition.
/// </summary>
/// <remarks>
/// The decoder reconstructs each sample as
/// <c>samples[i] = residual[i] + Σ coeffs[k] · samples[i-1-k]</c> where the
/// coefficients are the constant rows of Pascal's triangle (alternating sign):
/// order 1 → {1}, order 2 → {2,-1}, order 3 → {3,-3,1}, order 4 → {4,-6,4,-1}.
/// The encoder therefore subtracts the same prediction from each sample to
/// produce the residuals. The first <c>order</c> samples are stored verbatim
/// ("warmup" samples) so the decoder can prime its history.
/// </remarks>
internal static class FlacFixedPredictor
{
    /// <summary>Maximum supported Fixed predictor order.</summary>
    public const int MaxOrder = 4;

    /// <summary>
    /// Per-channel scratch budget (in <c>int</c> elements) the caller must
    /// supply for residual / workspace buffers. Equal to <c>blockSize</c>.
    /// </summary>
    public static int ResidualBufferSize(int blockSize) => blockSize;

    /// <summary>
    /// Compute the residuals for the given Fixed predictor order. Returns
    /// <c>false</c> if any residual overflows the 32-bit signed range — the
    /// caller should fall back to VERBATIM or a lower order in that case.
    /// </summary>
    public static bool TryComputeResiduals(
        ReadOnlySpan<int> samples, int order, Span<int> residuals)
    {
        int blockSize = samples.Length;
        int n = blockSize - order;
        if (residuals.Length < n) return false;

        switch (order)
        {
            case 0:
                for (int i = 0; i < blockSize; i++) residuals[i] = samples[i];
                return true;
            case 1:
                for (int i = 1; i < blockSize; i++)
                {
                    long r = (long)samples[i] - samples[i - 1];
                    if (r > int.MaxValue || r < int.MinValue) return false;
                    residuals[i - 1] = (int)r;
                }
                return true;
            case 2:
                for (int i = 2; i < blockSize; i++)
                {
                    long r = (long)samples[i] - 2L * samples[i - 1] + samples[i - 2];
                    if (r > int.MaxValue || r < int.MinValue) return false;
                    residuals[i - 2] = (int)r;
                }
                return true;
            case 3:
                for (int i = 3; i < blockSize; i++)
                {
                    long r = (long)samples[i] - 3L * samples[i - 1] + 3L * samples[i - 2] - samples[i - 3];
                    if (r > int.MaxValue || r < int.MinValue) return false;
                    residuals[i - 3] = (int)r;
                }
                return true;
            case 4:
                for (int i = 4; i < blockSize; i++)
                {
                    long r = (long)samples[i] - 4L * samples[i - 1] + 6L * samples[i - 2]
                            - 4L * samples[i - 3] + samples[i - 4];
                    if (r > int.MaxValue || r < int.MinValue) return false;
                    residuals[i - 4] = (int)r;
                }
                return true;
            default:
                return false;
        }
    }

    /// <summary>
    /// Pick the best (order, residualMethod, riceK) for the given channel block
    /// and write the chosen FIXED subframe into <paramref name="bw"/>. Returns
    /// <c>true</c> on success. Returns <c>false</c> when the cheapest FIXED
    /// configuration is not strictly better than VERBATIM (e.g. because every
    /// order overflowed) — the caller is then expected to emit VERBATIM.
    /// </summary>
    public static bool TryEncodeBestSubframe(
        ref BitWriter bw,
        ReadOnlySpan<int> samples,
        int bps,
        Span<int> residualScratch,
        long verbatimBodyBits)
    {
        int blockSize = samples.Length;

        int bestOrder = -1;
        int bestMethod = 0;
        int bestK = 0;
        long bestBits = long.MaxValue;

        Span<int> tempResidual = residualScratch[..blockSize];

        for (int order = 0; order <= MaxOrder && order <= blockSize; order++)
        {
            int n = blockSize - order;
            long cost;
            int method = 0;
            int k = 0;

            if (n == 0)
            {
                // No residuals: just warmup + empty residual partition header.
                cost = 8L + (long)order * bps + FlacRice.HeaderBitsMethod0;
            }
            else
            {
                if (!TryComputeResiduals(samples, order, tempResidual[..n])) continue;
                (method, k, long residualBits) = FlacRice.ChooseParameter(tempResidual[..n]);
                cost = 8L + (long)order * bps + residualBits;
            }

            if (cost < bestBits)
            {
                bestBits = cost;
                bestOrder = order;
                bestMethod = method;
                bestK = k;
            }
        }

        if (bestOrder < 0 || bestBits >= verbatimBodyBits) return false;

        // Recompute residuals for the winning order.
        int winN = blockSize - bestOrder;
        if (winN > 0)
        {
            bool ok = TryComputeResiduals(samples, bestOrder, tempResidual[..winN]);
            if (!ok) return false; // defensive — should not happen since we just succeeded above
        }

        // Subframe header byte: 0 (pad) | 001NNN (type) | 0 (wasted-bits flag).
        // type = 0b001000 | order → byte = ((0b001000 | order) << 1) & 0xFE.
        uint headerByte = (uint)((0b001000 | bestOrder) << 1);
        bw.WriteBits(headerByte, 8);

        // Warmup samples: bps-bit signed (two's complement) values.
        for (int i = 0; i < bestOrder; i++)
        {
            WriteSignedSample(ref bw, samples[i], bps);
        }

        FlacRice.WriteSinglePartition(ref bw, tempResidual[..winN], bestMethod, bestK);
        return true;
    }

    /// <summary>Emit <paramref name="value"/> as a <paramref name="bps"/>-bit two's-complement MSB-first signed integer.</summary>
    private static void WriteSignedSample(ref BitWriter bw, int value, int bps)
    {
        if (bps >= 32)
        {
            bw.WriteBits((uint)value, 32);
            return;
        }
        uint mask = (1u << bps) - 1u;
        bw.WriteBits((uint)value & mask, bps);
    }
}
