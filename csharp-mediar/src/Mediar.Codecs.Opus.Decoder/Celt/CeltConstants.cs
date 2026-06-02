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
}
