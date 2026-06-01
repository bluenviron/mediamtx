using System.Collections.Immutable;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of the AAC <c>tns_data()</c> (Temporal Noise Shaping)
/// element per ISO/IEC 14496-3 §4.6.9 / Table 4.40. Describes up to a
/// few all-pole filters per window that shape the temporal envelope of
/// the dequantised spectral coefficients before the inverse MDCT.
/// </summary>
/// <remarks>
/// <para>
/// The element's bit layout is window-sequence dependent: long blocks
/// (OnlyLong / LongStart / LongStop) use a single window with up to
/// <see cref="MaxFiltersPerLongWindow"/> filters and wide
/// length/order fields; EightShort blocks have <see cref="ShortWindowCount"/>
/// physical windows each carrying at most <see cref="MaxFiltersPerShortWindow"/>
/// filter and narrower length/order fields.
/// </para>
/// <para>
/// Coefficient widths are derived from the per-window <c>coef_res</c>
/// flag and the per-filter <c>coef_compress</c> flag:
/// <c>coef_bits = (coef_res ? 4 : 3) - (coef_compress ? 1 : 0)</c>,
/// yielding 2-, 3- or 4-bit raw unsigned values. The actual
/// inverse-quantised filter coefficients are recovered downstream via
/// the sign-extension + lookup tables defined in Annex 4.A; the parser
/// preserves only the raw on-wire bits.
/// </para>
/// </remarks>
public sealed record AacTnsData
{
    /// <summary>Number of physical windows in an EightShort block.</summary>
    public const int ShortWindowCount = 8;

    /// <summary>Maximum filters per window for long-window sequences (2-bit field).</summary>
    public const int MaxFiltersPerLongWindow = 3;

    /// <summary>Maximum filters per window for EightShort sequences (1-bit field).</summary>
    public const int MaxFiltersPerShortWindow = 1;

    /// <summary>Maximum <c>length[w][filt]</c> for long-window sequences (6-bit field).</summary>
    public const int MaxLengthLong = 63;

    /// <summary>Maximum <c>length[w][filt]</c> for EightShort sequences (4-bit field).</summary>
    public const int MaxLengthShort = 15;

    /// <summary>Maximum <c>order[w][filt]</c> for long-window sequences (5-bit field).</summary>
    public const int MaxOrderLong = 31;

    /// <summary>Maximum <c>order[w][filt]</c> for EightShort sequences (3-bit field).</summary>
    public const int MaxOrderShort = 7;

    /// <summary>Window sequence this element was parsed against. Determines the bit layout.</summary>
    public required AacWindowSequence WindowSequence { get; init; }

    /// <summary>One entry per physical window (1 for long sequences, 8 for EightShort).</summary>
    public required ImmutableArray<AacTnsWindow> Windows { get; init; }

