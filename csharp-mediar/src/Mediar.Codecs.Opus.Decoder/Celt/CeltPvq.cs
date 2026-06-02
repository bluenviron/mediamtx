namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Pyramid Vector Quantizer (PVQ) shape decoder for the CELT layer
/// (Phase 2c.3b.2). Ports the SMALL_FOOTPRINT variant of libopus
/// <c>celt/cwrs.c</c> — a recurrence-based combinatorial decoder that
/// converts a single uniform-distributed range-coder integer back into
/// a vector of <c>N</c> signed pulses summing in absolute value to
/// <c>K</c>.
/// </summary>
/// <remarks>
/// The small-footprint variant avoids the 1272-entry <c>CELT_PVQ_U</c>
/// static table by recomputing the needed row of U(n,k) on the fly via
/// the recurrence <c>U(n,k) = U(n-1,k) + U(n,k-1) + U(n-1,k-1)</c>.
/// Allocation is O(k) on the stack — trivially safe for the CELT max of
/// <see cref="CeltPvqMath.CeltMaxPulses"/>=128 pulses.
/// </remarks>
internal static class CeltPvq
{
    /// <summary>
    /// Forward recurrence step: replaces row U(n,·) with row U(n+1,·)
    /// in place. <paramref name="u0"/> is the base case for column 0 of
    /// the new row.
    /// </summary>
    private static void Unext(Span<uint> u, uint u0)
    {
        if (u.Length == 0) return;
        for (int j = 1; j < u.Length; j++)
        {
            uint ui1 = unchecked(u[j] + u[j - 1] + u0);
            u[j - 1] = u0;
            u0 = ui1;
        }
        u[u.Length - 1] = u0;
    }

    /// <summary>
    /// Inverse of <see cref="Unext"/>: walks the recurrence backwards
    /// so the decoder can shrink its working row as pulses are placed.
    /// </summary>
    private static void Uprev(Span<uint> u, uint u0)
    {
        if (u.Length == 0) return;
        for (int j = 1; j < u.Length; j++)
        {
            uint ui1 = unchecked(u[j] - u[j - 1] - u0);
            u[j - 1] = u0;
            u0 = ui1;
        }
        u[u.Length - 1] = u0;
    }

    /// <summary>
    /// Computes V(n,k) — the number of PVQ codewords for a band of size
    /// <paramref name="n"/> with <paramref name="k"/> pulses — while
    /// also filling <paramref name="u"/> with row n of U(0..k+1) so the
    /// decoder can walk it in <see cref="Cwrsi"/>. <paramref name="u"/>
    /// must have length ≥ k+2.
    /// </summary>
    internal static uint NcwrsUrow(int n, int k, Span<uint> u)
    {
        if (n < 2) throw new ArgumentOutOfRangeException(nameof(n), "PVQ requires n >= 2");
        if (k < 1) throw new ArgumentOutOfRangeException(nameof(k), "PVQ requires k >= 1");
        if (u.Length < k + 2) throw new ArgumentException("u must have length >= k+2", nameof(u));

        u[0] = 0;
        u[1] = 1;
        for (int kk = 2; kk < k + 2; kk++)
            u[kk] = (uint)((kk << 1) - 1);
        for (int nn = 2; nn < n; nn++)
            Unext(u.Slice(1, k + 1), 1);
        return unchecked(u[k] + u[k + 1]);
    }

    /// <summary>
    /// Convert a PVQ codeword index <paramref name="i"/> ∈ [0, V(n,k))
    /// back into a length-<paramref name="n"/> vector of signed pulses
    /// summing in absolute value to <paramref name="k"/>. <paramref
    /// name="u"/> must contain row n of U(·) on entry; it will be
    /// destructively modified. Returns the energy yy = Σ y[j]² of the
    /// decoded vector.
    /// </summary>
    internal static int Cwrsi(int n, int k, uint i, Span<int> y, Span<uint> u)
    {
        int yy = 0;
        for (int j = 0; j < n; j++)
        {
            uint p = u[k + 1];
            int s = i >= p ? -1 : 0;
            i = unchecked(i - (p & (uint)s));
            int yj = k;
            p = u[k];
            while (p > i) p = u[--k];
            i = unchecked(i - p);
            yj -= k;
            int val = (yj + s) ^ s;
            y[j] = val;
            yy += val * val;
            Uprev(u.Slice(0, k + 2), 0);
        }
        return yy;
    }

    /// <summary>
    /// PVQ shape decode driver — equivalent to libopus
    /// <c>decode_pulses(_y, _n, _k, _dec)</c>. Draws a uniform integer
    /// in [0, V(n,k)) from the range coder and converts it into the
    /// signed pulse vector. Returns the decoded energy
    /// <c>yy = Σ y[j]²</c>. <paramref name="y"/> must have length ≥ n.
    /// </summary>
    public static int DecodePulses(ref OpusRangeDecoder dec, int n, int k, Span<int> y)
    {
        if (y.Length < n) throw new ArgumentException("y must have length >= n", nameof(y));

        Span<uint> u = stackalloc uint[k + 2];
        uint nc = NcwrsUrow(n, k, u);
        uint idx = dec.DecodeUint(nc);
        return Cwrsi(n, k, idx, y.Slice(0, n), u);
    }

    /// <summary>
    /// Convenience helper exposed primarily for testing: decode a
    /// caller-supplied index instead of reading from the range coder.
    /// </summary>
    internal static int DecodePulsesAtIndex(int n, int k, uint index, Span<int> y)
    {
        if (y.Length < n) throw new ArgumentException("y must have length >= n", nameof(y));
        Span<uint> u = stackalloc uint[k + 2];
        uint nc = NcwrsUrow(n, k, u);
        if (index >= nc) throw new ArgumentOutOfRangeException(nameof(index), $"index must be < V(n,k) = {nc}");
        return Cwrsi(n, k, index, y.Slice(0, n), u);
    }

    /// <summary>
    /// Compute V(n,k) standalone (allocates k+2 uints on the stack).
    /// Useful for callers that just need the codebook size, e.g. when
    /// computing bit budgets without actually decoding pulses.
    /// </summary>
    internal static uint ComputeV(int n, int k)
    {
        Span<uint> u = stackalloc uint[k + 2];
        return NcwrsUrow(n, k, u);
    }
}
