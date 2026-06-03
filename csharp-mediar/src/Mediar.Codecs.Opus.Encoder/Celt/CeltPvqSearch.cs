using System.Runtime.CompilerServices;
using Mediar.Codecs.Opus.Decoder.Celt;

namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// Encoder-side PVQ shape search, inverse of the decoder's
/// <see cref="CeltShape.AlgUnquant"/>. Ports libopus
/// <c>celt/vq.c:alg_quant</c> (float build, non-QEXT branch).
/// </summary>
/// <remarks>
/// References:
/// <list type="bullet">
///   <item><description>RFC 6716 §4.3.3 — CELT spherical vector quantisation.</description></item>
///   <item><description>libopus <c>celt/vq.c</c>, <c>celt/cwrs.c</c>.</description></item>
/// </list>
/// </remarks>
internal static class CeltPvqSearch
{
    /// <summary>
    /// Find the K-pulse codeword approximating <paramref name="X"/>,
    /// write it to the range encoder, and replace <paramref name="X"/>
    /// with the reconstructed unit-norm shape vector (matching what the
    /// decoder will produce). Returns the per-block collapse mask
    /// (same definition as the decoder's <see cref="CeltShape.AlgUnquant"/>).
    /// </summary>
    /// <param name="X">Input shape vector (length ≥ N). Overwritten with
    /// the reconstructed unit-norm vector on return.</param>
    /// <param name="N">Partition size.</param>
    /// <param name="K">Pulse count (&gt; 0).</param>
    /// <param name="spread">Spread mode (0..3); 0 disables the rotation.</param>
    /// <param name="B">MDCT block count for this partition.</param>
    /// <param name="enc">Range encoder.</param>
    /// <param name="gain">Output gain (≥ 0) applied to the reconstructed vector.</param>
    /// <param name="complexity">Encoder complexity 0..10; selects between
    /// the fast greedy path (≤ 5) and the higher-quality local-search
    /// refinement (≥ 6).</param>
    public static uint AlgQuant(
        Span<float> X, int N, int K, int spread, int B,
        ref OpusRangeEncoder enc, float gain, int complexity)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(K, 0);
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(N, 1);
        if (X.Length < N) throw new ArgumentException("X must hold at least N samples.", nameof(X));

        // Match the decoder by pre-rotating with dir=+1 (the decoder runs dir=-1).
        CeltShape.ExpRotation(X, N, dir: 1, B, K, spread);

        int[] iy = new int[N];
        int yy = PulseSearch(X, N, K, iy, complexity);

        EncodePulses(ref enc, iy, N, K);

