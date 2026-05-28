using System.Collections.Immutable;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of the AAC <c>pulse_data()</c> noiseless-coding extension
/// per ISO/IEC 14496-3 §4.6.2.3.4, Table 4.39. Adds up to four sign-
/// inferred amplitude pulses to a long-window block's dequantised
/// spectrum. Only valid when the enclosing <c>ics_info()</c> declares
/// <c>window_sequence != EIGHT_SHORT_SEQUENCE</c>; the caller is
/// responsible for that context check.
/// </summary>
public sealed record AacPulseData
{
    /// <summary>Maximum number of pulses per block (<c>number_pulse + 1</c> where the field is 2 bits).</summary>
    public const int MaxPulses = 4;

    /// <summary>Maximum legal <c>pulse_start_sfb</c> value (6 bits).</summary>
    public const int MaxStartScaleFactorBand = 63;

    /// <summary>Maximum legal <c>pulse_offset[i]</c> value (5 bits).</summary>
    public const int MaxPulseOffset = 31;

    /// <summary>Maximum legal <c>pulse_amplitude[i]</c> value (4 bits).</summary>
    public const int MaxPulseAmplitude = 15;

    private AacPulseData(int startScaleFactorBand, ImmutableArray<AacPulse> pulses, int bitsConsumed)
    {
        StartScaleFactorBand = startScaleFactorBand;
        Pulses = pulses;
        BitsConsumed = bitsConsumed;
    }

    /// <summary>Scale-factor band at which the first pulse is anchored (<c>pulse_start_sfb</c>).</summary>
    public int StartScaleFactorBand { get; init; }

    /// <summary>The pulses themselves in the order they appear in the bitstream.</summary>
    public ImmutableArray<AacPulse> Pulses { get; init; }

    /// <summary>
    /// Number of bits consumed for this element (always <c>8 + 9 * Pulses.Length</c>).
    /// </summary>
    public int BitsConsumed { get; init; }

    /// <summary>
    /// Reads a complete <c>pulse_data()</c> element from <paramref name="reader"/>.
    /// </summary>
    /// <param name="reader">Bit reader positioned at the start of the element.</param>
    /// <param name="data">Parsed element on success; <see langword="null"/> on failure.</param>
    /// <returns>
    /// <see langword="true"/> when the full element fits within the remaining bits;
    /// <see langword="false"/> on stream underflow.
    /// </returns>
    internal static bool TryRead(scoped ref BitReader reader, out AacPulseData? data)
    {
        data = null;

        if (reader.Remaining < 8)
        {
            return false;
        }

        int numberPulse = (int)reader.ReadBits(2);
        int startScaleFactorBand = (int)reader.ReadBits(6);
        int count = numberPulse + 1;

        if (reader.Remaining < count * 9)
        {
            return false;
        }

        var builder = ImmutableArray.CreateBuilder<AacPulse>(count);
        for (int i = 0; i < count; i++)
        {
            int offset = (int)reader.ReadBits(5);
            int amplitude = (int)reader.ReadBits(4);
            builder.Add(new AacPulse(offset, amplitude));
        }

        data = new AacPulseData(startScaleFactorBand, builder.MoveToImmutable(), 8 + count * 9);
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>pulse_data()</c> element from <paramref name="bytes"/>.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> bytes, out AacPulseData? data)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, out data);
    }

    /// <summary>
    /// Serialises this element back to its on-wire form via
    /// <paramref name="writer"/>. Throws if any captured field overflows
    /// its bit width.
    /// </summary>
    internal void WriteTo(BitWriter writer)
    {
        ArgumentNullException.ThrowIfNull(writer);

        int count = Pulses.Length;
        if (count is < 1 or > MaxPulses)
        {
            throw new InvalidOperationException(
                $"pulse_data() must contain between 1 and {MaxPulses} pulses (was {count}).");
        }
        if ((uint)StartScaleFactorBand > MaxStartScaleFactorBand)
        {
            throw new InvalidOperationException(
                $"pulse_start_sfb {StartScaleFactorBand} exceeds 6-bit field maximum {MaxStartScaleFactorBand}.");
        }

        writer.Write((uint)(count - 1), 2);
        writer.Write((uint)StartScaleFactorBand, 6);
        for (int i = 0; i < count; i++)
        {
            var pulse = Pulses[i];
            if ((uint)pulse.Offset > MaxPulseOffset)
            {
                throw new InvalidOperationException(
                    $"pulse_offset[{i}] {pulse.Offset} exceeds 5-bit field maximum {MaxPulseOffset}.");
            }
            if ((uint)pulse.Amplitude > MaxPulseAmplitude)
            {
                throw new InvalidOperationException(
                    $"pulse_amplitude[{i}] {pulse.Amplitude} exceeds 4-bit field maximum {MaxPulseAmplitude}.");
            }
            writer.Write((uint)pulse.Offset, 5);
            writer.Write((uint)pulse.Amplitude, 4);
        }
    }

    /// <summary>
    /// Serialises this element back to a byte buffer padded to the next
    /// byte boundary with trailing zero bits.
    /// </summary>
    public byte[] ToBytes()
    {
        var writer = new BitWriter();
        WriteTo(writer);
        return writer.ToArray();
    }
}

/// <summary>
/// A single pulse in an AAC <c>pulse_data()</c> element. <see cref="Offset"/>
/// advances the spectral-coefficient cursor; <see cref="Amplitude"/> is the
/// absolute magnitude to add (the sign is inferred from the spectral
/// coefficient being augmented).
/// </summary>
/// <param name="Offset">5-bit unsigned coefficient-cursor step (<c>pulse_offset[i]</c>).</param>
/// <param name="Amplitude">4-bit unsigned magnitude (<c>pulse_amplitude[i]</c>).</param>
public readonly record struct AacPulse(int Offset, int Amplitude);
