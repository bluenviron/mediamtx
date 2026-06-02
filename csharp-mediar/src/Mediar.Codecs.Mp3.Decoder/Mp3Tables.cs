namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Small static tables for the MPEG-1/2/2.5 Layer III decoder, derived from
/// ISO 11172-3 (MPEG-1) and ISO 13818-3 (MPEG-2 LSF) Annex B. Tables that are
/// non-trivial in size live in dedicated files (Huffman, scalefactor bands,
/// D-window).
/// </summary>
internal static class Mp3Tables
{
    /// <summary>Pretab vector (long-block preflag bias) per ISO 11172-3 Table B.6.</summary>
    public static readonly int[] PreTab = { 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 3, 3, 3, 2, 0 };

    /// <summary>MPEG-1 scalefactor compression table (slen1, slen2) indexed by scalefac_compress (0..15).</summary>
    public static readonly int[,] Slen =
    {
        { 0, 0 }, { 0, 1 }, { 0, 2 }, { 0, 3 },
        { 3, 0 }, { 1, 1 }, { 1, 2 }, { 1, 3 },
        { 2, 1 }, { 2, 2 }, { 2, 3 }, { 3, 1 },
        { 3, 2 }, { 3, 3 }, { 4, 2 }, { 4, 3 },
    };

    /// <summary>
    /// Alias-reduction coefficients (cs, ca) for the 8 butterfly taps. Derived
    /// from ci = [-0.6, -0.535, -0.33, -0.185, -0.095, -0.041, -0.0142, -0.0037]
    /// with cs = 1/sqrt(1+ci^2), ca = ci/sqrt(1+ci^2).
    /// </summary>
    public static readonly float[] Cs;
    public static readonly float[] Ca;

    /// <summary>
    /// IMDCT window for block_type 0 (normal long), length 36.
    /// w[i] = sin(pi/36 * (i + 0.5)).
    /// </summary>
    public static readonly float[] WindowLong = new float[36];

    /// <summary>
    /// Block_type 1 (start) window, length 36: long-rise then 6-sample plateau
    /// then short-fall.
    /// </summary>
    public static readonly float[] WindowStart = new float[36];

    /// <summary>
    /// Block_type 3 (stop) window, length 36: short-rise then 6-sample plateau
    /// then long-fall.
    /// </summary>
    public static readonly float[] WindowStop = new float[36];

    /// <summary>
    /// Block_type 2 (short) window, length 12. Applied to each of the 3 short
    /// IMDCT outputs separately. w[i] = sin(pi/12 * (i + 0.5)).
    /// </summary>
    public static readonly float[] WindowShort = new float[12];

    /// <summary>
    /// IMDCT 12-point cosine table: cos((pi/24) * (2k+1) * (2n+7)) for the
    /// short-window inverse MDCT (output length 12 from 6 spectral inputs).
    /// </summary>
    public static readonly float[,] ImdctShort = new float[6, 12];

    /// <summary>
    /// IMDCT 36-point cosine table: cos((pi/72) * (2k+1) * (2n+19)) for the
    /// long-window inverse MDCT (output length 36 from 18 spectral inputs).
    /// </summary>
    public static readonly float[,] ImdctLong = new float[18, 36];

    /// <summary>
    /// Polyphase synthesis N[i][j] = cos((16+i) * (2j+1) * pi / 64),
    /// 64 rows × 32 columns. Used as the matrixing step from 32 subband
    /// samples to 64 V-buffer entries.
    /// </summary>
    public static readonly float[,] N = new float[64, 32];

    /// <summary>
    /// Powers of 2 for the requantization "global_gain" term, indexed by
    /// (global_gain - 210 + 256). Range covers all valid global_gain (0..255)
    /// minus the worst-case subblock_gain bias (-8*7 = -56).
    /// </summary>
    public static readonly float[] GainPow2 = new float[256 + 118];

    /// <summary>
    /// Powers of 2 for short-block subblock_gain (0..7) → 2^(-2*g).
    /// </summary>
    public static readonly float[] SubblockGainPow2 = new float[8];

    /// <summary>
    /// |is|^(4/3) for is = 0..8206 (max Huffman value plus linbits saturated).
    /// </summary>
    public static readonly float[] Pow43 = new float[8207];

