namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Static lookup tables for the CELT layer of Opus (RFC 6716 §4.3).
/// </summary>
/// <remarks>
/// <para>
/// All values are mathematical constants drawn straight from RFC 6716 and
/// its reference implementation. They describe band edges, log-energy
/// means, and the trim / probability models the range decoder uses. None
/// of this is copyrightable — same data shows up verbatim in every
/// conforming Opus decoder.
/// </para>
/// </remarks>
internal static class CeltConstants
{
    /// <summary>
    /// Maximum number of Bark-aligned bands CELT subdivides a Fullband
    /// frame into. Narrower bandwidths use a prefix of this table.
    /// </summary>
    public const int MaxBands = 21;

    /// <summary>
    /// Smallest MDCT used for short (transient) blocks, in 48 kHz samples.
    /// 2.5 ms × 48 kHz = 120 samples.
    /// </summary>
    public const int ShortMdctSize = 120;

    /// <summary>
    /// Length of the analysis / synthesis window overlap, in 48 kHz
    /// samples (always equal to <see cref="ShortMdctSize"/>).
    /// </summary>
    public const int OverlapSize = 120;

    /// <summary>
    /// Band edges in units of short-block (5 ms) MDCT bins. <c>EBands[i]</c>
    /// is the first bin of band <c>i</c>; <c>EBands[i+1] - EBands[i]</c>
    /// is the band's width. The table has <see cref="MaxBands"/> + 1
    /// entries so the last entry is the upper edge of the final band.
    /// </summary>
    /// <remarks>
    /// Frequencies (at 48 kHz, in Hz, for context):
    /// 0, 200, 400, 600, 800, 1000, 1200, 1400, 1600, 2000, 2400, 2800,
    /// 3200, 4000, 4800, 5600, 6800, 8000, 9600, 12000, 15600, 20000.
    /// </remarks>
    public static ReadOnlySpan<short> EBands => new short[]
    {
        0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 12, 14, 16, 20, 24, 28, 34, 40, 48, 60, 78, 100,
    };

    /// <summary>
    /// Per-band log-energy means used to bias the coarse energy decoder
    /// (RFC 6716 §4.3.2.1). 25 entries — bands beyond 21 are reserved
    /// for future Bark extensions and are referenced by the existing
    /// reference implementation.
    /// </summary>
    public static ReadOnlySpan<float> EMeans => new float[]
    {
        6.437500f, 6.250000f, 5.750000f, 5.312500f, 5.062500f, 4.812500f, 4.500000f, 4.375000f,
        4.875000f, 4.687500f, 4.562500f, 4.437500f, 4.875000f, 4.625000f, 4.312500f, 4.500000f,
        4.375000f, 4.625000f, 4.750000f, 4.437500f, 3.750000f, 3.750000f, 3.750000f, 3.750000f,
        3.750000f,
    };

    /// <summary>
    /// Bands the CELT layer covers when operating in Hybrid mode (SILK
    /// covers low bands, CELT covers from 8 kHz upward — i.e. starting at
    /// band 17 in the <see cref="EBands"/> table).
    /// </summary>
    public const int HybridStartBand = 17;

    /// <summary>
    /// Effective last band for each bandwidth (exclusive upper bound — i.e.
    /// the CELT loop runs <c>for (b = start; b &lt; end; b++)</c>).
    /// Index by <see cref="OpusBandwidth"/>:
    /// <list type="bullet">
    ///   <item><description>NB  → 13 (covers up to ~4 kHz)</description></item>
    ///   <item><description>MB  → 15 (~6 kHz, used only by SILK)</description></item>
    ///   <item><description>WB  → 17 (~8 kHz)</description></item>
    ///   <item><description>SWB → 19 (~12 kHz)</description></item>
    ///   <item><description>FB  → 21 (~20 kHz)</description></item>
    /// </list>
    /// </summary>
    public static int EndBandFor(OpusBandwidth bandwidth) => bandwidth switch
    {
        OpusBandwidth.Narrowband => 13,
        OpusBandwidth.Mediumband => 15,
        OpusBandwidth.Wideband => 17,
        OpusBandwidth.SuperWideband => 19,
        OpusBandwidth.Fullband => 21,
        _ => throw new ArgumentOutOfRangeException(nameof(bandwidth)),
    };

    // ----------------------------------------------------------------
    // Energy / flag-decode tables (RFC 6716 §4.3.2.1, libopus celt/quant_bands.c)
    // ----------------------------------------------------------------

    /// <summary>
    /// Bit-shift used to store log-energy values internally. A stored value
    /// of <c>1024</c> equals one raw log2 unit (≈6 dB). Mirrors libopus's
    /// <c>DB_SHIFT</c>.
    /// </summary>
    public const int DbShift = 10;

    /// <summary>One log2 unit, in DB_SHIFT-scaled storage units (= 1024).</summary>
    public const float DbUnit = 1 << DbShift;

    /// <summary>Lower clamp applied to the previous-frame energy before prediction.</summary>
    public const float DbMinClamp = -9.0f * DbUnit;

    /// <summary>Replacement value written when a frame is flagged as silent.</summary>
    public const float DbSilentReplacement = -28.0f * DbUnit;

