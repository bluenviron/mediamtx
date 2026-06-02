using System.Numerics;

namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// CELT energy split decoder — ports the decoder-side of libopus
/// <c>celt/bands.c</c>: <c>compute_qn</c>, <c>compute_theta</c>, and
/// <c>quant_band_n1</c>.
/// </summary>
/// <remarks>
/// These primitives sit in front of the recursive <c>quant_band</c> /
/// <c>quant_partition</c> work (Phase 2c.3b.4). For each band we ask:
/// how much resolution should the angle parameter <c>theta</c> get
/// (<c>compute_qn</c>), then decode it (<c>compute_theta</c>) to derive
/// the mid/side amplitudes <c>imid</c> and <c>iside</c> plus the bit
/// redistribution <c>delta</c>. The N==1 case is degenerate: just one
/// sign bit per channel, so it has its own fast path
/// (<c>quant_band_n1</c>).
/// <para>This port implements the decoder branch only — all <c>encode</c>
/// guarded code (including <c>theta_round</c>, <c>avoid_split_noise</c>,
/// and the <c>ENABLE_QEXT</c> extension) is omitted.</para>
/// </remarks>
public static class CeltSplit
{
    private static readonly short[] _exp2Table8 =
        { 16384, 17866, 19483, 21247, 23170, 25267, 27554, 30048 };

    /// <summary>
    /// Output of <see cref="ComputeTheta"/>. Mirrors libopus
    /// <c>struct split_ctx</c> (decoder-relevant fields only).
    /// </summary>
    public struct BandSplitContext
    {
        /// <summary>Stereo inversion flag (only set when qn==1 and bits allow).</summary>
        public int Inv;

        /// <summary>Quantised cosine of theta (Q15).</summary>
        public int IMid;

        /// <summary>Quantised sine of theta (Q15).</summary>
        public int ISide;

        /// <summary>Bit-allocation delta between mid and side (in 1/128 bits).</summary>
        public int Delta;

        /// <summary>Decoded angle parameter (Q14, range [0, 16384]).</summary>
        public int ITheta;

        /// <summary>Fractional bits consumed by the theta decode (1/8 bit units).</summary>
        public int QAlloc;
    }

    /// <summary>
    /// Decides how many quantisation levels (<c>qn</c>) the angle
    /// parameter theta gets, given the band's available bit budget.
    /// Mirrors libopus <c>compute_qn</c> from <c>celt/bands.c</c>.
    /// </summary>
    /// <param name="n">Partition size (samples).</param>
    /// <param name="b">Bit budget for this partition (1/8 bit units).</param>
    /// <param name="offset">Per-band offset (pre-computed by caller).</param>
    /// <param name="pulseCap">Maximum pulse cost for this band (1/8 bit units).</param>
    /// <param name="stereo">True for the stereo split.</param>
    /// <returns>Number of theta quantisation levels (always even, ≤ 256).</returns>
    public static int ComputeQn(int n, int b, int offset, int pulseCap, bool stereo)
    {
        int n2 = 2 * n - 1;
        if (stereo && n == 2) n2--;

        // celt_sudiv is signed integer division — same as C99 `/` (truncates toward zero).
        int qb = (b + n2 * offset) / n2;
        qb = Math.Min(b - pulseCap - (4 << CeltConstants.BitRes), qb);
        qb = Math.Min(8 << CeltConstants.BitRes, qb);

        int qn;
        if (qb < ((1 << CeltConstants.BitRes) >> 1))
        {
            qn = 1;
        }
        else
        {
            qn = _exp2Table8[qb & 0x7] >> (14 - (qb >> CeltConstants.BitRes));
            qn = ((qn + 1) >> 1) << 1;
        }
        // libopus celt_assert(qn <= 256).
        return qn;
    }