    /// <summary>
    /// Intensity stereo ratio table (MPEG-1) tan(is_pos * pi/12) for
    /// is_pos = 0..6; is_pos = 7 means use the right channel only.
    /// </summary>
    public static readonly float[] IsRatioTan = new float[7];

    /// <summary>
    /// MPEG-2 LSF intensity stereo ratio table: pow(2.0, -k/2) for the
    /// "k = (is_pos+1)>>1" formulation, indexed by is_pos = 0..31. The actual
    /// pair (l, r) factors are derived per is_pos parity at decode time.
    /// </summary>
    public static readonly float[] LsfIsRatio = new float[32];

    static Mp3Tables()
    {
        Cs = new float[8];
        Ca = new float[8];
        double[] ci = { -0.6, -0.535, -0.33, -0.185, -0.095, -0.041, -0.0142, -0.0037 };
        for (int i = 0; i < 8; i++)
        {
            double cs = 1.0 / Math.Sqrt(1.0 + ci[i] * ci[i]);
            Cs[i] = (float)cs;
            Ca[i] = (float)(ci[i] * cs);
        }

        for (int i = 0; i < 36; i++) WindowLong[i] = (float)Math.Sin(Math.PI / 36.0 * (i + 0.5));
        for (int i = 0; i < 12; i++) WindowShort[i] = (float)Math.Sin(Math.PI / 12.0 * (i + 0.5));

        // Start: i in [0..17] = sin(pi/36 * (i+0.5)); [18..23] = 1; [24..29] = sin(pi/12 * (i-18+0.5)); [30..35] = 0.
        for (int i = 0; i < 18; i++) WindowStart[i] = (float)Math.Sin(Math.PI / 36.0 * (i + 0.5));
        for (int i = 18; i < 24; i++) WindowStart[i] = 1f;
        for (int i = 24; i < 30; i++) WindowStart[i] = (float)Math.Sin(Math.PI / 12.0 * (i - 18 + 0.5));
        for (int i = 30; i < 36; i++) WindowStart[i] = 0f;

        // Stop: mirror of Start.
        for (int i = 0; i < 6; i++) WindowStop[i] = 0f;
        for (int i = 6; i < 12; i++) WindowStop[i] = (float)Math.Sin(Math.PI / 12.0 * (i - 6 + 0.5));
        for (int i = 12; i < 18; i++) WindowStop[i] = 1f;
        for (int i = 18; i < 36; i++) WindowStop[i] = (float)Math.Sin(Math.PI / 36.0 * (i + 0.5));

        for (int k = 0; k < 6; k++)
            for (int n = 0; n < 12; n++)
                ImdctShort[k, n] = (float)Math.Cos(Math.PI / 24.0 * (2 * k + 1) * (2 * n + 7));

        for (int k = 0; k < 18; k++)
            for (int n = 0; n < 36; n++)
                ImdctLong[k, n] = (float)Math.Cos(Math.PI / 72.0 * (2 * k + 1) * (2 * n + 19));

        for (int i = 0; i < 64; i++)
            for (int j = 0; j < 32; j++)
                N[i, j] = (float)Math.Cos((16 + i) * (2 * j + 1) * Math.PI / 64.0);

        for (int i = 0; i < GainPow2.Length; i++)
            GainPow2[i] = (float)Math.Pow(2.0, (i - 256) * 0.25);

        for (int g = 0; g < 8; g++) SubblockGainPow2[g] = (float)Math.Pow(2.0, -2 * g);

        for (int i = 0; i < Pow43.Length; i++) Pow43[i] = (float)Math.Pow(i, 4.0 / 3.0);

        for (int p = 0; p < 7; p++) IsRatioTan[p] = (float)Math.Tan(p * Math.PI / 12.0);

        for (int p = 0; p < 32; p++) LsfIsRatio[p] = (float)Math.Pow(2.0, -(p + 1) * 0.5);
    }

    /// <summary>Look up the |is|^(4/3) requantization factor for arbitrary value.</summary>
    public static float Pow43Of(int is_)
    {
        int abs = is_ < 0 ? -is_ : is_;
        return abs < Pow43.Length ? Pow43[abs] : (float)Math.Pow(abs, 4.0 / 3.0);
    }
}