    /// <summary>Total number of bits consumed for this element.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Reads a complete <c>tns_data()</c> element and additionally
    /// validates each filter's order against the AOT-specific max
    /// returned by <see cref="AacTnsSpecLimits.GetMaxOrder"/>. Returns
    /// <see langword="false"/> when any filter's order exceeds the
    /// permitted maximum for the given AOT/window combination, which
    /// matches FFmpeg's <c>AVERROR_INVALIDDATA</c> behaviour in
    /// <c>decode_tns()</c>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at the start of <c>tns_data()</c>.</param>
    /// <param name="windowSequence">Window sequence from the enclosing <c>ics_info()</c>.</param>
    /// <param name="objectType">
    /// AAC audio object type, used to pick the per-AOT max-order
    /// (see <see cref="AacTnsSpecLimits.GetMaxOrder"/>). Must be one
    /// of <see cref="AacAudioObjectType.AacMain"/>,
    /// <see cref="AacAudioObjectType.AacLc"/>,
    /// <see cref="AacAudioObjectType.AacLtp"/>, or
    /// <see cref="AacAudioObjectType.ErAacLc"/>; other AOTs throw.
    /// </param>
    /// <param name="data">Parsed element on success; <see langword="null"/> on failure.</param>
    /// <returns>
    /// <see langword="true"/> on a complete read with every filter
    /// order within the AOT-specific max; <see langword="false"/> on
    /// stream underflow or on an over-large order.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacWindowSequence windowSequence,
        AacAudioObjectType objectType,
        out AacTnsData? data)
    {
        if (!TryRead(ref reader, windowSequence, out data) || data is null)
        {
            return false;
        }

        int maxOrder = AacTnsSpecLimits.GetMaxOrder(objectType, windowSequence);
        foreach (var window in data.Windows)
        {
            foreach (var filter in window.Filters)
            {
                if (filter.Order > maxOrder)
                {
                    data = null;
                    return false;
                }
            }
        }
        return true;
    }

    /// <summary>
    /// Reads a complete <c>tns_data()</c> element from
    /// <paramref name="reader"/> using the layout implied by
    /// <paramref name="windowSequence"/>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at the start of <c>tns_data()</c>.</param>
    /// <param name="windowSequence">Window sequence from the enclosing <c>ics_info()</c>.</param>
    /// <param name="data">Parsed element on success; <see langword="null"/> on failure.</param>
    /// <returns>
    /// <see langword="true"/> on a complete read; <see langword="false"/>
    /// on stream underflow at any field.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacWindowSequence windowSequence,
        out AacTnsData? data)
    {
        data = null;

        bool isShort = windowSequence == AacWindowSequence.EightShort;
        int numWindows = isShort ? ShortWindowCount : 1;
        int nFiltBits = isShort ? 1 : 2;
        int lengthBits = isShort ? 4 : 6;
        int orderBits = isShort ? 3 : 5;

        var windows = ImmutableArray.CreateBuilder<AacTnsWindow>(numWindows);
        int bitsConsumed = 0;

        for (int w = 0; w < numWindows; w++)
        {
            if (reader.Remaining < nFiltBits) return false;
            int nFilt = (int)reader.ReadBits(nFiltBits);
            bitsConsumed += nFiltBits;

            bool coefRes = false;
            ImmutableArray<AacTnsFilter> filters;

            if (nFilt > 0)
            {
                if (reader.Remaining < 1) return false;
                coefRes = reader.ReadBit();
                bitsConsumed += 1;

                var filterBuilder = ImmutableArray.CreateBuilder<AacTnsFilter>(nFilt);
                for (int f = 0; f < nFilt; f++)
                {
                    if (reader.Remaining < lengthBits + orderBits) return false;
                    int length = (int)reader.ReadBits(lengthBits);
                    int order = (int)reader.ReadBits(orderBits);
                    bitsConsumed += lengthBits + orderBits;

                    bool direction = false;
                    bool coefCompress = false;
                    ImmutableArray<int> coefficients = ImmutableArray<int>.Empty;
                    int coefBits = 0;

                    if (order > 0)
                    {
                        if (reader.Remaining < 2) return false;
                        direction = reader.ReadBit();
                        coefCompress = reader.ReadBit();
                        bitsConsumed += 2;

                        coefBits = (coefRes ? 4 : 3) - (coefCompress ? 1 : 0);

                        if (reader.Remaining < order * coefBits) return false;
                        var coefBuilder = ImmutableArray.CreateBuilder<int>(order);
                        for (int i = 0; i < order; i++)
                        {
                            coefBuilder.Add((int)reader.ReadBits(coefBits));
                        }
                        coefficients = coefBuilder.MoveToImmutable();
                        bitsConsumed += order * coefBits;
                    }

                    filterBuilder.Add(new AacTnsFilter
                    {
                        Length = length,
                        Order = order,
                        Direction = direction,
                        CoefCompress = coefCompress,
                        Coefficients = coefficients,
                        CoefBits = coefBits,
                    });
                }

                filters = filterBuilder.MoveToImmutable();
            }
            else
            {
                filters = ImmutableArray<AacTnsFilter>.Empty;
            }

            windows.Add(new AacTnsWindow
            {
                CoefResHigh = coefRes,
                Filters = filters,
            });
        }

        data = new AacTnsData
        {
            WindowSequence = windowSequence,
            Windows = windows.MoveToImmutable(),
            BitsConsumed = bitsConsumed,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>tns_data()</c> element from
    /// <paramref name="bytes"/> using the layout implied by
    /// <paramref name="windowSequence"/>.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacWindowSequence windowSequence,
        out AacTnsData? data)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, windowSequence, out data);
    }

    /// <summary>
    /// Parses a contiguous <c>tns_data()</c> element from
    /// <paramref name="bytes"/> using the layout implied by
    /// <paramref name="windowSequence"/> and additionally rejects
    /// any filter whose order exceeds the AOT-specific maximum
    /// returned by <see cref="AacTnsSpecLimits.GetMaxOrder"/>.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacWindowSequence windowSequence,
        AacAudioObjectType objectType,
        out AacTnsData? data)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, windowSequence, objectType, out data);
    }

    /// <summary>
    /// Serialises this element back to its on-wire form via
    /// <paramref name="writer"/>. Throws when any captured field
    /// overflows the field width implied by <see cref="WindowSequence"/>.
    /// </summary>
    internal void WriteTo(BitWriter writer)
    {
        ArgumentNullException.ThrowIfNull(writer);

        bool isShort = WindowSequence == AacWindowSequence.EightShort;
        int expectedWindows = isShort ? ShortWindowCount : 1;
        int maxFilters = isShort ? MaxFiltersPerShortWindow : MaxFiltersPerLongWindow;
        int maxLength = isShort ? MaxLengthShort : MaxLengthLong;
        int maxOrder = isShort ? MaxOrderShort : MaxOrderLong;
        int nFiltBits = isShort ? 1 : 2;
        int lengthBits = isShort ? 4 : 6;
        int orderBits = isShort ? 3 : 5;

        if (Windows.Length != expectedWindows)
        {
            throw new InvalidOperationException(
                $"tns_data() for {WindowSequence} must carry exactly {expectedWindows} window(s) " +
                $"(was {Windows.Length}).");
        }

        for (int w = 0; w < Windows.Length; w++)
        {
            var window = Windows[w];
            int nFilt = window.Filters.Length;
            if ((uint)nFilt > (uint)maxFilters)
            {
                throw new InvalidOperationException(
                    $"window {w} has {nFilt} filters but the limit for {WindowSequence} is {maxFilters}.");
            }

            writer.Write((uint)nFilt, nFiltBits);
            if (nFilt == 0) continue;

            writer.Write(window.CoefResHigh ? 1u : 0u, 1);

            for (int f = 0; f < nFilt; f++)
            {
                var filter = window.Filters[f];

                if ((uint)filter.Length > (uint)maxLength)
                {
                    throw new InvalidOperationException(
                        $"window {w} filter {f} length {filter.Length} exceeds {WindowSequence} maximum {maxLength}.");
                }
                if ((uint)filter.Order > (uint)maxOrder)
                {
                    throw new InvalidOperationException(
                        $"window {w} filter {f} order {filter.Order} exceeds {WindowSequence} maximum {maxOrder}.");
                }

                writer.Write((uint)filter.Length, lengthBits);
                writer.Write((uint)filter.Order, orderBits);

                if (filter.Order == 0)
                {
                    if (!filter.Coefficients.IsDefaultOrEmpty)
                    {
                        throw new InvalidOperationException(
                            $"window {w} filter {f} has order 0 but carries {filter.Coefficients.Length} coefficient(s).");
                    }
                    continue;
                }

                writer.Write(filter.Direction ? 1u : 0u, 1);
                writer.Write(filter.CoefCompress ? 1u : 0u, 1);

                int coefBits = (window.CoefResHigh ? 4 : 3) - (filter.CoefCompress ? 1 : 0);
                if (filter.Coefficients.Length != filter.Order)
                {
                    throw new InvalidOperationException(
                        $"window {w} filter {f} declares order {filter.Order} but carries " +
                        $"{filter.Coefficients.Length} coefficient(s).");
                }

                uint coefMask = (1u << coefBits) - 1u;
                for (int i = 0; i < filter.Coefficients.Length; i++)
                {
                    int coef = filter.Coefficients[i];
                    if ((uint)coef > coefMask)
                    {
                        throw new InvalidOperationException(
                            $"window {w} filter {f} coef[{i}] {coef} exceeds {coefBits}-bit field maximum {coefMask}.");
                    }
                    writer.Write((uint)coef, coefBits);
                }
            }
        }
    }

    /// <summary>
    /// Serialises this element to a byte buffer padded to the next byte
    /// boundary with trailing zero bits.
    /// </summary>
    public byte[] ToBytes()
    {
        var writer = new BitWriter();
        WriteTo(writer);
        return writer.ToArray();
    }
}

