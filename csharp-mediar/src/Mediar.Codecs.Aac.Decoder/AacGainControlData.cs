using System.Collections.Immutable;

#pragma warning disable CA1711 // The type name mirrors the ISO/IEC 14496-3 syntactic element gain_control_data().

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of the AAC <c>gain_control_data()</c> element per
/// ISO/IEC 14496-3 §4.4.2.7 Table 4.41. SSR-profile-only side data
/// that describes per-band, per-window gain adjustments applied to
/// the time-domain output of the four-band PQF synthesis filter
/// bank used by the SSR (Scalable Sample Rate) audio object type.
/// </summary>
/// <remarks>
/// <para>
/// The element's bit layout is window-sequence dependent: <c>max_band</c>
/// (2 bits) selects how many of the four PQF sub-bands carry
/// adjustments (bands are indexed <c>bd = 1..max_band</c>; band 0 is
/// always implicit). Each (bd, wd) cell stores a 3-bit
/// <c>adjust_num</c> count of (<c>alevcode</c>, <c>aloccode</c>)
/// pairs. <c>alevcode</c> is always 4 bits; <c>aloccode</c> width
/// is dispatched per window-sequence:
/// </para>
/// <list type="table">
/// <listheader><term>WindowSequence</term><description>numWindows / aloccode widths</description></listheader>
/// <item><term>OnlyLong</term><description>1 / 5</description></item>
/// <item><term>LongStart</term><description>2 / wd==0 ? 4 : 2</description></item>
/// <item><term>EightShort</term><description>8 / 2</description></item>
/// <item><term>LongStop</term><description>2 / wd==0 ? 4 : 5</description></item>
/// </list>
/// <para>
/// Valid only when the enclosing audio object type is SSR
/// (<see cref="AacAudioObjectType.AacSsr"/>); the parser does not
/// enforce that and accepts every window sequence. <see cref="MaxBand"/>
/// = 0 is legal and results in zero bands.
/// </para>
/// </remarks>
public sealed record AacGainControlData
{
    /// <summary>Maximum legal <c>max_band</c> value (2-bit field).</summary>
    public const int MaxMaxBand = 3;

    /// <summary>Maximum legal <c>adjust_num</c> value (3-bit field).</summary>
    public const int MaxAdjustNum = 7;

    /// <summary>Maximum legal <c>alevcode</c> value (4-bit field).</summary>
    public const int MaxAlevCode = 15;

    /// <summary>Window sequence this element was parsed against. Determines the bit layout.</summary>
    public required AacWindowSequence WindowSequence { get; init; }

    /// <summary>2-bit <c>max_band</c> selector; the per-band loops iterate <c>bd = 1..MaxBand</c>.</summary>
    public required int MaxBand { get; init; }

    /// <summary>
    /// One entry per active band (<see cref="MaxBand"/> entries; index 0 corresponds to
    /// bd = 1, index 1 to bd = 2, and so on). Empty when <see cref="MaxBand"/> is 0.
    /// </summary>
    public required ImmutableArray<AacGainControlBand> Bands { get; init; }

