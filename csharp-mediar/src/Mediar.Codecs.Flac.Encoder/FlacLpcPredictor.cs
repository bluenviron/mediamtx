using Mediar.IO;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Linear-Prediction-Coefficient subframe coder for FLAC (RFC 9639 §10.3.4).
/// Estimates per-block LPC coefficients via the Welch-windowed autocorrelation
/// method + Levinson-Durbin recursion, quantises them to a fixed precision,
/// and emits the resulting LPC subframe with Rice-coded residuals (single
/// partition).
/// </summary>
/// <remarks>
/// LPC coefficients <c>c[0..order-1]</c> predict
/// <c>x_hat[i] = ⌊(Σ c[k] · x[i-1-k]) ≫ shift⌋</c> and the residual is
/// <c>r[i] = x[i] - x_hat[i]</c>. The decoder reconstructs the original
/// sample via <c>x[i] = r[i] + (int)((Σ c[k] · x[i-1-k]) ≫ shift)</c>; the
/// encoder uses the same arithmetic so the round-trip is bit-exact provided
/// the intermediate <c>acc</c> fits in <see cref="long"/> and the shifted
/// prediction fits in <see cref="int"/>.
///
/// Maximum order is 12 (covers the bulk of useful LPC gain on real music
/// without paying for the long tail of marginal compression). Coefficient
/// precision is fixed at 12 bits — enough to express typical LPC ranges of
/// ±2 with one bit of headroom while keeping the per-coefficient bit cost
/// low. For <c>bps &gt; 24</c> the predictor sit-out: the
/// <c>order × precision × sample</c> accumulator can overflow even with the
/// minimum shift, and the residual would be wrong on decode.
/// </remarks>
internal static class FlacLpcPredictor
{
    /// <summary>Maximum supported LPC order (RFC permits up to 32; libFLAC ships 12 by default).</summary>
    public const int MaxOrder = 12;

    /// <summary>Quantised-coefficient precision in bits.</summary>
    public const int Precision = 12;

    /// <summary>
    /// True when this predictor is willing to compete for the given parameters.
    /// LPC is skipped for <c>bps &gt; 24</c> (overflow risk in 32-bit residual
    /// accumulation) and for blocks too short to fit at least one residual
    /// sample beyond the maximum order's warmup.
    /// </summary>
    public static bool IsSupported(int bps, int blockSize) =>
        bps <= 24 && blockSize >= 2;

