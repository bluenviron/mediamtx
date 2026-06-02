namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Derived band layout for one CELT decode invocation. CELT's algorithmic
/// parameters are fully determined by the active <see cref="OpusBandwidth"/>,
/// the frame duration, and whether the frame is being decoded by CELT
/// alone or by CELT as the high-frequency partner in a hybrid frame.
/// </summary>
public readonly record struct CeltMode
{
    /// <summary>Total number of samples produced per audio frame at 48 kHz.</summary>
    public int SamplesPerFrame { get; init; }

    /// <summary>
    /// Number of short (2.5 ms) MDCTs that make up one decode invocation
    /// when the frame is marked as transient. For non-transient frames a
    /// single long MDCT of length <see cref="SamplesPerFrame"/> is used.
    /// </summary>
    public int ShortBlocksPerFrame { get; init; }

    /// <summary>
    /// Index (inclusive) of the first band CELT codes for this frame.
    /// 0 for CELT-only, <see cref="CeltConstants.HybridStartBand"/> for
    /// Hybrid mode (the lower bands are handled by SILK).
    /// </summary>
    public int StartBand { get; init; }

    /// <summary>
    /// Index (exclusive) of the last band CELT codes for this frame.
    /// </summary>
    public int EndBand { get; init; }

    /// <summary>
    /// True when CELT runs as the high-band partner in a Hybrid frame.
    /// </summary>
    public bool IsHybrid { get; init; }

    /// <summary>
    /// Number of effective bands this mode covers
    /// (<see cref="EndBand"/> − <see cref="StartBand"/>).
    /// </summary>
    public int BandCount => EndBand - StartBand;

    /// <summary>
    /// Band edges (one entry per band boundary, so length =
    /// <see cref="CeltConstants.MaxBands"/> + 1) in units of short-block
    /// (5 ms) MDCT bins. Multiply by <see cref="SamplesPerFrame"/> / 120
    /// to convert to bins at the current frame's long-MDCT size.
    /// </summary>
    public ReadOnlySpan<short> EBands => CeltConstants.EBands;

    /// <summary>
    /// Convert a 5 ms band-edge value to bins at this mode's long-MDCT
    /// size.
    /// </summary>
    public int BinsAtLongBlock(int eBandEntry)
        => eBandEntry * (SamplesPerFrame / CeltConstants.ShortMdctSize);

    /// <summary>
    /// Build a mode descriptor for a CELT-only frame (RFC 6716 Table 2
    /// configs 16..31).
    /// </summary>
    public static CeltMode ForCeltOnly(OpusBandwidth bandwidth, int frameSizeMicroseconds)
    {
        ValidateCeltOnlyBandwidth(bandwidth);
        return new CeltMode
        {
            SamplesPerFrame = SamplesFor(frameSizeMicroseconds),
            ShortBlocksPerFrame = ShortBlocksFor(frameSizeMicroseconds),
            StartBand = 0,
            EndBand = CeltConstants.EndBandFor(bandwidth),
            IsHybrid = false,
        };
    }

    /// <summary>
    /// Build a mode descriptor for the CELT half of a Hybrid frame
    /// (configs 12..15 — always SWB or FB, always 10 ms or 20 ms).
    /// </summary>
    public static CeltMode ForHybrid(int frameSizeMicroseconds)
    {
        if (frameSizeMicroseconds is not (10_000 or 20_000))
            throw new ArgumentException("Hybrid mode is only defined for 10 ms or 20 ms frames.", nameof(frameSizeMicroseconds));

        return new CeltMode
        {
            SamplesPerFrame = SamplesFor(frameSizeMicroseconds),
            ShortBlocksPerFrame = ShortBlocksFor(frameSizeMicroseconds),
            StartBand = CeltConstants.HybridStartBand,
            EndBand = CeltConstants.MaxBands,
            IsHybrid = true,
        };
    }

    private static int SamplesFor(int frameSizeMicroseconds) => frameSizeMicroseconds switch
    {
        2_500 => 120,
        5_000 => 240,
        10_000 => 480,
        20_000 => 960,
        _ => throw new ArgumentOutOfRangeException(nameof(frameSizeMicroseconds),
            "CELT frame size must be 2.5, 5, 10 or 20 milliseconds."),
    };

    private static int ShortBlocksFor(int frameSizeMicroseconds) => frameSizeMicroseconds switch
    {
        2_500 => 1,
        5_000 => 2,
        10_000 => 4,
        20_000 => 8,
        _ => throw new ArgumentOutOfRangeException(nameof(frameSizeMicroseconds)),
    };

    private static void ValidateCeltOnlyBandwidth(OpusBandwidth bandwidth)
    {
        if (bandwidth is OpusBandwidth.Mediumband)
        {
            throw new ArgumentException(
                "CELT-only mode does not support Mediumband — that's a SILK-only configuration.",
                nameof(bandwidth));
        }
    }
}
