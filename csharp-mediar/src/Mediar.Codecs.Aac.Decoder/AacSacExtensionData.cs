namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Framing-level view of an AAC FIL <c>sac_extension_data()</c> payload
/// (ISO/IEC 14496-3 §4.5.2.13, Table 4.61). The FIL extension type
/// <see cref="AacFillExtensionType.SacData"/> (0xC) carries the MPEG
/// Surround Audio Coding bitstream; the full SAC / MPEG Surround
/// decoder is out of scope, so the body is preserved as an opaque
/// MSB-first bit slice for downstream consumers.
/// </summary>
public sealed record AacSacExtensionData
{
    /// <summary>
    /// Opaque <c>sac_extension_data()</c> bits packed MSB-first into a
    /// byte buffer. The last byte may have unused low-order padding
    /// bits; the exact bit count is <see cref="PayloadBitLength"/>.
    /// </summary>
    public required ReadOnlyMemory<byte> Payload { get; init; }

    /// <summary>Number of valid MSB-first bits in <see cref="Payload"/>.</summary>
    public required int PayloadBitLength { get; init; }

    /// <summary>
    /// Captures the FIL extension body as an opaque SAC payload.
    /// Returns false on malformed input (negative bit length or
    /// buffer smaller than the declared bit length).
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> body,
        int bodyBitLength,
        out AacSacExtensionData? data)
    {
        data = null;
        if (bodyBitLength < 0) return false;
        if (body.Length * 8 < bodyBitLength) return false;

        try
        {
            var reader = new BitReader(body);
            byte[] payload = ExtractPayload(in reader, bodyBitLength);
            data = new AacSacExtensionData
            {
                Payload = payload,
                PayloadBitLength = bodyBitLength,
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
        var reader = source;
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
