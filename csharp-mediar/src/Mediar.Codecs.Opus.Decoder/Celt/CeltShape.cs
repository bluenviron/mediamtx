namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// CELT band shape primitives — float-build ports of the helpers
/// libopus uses below the recursive <c>quant_partition</c> /
/// <c>quant_band</c> state machine:
/// <c>haar1</c>, <c>deinterleave_hadamard</c>,
/// <c>interleave_hadamard</c>, <c>exp_rotation</c> (the spread Givens
/// rotation), <c>normalise_residual</c>, <c>extract_collapse_mask</c>,
/// and the leaf decoder <c>alg_unquant</c>. Phase 2c.3b.4.
/// </summary>
/// <remarks>
/// All routines operate on <c>celt_norm</c>; in our float build this
/// is <see cref="float"/>. Fixed-point shifts (<c>norm_scaledown</c>
/// / <c>norm_scaleup</c>, <c>PSHR32</c>, <c>SHR16</c>) collapse to
/// no-ops in libopus' float configuration, so the C# ports drop them
/// entirely.
/// </remarks>
public static class CeltShape
{
    /// <summary>
    /// Permutation table for "inverted Hadamard" ordering used by
    /// <see cref="DeinterleaveHadamard"/> / <see cref="InterleaveHadamard"/>.
    /// Lines are for N=2, N=4, N=8, N=16 — index with
    /// <c>orderyTable[stride - 2 + i]</c>.
    /// </summary>
    private static readonly int[] _orderyTable =
    {
         1, 0,
         3, 0,  2,  1,
         7, 0,  4,  3,  6,  1,  5,  2,
        15, 0,  8,  7, 12,  3, 11,  4, 14,  1,  9,  6, 13,  2, 10,  5,
    };

    private static readonly int[] _spreadFactor = { 15, 10, 5 };

    private const float Sqrt1Over2 = 0.70710678118654752440f;

