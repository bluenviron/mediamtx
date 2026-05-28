namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Single dynamic-range-control band entry: the <c>dyn_rng_sgn</c> sign
/// bit and the 7-bit <c>dyn_rng_ctl</c> control value (ISO/IEC 14496-3
/// Table 4.52). Per-band pairs are emitted at the end of every
/// <c>dynamic_range_info()</c> payload, even when no band layout is sent
/// (the default <c>drc_num_bands = 1</c> case still requires one pair).
/// </summary>
public readonly record struct AacDrcBand
{
    /// <summary>1-bit <c>dyn_rng_sgn</c> - sign of the band attenuation.</summary>
    public required bool Sign { get; init; }

    /// <summary>7-bit <c>dyn_rng_ctl</c> - magnitude of the band attenuation (0..127).</summary>
    public required byte Ctl { get; init; }
}

/// <summary>
/// Variable-length <c>excluded_channels()</c> payload (ISO/IEC 14496-3
/// Table 4.53). Channels are flagged via a stream of 7-bit
/// <c>exclude_mask</c> chunks, each followed by a 1-bit
/// <c>additional_excluded_chns</c> continuation flag; the loop ends when
/// the continuation flag is zero. The whole structure is therefore
/// always a multiple of 8 bits.
/// </summary>
public sealed record AacDrcExcludedChannels
{
    /// <summary>
    /// Raw chunk bytes as they appear on the wire. Each byte holds
    /// <c>(exclude_mask &lt;&lt; 1) | additional_excluded_chns</c>; the
    /// continuation flag is 1 for every byte except the last.
    /// </summary>
    public required ReadOnlyMemory<byte> ChunkBytes { get; init; }

    /// <summary>Number of 8-bit chunks emitted (always &gt;= 1).</summary>
    public int ChunkCount => ChunkBytes.Length;

    /// <summary>Total bits consumed by the structure (<c>ChunkCount * 8</c>).</summary>
    public int BitsConsumed => ChunkBytes.Length * 8;
}

/// <summary>
/// Typed view over an AAC FIL <c>EXT_DYNAMIC_RANGE</c> payload
/// (ISO/IEC 14496-3 §4.5.2.13, Tables 4.52 and 4.53). Constructed from
/// the body bits of an <see cref="AacFillExtensionPayload"/> when its
/// <see cref="AacFillExtensionPayload.RawType"/> is <c>0xB</c>.
/// </summary>
public sealed record AacDynamicRangeInfo
{
    /// <summary>1-bit <c>pce_tag_present</c>.</summary>
    public required bool PceTagPresent { get; init; }

    /// <summary>4-bit <c>pce_instance_tag</c> when <see cref="PceTagPresent"/> is true; otherwise 0.</summary>
    public byte PceInstanceTag { get; init; }

    /// <summary>4-bit <c>drc_tag_reserved_bits</c> when <see cref="PceTagPresent"/> is true; otherwise 0.</summary>
    public byte DrcTagReservedBits { get; init; }

    /// <summary>1-bit <c>excluded_chns_present</c>.</summary>
    public required bool ExcludedChannelsPresent { get; init; }

    /// <summary>Populated when <see cref="ExcludedChannelsPresent"/> is true.</summary>
    public AacDrcExcludedChannels? ExcludedChannels { get; init; }

    /// <summary>1-bit <c>drc_bands_present</c>.</summary>
    public required bool DrcBandsPresent { get; init; }

    /// <summary>4-bit <c>drc_band_incr</c> when <see cref="DrcBandsPresent"/> is true; otherwise 0.</summary>
    public byte DrcBandIncr { get; init; }

    /// <summary>4-bit <c>drc_interpolation_scheme</c> when <see cref="DrcBandsPresent"/> is true; otherwise 0.</summary>
    public byte DrcInterpolationScheme { get; init; }

    /// <summary>
    /// Number of DRC bands. Defaults to 1; equals <c>1 + drc_band_incr</c>
    /// (range 1..16) when <see cref="DrcBandsPresent"/> is true.
    /// </summary>
    public required int DrcNumBands { get; init; }

    /// <summary><c>drc_band_top[i]</c> values (one byte per band) when <see cref="DrcBandsPresent"/> is true; otherwise empty.</summary>
    public ReadOnlyMemory<byte> DrcBandTop { get; init; }

    /// <summary>1-bit <c>prog_ref_level_present</c>.</summary>
    public required bool ProgRefLevelPresent { get; init; }

    /// <summary>7-bit <c>prog_ref_level</c> when <see cref="ProgRefLevelPresent"/> is true; otherwise 0.</summary>
    public byte ProgRefLevel { get; init; }

    /// <summary>1-bit <c>prog_ref_level_reserved_bits</c> when <see cref="ProgRefLevelPresent"/> is true; otherwise 0.</summary>
    public byte ProgRefLevelReservedBit { get; init; }

    /// <summary>
    /// Per-band <c>(dyn_rng_sgn, dyn_rng_ctl)</c> pairs - always
    /// <see cref="DrcNumBands"/> entries.
    /// </summary>
    public required IReadOnlyList<AacDrcBand> Bands { get; init; }