/// <summary>
/// One physical window of an AAC <c>tns_data()</c> element. A long-window
/// <c>tns_data()</c> has a single <see cref="AacTnsWindow"/>; EightShort
/// has eight.
/// </summary>
public sealed record AacTnsWindow
{
    /// <summary>
    /// Per-window <c>coef_res</c> flag. <see langword="false"/> selects
    /// the 3-bit base coefficient width; <see langword="true"/> selects
    /// the 4-bit base. Only meaningful when <see cref="Filters"/> is
    /// non-empty (the field is omitted in the bitstream when
    /// <c>n_filt[w] == 0</c>).
    /// </summary>
    public bool CoefResHigh { get; init; }

    /// <summary>Filters declared for this window (0..<see cref="AacTnsData.MaxFiltersPerLongWindow"/>).</summary>
    public required ImmutableArray<AacTnsFilter> Filters { get; init; }
}

/// <summary>
/// One TNS filter inside an <see cref="AacTnsWindow"/>. Captures the raw
/// on-wire fields; the parser does not perform sign extension or apply
/// the inverse-quantisation lookup tables.
/// </summary>
public sealed record AacTnsFilter
{
    /// <summary>Filter <c>length[w][filt]</c> (number of MDCT lines covered).</summary>
    public required int Length { get; init; }

