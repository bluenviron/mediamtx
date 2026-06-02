namespace Mediar.Codecs.Opus.Decoder;

/// <summary>Which Opus sub-codec encoded a frame.</summary>
public enum OpusMode
{
    /// <summary>SILK-only (LP-based, optimised for speech at low bitrate).</summary>
    SilkOnly,
    /// <summary>Hybrid: SILK below 8 kHz plus CELT for the high band.</summary>
    Hybrid,
    /// <summary>CELT-only (MDCT-based, optimised for music).</summary>
    CeltOnly,
}

/// <summary>
/// Audio bandwidth as enumerated by RFC 6716 §2. Numeric values match
/// libopus's <c>OPUS_BANDWIDTH_*</c> ordering so they can be ordered by
/// <see cref="System.IComparable"/>.
/// </summary>
public enum OpusBandwidth
{
    /// <summary>4 kHz cutoff (8 kHz sample rate).</summary>
    Narrowband = 0,
    /// <summary>6 kHz cutoff (12 kHz sample rate).</summary>
    Mediumband = 1,
    /// <summary>8 kHz cutoff (16 kHz sample rate).</summary>
    Wideband = 2,
    /// <summary>12 kHz cutoff (24 kHz sample rate).</summary>
    SuperWideband = 3,
    /// <summary>20 kHz cutoff (48 kHz sample rate).</summary>
    Fullband = 4,
}