    /// <summary>
    /// Try every order in <c>[1, min(MaxOrder, blockSize/2)]</c> and return the
    /// cheapest configuration that strictly beats <paramref name="maxBits"/>.
    /// On success, <paramref name="qcoefOut"/> is populated with the winning
    /// quantised coefficients (use <c>qcoefOut[..order]</c>); the residual
    /// workspace is left in an undefined state and must be recomputed by
    /// <see cref="WriteSubframe"/>.
    /// </summary>
    public static bool TryEstimateBest(
        ReadOnlySpan<int> samples,
        int bps,
        int blockSize,
        int maxPartitionOrder,
        Span<int> residualWorkspace,
        Span<double> windowedWorkspace,
        Span<int> qcoefOut,
        Span<int> ksWorkspace,
        long maxBits,
        out int order,
        out int precision,
        out int shift,
        out long bits)
    {
        order = -1;
        precision = Precision;
        shift = 0;
        bits = maxBits;

        if (!IsSupported(bps, blockSize)) return false;
        int maxOrder = Math.Min(MaxOrder, blockSize / 2);
        if (maxOrder < 1) return false;

        Span<double> windowed = windowedWorkspace[..blockSize];
        ApplyWelchWindow(samples, windowed);

        Span<double> autocorr = stackalloc double[MaxOrder + 1];
        ComputeAutocorrelation(windowed, autocorr[..(maxOrder + 1)]);

        if (autocorr[0] <= 0 || double.IsNaN(autocorr[0])) return false;

        Span<double> lpc = stackalloc double[MaxOrder];
        Span<double> tmp = stackalloc double[MaxOrder];
        Span<int> qcoef = stackalloc int[MaxOrder];

        double e = autocorr[0];

        for (int m = 1; m <= maxOrder; m++)
        {
            // Reflection coefficient k_m = (R[m] - Σ lpc_old[j]·R[m-1-j]) / e_{m-1}
            double kk = autocorr[m];
            for (int j = 0; j < m - 1; j++) kk -= lpc[j] * autocorr[m - 1 - j];
            kk /= e;

            // Update LPC coefficients in-place via tmp:
            //   new_lpc[j] = lpc_old[j] - k_m · lpc_old[m-2-j]  for j = 0..m-2
            //   new_lpc[m-1] = k_m
            if (m > 1)
            {
                for (int j = 0; j < m - 1; j++) tmp[j] = lpc[j];
                for (int j = 0; j < m - 1; j++) lpc[j] = tmp[j] - kk * tmp[m - 2 - j];
            }
            lpc[m - 1] = kk;

            // Quantise + cost-estimate BEFORE updating e, so a numerical
            // divergence at order m+1 doesn't disqualify a still-usable order m.
            if (TryQuantize(lpc[..m], Precision, qcoef[..m], out int s))
            {
                int n = blockSize - m;
                if (TryComputeResiduals(samples, qcoef[..m], s, residualWorkspace[..n]))
                {
                    if (FlacRice.TryChooseBestPartitioning(
                            residualWorkspace[..n], m, blockSize, maxPartitionOrder,
                            ksWorkspace, out _, out _, out long residualBits))
                    {
                        long cost = 8L                        // subframe header byte
                                  + (long)m * bps             // warmup samples
                                  + 4 + 5                     // precision + shift fields
                                  + (long)m * Precision       // quantised coefficient bits
                                  + residualBits;

                        if (cost < bits)
                        {
                            bits = cost;
                            order = m;
                            shift = s;
                            qcoef[..m].CopyTo(qcoefOut[..m]);
                        }
                    }
                }
            }

            e *= 1.0 - kk * kk;
            if (e <= 0 || double.IsNaN(e)) break;
        }

        return order > 0;
    }

    /// <summary>
    /// Emit an LPC subframe with the parameters from a prior
    /// <see cref="TryEstimateBest"/> call. Residuals are recomputed into
    /// <paramref name="residualWorkspace"/> from the quantised coefficients
    /// and the multi-partition Rice layout is rediscovered.
    /// </summary>
    public static void WriteSubframe(
        ref BitWriter bw,
        ReadOnlySpan<int> samples,
        int bps,
        int order,
        int precision,
        int shift,
        int blockSize,
        int maxPartitionOrder,
        ReadOnlySpan<int> qcoef,
        Span<int> residualWorkspace,
        Span<int> ksWorkspace)
    {
        int n = blockSize - order;
        if (!TryComputeResiduals(samples, qcoef, shift, residualWorkspace[..n]))
        {
            throw new InvalidOperationException("LPC residual recomputation overflowed int32 during WriteSubframe.");
        }

        // Subframe header byte: 0 (pad) | 1NNNNN (type, N = order-1) | 0 (wasted-bits flag).
        // type = 0b100000 | (order - 1) → byte = type << 1.
        uint headerByte = (uint)((0b100000 | (order - 1)) << 1);
        bw.WriteBits(headerByte, 8);

        // Warmup samples (bps-bit signed, two's complement).
        for (int i = 0; i < order; i++) WriteSignedSample(ref bw, samples[i], bps);

        // Precision field stores (precision - 1) in 4 bits; shift in 5 bits signed (we always emit ≥ 0).
        bw.WriteBits((uint)(precision - 1), 4);
        bw.WriteBits((uint)shift & 0x1F, 5);

        for (int i = 0; i < order; i++) WriteSignedSample(ref bw, qcoef[i], precision);

        if (!FlacRice.TryChooseBestPartitioning(
                residualWorkspace[..n], order, blockSize, maxPartitionOrder,
                ksWorkspace, out int partitionOrder, out int method, out _))
        {
            throw new InvalidOperationException("LpcPredictor failed to re-derive partitioning during WriteSubframe.");
        }
        FlacRice.WritePartitions(ref bw, residualWorkspace[..n], order, blockSize, partitionOrder, method, ksWorkspace);
    }

