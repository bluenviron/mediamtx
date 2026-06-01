namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Scale-factor-band (SWB) offset tables for AAC long (1024-sample)
/// and short (128-sample) MDCT windows per ISO/IEC 14496-3 Annex 4.A
/// (Tables 4.130 - 4.146). Each table maps SWB index → first
/// spectral coefficient in that band; the closing entry equals the
/// transform length (1024 or 128) so adjacent differences give the
/// SWB width in coefficients.
/// </summary>
/// <remarks>
/// <para>
/// The tables are dispatched by sample rate, with several sample
/// rates sharing a table:
/// </para>
/// <list type="table">
/// <listheader><term>Sample rate (Hz)</term><description>Long table / Short table</description></listheader>
/// <item><term>96 000, 88 200</term>             <description>swb_offset_1024_96 / swb_offset_128_96</description></item>
/// <item><term>64 000</term>                     <description>swb_offset_1024_64 / swb_offset_128_64</description></item>
/// <item><term>48 000, 44 100</term>             <description>swb_offset_1024_48 / swb_offset_128_48</description></item>
/// <item><term>32 000</term>                     <description>swb_offset_1024_32 / swb_offset_128_48</description></item>
/// <item><term>24 000, 22 050</term>             <description>swb_offset_1024_24 / swb_offset_128_24</description></item>
/// <item><term>16 000, 12 000, 11 025</term>     <description>swb_offset_1024_16 / swb_offset_128_16</description></item>
/// <item><term>8 000, 7 350</term>               <description>swb_offset_1024_8  / swb_offset_128_8</description></item>
/// </list>
/// <para>
/// The returned spans always close with the transform length, so
/// the SWB count is the span length minus one. Lookups accept either
/// the raw sample rate in Hz or the 4-bit
/// <c>samplingFrequencyIndex</c> from <c>AudioSpecificConfig</c>.
/// </para>
/// <para>
/// These tables apply to AOT 2 (AAC-LC) and equivalent low-complexity
/// AOTs that use the 1024 / 128 transform pair. Long-term-prediction
/// (LTP) and 960 / 120 transform variants reuse the SWB counts and a
/// rescaled offset table that the spec encodes via the same
/// underlying SWB structure; this class exposes only the canonical
/// 1024 / 128 tables and leaves the 960 / 120 derivation to the
/// caller.
/// </para>
/// </remarks>
public static class AacSwbOffsets
{
    /// <summary>Coefficient count covered by one long block (samples per frame).</summary>
    public const int LongTransformLength = 1024;

    /// <summary>Coefficient count covered by one short block (samples per short window).</summary>
    public const int ShortTransformLength = 128;