    /// <summary>Total number of bits consumed for this element.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Reads a complete <c>gain_control_data()</c> element from
    /// <paramref name="reader"/> using the layout implied by
    /// <paramref name="windowSequence"/>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at the start of <c>gain_control_data()</c>.</param>
    /// <param name="windowSequence">Window sequence from the enclosing <c>ics_info()</c>.</param>
    /// <param name="data">Parsed element on success; <see langword="null"/> on failure.</param>
    /// <returns>
    /// <see langword="true"/> on a complete read; <see langword="false"/>
    /// on stream underflow at any field.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacWindowSequence windowSequence,
        out AacGainControlData? data)
    {
        data = null;

        if (reader.Remaining < 2) return false;
        int maxBand = (int)reader.ReadBits(2);
        int bitsConsumed = 2;

        int numWindows = GetWindowCount(windowSequence);
        Span<int> alocBits = stackalloc int[numWindows];
        FillAlocBitWidths(windowSequence, alocBits);

        var bandsBuilder = ImmutableArray.CreateBuilder<AacGainControlBand>(maxBand);
        for (int bd = 0; bd < maxBand; bd++)
        {
            var windowsBuilder = ImmutableArray.CreateBuilder<AacGainControlWindow>(numWindows);
            for (int wd = 0; wd < numWindows; wd++)
            {
                if (reader.Remaining < 3) return false;
                int adjustNum = (int)reader.ReadBits(3);
                bitsConsumed += 3;

                ImmutableArray<AacGainControlAdjustment> adjustments;
                if (adjustNum > 0)
                {
                    int alocBitWidth = alocBits[wd];
                    int totalAdjBits = adjustNum * (4 + alocBitWidth);
                    if (reader.Remaining < totalAdjBits) return false;

                    var adjBuilder = ImmutableArray.CreateBuilder<AacGainControlAdjustment>(adjustNum);
                    for (int ad = 0; ad < adjustNum; ad++)
                    {
                        int alev = (int)reader.ReadBits(4);
                        int aloc = (int)reader.ReadBits(alocBitWidth);
                        adjBuilder.Add(new AacGainControlAdjustment(alev, aloc));
                    }
                    adjustments = adjBuilder.MoveToImmutable();
                    bitsConsumed += totalAdjBits;
                }
                else
                {
                    adjustments = ImmutableArray<AacGainControlAdjustment>.Empty;
                }

                windowsBuilder.Add(new AacGainControlWindow
                {
                    AdjustNum = adjustNum,
                    Adjustments = adjustments,
                });
            }

            bandsBuilder.Add(new AacGainControlBand
            {
                Windows = windowsBuilder.MoveToImmutable(),
            });
        }

        data = new AacGainControlData
        {
            WindowSequence = windowSequence,
            MaxBand = maxBand,
            Bands = bandsBuilder.MoveToImmutable(),
            BitsConsumed = bitsConsumed,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>gain_control_data()</c> element from
    /// <paramref name="bytes"/> using the layout implied by
    /// <paramref name="windowSequence"/>.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacWindowSequence windowSequence,
        out AacGainControlData? data)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, windowSequence, out data);
    }

    /// <summary>
    /// Serialises this element back to its on-wire form via
    /// <paramref name="writer"/>. Throws when any captured field
    /// overflows the field width implied by <see cref="WindowSequence"/>.
    /// </summary>
    internal void WriteTo(BitWriter writer)
    {
        ArgumentNullException.ThrowIfNull(writer);

        if ((uint)MaxBand > MaxMaxBand)
        {
            throw new InvalidOperationException(
                $"max_band {MaxBand} exceeds 2-bit field maximum {MaxMaxBand}.");
        }
        if (Bands.Length != MaxBand)
        {
            throw new InvalidOperationException(
                $"gain_control_data() declares max_band={MaxBand} but carries {Bands.Length} band entries.");
        }

        int numWindows = GetWindowCount(WindowSequence);
        Span<int> alocBits = stackalloc int[numWindows];
        FillAlocBitWidths(WindowSequence, alocBits);

        writer.Write((uint)MaxBand, 2);

        for (int bd = 0; bd < Bands.Length; bd++)
        {
            var band = Bands[bd];
            if (band.Windows.Length != numWindows)
            {
                throw new InvalidOperationException(
                    $"band {bd} carries {band.Windows.Length} window(s) but {WindowSequence} expects {numWindows}.");
            }

            for (int wd = 0; wd < band.Windows.Length; wd++)
            {
                var window = band.Windows[wd];
                if ((uint)window.AdjustNum > MaxAdjustNum)
                {
                    throw new InvalidOperationException(
                        $"band {bd} window {wd} adjust_num {window.AdjustNum} exceeds 3-bit field maximum {MaxAdjustNum}.");
                }
                if (window.Adjustments.Length != window.AdjustNum)
                {
                    throw new InvalidOperationException(
                        $"band {bd} window {wd} declares adjust_num={window.AdjustNum} but carries " +
                        $"{window.Adjustments.Length} adjustment(s).");
                }

                writer.Write((uint)window.AdjustNum, 3);

                if (window.AdjustNum == 0) continue;

                int alocBitWidth = alocBits[wd];
                uint alocMask = (1u << alocBitWidth) - 1u;
                for (int ad = 0; ad < window.Adjustments.Length; ad++)
                {
                    var adj = window.Adjustments[ad];
                    if ((uint)adj.AlevCode > MaxAlevCode)
                    {
                        throw new InvalidOperationException(
                            $"band {bd} window {wd} adj[{ad}] alevcode {adj.AlevCode} exceeds 4-bit field maximum {MaxAlevCode}.");
                    }
                    if ((uint)adj.AlocCode > alocMask)
                    {
                        throw new InvalidOperationException(
                            $"band {bd} window {wd} adj[{ad}] aloccode {adj.AlocCode} exceeds " +
                            $"{alocBitWidth}-bit field maximum {alocMask}.");
                    }
                    writer.Write((uint)adj.AlevCode, 4);
                    writer.Write((uint)adj.AlocCode, alocBitWidth);
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

    private static int GetWindowCount(AacWindowSequence windowSequence) => windowSequence switch
    {
        AacWindowSequence.OnlyLong => 1,
        AacWindowSequence.LongStart => 2,
        AacWindowSequence.EightShort => 8,
        AacWindowSequence.LongStop => 2,
        _ => throw new ArgumentOutOfRangeException(nameof(windowSequence), windowSequence, "Unknown window sequence."),
    };

    private static void FillAlocBitWidths(AacWindowSequence windowSequence, Span<int> alocBits)
    {
        switch (windowSequence)
        {
            case AacWindowSequence.OnlyLong:
                alocBits[0] = 5;
                break;
            case AacWindowSequence.LongStart:
                alocBits[0] = 4;
                alocBits[1] = 2;
                break;
            case AacWindowSequence.EightShort:
                for (int i = 0; i < 8; i++) alocBits[i] = 2;
                break;
            case AacWindowSequence.LongStop:
                alocBits[0] = 4;
                alocBits[1] = 5;
                break;
            default:
                throw new ArgumentOutOfRangeException(nameof(windowSequence), windowSequence, "Unknown window sequence.");
        }
    }
}

/// <summary>
/// One PQF sub-band entry inside an AAC <c>gain_control_data()</c>
/// element. Index 0 corresponds to PQF band <c>bd = 1</c> (band 0 is
/// always implicit).
/// </summary>
public sealed record AacGainControlBand
{
    /// <summary>
    /// One entry per window slot. Length depends on
    /// <see cref="AacGainControlData.WindowSequence"/>: 1 for OnlyLong,
    /// 2 for LongStart / LongStop, 8 for EightShort.
    /// </summary>
    public required ImmutableArray<AacGainControlWindow> Windows { get; init; }
}

/// <summary>
/// One window slot of an <see cref="AacGainControlBand"/>. Carries the
/// 3-bit <c>adjust_num</c> count and the matching
/// (<c>alevcode</c>, <c>aloccode</c>) adjustment list.
/// </summary>
public sealed record AacGainControlWindow
{
    /// <summary>3-bit <c>adjust_num[bd][wd]</c>: number of (alevcode, aloccode) pairs (0..7).</summary>
    public required int AdjustNum { get; init; }

    /// <summary>The (alevcode, aloccode) adjustment pairs in the order they appear in the bitstream.</summary>
    public required ImmutableArray<AacGainControlAdjustment> Adjustments { get; init; }
}

/// <summary>
/// A single (alevcode, aloccode) adjustment pair. <see cref="AlevCode"/>
/// is always a 4-bit unsigned value; <see cref="AlocCode"/> width varies
/// per window-sequence + window-index per Table 4.41 (2, 4 or 5 bits).
/// </summary>
/// <param name="AlevCode">4-bit unsigned <c>alevcode[bd][wd][ad]</c>.</param>
/// <param name="AlocCode">Unsigned <c>aloccode[bd][wd][ad]</c> (2, 4, or 5 bits per dispatch).</param>
public readonly record struct AacGainControlAdjustment(int AlevCode, int AlocCode);