    /// <summary>
    /// Apply a Welch window <c>w[i] = 1 - ((2i/(N-1)) - 1)²</c>. Tapering the
    /// signal stops aliasing at the block boundaries from inflating the higher
    /// autocorrelation lags, which produces noticeably better LPC fit on real
    /// music for a negligible per-sample cost.
    /// </summary>
    private static void ApplyWelchWindow(ReadOnlySpan<int> samples, Span<double> windowed)
    {
        int n = samples.Length;
        if (n == 1)
        {
            windowed[0] = samples[0];
            return;
        }

        double scale = 2.0 / (n - 1);
        for (int i = 0; i < n; i++)
        {
            double t = i * scale - 1.0;
            double w = 1.0 - t * t;
            windowed[i] = samples[i] * w;
        }
    }

    /// <summary>
    /// Compute autocorrelation <c>R[lag] = Σ_{i=lag}^{N-1} x[i]·x[i-lag]</c>
    /// for <c>lag = 0..maxLag</c>.
    /// </summary>
    private static void ComputeAutocorrelation(ReadOnlySpan<double> samples, Span<double> r)
    {
        int n = samples.Length;
        int maxLag = r.Length - 1;
        for (int lag = 0; lag <= maxLag; lag++)
        {
            double sum = 0;
            for (int i = lag; i < n; i++) sum += samples[i] * samples[i - lag];
            r[lag] = sum;
        }
    }

    /// <summary>
    /// Quantise floating-point LPC coefficients to <paramref name="precision"/>-bit
    /// signed integers with a power-of-two shift, using libFLAC-style error
    /// feedback (apply quantisation error backwards from the largest lag to
    /// the smallest so the round-off cancels across lags).
    /// </summary>
    private static bool TryQuantize(ReadOnlySpan<double> lpc, int precision, Span<int> qcoef, out int shift)
    {
        int order = lpc.Length;
        double maxAbs = 0;
        for (int i = 0; i < order; i++)
        {
            double a = Math.Abs(lpc[i]);
            if (a > maxAbs) maxAbs = a;
        }

        if (maxAbs == 0 || double.IsNaN(maxAbs) || double.IsInfinity(maxAbs))
        {
            shift = 0;
            return false;
        }

        // Want |quantised| < 2^(precision-1). Pick shift so that
        // maxAbs * 2^shift < 2^(precision-1) with one bit of headroom.
        int log2MaxAbs = (int)Math.Ceiling(Math.Log2(maxAbs));
        shift = precision - 2 - log2MaxAbs;

        if (shift < 0 || shift > 15)
        {
            // shift > 15 doesn't fit in the 5-bit signed shift field; shift < 0
            // would need a 5-bit negative shift, which the FLAC decoder rejects.
            shift = 0;
            return false;
        }

        double scaleD = 1L << shift;
        int qmax = (1 << (precision - 1)) - 1;
        int qmin = -(1 << (precision - 1));

        double error = 0;
        for (int i = order - 1; i >= 0; i--)
        {
            double scaled = lpc[i] * scaleD + error;
            long q = (long)Math.Round(scaled);
            if (q > qmax) q = qmax;
            else if (q < qmin) q = qmin;
            error = scaled - q;
            qcoef[i] = (int)q;
        }

        return true;
    }

    /// <summary>
    /// Compute residuals <c>r[i] = x[order+i] - ⌊(Σ qcoef[k] · x[order+i-1-k]) ≫ shift⌋</c>.
    /// Returns <c>false</c> if either the shifted prediction or the resulting
    /// residual escapes int32 range — the caller should then fall back to a
    /// different predictor / VERBATIM.
    /// </summary>
    private static bool TryComputeResiduals(
        ReadOnlySpan<int> samples,
        ReadOnlySpan<int> qcoef,
        int shift,
        Span<int> residuals)
    {
        int order = qcoef.Length;
        int n = samples.Length - order;

        for (int i = 0; i < n; i++)
        {
            long acc = 0;
            for (int kk = 0; kk < order; kk++)
            {
                acc += (long)qcoef[kk] * samples[order + i - 1 - kk];
            }
            long predLong = acc >> shift;
            int predInt = (int)predLong;
            if (predInt != predLong) return false;
            long r = (long)samples[order + i] - predInt;
            if (r > int.MaxValue || r < int.MinValue) return false;
            residuals[i] = (int)r;
        }
        return true;
    }

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
