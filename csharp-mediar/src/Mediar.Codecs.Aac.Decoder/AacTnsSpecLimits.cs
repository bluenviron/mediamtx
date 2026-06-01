namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Canonical limits applied by AAC Temporal Noise Shaping per
/// ISO/IEC 14496-3 §4.6.9: the maximum scalefactor band that may
/// carry a TNS filter (<c>tns_max_sfb</c>) and the maximum LPC
/// order a filter may declare (<c>tns_max_order</c>).
/// </summary>
/// <remarks>
/// <para>
/// The SFB-bound tables mirror FFmpeg's
/// <c>ff_tns_max_bands_1024</c>, <c>ff_tns_max_bands_128</c>,
/// <c>ff_tns_max_bands_512</c> and <c>ff_tns_max_bands_480</c>
/// (<c>libavcodec/aactab.c</c>), which in turn implement
/// Table 4.A.10 of the spec. Each table is indexed by the 4-bit
/// MPEG-4 <c>samplingFrequencyIndex</c> (0..12); the inline-rate
/// escape (15) and reserved slots (13, 14) are not supported here
/// and must be resolved by the caller.
/// </para>
/// <para>
/// The <see cref="GetMaxBands"/> and <see cref="GetMaxOrder"/>
/// convenience accessors are limited to the MPEG-4 AAC variants
/// that share the 1024-line long / 128-line short tiling
/// (<see cref="AacAudioObjectType.AacMain"/>,
/// <see cref="AacAudioObjectType.AacLc"/>,
/// <see cref="AacAudioObjectType.AacLtp"/>,
/// <see cref="AacAudioObjectType.ErAacLc"/>). Other object types
/// (AAC SSR uses 256-line short subwindows; AAC LD/ELD uses 480/512
/// long frames; USAC uses different rules entirely) require the
/// explicit per-frame-length accessors plus caller-supplied order
/// logic and will throw if passed to the convenience methods.
/// </para>
/// </remarks>
public static class AacTnsSpecLimits
{
    /// <summary>
    /// Number of MPEG-4 sampling-frequency-index slots covered by the
    /// SFB tables (0..12 inclusive).
    /// </summary>
    public const int SampleRateIndexCount = 13;

    /// <summary>
    /// Maximum LPC order accepted for an EIGHT_SHORT TNS filter
    /// across every supported MPEG-4 AAC variant (ISO/IEC 14496-3
    /// Table 4.156).
    /// </summary>
    public const int MaxOrderShort = 7;

    /// <summary>
    /// Maximum LPC order accepted for a long-window TNS filter under
    /// the AAC Main profile (ISO/IEC 14496-3 Table 4.156).
    /// </summary>
    public const int MaxOrderLongMain = 20;

    /// <summary>
    /// Maximum LPC order accepted for a long-window TNS filter under
    /// non-Main MPEG-4 AAC profiles such as LC, LTP, ER-AAC-LC.
    /// </summary>
    public const int MaxOrderLongOther = 12;

    private static readonly byte[] MaxBandsLong1024 =
    [
        31, 31, 34, 40, 42, 51, 46, 46, 42, 42, 42, 39, 39,
    ];

    private static readonly byte[] MaxBandsShort128 =
    [
        9, 9, 10, 14, 14, 14, 14, 14, 14, 14, 14, 14, 14,
    ];

    private static readonly byte[] MaxBandsLong512 =
    [
        0, 0, 0, 31, 32, 37, 31, 31, 0, 0, 0, 0, 0,
    ];

    private static readonly byte[] MaxBandsLong480 =
    [
        0, 0, 0, 31, 32, 37, 30, 30, 0, 0, 0, 0, 0,
    ];

    static AacTnsSpecLimits()
    {
        if (MaxBandsLong1024.Length != SampleRateIndexCount ||
            MaxBandsShort128.Length != SampleRateIndexCount ||
            MaxBandsLong512.Length != SampleRateIndexCount ||
            MaxBandsLong480.Length != SampleRateIndexCount)
        {
            throw new InvalidOperationException(
                "AacTnsSpecLimits tables must contain exactly " +
                SampleRateIndexCount + " entries.");
        }
    }

    /// <summary>
    /// Maximum SFB that may carry a TNS filter for a long
    /// (1024-line) MPEG-4 AAC window at the given sample rate.
    /// </summary>
    /// <param name="samplingFrequencyIndex">
    /// MPEG-4 4-bit sampling-frequency-index in [0..12]. The escape
    /// (15) and reserved slots (13, 14) are not accepted and must
    /// be resolved by the caller.
    /// </param>
    public static int GetMaxBandsLong1024(int samplingFrequencyIndex)
    {
        EnsureSampleRateIndex(samplingFrequencyIndex);
        return MaxBandsLong1024[samplingFrequencyIndex];
    }

    /// <summary>
    /// Maximum SFB that may carry a TNS filter for an 8-window
    /// EIGHT_SHORT (128-line) MPEG-4 AAC sequence.
    /// </summary>
    public static int GetMaxBandsShort128(int samplingFrequencyIndex)
    {
        EnsureSampleRateIndex(samplingFrequencyIndex);
        return MaxBandsShort128[samplingFrequencyIndex];
    }

