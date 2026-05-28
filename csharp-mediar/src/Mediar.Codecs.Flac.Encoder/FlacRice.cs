using Mediar.IO;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Rice / Rice2 residual coder for FLAC (RFC 9639 §10.3.5) with multi-partition support.
/// </summary>
/// <remarks>
/// Residuals are folded to unsigned via zigzag (<c>folded = (v &lt;&lt; 1) ^ (v &gt;&gt; 31)</c>)
/// and split as <c>folded = (q &lt;&lt; k) | r</c>; the quotient <c>q</c> is unary-coded
/// (q zero bits + one stop bit) and the remainder <c>r</c> is written in <c>k</c> LSB-first bits.
///
/// The block of residuals is partitioned into <c>2^partition_order</c> equal-sized regions and
/// each region picks its own Rice parameter <c>k</c>. The whole subframe shares one method
/// (0 = 4-bit k field per partition with k ∈ [0,14]; 1 = 5-bit k with k ∈ [0,30]). The first
/// partition holds <c>partitionSize - predictorOrder</c> residuals (the first
/// <c>predictorOrder</c> samples in the block are warm-up, not residuals); every subsequent
/// partition holds <c>partitionSize</c> residuals.
///
/// The escape paths (k = 15 / 31, raw signed binary) are not emitted by this floor encoder.
/// </remarks>
internal static class FlacRice
{
    /// <summary>Largest Rice parameter the Method-0 4-bit field can carry.</summary>
    public const int MaxKMethod0 = 14;

    /// <summary>Largest Rice parameter the Method-1 5-bit field can carry.</summary>
    public const int MaxKMethod1 = 30;

    /// <summary>Hard upper bound on the partition_order field (4 bits in the wire format).</summary>
    public const int MaxPartitionOrder = 8;

    /// <summary>Default per-subframe partition_order ceiling (matches libFLAC compression level 5).</summary>
    public const int DefaultMaxPartitionOrder = 6;

    /// <summary>
    /// Pick the cheapest (partition_order, method, per-partition k[]) for the residual sequence.
    /// Sweeps <c>partition_order ∈ [0, maxPartitionOrder]</c> and both methods, skipping any
    /// candidate whose layout would split the block unevenly or leave partition 0 with no
    /// residuals.
    /// </summary>
    /// <param name="residuals">
    /// Residual values, length <c>blockSize - predictorOrder</c>. Partition 0 owns the first
    /// <c>(blockSize / 2^partition_order) - predictorOrder</c> residuals; subsequent partitions
    /// each own <c>blockSize / 2^partition_order</c> residuals.
    /// </param>
    /// <param name="predictorOrder">Number of warmup samples consumed before the residual stream begins.</param>
    /// <param name="blockSize">Total samples in the channel block (residuals.Length + predictorOrder).</param>
    /// <param name="maxPartitionOrder">Inclusive upper bound on partition_order. Clamped to <see cref="MaxPartitionOrder"/>.</param>
    /// <param name="ksWorkspace">
    /// Output buffer; on success, the first <c>1 &lt;&lt; partitionOrder</c> entries hold the
    /// per-partition Rice parameters. Must be at least <c>1 &lt;&lt; maxPartitionOrder</c> long.
    /// </param>
    /// <param name="partitionOrder">Winning partition order (0..maxPartitionOrder).</param>
    /// <param name="method">Winning Rice method (0 or 1).</param>
    /// <param name="totalBits">Total residual-coding bit cost (header + body) of the winner.</param>
    /// <returns><c>true</c> when at least one (partition_order, method) candidate fits.</returns>
    public static bool TryChooseBestPartitioning(
        ReadOnlySpan<int> residuals,
        int predictorOrder,
        int blockSize,
        int maxPartitionOrder,
        Span<int> ksWorkspace,
        out int partitionOrder,
        out int method,
        out long totalBits)
    {
        partitionOrder = 0;
        method = 0;
        totalBits = 0;

        if (maxPartitionOrder < 0) maxPartitionOrder = 0;
        if (maxPartitionOrder > MaxPartitionOrder) maxPartitionOrder = MaxPartitionOrder;

        // Empty-residual fast path: only partition_order = 0 is legal and the body is empty.
        if (residuals.Length == 0)
        {
            if (ksWorkspace.Length == 0) return false;
            ksWorkspace[0] = 0;
            partitionOrder = 0;
            method = 0;
            totalBits = 2 + 4 + 4; // method + partition_order + k field
            return true;
        }

        Span<int> candidateKs = stackalloc int[1 << MaxPartitionOrder];

        long bestTotal = long.MaxValue;
        int bestPo = -1;
        int bestMethod = -1;

        for (int po = 0; po <= maxPartitionOrder; po++)
        {
            int numPartitions = 1 << po;
            if ((blockSize & (numPartitions - 1)) != 0) continue;
            int partitionSize = blockSize >> po;
            // Partition 0 carries partitionSize - predictorOrder residuals; need at least one.
            if (partitionSize <= predictorOrder) continue;

            for (int m = 0; m <= 1; m++)
            {
                int maxK = m == 0 ? MaxKMethod0 : MaxKMethod1;
                int kBits = m == 0 ? 4 : 5;
                long headerBits = 2L + 4L + (long)numPartitions * kBits;
                long body = 0;
                int residualIdx = 0;
                bool ok = true;

                for (int p = 0; p < numPartitions; p++)
                {
                    int count = p == 0 ? partitionSize - predictorOrder : partitionSize;
                    ReadOnlySpan<int> slice = residuals.Slice(residualIdx, count);
                    int chosenK = ChoosePartitionK(slice, maxK, out long partitionBits);
                    if (chosenK < 0) { ok = false; break; }
                    candidateKs[p] = chosenK;
                    body += partitionBits;
                    residualIdx += count;
                }

                if (!ok) continue;
                long total = headerBits + body;
                if (total < bestTotal)
                {
                    bestTotal = total;
                    bestPo = po;
                    bestMethod = m;
                    candidateKs[..numPartitions].CopyTo(ksWorkspace);
                }
            }
        }

        if (bestPo < 0) return false;

        partitionOrder = bestPo;
        method = bestMethod;
        totalBits = bestTotal;
        return true;
    }

