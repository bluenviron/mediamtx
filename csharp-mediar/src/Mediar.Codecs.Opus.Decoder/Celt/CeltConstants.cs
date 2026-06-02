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

    // ----------------------------------------------------------------
    // Time-frequency resolution + spread tables (RFC 6716 §4.3.4)
    // ----------------------------------------------------------------

    /// <summary>
    /// TF resolution adjustment lookup. Indexed as
    /// <c>TfSelectTable[LM][4*isTransient + 2*tf_select + tf_changed]</c>.
    /// Values are signed; the result is added to the per-band MDCT layer
    /// offset during synthesis (Phase 2d).
    /// </summary>
    public static ReadOnlySpan<sbyte> TfSelectTable => new sbyte[]
    {
        0, -1, 0, -1,  0, -1, 0, -1, // LM = 0 (2.5 ms)
        0, -1, 0, -2,  1,  0, 1, -1, // LM = 1 (5 ms)
        0, -2, 0, -3,  2,  0, 1, -1, // LM = 2 (10 ms)
        0, -2, 0, -3,  3,  0, 1, -1, // LM = 3 (20 ms)
    };

    /// <summary>
    /// ICDF for the CELT <c>spread</c> symbol — selects how aggressively
    /// the PVQ decoder spreads pulses across band bins. 4 outcomes
    /// (NONE=0, LIGHT=1, NORMAL=2, AGGRESSIVE=3); ftb = 5.
    /// </summary>
    public static ReadOnlySpan<byte> SpreadIcdf => new byte[] { 25, 23, 2, 0 };

    /// <summary>
    /// Spread modes selected by <see cref="SpreadIcdf"/>. Values match
    /// libopus <c>SPREAD_NONE</c> / <c>SPREAD_LIGHT</c> /
    /// <c>SPREAD_NORMAL</c> / <c>SPREAD_AGGRESSIVE</c>.
    /// </summary>
    public const int SpreadNone = 0;
    /// <inheritdoc cref="SpreadNone"/>
    public const int SpreadLight = 1;
    /// <inheritdoc cref="SpreadNone"/>
    public const int SpreadNormal = 2;
    /// <inheritdoc cref="SpreadNone"/>
    public const int SpreadAggressive = 3;

    // ----------------------------------------------------------------
    // Bit allocation tables (RFC 6716 §4.3.3)
    // ----------------------------------------------------------------

    /// <summary>
    /// Fractional-bit unit shift used throughout the CELT bit allocator.
    /// libopus refers to this as <c>BITRES</c>. One whole bit equals
    /// <c>1 &lt;&lt; BitRes = 8</c> fractional-bit units.
    /// </summary>
    public const int BitRes = 3;

    /// <summary>
    /// Default <c>alloc_trim</c> value returned when the bit budget does
    /// not admit the 6-bit symbol. Index into the 11-entry trim table —
    /// libopus comment: "right in the middle so we don't over-correct".
    /// </summary>
    public const int AllocTrimDefault = 5;

    /// <summary>
    /// Initial value of <c>dynalloc_logp</c> at the start of the dyn_alloc
    /// loop. Decremented by 1 (clamped at 2) for every band that consumed
    /// any boost bits, so subsequent bands become cheaper to boost.
    /// </summary>
    public const int DynAllocLogPStart = 6;

    /// <summary>
    /// ICDF for <c>alloc_trim</c>. 11 outcomes, ftb = 7. Selects the
    /// global trim that biases bit allocation towards low or high bands.
    /// </summary>
    public static ReadOnlySpan<byte> AllocTrimIcdf => new byte[]
    {
        126, 124, 119, 109, 87, 41, 19, 9, 4, 2, 0,
    };

    /// <summary>
    /// Per-band pulse-cache caps from libopus <c>cache_caps50[168]</c>.
    /// Indexed as <c>CacheCaps50[MaxBands * (2*LM + (channels-1)) + band]</c>.
    /// Stored as unsigned bytes in 1/32-bit-per-sample units; the actual
    /// per-band cap (in fractional bits) is
    /// <c>cap[i] = (CacheCaps50[idx] + 64) * C * N &gt;&gt; 2</c>
    /// where <c>N = (eBands[i+1] - eBands[i]) &lt;&lt; LM</c>.
    /// </summary>
    public static ReadOnlySpan<byte> CacheCaps50 => new byte[]
    {
        224, 224, 224, 224, 224, 224, 224, 224, 160, 160, 160, 160, 185, 185, 185,
        178, 178, 168, 134,  61,  37, 224, 224, 224, 224, 224, 224, 224, 224, 240,
        240, 240, 240, 207, 207, 207, 198, 198, 183, 144,  66,  40, 160, 160, 160,
        160, 160, 160, 160, 160, 185, 185, 185, 185, 193, 193, 193, 183, 183, 172,
        138,  64,  38, 240, 240, 240, 240, 240, 240, 240, 240, 207, 207, 207, 207,
        204, 204, 204, 193, 193, 180, 143,  66,  40, 185, 185, 185, 185, 185, 185,
        185, 185, 193, 193, 193, 193, 193, 193, 193, 183, 183, 172, 138,  65,  39,
        207, 207, 207, 207, 207, 207, 207, 207, 204, 204, 204, 204, 201, 201, 201,
        188, 188, 176, 141,  66,  40, 193, 193, 193, 193, 193, 193, 193, 193, 193,
        193, 193, 193, 194, 194, 194, 184, 184, 173, 139,  65,  39, 204, 204, 204,
        204, 204, 204, 204, 204, 201, 201, 201, 201, 198, 198, 198, 187, 187, 175,
        140,  66,  40,
    };

    /// <summary>
    /// Number of allocation hypothesis vectors in <see cref="BandAllocation"/>.
    /// Matches libopus <c>BITALLOC_SIZE</c>.
    /// </summary>
    public const int NbAllocVectors = 11;

    /// <summary>
    /// Bit-budget binary-search bisection depth used by
    /// <c>interp_bits2pulses</c>. libopus <c>ALLOC_STEPS</c>.
    /// </summary>
    public const int AllocSteps = 6;

    /// <summary>libopus <c>FINE_OFFSET</c> — offset for fine bit allocation.</summary>
    public const int FineOffset = 21;

    /// <summary>libopus <c>MAX_FINE_BITS</c> — fine bits never exceed 8.</summary>
    public const int MaxFineBits = 8;

    /// <summary>libopus <c>QTHETA_OFFSET</c>.</summary>
    public const int QThetaOffset = 4;

    /// <summary>libopus <c>QTHETA_OFFSET_TWOPHASE</c> — special offset
    /// applied to the stereo split when <c>N==2</c>.</summary>
    public const int QThetaOffsetTwoPhase = 16;

    /// <summary>
    /// Bit allocation table — 11 quality hypotheses × 21 bands, in units of
    /// 1/32 bit per sample (0.1875 dB SNR). Indexed as
    /// <c>BandAllocation[quality * MaxBands + band]</c>. The binary search
    /// in <c>compute_allocation</c> picks the highest quality level that
    /// still fits in the available bit budget.
    /// </summary>
    public static ReadOnlySpan<byte> BandAllocation => new byte[]
    {
        // 0   200  400  600  800  1k  1.2  1.4  1.6  2k  2.4  2.8  3.2  4k  4.8  5.6  6.8  8k  9.6  12k 15.6
          0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,   0,
         90,  80,  75,  69,  63,  56,  49,  40,  34,  29,  20,  18,  10,   0,   0,   0,   0,   0,   0,   0,   0,
        110, 100,  90,  84,  78,  71,  65,  58,  51,  45,  39,  32,  26,  20,  12,   0,   0,   0,   0,   0,   0,
        118, 110, 103,  93,  86,  80,  75,  70,  65,  59,  53,  47,  40,  31,  23,  15,   4,   0,   0,   0,   0,
        126, 119, 112, 104,  95,  89,  83,  78,  72,  66,  60,  54,  47,  39,  32,  25,  17,  12,   1,   0,   0,
        134, 127, 120, 114, 103,  97,  91,  85,  78,  72,  66,  60,  54,  47,  41,  35,  29,  23,  16,  10,   1,
        144, 137, 130, 124, 113, 107, 101,  95,  88,  82,  76,  70,  64,  57,  51,  45,  39,  33,  26,  15,   1,
        152, 145, 138, 132, 123, 117, 111, 105,  98,  92,  86,  80,  74,  67,  61,  55,  49,  43,  36,  20,   1,
        162, 155, 148, 142, 133, 127, 121, 115, 108, 102,  96,  90,  84,  77,  71,  65,  59,  53,  46,  30,   1,
        172, 165, 158, 152, 143, 137, 131, 125, 118, 112, 106, 100,  94,  87,  81,  75,  69,  63,  56,  45,  20,
        200, 200, 200, 200, 200, 200, 200, 200, 198, 193, 188, 183, 178, 173, 168, 163, 158, 153, 148, 129, 104,
    };

    /// <summary>
    /// Per-band log2 of the band width in fractional bits — libopus
    /// <c>logN400</c>. Used by the fine-bit allocator inside
    /// <c>interp_bits2pulses</c>.
    /// </summary>
    public static ReadOnlySpan<short> LogN400 => new short[]
    {
        0, 0, 0, 0, 0, 0, 0, 0, 8, 8, 8, 8, 16, 16, 16, 21, 21, 24, 29, 34, 36,
    };

    /// <summary>
    /// libopus <c>LOG2_FRAC_TABLE</c> — encodes
    /// <c>round(log2(i+1) * 8)</c> for i in 0..23. Used to size the
    /// intensity-stereo reservation in <c>compute_allocation</c>.
    /// </summary>
    public static ReadOnlySpan<byte> Log2FracTable => new byte[]
    {
         0,
         8, 13,
        16, 19, 21, 23,
        24, 26, 27, 28, 29, 30, 31, 32,
        32, 33, 34, 34, 35, 36, 36, 37, 37,
    };
}
