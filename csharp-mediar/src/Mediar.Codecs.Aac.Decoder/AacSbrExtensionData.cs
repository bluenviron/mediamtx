namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Framing-level view of an AAC FIL <c>sbr_extension_data()</c> payload
/// (ISO/IEC 14496-3 §4.5.2.13, Table 4.55). The FIL extension types
/// <see cref="AacFillExtensionType.SbrData"/> (0xD) and
/// <see cref="AacFillExtensionType.SbrDataCrc"/> (0xE) share the same
/// payload shape - the CRC variant carries a leading 10-bit
/// <c>sbr_crc_bits</c> field; the remainder
/// (<c>sbr_header_flag</c> + optional <c>sbr_header()</c> + <c>sbr_data()</c>)
/// is preserved as an opaque MSB-first bit slice for the future SBR /
/// HE-AAC v1 decoder pipeline.
/// </summary>
public sealed record AacSbrExtensionData
{
    /// <summary>Width of the <c>sbr_crc_bits</c> field per Table 4.55.</summary>
    public const int CrcBitWidth = 10;

    /// <summary>
    /// Source FIL extension type - either <see cref="AacFillExtensionType.SbrData"/>
    /// (no CRC) or <see cref="AacFillExtensionType.SbrDataCrc"/>.
    /// </summary>
    public required AacFillExtensionType ExtensionType { get; init; }

    /// <summary>True when the source FIL was EXT_SBR_DATA_CRC.</summary>
    public bool HasCrc => ExtensionType == AacFillExtensionType.SbrDataCrc;

    /// <summary>10-bit <c>sbr_crc_bits</c> when <see cref="HasCrc"/> is true; otherwise 0.</summary>
    public ushort SbrCrc { get; init; }

    /// <summary>
    /// Remaining <c>sbr_extension_data()</c> bits packed MSB-first into a
    /// byte buffer. The last byte may have unused low-order padding bits;
    /// the exact bit count is <see cref="PayloadBitLength"/>.
    /// </summary>
    public required ReadOnlyMemory<byte> Payload { get; init; }

    /// <summary>Number of valid MSB-first bits in <see cref="Payload"/>.</summary>
    public required int PayloadBitLength { get; init; }

    /// <summary>
    /// Split a FIL extension body into the optional 10-bit
    /// <c>sbr_crc_bits</c> field and the opaque
    /// <c>sbr_extension_data()</c> remainder. Returns false on truncation
    /// or when <paramref name="type"/> is not one of the SBR variants.
    /// </summary>
    public static bool TryParse(
        AacFillExtensionType type,
        ReadOnlySpan<byte> body,
        int bodyBitLength,
        out AacSbrExtensionData? data)
    {
        data = null;
        if (type != AacFillExtensionType.SbrData && type != AacFillExtensionType.SbrDataCrc) return false;
        if (bodyBitLength < 0) return false;
        if (body.Length * 8 < bodyBitLength) return false;

        bool hasCrc = type == AacFillExtensionType.SbrDataCrc;
        int crcBits = hasCrc ? CrcBitWidth : 0;
        if (bodyBitLength < crcBits) return false;

        ushort crc = 0;
        try
        {
            var reader = new BitReader(body);
            if (hasCrc)
            {
                crc = (ushort)reader.ReadBits(CrcBitWidth);
            }
            int payloadBits = bodyBitLength - crcBits;
            byte[] payload = ExtractPayload(in reader, payloadBits);
            data = new AacSbrExtensionData
            {
                ExtensionType = type,
                SbrCrc = crc,
                Payload = payload,
                PayloadBitLength = payloadBits,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            data = null;
            return false;
        }
    }

    private static byte[] ExtractPayload(in BitReader source, int bitLength)
    {
        if (bitLength == 0) return Array.Empty<byte>();
        int byteCount = (bitLength + 7) >> 3;
        var output = new byte[byteCount];
        var reader = source; // copy so we don't mutate caller's reader
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