    /// <summary>
    /// Decodes the band split parameter theta and the derived
    /// <c>imid</c>/<c>iside</c>/<c>delta</c>. Mirrors the decoder branch
    /// of libopus <c>compute_theta</c>.
    /// </summary>
    /// <param name="dec">Range decoder.</param>
    /// <param name="logNAtBand">Value of <c>mode.logN[band]</c> for the current band.</param>
    /// <param name="bandIndex">Band index (used to gate stereo qn=1 collapse).</param>
    /// <param name="intensity">Intensity-stereo cutoff band.</param>
    /// <param name="n">Partition size.</param>
    /// <param name="b">Bit budget (1/8 bit units). Updated to reflect spent bits.</param>
    /// <param name="blocks">Number of MDCT blocks in this band (libopus <c>B</c>).</param>
    /// <param name="blocks0">Original block count before time-splitting recursion (libopus <c>B0</c>).</param>
    /// <param name="LM">Log2 of MDCT block size.</param>
    /// <param name="stereo">True when called from the stereo wrapper.</param>
    /// <param name="fill">Per-block fill mask. Updated in-place.</param>
    /// <param name="disableInv">If true, suppresses the qn=1 stereo inversion bit.</param>
    /// <param name="remainingBits">Caller's running <c>remaining_bits</c> (1/8 bit units).</param>
    /// <param name="sctx">Result.</param>
    public static void ComputeTheta(
        ref OpusRangeDecoder dec,
        int logNAtBand,
        int bandIndex,
        int intensity,
        int n,
        ref int b,
        int blocks,
        int blocks0,
        int LM,
        bool stereo,
        ref int fill,
        bool disableInv,
        int remainingBits,
        out BandSplitContext sctx)
    {
        int pulseCap = logNAtBand + LM * (1 << CeltConstants.BitRes);
        int offset = (pulseCap >> 1)
            - ((stereo && n == 2) ? CeltConstants.QThetaOffsetTwoPhase : CeltConstants.QThetaOffset);
        int qn = ComputeQn(n, b, offset, pulseCap, stereo);
        if (stereo && bandIndex >= intensity) qn = 1;

        uint tellBefore = dec.TellFrac();
        int itheta = 0;
        int inv = 0;

        if (qn != 1)
        {
            itheta = DecodeITheta(ref dec, qn, n, blocks0, stereo);
            // Scale to Q14 ([0, 16384]).
            itheta = (int)((long)itheta * 16384 / qn);
        }
        else if (stereo)
        {
            int twoFrac = 2 << CeltConstants.BitRes;
            if (b > twoFrac && remainingBits > twoFrac)
                inv = dec.DecodeBitLogP(2);
            if (disableInv) inv = 0;
            itheta = 0;
        }

        int qalloc = (int)(dec.TellFrac() - tellBefore);
        b -= qalloc;

        FinaliseSplit(itheta, n, blocks, ref fill, out int imid, out int iside, out int delta);

        sctx = new BandSplitContext
        {
            Inv = inv,
            IMid = imid,
            ISide = iside,
            Delta = delta,
            ITheta = itheta,
            QAlloc = qalloc,
        };
    }

    /// <summary>
    /// Computes the post-decode <c>imid</c>/<c>iside</c>/<c>delta</c>
    /// values and updates <paramref name="fill"/>. Extracted from
    /// <see cref="ComputeTheta"/> so the pure-math part is independently
    /// testable without a range decoder.
    /// </summary>
    internal static void FinaliseSplit(
        int itheta, int n, int blocks, ref int fill,
        out int imid, out int iside, out int delta)
    {
        if (itheta == 0)
        {
            imid = 32767;
            iside = 0;
            fill &= (1 << blocks) - 1;
            delta = -16384;
        }
        else if (itheta == 16384)
        {
            imid = 0;
            iside = 32767;
            fill &= ((1 << blocks) - 1) << blocks;
            delta = 16384;
        }
        else
        {
            imid = CeltPvqMath.BitexactCos((short)itheta);
            iside = CeltPvqMath.BitexactCos((short)(16384 - itheta));
            delta = FracMul16((short)((n - 1) << 7),
                              (short)CeltPvqMath.BitexactLog2Tan(iside, imid));
        }
    }

    /// <summary>
    /// Decodes a single PVQ partition of width 1 (N=1) — just a sign
    /// bit per channel, then resynthesise to ±1 (NORM_SCALING in float
    /// build). Mirrors libopus <c>quant_band_n1</c>.
    /// </summary>
    /// <param name="dec">Range decoder.</param>
    /// <param name="remainingBits">Running bit budget (1/8 bit units), updated in-place.</param>
    /// <param name="X">First-channel sample (length ≥ 1).</param>
    /// <param name="Y">Second-channel sample (length ≥ 1), or empty for mono.</param>
    /// <param name="lowbandOut">Optional output sample for the next band's lowband, or empty.</param>
    /// <returns>Codeword bitmask — always 1 for N=1.</returns>
    public static uint QuantBandN1(
        ref OpusRangeDecoder dec,
        ref int remainingBits,
        Span<float> X,
        Span<float> Y,
        Span<float> lowbandOut)
    {
        if (X.Length < 1) throw new ArgumentException("X must have at least one sample", nameof(X));

        bool stereo = !Y.IsEmpty;
        if (stereo && Y.Length < 1) throw new ArgumentException("Y must have at least one sample when non-empty", nameof(Y));

        int channels = stereo ? 2 : 1;
        for (int c = 0; c < channels; c++)
        {
            int sign = 0;
            if (remainingBits >= (1 << CeltConstants.BitRes))
            {
                sign = (int)dec.DecodeBits(1);
                remainingBits -= 1 << CeltConstants.BitRes;
            }
            Span<float> x = c == 0 ? X : Y;
            x[0] = sign != 0 ? -1.0f : 1.0f;
        }
        if (!lowbandOut.IsEmpty)
        {
            // Float build of libopus: lowband_out[0] = X[0] / (1 << 4).
            lowbandOut[0] = X[0] * (1.0f / 16.0f);
        }
        return 1u;
    }