    /// <summary>
    /// Maximum SFB that may carry a TNS filter for an AAC LD/ELD
    /// 512-sample frame. Returns 0 at sample rates the LD profile
    /// does not target (≤32 kHz only).
    /// </summary>
    public static int GetMaxBandsLong512(int samplingFrequencyIndex)
    {
        EnsureSampleRateIndex(samplingFrequencyIndex);
        return MaxBandsLong512[samplingFrequencyIndex];
    }

    /// <summary>
    /// Maximum SFB that may carry a TNS filter for an AAC LD/ELD
    /// 480-sample frame. Returns 0 at sample rates the LD profile
    /// does not target.
    /// </summary>
    public static int GetMaxBandsLong480(int samplingFrequencyIndex)
    {
        EnsureSampleRateIndex(samplingFrequencyIndex);
        return MaxBandsLong480[samplingFrequencyIndex];
    }

    /// <summary>
    /// Look up <c>tns_max_sfb</c> for one of the MPEG-4 AAC variants
    /// that share the 1024/128 tiling
    /// (<see cref="AacAudioObjectType.AacMain"/>,
    /// <see cref="AacAudioObjectType.AacLc"/>,
    /// <see cref="AacAudioObjectType.AacLtp"/>,
    /// <see cref="AacAudioObjectType.ErAacLc"/>). Any window
    /// sequence other than <see cref="AacWindowSequence.EightShort"/>
    /// is treated as a long-window case (i.e. <c>LongStart</c> and
    /// <c>LongStop</c> route to the 1024 table).
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="objectType"/> is not one of the four supported
    /// variants, or <paramref name="samplingFrequencyIndex"/> is
    /// outside [0..12], or <paramref name="windowSequence"/> is an
    /// undefined enum value.
    /// </exception>
    public static int GetMaxBands(
        AacAudioObjectType objectType,
        int samplingFrequencyIndex,
        AacWindowSequence windowSequence)
    {
        EnsureSupportedObjectType(objectType, nameof(objectType));
        EnsureKnownWindowSequence(windowSequence);
        return windowSequence == AacWindowSequence.EightShort
            ? GetMaxBandsShort128(samplingFrequencyIndex)
            : GetMaxBandsLong1024(samplingFrequencyIndex);
    }

    /// <summary>
    /// Look up <c>tns_max_order</c> for the supported MPEG-4 AAC
    /// variants (Main / LC / LTP / ER-AAC-LC).
    /// EIGHT_SHORT → <see cref="MaxOrderShort"/>; long window under
    /// Main → <see cref="MaxOrderLongMain"/>; long window under any
    /// other supported variant → <see cref="MaxOrderLongOther"/>.
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="objectType"/> is not one of the four supported
    /// variants, or <paramref name="windowSequence"/> is an undefined
    /// enum value.
    /// </exception>
    public static int GetMaxOrder(
        AacAudioObjectType objectType,
        AacWindowSequence windowSequence)
    {
        EnsureSupportedObjectType(objectType, nameof(objectType));
        EnsureKnownWindowSequence(windowSequence);
        if (windowSequence == AacWindowSequence.EightShort)
            return MaxOrderShort;
        return objectType == AacAudioObjectType.AacMain
            ? MaxOrderLongMain
            : MaxOrderLongOther;
    }

    private static void EnsureSampleRateIndex(int samplingFrequencyIndex)
    {
        if ((uint)samplingFrequencyIndex >= (uint)SampleRateIndexCount)
        {
            throw new ArgumentOutOfRangeException(
                nameof(samplingFrequencyIndex),
                samplingFrequencyIndex,
                "samplingFrequencyIndex must be in [0.." +
                (SampleRateIndexCount - 1) +
                "]. The escape value (15) must be resolved by the caller.");
        }
    }

    private static void EnsureKnownWindowSequence(AacWindowSequence windowSequence)
    {
        switch (windowSequence)
        {
            case AacWindowSequence.OnlyLong:
            case AacWindowSequence.LongStart:
            case AacWindowSequence.EightShort:
            case AacWindowSequence.LongStop:
                return;
            default:
                throw new ArgumentOutOfRangeException(
                    nameof(windowSequence),
                    windowSequence,
                    "Unknown AacWindowSequence value.");
        }
    }

    private static void EnsureSupportedObjectType(
        AacAudioObjectType objectType, string paramName)
    {
        switch (objectType)
        {
            case AacAudioObjectType.AacMain:
            case AacAudioObjectType.AacLc:
            case AacAudioObjectType.AacLtp:
            case AacAudioObjectType.ErAacLc:
                return;
            default:
                throw new ArgumentOutOfRangeException(
                    paramName,
                    objectType,
                    "AacTnsSpecLimits convenience accessors only cover " +
                    "AacMain, AacLc, AacLtp and ErAacLc. AAC SSR uses " +
                    "256-line short subwindows; AAC LD/ELD uses 480/512 " +
                    "long frames; USAC has different rules entirely. " +
                    "Use the per-frame-length accessors for those.");
        }
    }
}