    /// <summary>Filter <c>order[w][filt]</c>. When 0, no further fields are present.</summary>
    public required int Order { get; init; }

    /// <summary>Filter <c>direction[w][filt]</c>: <see langword="false"/> = up-sweep, <see langword="true"/> = down-sweep.</summary>
    public bool Direction { get; init; }

    /// <summary>
    /// Filter <c>coef_compress[w][filt]</c>. When <see langword="true"/>
    /// reduces the per-coefficient field width by one bit (drops the
    /// LSB; the decoder restores it from a sign-extension lookup).
    /// </summary>
    public bool CoefCompress { get; init; }

    /// <summary>
    /// Raw transmitted coefficients (length equals <see cref="Order"/>).
    /// Each value is an unsigned integer of width
    /// <see cref="CoefBits"/>; sign extension and the
    /// PARCOR-to-LPC conversion are part of the inverse-quantisation
    /// stage and not performed here.
    /// </summary>
    public ImmutableArray<int> Coefficients { get; init; } = ImmutableArray<int>.Empty;

    /// <summary>
    /// Width in bits of each entry in <see cref="Coefficients"/> (2..4).
    /// Derived from the per-window <c>coef_res</c> flag and the per-filter
    /// <c>coef_compress</c> flag.
    /// </summary>
    public int CoefBits { get; init; }
}
