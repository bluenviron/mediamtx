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
    /// Estimate the cheapest FIXED predictor order for the given channel
    /// block using multi-partition Rice coding. Returns <c>true</c> if at
    /// least one order's total bit cost is strictly cheaper than
    /// <paramref name="maxBits"/>. The residual + ks workspaces are left in
    /// an undefined state on return — the caller must reproduce them via
    /// <see cref="WriteSubframe"/> using the returned <paramref name="order"/>.
    /// </summary>
    public static bool TryEstimateBest(
        ReadOnlySpan<int> samples,
        int bps,
        int blockSize,
        int maxPartitionOrder,
        Span<int> residualWorkspace,
        Span<int> ksWorkspace,
        long maxBits,
        out int order,
        out long bits)
    {
        order = -1;
        bits = maxBits;

        for (int o = 0; o <= MaxOrder && o <= blockSize; o++)
        {
            int n = blockSize - o;
            long cost;

            if (n == 0)
            {
                // Empty residual partition still costs the method + partition_order + k header.
                cost = 8L + (long)o * bps + (2 + 4 + 4);
            }
            else
            {
                if (!TryComputeResiduals(samples, o, residualWorkspace[..n])) continue;
                if (!FlacRice.TryChooseBestPartitioning(
                        residualWorkspace[..n], o, blockSize, maxPartitionOrder,
                        ksWorkspace, out _, out _, out long residualBits))
                {
                    continue;
                }
                cost = 8L + (long)o * bps + residualBits;
            }

            if (cost < bits)
            {
                bits = cost;
                order = o;
            }
        }

        return order >= 0;
    }

    /// <summary>
    /// Emit a FIXED subframe with the parameters from a prior
    /// <see cref="TryEstimateBest"/> call. Residuals are recomputed into
    /// <paramref name="residualScratch"/> and the partitioning is
    /// rediscovered (the per-order Rice sweep is cheap relative to the
    /// residual computation).
    /// </summary>
    public static void WriteSubframe(
        ref BitWriter bw,
        ReadOnlySpan<int> samples,
        int bps,
        int order,
        int blockSize,
        int maxPartitionOrder,
        Span<int> residualScratch,
        Span<int> ksScratch)
    {
        int n = blockSize - order;
        if (n > 0 && !TryComputeResiduals(samples, order, residualScratch[..n]))
        {
            throw new InvalidOperationException("FixedPredictor residual recomputation overflowed int32 during WriteSubframe.");
        }

        // Subframe header byte: 0 (pad) | 001NNN (type) | 0 (wasted-bits flag).
        uint headerByte = (uint)((0b001000 | order) << 1);
        bw.WriteBits(headerByte, 8);

        for (int i = 0; i < order; i++) WriteSignedSample(ref bw, samples[i], bps);

        if (!FlacRice.TryChooseBestPartitioning(
                residualScratch[..n], order, blockSize, maxPartitionOrder,
                ksScratch, out int partitionOrder, out int method, out _))
        {
            throw new InvalidOperationException("FixedPredictor failed to re-derive partitioning during WriteSubframe.");
        }
        FlacRice.WritePartitions(ref bw, residualScratch[..n], order, blockSize, partitionOrder, method, ksScratch);
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