        uint mask = CeltShape.ExtractCollapseMask(iy, N, B);
        CeltShape.NormaliseResidual(iy, X, N, yy, gain);
        // Inverse of the encoder's pre-rotation, so X comes out matching
        // what the decoder will produce in its own AlgUnquant.
        CeltShape.ExpRotation(X, N, dir: -1, B, K, spread);
        return mask;
    }

    /// <summary>
    /// Greedy pulse placement with an optional second pass of single-pulse
    /// swaps. Mirrors the pulse-search core of libopus
    /// <c>celt/vq.c:alg_quant</c>.
    /// </summary>
    private static int PulseSearch(Span<float> X, int N, int K, Span<int> iy, int complexity)
    {
        // Operate on |X| with a separate sign vector.
        Span<float> xAbs = N <= 256 ? stackalloc float[N] : new float[N];
        Span<int> sign = N <= 256 ? stackalloc int[N] : new int[N];

        float sumAbs = 0f;
        for (int i = 0; i < N; i++)
        {
            sign[i] = X[i] < 0 ? -1 : 1;
            xAbs[i] = MathF.Abs(X[i]);
            sumAbs += xAbs[i];
            iy[i] = 0;
        }

        int pulsesPlaced = 0;
        int yy = 0;
        float xy = 0f;

        // Cheap warm-start: pre-place floor(K · |X[i]| / Σ|X|) pulses per
        // bin (libopus alg_quant does the same with rcp / inverse-sumAbs).
        if (sumAbs > 0f && K > N)
        {
            float invSum = K / sumAbs;
            for (int i = 0; i < N; i++)
            {
                int p = (int)MathF.Floor(xAbs[i] * invSum);
                iy[i] = p;
                pulsesPlaced += p;
                yy += p * p;
                xy += p * xAbs[i];
            }
        }

        // Greedy placement of the remaining pulses.
        for (int j = pulsesPlaced; j < K; j++)
        {
            int bestI = 0;
            // Compare (xy + xAbs[i])² / (yy + 2·iy[i] + 1) across i.
            // a/b > c/d  ⇔  a·d > c·b  (b,d > 0).
            float bestNum = xy + xAbs[0];
            float bestNumSq = bestNum * bestNum;
            int bestDen = yy + 2 * iy[0] + 1;
            for (int i = 1; i < N; i++)
            {
                float num = xy + xAbs[i];
                float numSq = num * num;
                int den = yy + 2 * iy[i] + 1;
                if (numSq * bestDen > bestNumSq * den)
                {
                    bestI = i;
                    bestNum = num;
                    bestNumSq = numSq;
                    bestDen = den;
                }
            }
            iy[bestI]++;
            xy = bestNum;
            yy = bestDen + 2 * iy[bestI] - 2; // new yy = old yy + 2·new_y - 1
        }

        // Optional single-pulse swap refinement (high-complexity path).
        if (complexity >= 6)
        {
            bool improved = true;
            int passes = 0;
            while (improved && passes < 2)
            {
                improved = LocalSwapRefine(xAbs, iy, N, ref xy, ref yy);
                passes++;
            }
        }

        // Re-apply signs to the integer codeword.
        for (int i = 0; i < N; i++)
        {
            if (sign[i] < 0) iy[i] = -iy[i];
        }

        return yy;
    }

    /// <summary>
    /// One pass of "remove one pulse from i, add it at j" swaps; keeps any
    /// swap that improves <c>xy² / yy</c> (the squared inner product per
    /// unit reconstructed energy — the quantity alg_quant maximises).
    /// </summary>
    private static bool LocalSwapRefine(ReadOnlySpan<float> xAbs, Span<int> iy, int N, ref float xy, ref int yy)
    {
        bool improved = false;
        for (int i = 0; i < N; i++)
        {
            if (iy[i] == 0) continue;
            // Score if we remove a pulse from i.
            float xyMinusI = xy - xAbs[i];
            int yyMinusI = yy - (2 * iy[i] - 1);
            for (int j = 0; j < N; j++)
            {
                if (j == i) continue;
                float newXy = xyMinusI + xAbs[j];
                int newYy = yyMinusI + (2 * iy[j] + 1);
                // Compare newXy² / newYy vs xy² / yy.
                if (newXy * newXy * yy > xy * xy * newYy)
                {
                    iy[i]--;
                    iy[j]++;
                    xy = newXy;
                    yy = newYy;
                    improved = true;
                    break; // i changed; restart outer with next i
                }
            }
        }
        return improved;
    }

    /// <summary>
    /// Encode the integer pulse codeword to the range coder. Mirror of
    /// libopus <c>encode_pulses</c> — runs <c>icwrs</c> to convert the
    /// pulse vector into a unique integer in [0, V(N, K)) and writes
    /// that integer through <c>OpusRangeEncoder.EncodeUint</c>, the
    /// inverse of the decoder's <see cref="CeltPvq.DecodePulses"/>.
    /// </summary>
    public static void EncodePulses(ref OpusRangeEncoder enc, ReadOnlySpan<int> iy, int N, int K)
    {
        if (iy.Length < N) throw new ArgumentException("iy must hold at least N entries.", nameof(iy));
        uint idx = Icwrs(N, K, iy, out uint v);
        enc.EncodeUint(idx, v);
    }

    /// <summary>
    /// Inverse of <see cref="CeltPvq.DecodePulsesAtIndex"/> /
    /// <c>cwrsi</c>: walks the small-footprint U(n,k) recurrence
    /// forwards to convert <paramref name="iy"/> into a unique index in
    /// <c>[0, V(N, K))</c>. <paramref name="v"/> receives V(N, K).
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static uint Icwrs(int n, int k, ReadOnlySpan<int> iy, out uint v)
    {
        Span<uint> u = stackalloc uint[k + 2];
        u[0] = 0;
        for (int kk = 1; kk <= k + 1; kk++) u[kk] = (uint)((kk << 1) - 1);

        uint i = (uint)(iy[n - 1] < 0 ? 1 : 0);
        int kCur = Math.Abs(iy[n - 1]);
        int j = n - 2;
        i = unchecked(i + u[kCur]);
        kCur += Math.Abs(iy[j]);
        if (iy[j] < 0) i = unchecked(i + u[kCur + 1]);
        while (j-- > 0)
        {
            UnextInPlace(u, k + 2, 0);
            i = unchecked(i + u[kCur]);
            kCur += Math.Abs(iy[j]);
            if (iy[j] < 0) i = unchecked(i + u[kCur + 1]);
        }
        v = unchecked(u[kCur] + u[kCur + 1]);
        return i;
    }

    private static void UnextInPlace(Span<uint> u, int len, uint u0)
    {
        for (int j = 1; j < len; j++)
        {
            uint ui1 = unchecked(u[j] + u[j - 1] + u0);
            u[j - 1] = u0;
            u0 = ui1;
        }
        u[len - 1] = u0;
    }
}
