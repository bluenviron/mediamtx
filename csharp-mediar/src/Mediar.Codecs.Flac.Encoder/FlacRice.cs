using Mediar.IO;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Rice / Rice2 residual coder for FLAC (RFC 9639 §10.3.5).
/// </summary>
/// <remarks>
/// Each residual is mapped to an unsigned "folded" value via classic zigzag
/// (<c>folded = (v &lt;&lt; 1) ^ (v &gt;&gt; 31)</c>) and then split as
/// <c>folded = (q &lt;&lt; k) | r</c> with the quotient <c>q</c> emitted in
/// unary (q zero bits + a single 1 stop bit) and the remainder <c>r</c> emitted
/// in <c>k</c> LSB-first bits. The Rice parameter <c>k</c> is shared across
/// the whole partition (we always emit a single-partition residual here).
/// Method 0 stores the parameter in 4 bits (k ∈ [0,14]; 15 = escape); method 1
/// in 5 bits (k ∈ [0,30]; 31 = escape). We use whichever has the smaller
/// header for the chosen <c>k</c>; the escape path is not emitted in this
/// floor encoder (we fall back to VERBATIM if k > 30 would be required, which
/// only happens for genuinely random / high-energy s32 input).
/// </remarks>
internal static class FlacRice
{
    /// <summary>Largest Rice parameter the Method-0 4-bit field can carry.</summary>
    public const int MaxKMethod0 = 14;

    /// <summary>Largest Rice parameter the Method-1 5-bit field can carry.</summary>
    public const int MaxKMethod1 = 30;

    /// <summary>Per-residual fixed overhead in bits for method 0 / method 1 partition headers.</summary>
    public const int HeaderBitsMethod0 = 2 + 4 + 4; // method + partition_order + Rice param

    /// <summary>Per-residual fixed overhead in bits for method 1.</summary>
    public const int HeaderBitsMethod1 = 2 + 4 + 5;

    /// <summary>
    /// Pick the Rice parameter <c>k</c> minimising total encoded body bits over
    /// the residual span. Method 0 is preferred when <c>k ≤ 14</c>; method 1
    /// is used for <c>k ∈ [15, 30]</c>. The returned <c>bits</c> includes the
    /// residual-coding partition header (2+4+4 or 2+4+5) plus the per-residual
    /// unary + stop + remainder bits.
    /// </summary>
    /// <returns>
    /// <c>(method, k, bits)</c>. If no k ∈ [0,30] yields a fitting cost the
    /// caller should fall back to VERBATIM; this method always returns SOME
    /// (method, k) and the caller is expected to compare cost.
    /// </returns>
    public static (int Method, int K, long Bits) ChooseParameter(ReadOnlySpan<int> residuals)
    {
        int n = residuals.Length;
        if (n == 0)
        {
            // Empty partition still pays the residual-coding header.
            return (0, 0, HeaderBitsMethod0);
        }

        Span<long> perK = stackalloc long[MaxKMethod1 + 1];
        for (int k = 0; k <= MaxKMethod1; k++)
        {
            perK[k] = (long)n * (k + 1);
        }

        for (int i = 0; i < n; i++)
        {
            int v = residuals[i];
            uint z = (uint)((v << 1) ^ (v >> 31));
            for (int k = 0; k <= MaxKMethod1; k++)
            {
                perK[k] += (long)(z >> k);
            }
        }

        int bestK = 0;
        long bestRice = perK[0];
        for (int k = 1; k <= MaxKMethod1; k++)
        {
            if (perK[k] < bestRice)
            {
                bestRice = perK[k];
                bestK = k;
            }
        }

        int method = bestK <= MaxKMethod0 ? 0 : 1;
        int headerBits = method == 0 ? HeaderBitsMethod0 : HeaderBitsMethod1;
        return (method, bestK, bestRice + headerBits);
    }

    /// <summary>
    /// Write the residual coding for a single partition (partition_order = 0)
    /// with the given <paramref name="method"/> and Rice parameter <paramref name="k"/>.
    /// Emits: 2-bit method + 4-bit partition_order (=0) + 4/5-bit k + N Rice codes.
    /// </summary>
    public static void WriteSinglePartition(
        ref BitWriter bw,
        ReadOnlySpan<int> residuals,
        int method,
        int k)
    {
        bw.WriteBits((uint)method, 2);
        bw.WriteBits(0u, 4); // partition_order = 0
        bw.WriteBits((uint)k, method == 0 ? 4 : 5);

        for (int i = 0; i < residuals.Length; i++)
        {
            int v = residuals[i];
            uint folded = (uint)((v << 1) ^ (v >> 31));
            uint q = folded >> k;

            // Unary: q zero bits, in chunks of 32.
            while (q >= 32)
            {
                bw.WriteBits(0u, 32);
                q -= 32;
            }
            if (q > 0) bw.WriteBits(0u, (int)q);

            bw.WriteBit(true); // stop bit

            if (k > 0)
            {
                uint remainder = folded & ((1u << k) - 1u);
                bw.WriteBits(remainder, k);
            }
        }
    }
}