    /// <summary>Decoder-side dispatch over the three theta pdfs.</summary>
    private static int DecodeITheta(ref OpusRangeDecoder dec, int qn, int n, int blocks0, bool stereo)
    {
        if (stereo && n > 2)
        {
            // Step pdf — used for stereo with N>2.
            return DecodeIThetaStep(ref dec, qn);
        }
        if (blocks0 > 1 || stereo)
        {
            // Uniform pdf — time-split (>1 block) or stereo with N<=2.
            return (int)dec.DecodeUint((uint)(qn + 1));
        }
        // Triangular pdf — mono, single block.
        return DecodeIThetaTriangular(ref dec, qn);
    }

    /// <summary>
    /// Decodes itheta from the step pdf used for stereo splits with N&gt;2.
    /// Mirrors the decoder half of the corresponding libopus branch.
    /// </summary>
    internal static int DecodeIThetaStep(ref OpusRangeDecoder dec, int qn)
    {
        const int p0 = 3;
        int x0 = qn / 2;
        uint ft = (uint)(p0 * (x0 + 1) + x0);
        uint fs = dec.Decode(ft);
        int x;
        uint fl, fh;
        if (fs < (uint)((x0 + 1) * p0))
        {
            x = (int)(fs / p0);
            fl = (uint)(p0 * x);
            fh = (uint)(p0 * (x + 1));
        }
        else
        {
            x = x0 + 1 + ((int)fs - (x0 + 1) * p0);
            fl = (uint)((x - 1 - x0) + (x0 + 1) * p0);
            fh = (uint)((x - x0) + (x0 + 1) * p0);
        }
        dec.Update(fl, fh, ft);
        return x;
    }

    /// <summary>
    /// Decodes itheta from the triangular pdf used for mono splits with B0==1.
    /// </summary>
    internal static int DecodeIThetaTriangular(ref OpusRangeDecoder dec, int qn)
    {
        int half = qn >> 1;
        uint ft = (uint)((half + 1) * (half + 1));
        uint fm = dec.Decode(ft);
        int itheta;
        int fs;
        uint fl;
        uint pivot = (uint)((half * (half + 1)) >> 1);
        if (fm < pivot)
        {
            itheta = (int)((IsqrtU32(8 * fm + 1) - 1) >> 1);
            fs = itheta + 1;
            fl = (uint)((itheta * (itheta + 1)) >> 1);
        }
        else
        {
            itheta = (2 * (qn + 1) - (int)IsqrtU32(8 * (ft - fm - 1) + 1)) >> 1;
            fs = qn + 1 - itheta;
            fl = ft - (uint)(((qn + 1 - itheta) * (qn + 2 - itheta)) >> 1);
        }
        dec.Update(fl, fl + (uint)fs, ft);
        return itheta;
    }

    /// <summary>
    /// libopus <c>FRAC_MUL16(a, b) = (16384 + (int16)a * (int16)b) &gt;&gt; 15</c>.
    /// Inputs are first reduced to int16 (matching the libopus macro's
    /// implicit cast) before multiplication.
    /// </summary>
    private static int FracMul16(short a, short b) => (16384 + a * b) >> 15;

    /// <summary>
    /// Integer floor square root of a uint32 — libopus <c>isqrt32</c>.
    /// Bit-by-bit Newton variant; bit-exact for all uint inputs.
    /// </summary>
    internal static uint IsqrtU32(uint val)
    {
        if (val == 0) return 0;
        // Largest power-of-two whose square does not exceed val.
        int ilog = BitOperations.Log2(val) + 1; // libopus EC_ILOG
        int bshift = (ilog - 1) >> 1;
        uint b = 1u << bshift;
        uint g = 0;
        do
        {
            uint t = ((g << 1) + b) << bshift;
            if (t <= val)
            {
                g += b;
                val -= t;
            }
            b >>= 1;
            bshift--;
        } while (bshift >= 0);
        return g;
    }
}