    private static readonly int[] Long96 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,
           32,   36,   40,   44,   48,   52,   56,   64,
           72,   80,   88,   96,  108,  120,  132,  144,
          156,  172,  188,  212,  240,  276,  320,  384,
          448,  512,  576,  640,  704,  768,  832,  896,
          960, 1024,
    ];

    private static readonly int[] Long64 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,
           32,   36,   40,   44,   48,   52,   56,   64,
           72,   80,   88,  100,  112,  124,  140,  156,
          172,  192,  216,  240,  268,  304,  344,  384,
          424,  464,  504,  544,  584,  624,  664,  704,
          744,  784,  824,  864,  904,  944,  984, 1024,
    ];

    private static readonly int[] Long48 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,
           32,   36,   40,   48,   56,   64,   72,   80,
           88,   96,  108,  120,  132,  144,  160,  176,
          196,  216,  240,  264,  292,  320,  352,  384,
          416,  448,  480,  512,  544,  576,  608,  640,
          672,  704,  736,  768,  800,  832,  864,  896,
          928, 1024,
    ];

    private static readonly int[] Long32 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,
           32,   36,   40,   48,   56,   64,   72,   80,
           88,   96,  108,  120,  132,  144,  160,  176,
          196,  216,  240,  264,  292,  320,  352,  384,
          416,  448,  480,  512,  544,  576,  608,  640,
          672,  704,  736,  768,  800,  832,  864,  896,
          928,  960,  992, 1024,
    ];

    private static readonly int[] Long24 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,
           32,   36,   40,   44,   52,   60,   68,   76,
           84,   92,  100,  108,  116,  124,  136,  148,
          160,  172,  188,  204,  220,  240,  260,  284,
          308,  336,  364,  396,  432,  468,  508,  552,
          600,  652,  704,  768,  832,  896,  960, 1024,
    ];

    private static readonly int[] Long16 =
    [
            0,    8,   16,   24,   32,   40,   48,   56,
           64,   72,   80,   88,  100,  112,  124,  136,
          148,  160,  172,  184,  196,  212,  228,  244,
          260,  280,  300,  320,  344,  368,  396,  424,
          456,  492,  532,  572,  616,  664,  716,  772,
          832,  896,  960, 1024,
    ];

    private static readonly int[] Long8 =
    [
            0,   12,   24,   36,   48,   60,   72,   84,
           96,  108,  120,  132,  144,  156,  172,  188,
          204,  220,  236,  252,  268,  288,  308,  328,
          348,  372,  396,  420,  448,  476,  508,  544,
          580,  620,  664,  712,  764,  820,  880,  944,
         1024,
    ];

    private static readonly int[] Short96 =
    [
            0,    4,    8,   12,   16,   20,   24,   32,   40,   48,   64,   92,  128,
    ];

    private static readonly int[] Short64 =
    [
            0,    4,    8,   12,   16,   20,   24,   32,   40,   48,   64,   92,  128,
    ];

    private static readonly int[] Short48 =
    [
            0,    4,    8,   12,   16,   20,   28,   36,   44,   56,   68,   80,   96,  112,  128,
    ];

    private static readonly int[] Short24 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,   36,   44,   52,   64,   76,   92,  108,  128,
    ];

    private static readonly int[] Short16 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,   32,   40,   48,   60,   72,   88,  108,  128,
    ];

    private static readonly int[] Short8 =
    [
            0,    4,    8,   12,   16,   20,   24,   28,   36,   44,   52,   60,   72,   88,  108,  128,
    ];

    /// <summary>
    /// Returns the long-block (1024-sample MDCT) SWB offset table for
    /// <paramref name="sampleRate"/>. The returned span always closes
    /// with <see cref="LongTransformLength"/>; the SWB count is
    /// <c>span.Length - 1</c>.
    /// </summary>
    /// <returns>
    /// The SWB offset span on success, or an empty span when
    /// <paramref name="sampleRate"/> does not match any AAC-LC
    /// sample-frequency-index entry.
    /// </returns>
    public static ReadOnlySpan<int> GetLongOffsets(int sampleRate) => sampleRate switch
    {
        96_000 or 88_200            => Long96,
        64_000                      => Long64,
        48_000 or 44_100            => Long48,
        32_000                      => Long32,
        24_000 or 22_050            => Long24,
        16_000 or 12_000 or 11_025  => Long16,
        8_000  or 7_350             => Long8,
        _                           => ReadOnlySpan<int>.Empty,
    };

    /// <summary>
    /// Returns the short-block (128-sample MDCT) SWB offset table for
    /// <paramref name="sampleRate"/>. The returned span always closes
    /// with <see cref="ShortTransformLength"/>; the SWB count is
    /// <c>span.Length - 1</c>.
    /// </summary>
    public static ReadOnlySpan<int> GetShortOffsets(int sampleRate) => sampleRate switch
    {
        96_000 or 88_200 or 64_000              => Short96,
        48_000 or 44_100 or 32_000              => Short48,
        24_000 or 22_050                        => Short24,
        16_000 or 12_000 or 11_025              => Short16,
        8_000  or 7_350                         => Short8,
        _                                       => ReadOnlySpan<int>.Empty,
    };

    /// <summary>
    /// Returns the long-block SWB offset table for the 4-bit
    /// <c>samplingFrequencyIndex</c> field from
    /// <c>AudioSpecificConfig</c> (per <see cref="AacSampleRates"/>).
    /// </summary>
    public static ReadOnlySpan<int> GetLongOffsetsForIndex(int samplingFrequencyIndex) =>
        GetLongOffsets(AacSampleRates.FromIndex(samplingFrequencyIndex));

    /// <summary>
    /// Returns the short-block SWB offset table for the 4-bit
    /// <c>samplingFrequencyIndex</c> field from
    /// <c>AudioSpecificConfig</c> (per <see cref="AacSampleRates"/>).
    /// </summary>
    public static ReadOnlySpan<int> GetShortOffsetsForIndex(int samplingFrequencyIndex) =>
        GetShortOffsets(AacSampleRates.FromIndex(samplingFrequencyIndex));

    /// <summary>
    /// Number of long-block scale factor bands for
    /// <paramref name="sampleRate"/>. Equals <c>span.Length - 1</c>
    /// from <see cref="GetLongOffsets(int)"/>; returns 0 for unknown
    /// sample rates.
    /// </summary>
    public static int GetNumSwbLong(int sampleRate)
    {
        var span = GetLongOffsets(sampleRate);
        return span.IsEmpty ? 0 : span.Length - 1;
    }

    /// <summary>
    /// Number of short-block scale factor bands for
    /// <paramref name="sampleRate"/>. Equals <c>span.Length - 1</c>
    /// from <see cref="GetShortOffsets(int)"/>; returns 0 for unknown
    /// sample rates.
    /// </summary>
    public static int GetNumSwbShort(int sampleRate)
    {
        var span = GetShortOffsets(sampleRate);
        return span.IsEmpty ? 0 : span.Length - 1;
    }

    /// <summary>
    /// Width (in spectral coefficients) of long-block scale factor
    /// band <paramref name="swb"/> for <paramref name="sampleRate"/>.
    /// </summary>
    /// <returns>
    /// The width on success, or 0 when <paramref name="sampleRate"/>
    /// is unknown or <paramref name="swb"/> is out of range.
    /// </returns>
    public static int GetLongSwbWidth(int sampleRate, int swb)
    {
        var span = GetLongOffsets(sampleRate);
        if (span.IsEmpty || (uint)swb >= (uint)(span.Length - 1)) return 0;
        return span[swb + 1] - span[swb];
    }

    /// <summary>
    /// Width (in spectral coefficients) of short-block scale factor
    /// band <paramref name="swb"/> for <paramref name="sampleRate"/>.
    /// </summary>
    /// <returns>
    /// The width on success, or 0 when <paramref name="sampleRate"/>
    /// is unknown or <paramref name="swb"/> is out of range.
    /// </returns>
    public static int GetShortSwbWidth(int sampleRate, int swb)
    {
        var span = GetShortOffsets(sampleRate);
        if (span.IsEmpty || (uint)swb >= (uint)(span.Length - 1)) return 0;
        return span[swb + 1] - span[swb];
    }
}