    /// <summary>
    /// Inter-frame prediction coefficients (Q15) used by the coarse energy
    /// decoder when the frame is NOT marked intra. Index by
    /// <c>LM = log2(samplesPerFrame / 120)</c>.
    /// </summary>
    public static ReadOnlySpan<short> PredCoef => new short[]
    {
        29440, 26112, 21248, 16384,
    };

    /// <summary>
    /// Inter-frame energy decay coefficients (Q15) used by the coarse
    /// energy decoder when the frame is NOT marked intra.
    /// </summary>
    public static ReadOnlySpan<short> BetaCoef => new short[]
    {
        30147, 22282, 12124, 6554,
    };

    /// <summary>Energy decay coefficient (Q15) used for intra frames.</summary>
    public const short BetaIntra = 4915;

    /// <summary>ICDF for the post-filter <c>tapset</c> symbol (3 outcomes, ftb=2).</summary>
    public static ReadOnlySpan<byte> TapsetIcdf => new byte[] { 2, 1, 0 };

    /// <summary>
    /// ICDF used by the coarse energy decoder when only 2 bits of budget
    /// remain for a band. Yields a signed remainder via folded mapping.
    /// </summary>
    public static ReadOnlySpan<byte> SmallEnergyIcdf => new byte[] { 2, 1, 0 };

    /// <summary>
    /// Laplace decoder per-band (freq, decay) parameters, indexed as
    /// <c>EProbModel[LM * 84 + (intra ? 42 : 0) + 2*i + {0,1}]</c> where
    /// <c>LM ∈ [0,3]</c> and <c>i ∈ [0,21)</c>. Even slots are the base
    /// frequency (Q8, shift left by 7 before passing to Laplace), odd slots
    /// are the decay constant (Q8, shift left by 6).
    /// </summary>
    public static ReadOnlySpan<byte> EProbModel => new byte[]
    {
        // LM=0 (120 samples), intra=0
        72, 127,  65, 129,  66, 128,  65, 128,  64, 128,  62, 128,  64, 128,
        64, 128,  92,  78,  92,  79,  92,  78,  90,  79, 116,  41, 115,  40,
       114,  40, 132,  26, 132,  26, 145,  17, 161,  12, 176,  10, 177,  11,
        // LM=0, intra=1
        24, 179,  48, 138,  54, 135,  54, 132,  53, 134,  56, 133,  55, 132,
        55, 132,  61, 114,  70,  96,  74,  88,  75,  88,  87,  74,  89,  66,
        91,  67, 100,  59, 108,  50, 120,  40, 122,  37,  97,  43,  78,  50,
        // LM=1 (240 samples), intra=0
        83,  78,  84,  81,  88,  75,  86,  74,  87,  71,  90,  73,  93,  74,
        93,  74, 109,  40, 114,  36, 117,  34, 117,  34, 143,  17, 145,  18,
       146,  19, 162,  12, 165,  10, 178,   7, 189,   6, 190,   8, 177,   9,
        // LM=1, intra=1
        23, 178,  54, 115,  63, 102,  66,  98,  69,  99,  74,  89,  71,  91,
        73,  91,  78,  89,  86,  80,  92,  66,  93,  64, 102,  59, 103,  60,
       104,  60, 117,  52, 123,  44, 138,  35, 133,  31,  97,  38,  77,  45,
        // LM=2 (480 samples), intra=0
        61,  90,  93,  60, 105,  42, 107,  41, 110,  45, 116,  38, 113,  38,
       112,  38, 124,  26, 132,  27, 136,  19, 140,  20, 155,  14, 159,  16,
       158,  18, 170,  13, 177,  10, 187,   8, 192,   6, 175,   9, 159,  10,
        // LM=2, intra=1
        21, 178,  59, 110,  71,  86,  75,  85,  84,  83,  91,  66,  88,  73,
        87,  72,  92,  75,  98,  72, 105,  58, 107,  54, 115,  52, 114,  55,
       112,  56, 129,  51, 132,  40, 150,  33, 140,  29,  98,  35,  77,  42,
        // LM=3 (960 samples), intra=0
        42, 121,  96,  66, 108,  43, 111,  40, 117,  44, 123,  32, 120,  36,
       119,  33, 127,  33, 134,  34, 139,  21, 147,  23, 152,  20, 158,  25,
       154,  26, 166,  21, 173,  16, 184,  13, 184,  10, 150,  13, 139,  15,
        // LM=3, intra=1
        22, 178,  63, 114,  74,  82,  84,  83,  92,  82, 103,  62,  96,  72,
        96,  67, 101,  73, 107,  72, 113,  55, 118,  52, 125,  52, 118,  52,
       117,  55, 135,  49, 137,  39, 157,  32, 145,  29,  97,  33,  77,  40,
    };

    /// <summary>
    /// Compute the CELT layer-mode index <c>LM</c> from the per-frame
    /// sample count. LM = log2(samplesPerFrame / 120).
    /// </summary>
    public static int LmFor(int samplesPerFrame) => samplesPerFrame switch
    {
        120 => 0,
        240 => 1,
        480 => 2,
        960 => 3,
        _ => throw new ArgumentOutOfRangeException(nameof(samplesPerFrame),
            "CELT frame must be 120, 240, 480, or 960 samples (2.5/5/10/20 ms at 48 kHz)."),
    };
}