    /// <summary>
    /// In-place single-level Haar transform on each of <paramref name="stride"/>
    /// interleaved length-<paramref name="N0"/> substreams. Splits adjacent
    /// pairs into sum/difference scaled by <c>1/sqrt(2)</c>.
    /// </summary>
    public static void Haar1(Span<float> X, int N0, int stride)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(stride, 1);
        int half = N0 >> 1;
        for (int i = 0; i < stride; i++)
        {
            for (int j = 0; j < half; j++)
            {
                int idx0 = stride * 2 * j + i;
                int idx1 = stride * (2 * j + 1) + i;
                float t1 = Sqrt1Over2 * X[idx0];
                float t2 = Sqrt1Over2 * X[idx1];
                X[idx0] = t1 + t2;
                X[idx1] = t1 - t2;
            }
        }
    }

    /// <summary>
    /// De-interleaves <paramref name="stride"/> substreams of length
    /// <paramref name="N0"/> into contiguous blocks. When
    /// <paramref name="hadamard"/> is true the substreams are reordered
    /// via the inverted Hadamard table (<see cref="_orderyTable"/>).
    /// </summary>
    public static void DeinterleaveHadamard(Span<float> X, int N0, int stride, bool hadamard)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(stride, 1);
        int n = N0 * stride;
        // Worst case N0*stride for CELT is 176*8 = 1408 floats ~ 5.5 KB —
        // heap-allocate when in doubt to keep the stack frame small.
        float[]? tmp = n <= 512 ? null : new float[n];
        System.Span<float> scratch = tmp ?? stackalloc float[n];
        if (hadamard)
        {
            int orderOffset = stride - 2;
            for (int i = 0; i < stride; i++)
            {
                int ord = _orderyTable[orderOffset + i];
                for (int j = 0; j < N0; j++)
                    scratch[ord * N0 + j] = X[j * stride + i];
            }
        }
        else
        {
            for (int i = 0; i < stride; i++)
                for (int j = 0; j < N0; j++)
                    scratch[i * N0 + j] = X[j * stride + i];
        }
        scratch.Slice(0, n).CopyTo(X.Slice(0, n));
    }

    /// <summary>Inverse of <see cref="DeinterleaveHadamard"/>.</summary>
    public static void InterleaveHadamard(Span<float> X, int N0, int stride, bool hadamard)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(stride, 1);
        int n = N0 * stride;
        float[]? tmp = n <= 512 ? null : new float[n];
        System.Span<float> scratch = tmp ?? stackalloc float[n];
        if (hadamard)
        {
            int orderOffset = stride - 2;
            for (int i = 0; i < stride; i++)
            {
                int ord = _orderyTable[orderOffset + i];
                for (int j = 0; j < N0; j++)
                    scratch[j * stride + i] = X[ord * N0 + j];
            }
        }
        else
        {
            for (int i = 0; i < stride; i++)
                for (int j = 0; j < N0; j++)
                    scratch[j * stride + i] = X[i * N0 + j];
        }
        scratch.Slice(0, n).CopyTo(X.Slice(0, n));
    }

    /// <summary>
    /// Pseudo-random Givens rotation that "spreads" the energy of a
    /// PVQ codeword across its dimension. Mirrors libopus
    /// <c>exp_rotation</c>; in the float build the operation is a pure
    /// cosine/sine rotation between adjacent samples (decoder uses
    /// <paramref name="dir"/>=-1 to undo what the encoder applied).
    /// </summary>
    public static void ExpRotation(Span<float> X, int len, int dir, int stride, int K, int spread)
    {
        if (2 * K >= len || spread == CeltConstants.SpreadNone) return;
        System.ArgumentOutOfRangeException.ThrowIfLessThan(spread, 1);
        System.ArgumentOutOfRangeException.ThrowIfGreaterThan(spread, 3);

        int factor = _spreadFactor[spread - 1];
        float gain = (float)len / (len + factor * K);
        float theta = 0.5f * gain * gain;
        // c = cos(π/2 · theta), s = cos(π/2 · (1 - theta)) = sin(π/2 · theta).
        float c = (float)System.Math.Cos(System.Math.PI * 0.5 * theta);
        float s = (float)System.Math.Sin(System.Math.PI * 0.5 * theta);

        int stride2 = 0;
        if (len >= 8 * stride)
        {
            stride2 = 1;
            while ((stride2 * stride2 + stride2) * stride + (stride >> 2) < len)
                stride2++;
        }
        int subLen = len / stride;
        for (int i = 0; i < stride; i++)
        {
            var sub = X.Slice(i * subLen, subLen);
            if (dir < 0)
            {
                if (stride2 != 0)
                    ExpRotation1(sub, subLen, stride2, s, c);
                ExpRotation1(sub, subLen, 1, c, s);
            }
            else
            {
                ExpRotation1(sub, subLen, 1, c, -s);
                if (stride2 != 0)
                    ExpRotation1(sub, subLen, stride2, s, -c);
            }
        }
    }

    /// <summary>Inner Givens rotation primitive used by <see cref="ExpRotation"/>.</summary>
    private static void ExpRotation1(Span<float> X, int len, int stride, float c, float s)
    {
        float ms = -s;
        int xi = 0;
        for (int i = 0; i < len - stride; i++)
        {
            float x1 = X[xi];
            float x2 = X[xi + stride];
            X[xi + stride] = c * x2 + s * x1;
            X[xi] = c * x1 + ms * x2;
            xi++;
        }
        xi = len - 2 * stride - 1;
        for (int i = len - 2 * stride - 1; i >= 0; i--)
        {
            float x1 = X[xi];
            float x2 = X[xi + stride];
            X[xi + stride] = c * x2 + s * x1;
            X[xi] = c * x1 + ms * x2;
            xi--;
        }
    }

    /// <summary>
    /// Normalises a decoded integer PVQ codeword to unit norm and
    /// scales by <paramref name="gain"/>. Float-build port — fixed
    /// point shifts collapse to no-ops.
    /// </summary>
    public static void NormaliseResidual(ReadOnlySpan<int> iy, Span<float> X, int N, float ryy, float gain)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThan(N, 1);
        if (iy.Length < N) throw new System.ArgumentException("iy length must be >= N", nameof(iy));
        if (X.Length < N) throw new System.ArgumentException("X length must be >= N", nameof(X));
        System.ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(ryy, 0f);

        float g = gain / (float)System.Math.Sqrt(ryy);
        for (int i = 0; i < N; i++)
            X[i] = iy[i] * g;
    }

    /// <summary>
    /// Builds the per-block collapse mask from the decoded integer
    /// codeword. Each set bit indicates the corresponding MDCT block
    /// received at least one pulse. Mirrors libopus
    /// <c>extract_collapse_mask</c>.
    /// </summary>
    public static uint ExtractCollapseMask(ReadOnlySpan<int> iy, int N, int B)
    {
        if (B <= 1) return 1u;
        int N0 = N / B;
        uint mask = 0;
        for (int i = 0; i < B; i++)
        {
            int sum = 0;
            for (int j = 0; j < N0; j++)
                sum |= iy[i * N0 + j];
            if (sum != 0) mask |= 1u << i;
        }
        return mask;
    }

    /// <summary>
    /// Decodes a PVQ codeword from the entropy stream and reconstructs
    /// the unit-norm shape vector in <paramref name="X"/>. Mirrors the
    /// float-build, non-QEXT branch of libopus <c>alg_unquant</c>.
    /// </summary>
    /// <param name="X">Output shape vector (length ≥ N).</param>
    /// <param name="N">Partition size.</param>
    /// <param name="K">Pulse count (&gt; 0).</param>
    /// <param name="spread">Spread mode (0..3); 0 disables the rotation.</param>
    /// <param name="B">MDCT block count for this partition.</param>
    /// <param name="dec">Range decoder.</param>
    /// <param name="gain">Output gain (≥ 0).</param>
    /// <returns>Collapse mask — one bit per MDCT block.</returns>
    public static uint AlgUnquant(
        Span<float> X, int N, int K, int spread, int B,
        ref OpusRangeDecoder dec, float gain)
    {
        System.ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(K, 0);
        System.ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(N, 1);
        if (X.Length < N) throw new System.ArgumentException("X must hold at least N samples.", nameof(X));

        // Always heap-allocate iy — the ref-struct decoder + Span<int> combo
        // tickles a CS8350 lifetime conflict when iy is stackalloc'd.
        int[] iy = new int[N];

        int ryy = CeltPvq.DecodePulses(ref dec, N, K, iy);
        uint mask = ExtractCollapseMask(iy, N, B);
        NormaliseResidual(iy, X, N, ryy, gain);
        ExpRotation(X, N, dir: -1, B, K, spread);
        return mask;
    }
}