/// <summary>
/// Decoded contents of the 1-byte Opus Table-Of-Contents header (RFC 6716 §3.1).
///
/// <para>
/// Layout (most-significant bit first):
/// </para>
/// <code>
///  0 1 2 3 4 5 6 7
/// +-+-+-+-+-+-+-+-+
/// |  config |s| c |
/// +-+-+-+-+-+-+-+-+
/// </code>
///
/// <list type="bullet">
///   <item><description><c>config</c> (bits 7..3): selects <see cref="Mode"/>, <see cref="Bandwidth"/>, and <see cref="FrameSizeMicroseconds"/> per Table 2.</description></item>
///   <item><description><c>s</c> (bit 2): 0 = mono, 1 = stereo.</description></item>
///   <item><description><c>c</c> (bits 1..0): frame count code 0..3 (see RFC §3.2).</description></item>
/// </list>
/// </summary>
public readonly record struct OpusToc
{
    /// <summary>Raw 5-bit config value (0..31).</summary>
    public byte Config { get; init; }

    /// <summary>True if the encoded packet carries stereo (2 channels).</summary>
    public bool IsStereo { get; init; }

    /// <summary>Frame-count code (0..3) as defined by RFC 6716 §3.2.</summary>
    public byte FrameCountCode { get; init; }

    /// <summary>Sub-codec for this config.</summary>
    public OpusMode Mode { get; init; }

    /// <summary>Audio bandwidth for this config.</summary>
    public OpusBandwidth Bandwidth { get; init; }

    /// <summary>
    /// Per-frame duration in microseconds. The spec uses fractional
    /// milliseconds (2.5 / 5 / 10 / 20 / 40 / 60) so microseconds gives an
    /// integer representation that's easy to multiply by the 48 kHz sample
    /// rate to get a frame length.
    /// </summary>
    public int FrameSizeMicroseconds { get; init; }

    /// <summary>
    /// Convenience accessor: number of 48 kHz samples per Opus frame for
    /// this config. (e.g. 20 ms → 960 samples; 2.5 ms → 120 samples.)
    /// </summary>
    public int SamplesPerFrameAt48k => FrameSizeMicroseconds * 48 / 1000;

    /// <summary>
    /// Parse a single TOC byte. Returns <c>false</c> only if
    /// <paramref name="tocByte"/>'s <c>config</c> field maps to a reserved
    /// value (none exist in the current spec, so this always succeeds —
    /// kept as a TryParse to match the rest of Mediar's parsing surface).
    /// </summary>
    public static bool TryParse(byte tocByte, out OpusToc toc)
    {
        byte config = (byte)(tocByte >> 3);
        bool stereo = (tocByte & 0x4) != 0;
        byte code = (byte)(tocByte & 0x3);
        var info = ConfigTable[config];
        toc = new OpusToc
        {
            Config = config,
            IsStereo = stereo,
            FrameCountCode = code,
            Mode = info.Mode,
            Bandwidth = info.Bandwidth,
            FrameSizeMicroseconds = info.FrameSizeMicroseconds,
        };
        return true;
    }

    /// <summary>
    /// Parse a TOC byte, throwing <see cref="InvalidDataException"/> when the
    /// byte cannot be interpreted. Currently never throws (no config value is
    /// reserved) but the contract is kept symmetrical with
    /// <see cref="TryParse(byte, out OpusToc)"/>.
    /// </summary>
    public static OpusToc Parse(byte tocByte)
    {
        if (!TryParse(tocByte, out var toc))
        {
            throw new InvalidDataException($"Invalid Opus TOC byte 0x{tocByte:X2}.");
        }
        return toc;
    }

    /// <summary>
    /// Reconstruct the 1-byte TOC. Round-trips with <see cref="Parse"/> when
    /// the struct was produced from a parse (i.e. <see cref="Config"/>,
    /// <see cref="IsStereo"/>, <see cref="FrameCountCode"/> are within
    /// range). Useful for tests and for tooling that wants to rewrite an
    /// existing packet.
    /// </summary>
    public byte ToByte()
    {
        if (Config > 31) throw new InvalidOperationException("Config must be 0..31.");
        if (FrameCountCode > 3) throw new InvalidOperationException("FrameCountCode must be 0..3.");
        return (byte)((Config << 3) | (IsStereo ? 0x4 : 0) | FrameCountCode);
    }

    private readonly record struct ConfigInfo(OpusMode Mode, OpusBandwidth Bandwidth, int FrameSizeMicroseconds);

    // RFC 6716 §3.1, Table 2. Indexed by the 5-bit config value.
    private static readonly ConfigInfo[] ConfigTable =
    [
        // 0..3 — SILK NB
        new(OpusMode.SilkOnly,  OpusBandwidth.Narrowband,    10_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Narrowband,    20_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Narrowband,    40_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Narrowband,    60_000),
        // 4..7 — SILK MB
        new(OpusMode.SilkOnly,  OpusBandwidth.Mediumband,    10_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Mediumband,    20_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Mediumband,    40_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Mediumband,    60_000),
        // 8..11 — SILK WB
        new(OpusMode.SilkOnly,  OpusBandwidth.Wideband,      10_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Wideband,      20_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Wideband,      40_000),
        new(OpusMode.SilkOnly,  OpusBandwidth.Wideband,      60_000),
        // 12..13 — Hybrid SWB
        new(OpusMode.Hybrid,    OpusBandwidth.SuperWideband, 10_000),
        new(OpusMode.Hybrid,    OpusBandwidth.SuperWideband, 20_000),
        // 14..15 — Hybrid FB
        new(OpusMode.Hybrid,    OpusBandwidth.Fullband,      10_000),
        new(OpusMode.Hybrid,    OpusBandwidth.Fullband,      20_000),
        // 16..19 — CELT NB
        new(OpusMode.CeltOnly,  OpusBandwidth.Narrowband,     2_500),
        new(OpusMode.CeltOnly,  OpusBandwidth.Narrowband,     5_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Narrowband,    10_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Narrowband,    20_000),
        // 20..23 — CELT WB
        new(OpusMode.CeltOnly,  OpusBandwidth.Wideband,       2_500),
        new(OpusMode.CeltOnly,  OpusBandwidth.Wideband,       5_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Wideband,      10_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Wideband,      20_000),
        // 24..27 — CELT SWB
        new(OpusMode.CeltOnly,  OpusBandwidth.SuperWideband,  2_500),
        new(OpusMode.CeltOnly,  OpusBandwidth.SuperWideband,  5_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.SuperWideband, 10_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.SuperWideband, 20_000),
        // 28..31 — CELT FB
        new(OpusMode.CeltOnly,  OpusBandwidth.Fullband,       2_500),
        new(OpusMode.CeltOnly,  OpusBandwidth.Fullband,       5_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Fullband,      10_000),
        new(OpusMode.CeltOnly,  OpusBandwidth.Fullband,      20_000),
    ];
}
