namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// ANC_DATA payload nested inside an AAC FIL <c>EXT_DATA_ELEMENT</c>
/// when <see cref="AacDataElementExtension.Version"/> is
/// <see cref="AacDataElementExtension.VersionAncData"/> (0x0). Per
/// ISO/IEC 14496-3 Table 4.59 the length is a variable-length quantity
/// of 8-bit <c>dataElementLength_part</c> values: every part except the
/// last must be 255, and the final part may be anything in 0..254. The
/// effective byte length is
/// <c>(LengthParts.Count - 1) * 255 + LengthParts[^1]</c>.
/// </summary>
public sealed record AacAncDataPayload
{
    /// <summary>Raw <c>dataElementLength_part</c> bytes in stream order.</summary>
    public required IReadOnlyList<byte> LengthParts { get; init; }

    /// <summary>Decoded <c>dataElementLength</c> in bytes.</summary>
    public int DataElementLength
        => LengthParts.Count == 0
            ? 0
            : ((LengthParts.Count - 1) * 255) + LengthParts[LengthParts.Count - 1];

    /// <summary><c>data_element_byte[]</c> values.</summary>
    public required ReadOnlyMemory<byte> DataElementBytes { get; init; }

    /// <summary>Bits consumed by the length-part chain plus the data bytes (no version nibble).</summary>
    public int BitsConsumed => 8 * LengthParts.Count + 8 * DataElementBytes.Length;
}

/// <summary>
/// Typed view of an AAC FIL <c>EXT_DATA_ELEMENT</c> payload
/// (extension_type 0x2) per ISO/IEC 14496-3 Table 4.59. Always
/// surfaces the 4-bit <c>data_element_version</c>. For the only
/// version defined by the standard - <see cref="VersionAncData"/>
/// (0x0, ANC_DATA) - <see cref="AncData"/> carries the parsed
/// length-prefixed payload. For other versions the parsed structural
/// span is just the 4-bit version field and the remainder is captured
/// opaquely in <see cref="Trailing"/>.
/// </summary>
public sealed record AacDataElementExtension
{
    /// <summary><c>data_element_version</c> value 0x0 (ANC_DATA) per Table 4.59.</summary>
    public const byte VersionAncData = 0x0;

    /// <summary>Hard cap on the number of <c>dataElementLength_part</c> bytes accepted by <see cref="TryParse"/>.</summary>
    public const int MaxLengthParts = 256;

    /// <summary>4-bit <c>data_element_version</c>.</summary>
    public required byte Version { get; init; }

    /// <summary>True when <see cref="Version"/> is <see cref="VersionAncData"/>.</summary>
    public bool IsAncData => Version == VersionAncData;

    /// <summary>Parsed ANC_DATA payload when <see cref="IsAncData"/> is true; otherwise null.</summary>
    public AacAncDataPayload? AncData { get; init; }

    /// <summary>
    /// Trailing body bits beyond the structurally parsed span, packed
    /// MSB-first into bytes (last byte may have unused low-order
    /// padding bits). Captures the FIL <c>other_bits</c> tail that the
    /// outer <c>extension_payload()</c> uses to align to the FIL byte
    /// boundary.
    /// </summary>
    public ReadOnlyMemory<byte> Trailing { get; init; }

    /// <summary>Exact bit count of valid bits in <see cref="Trailing"/>.</summary>
    public int TrailingBitLength { get; init; }

    /// <summary>
    /// Number of bits structurally consumed: 4 for the version field,
    /// plus the ANC_DATA structure when applicable.
    /// </summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Parse a FIL <c>EXT_DATA_ELEMENT</c> body. Returns false on
    /// truncation, on an ANC_DATA body that runs out of bits mid
    /// length-part chain or mid data byte, or when the length-part chain
    /// exceeds <see cref="MaxLengthParts"/>.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> body,
        int bodyBitLength,
        out AacDataElementExtension? data)
    {
        data = null;
        if (bodyBitLength < 4) return false;
        if (body.Length * 8 < bodyBitLength) return false;

        try
        {
            var reader = new BitReader(body);
            byte version = (byte)reader.ReadBits(4);

            AacAncDataPayload? ancData = null;
            int structuralBits = 4;

            if (version == VersionAncData)
            {
                if (reader.Position + 8 > bodyBitLength) return false;
                var lengthParts = new List<byte>();
                byte part;
                do
                {
                    if (reader.Position + 8 > bodyBitLength) return false;
                    part = (byte)reader.ReadBits(8);
                    lengthParts.Add(part);
                    if (lengthParts.Count > MaxLengthParts) return false;
                } while (part == 255);

                int dataLength = ((lengthParts.Count - 1) * 255) + part;
                if (reader.Position + 8 * dataLength > bodyBitLength) return false;
                byte[] dataBytes = dataLength == 0 ? Array.Empty<byte>() : new byte[dataLength];
                for (int i = 0; i < dataLength; i++) dataBytes[i] = (byte)reader.ReadBits(8);

                ancData = new AacAncDataPayload
                {
                    LengthParts = lengthParts,
                    DataElementBytes = dataBytes,
                };
                structuralBits = 4 + 8 * lengthParts.Count + 8 * dataLength;
            }

            int trailingBits = bodyBitLength - structuralBits;
            byte[] trailing = ExtractTrailing(body, structuralBits, trailingBits);

            data = new AacDataElementExtension
            {
                Version = version,
                AncData = ancData,
                Trailing = trailing,
                TrailingBitLength = trailingBits,
                BitsConsumed = structuralBits,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            data = null;
            return false;
        }
    }

    private static byte[] ExtractTrailing(ReadOnlySpan<byte> body, int startBit, int bitLength)
    {
        if (bitLength <= 0) return Array.Empty<byte>();
        int byteCount = (bitLength + 7) >> 3;
        var output = new byte[byteCount];
        var reader = new BitReader(body);
        reader.Skip(startBit);
        int remaining = bitLength;
        for (int i = 0; i < byteCount; i++)
        {
            int take = Math.Min(8, remaining);
            uint bits = reader.ReadBits(take);
            if (take < 8) bits <<= (8 - take);
            output[i] = (byte)bits;
            remaining -= take;
        }
        return output;
    }
}