    /// <summary>Number of valid bits consumed from the body.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Parse a <c>dynamic_range_info()</c> structure from the body bits of
    /// an <see cref="AacFillExtensionPayload"/>. <paramref name="bodyBitLength"/>
    /// must equal the payload's valid bit count - trailing padding bits
    /// inside <paramref name="body"/> beyond that count are ignored. Returns
    /// false on any truncation or malformed structure.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> body, int bodyBitLength, out AacDynamicRangeInfo? info)
    {
        info = null;
        if (bodyBitLength < 0) return false;
        if (body.Length * 8 < bodyBitLength) return false;

        try
        {
            var reader = new BitReader(body);

            if (!Available(in reader, bodyBitLength, 1)) return false;
            bool pceTagPresent = reader.ReadBit();

            byte pceInstanceTag = 0;
            byte drcTagReservedBits = 0;
            if (pceTagPresent)
            {
                if (!Available(in reader, bodyBitLength, 8)) return false;
                pceInstanceTag = (byte)reader.ReadBits(4);
                drcTagReservedBits = (byte)reader.ReadBits(4);
            }

            if (!Available(in reader, bodyBitLength, 1)) return false;
            bool excludedChannelsPresent = reader.ReadBit();

            AacDrcExcludedChannels? excludedChannels = null;
            if (excludedChannelsPresent)
            {
                if (!TryReadExcludedChannels(ref reader, bodyBitLength, out excludedChannels)) return false;
            }

            if (!Available(in reader, bodyBitLength, 1)) return false;
            bool drcBandsPresent = reader.ReadBit();

            byte drcBandIncr = 0;
            byte drcInterpolationScheme = 0;
            int drcNumBands = 1;
            byte[] drcBandTop = Array.Empty<byte>();
            if (drcBandsPresent)
            {
                if (!Available(in reader, bodyBitLength, 8)) return false;
                drcBandIncr = (byte)reader.ReadBits(4);
                drcInterpolationScheme = (byte)reader.ReadBits(4);
                drcNumBands = 1 + drcBandIncr;

                if (!Available(in reader, bodyBitLength, drcNumBands * 8)) return false;
                drcBandTop = new byte[drcNumBands];
                for (int i = 0; i < drcNumBands; i++) drcBandTop[i] = (byte)reader.ReadBits(8);
            }

            if (!Available(in reader, bodyBitLength, 1)) return false;
            bool progRefLevelPresent = reader.ReadBit();

            byte progRefLevel = 0;
            byte progRefLevelReservedBit = 0;
            if (progRefLevelPresent)
            {
                if (!Available(in reader, bodyBitLength, 8)) return false;
                progRefLevel = (byte)reader.ReadBits(7);
                progRefLevelReservedBit = (byte)reader.ReadBits(1);
            }

            if (!Available(in reader, bodyBitLength, drcNumBands * 8)) return false;
            var bands = new AacDrcBand[drcNumBands];
            for (int i = 0; i < drcNumBands; i++)
            {
                bool sign = reader.ReadBit();
                byte ctl = (byte)reader.ReadBits(7);
                bands[i] = new AacDrcBand { Sign = sign, Ctl = ctl };
            }

            info = new AacDynamicRangeInfo
            {
                PceTagPresent = pceTagPresent,
                PceInstanceTag = pceInstanceTag,
                DrcTagReservedBits = drcTagReservedBits,
                ExcludedChannelsPresent = excludedChannelsPresent,
                ExcludedChannels = excludedChannels,
                DrcBandsPresent = drcBandsPresent,
                DrcBandIncr = drcBandIncr,
                DrcInterpolationScheme = drcInterpolationScheme,
                DrcNumBands = drcNumBands,
                DrcBandTop = drcBandTop,
                ProgRefLevelPresent = progRefLevelPresent,
                ProgRefLevel = progRefLevel,
                ProgRefLevelReservedBit = progRefLevelReservedBit,
                Bands = bands,
                BitsConsumed = reader.Position,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            info = null;
            return false;
        }
        catch (ArgumentOutOfRangeException)
        {
            info = null;
            return false;
        }
    }

    private static bool Available(in BitReader reader, int budget, int needed)
        => reader.Position + needed <= budget;

    private static bool TryReadExcludedChannels(ref BitReader reader, int budget, out AacDrcExcludedChannels? excluded)
    {
        excluded = null;
        // Bounded loop: 7 mask bits + 1 continuation flag per chunk. Bail
        // out once the continuation flag is clear, or if the budget would
        // be exceeded mid-chunk.
        var chunks = new List<byte>();
        while (true)
        {
            if (!Available(in reader, budget, 8)) return false;
            uint mask = reader.ReadBits(7);
            bool more = reader.ReadBit();
            byte chunk = (byte)((mask << 1) | (more ? 1u : 0u));
            chunks.Add(chunk);
            if (!more) break;
            // Pathological caps: 7-bit chunks * 256 entries already covers
            // way more than any sane channel layout.
            if (chunks.Count >= 256) return false;
        }
        excluded = new AacDrcExcludedChannels { ChunkBytes = chunks.ToArray() };
        return true;
    }
}