    /// <summary>
    /// Emit the residual-coding section: 2-bit method + 4-bit partition_order
    /// + (per partition) k field + Rice-coded residuals. The partition layout
    /// is fully determined by <paramref name="blockSize"/>, <paramref name="predictorOrder"/>
    /// and <paramref name="partitionOrder"/> and must match the spec layout.
    /// </summary>
    public static void WritePartitions(
        ref BitWriter bw,
        ReadOnlySpan<int> residuals,
        int predictorOrder,
        int blockSize,
        int partitionOrder,
        int method,
        ReadOnlySpan<int> ks)
    {
        int numPartitions = 1 << partitionOrder;
        int partitionSize = blockSize >> partitionOrder;
        int kBits = method == 0 ? 4 : 5;

        bw.WriteBits((uint)method, 2);
        bw.WriteBits((uint)partitionOrder, 4);

        int residualIdx = 0;
        for (int p = 0; p < numPartitions; p++)
        {
            int count = p == 0 ? partitionSize - predictorOrder : partitionSize;
            int k = ks[p];
            bw.WriteBits((uint)k, kBits);

            for (int i = 0; i < count; i++)
            {
                int v = residuals[residualIdx + i];
                uint folded = (uint)((v << 1) ^ (v >> 31));
                uint q = folded >> k;

                while (q >= 32)
                {
                    bw.WriteBits(0u, 32);
                    q -= 32;
                }
                if (q > 0) bw.WriteBits(0u, (int)q);
                bw.WriteBit(true);

                if (k > 0)
                {
                    uint remainder = folded & ((1u << k) - 1u);
                    bw.WriteBits(remainder, k);
                }
            }
            residualIdx += count;
        }
    }

    /// <summary>
    /// Pick the Rice parameter <c>k ∈ [0, maxK]</c> minimising the per-residual
    /// body bit cost for the given partition. Returns the chosen <c>k</c> and
    /// the corresponding <c>bits</c> (the per-residual unary + stop + remainder
    /// cost; partition-header bits are added by the caller).
    /// </summary>
    private static int ChoosePartitionK(ReadOnlySpan<int> residuals, int maxK, out long bits)
    {
        int n = residuals.Length;
        if (n == 0)
        {
            bits = 0;
            return 0;
        }

        Span<long> perK = stackalloc long[MaxKMethod1 + 1];
        for (int k = 0; k <= maxK; k++) perK[k] = (long)n * (k + 1);

        for (int i = 0; i < n; i++)
        {
            int v = residuals[i];
            uint z = (uint)((v << 1) ^ (v >> 31));
            for (int k = 0; k <= maxK; k++) perK[k] += (long)(z >> k);
        }

        int bestK = 0;
        long best = perK[0];
        for (int k = 1; k <= maxK; k++)
        {
            if (perK[k] < best)
            {
                best = perK[k];
                bestK = k;
            }
        }

        bits = best;
        return bestK;
    }
}
