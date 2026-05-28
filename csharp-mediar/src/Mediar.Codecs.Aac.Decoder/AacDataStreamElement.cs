namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed AAC data_stream_element (ISO/IEC 14496-3 Table 4.10), the
/// opaque-byte payload element addressed by a 4-bit tag. The 8-bit count
/// field with an optional 8-bit escape allows up to 510 data bytes per
/// element; the element_instance_tag identifies which auxiliary stream the
/// bytes belong to so multiple DSEs can be interleaved in a single
/// raw_data_block.
/// </summary>
public sealed record AacDataStreamElement
{
    /// <summary>Largest payload size representable by count + esc_count (255 + 255).</summary>
    public const int MaxDataBytes = 510;

    /// <summary>4-bit <c>element_instance_tag</c>.</summary>
    public required int ElementInstanceTag { get; init; }

    /// <summary>True when the element opened with <c>byte_alignment()</c> before the data bytes.</summary>
    public required bool DataByteAlignFlag { get; init; }

    /// <summary>Opaque data payload (0..510 bytes).</summary>
    public required ReadOnlyMemory<byte> Data { get; init; }

    /// <summary>
    /// Parse a standalone DSE blob. <paramref name="data"/> must start on a
    /// byte boundary with the 4-bit <c>element_instance_tag</c> (the
    /// raw_data_block dispatcher strips the 3-bit element id before calling
    /// this method). Returns false on truncation.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AacDataStreamElement? dse)
        => TryParse(data, out dse, out _);

    /// <summary>
    /// Parse a standalone DSE blob and report the exact number of bytes
    /// consumed. Because DSE either ends on a byte boundary (data_byte_align
    /// flag set, all subsequent fields are 8-bit) or, when the flag is
    /// clear, leaves the cursor on the last bit of the last data byte (8-bit
    /// reads from a 5-bit-into-the-byte cursor still produce 8-bit values
    /// that wrap), the returned byte count is rounded up to the nearest
    /// whole byte.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AacDataStreamElement? dse, out int bytesConsumed)
    {
        dse = null;
        bytesConsumed = 0;
        if (data.IsEmpty) return false;

        try
        {
            var reader = new BitReader(data);
            if (!TryRead(ref reader, out dse)) return false;
            bytesConsumed = (reader.Position + 7) >> 3;
            return true;
        }
        catch (EndOfStreamException)
        {
            dse = null;
            bytesConsumed = 0;
            return false;
        }
        catch (ArgumentOutOfRangeException)
        {
            dse = null;
            bytesConsumed = 0;
            return false;
        }
    }

    /// <summary>
    /// Serialise this DSE back to the standalone byte layout produced by
    /// <see cref="TryParse(System.ReadOnlySpan{byte}, out AacDataStreamElement?)"/>.
    /// Throws <see cref="InvalidOperationException"/> when the payload
    /// exceeds <see cref="MaxDataBytes"/> or the tag does not fit in 4 bits.
    /// Trailing bits inside the final byte are zero-padded.
    /// </summary>
    public byte[] ToBytes()
    {
        var writer = new BitWriter();
        WriteTo(writer);
        return writer.ToArray();
    }

    internal static bool TryRead(ref BitReader reader, out AacDataStreamElement? dse)
    {
        dse = null;
        if (reader.Remaining < 4 + 1 + 8) return false;

        int tag = (int)reader.ReadBits(4);
        bool align = reader.ReadBit();
        int count = (int)reader.ReadBits(8);
        int cnt = count;
        if (count == 255)
        {
            if (reader.Remaining < 8) return false;
            int esc = (int)reader.ReadBits(8);
            cnt += esc;
        }

        if (align) reader.AlignToByte();

        if (reader.Remaining < cnt * 8) return false;

        byte[] payload = cnt == 0 ? Array.Empty<byte>() : new byte[cnt];
        for (int i = 0; i < cnt; i++) payload[i] = (byte)reader.ReadBits(8);

        dse = new AacDataStreamElement
        {
            ElementInstanceTag = tag,
            DataByteAlignFlag = align,
            Data = payload,
        };
        return true;
    }

    internal void WriteTo(BitWriter writer)
    {
        if ((uint)ElementInstanceTag > 15)
            throw new InvalidOperationException("ElementInstanceTag must fit in 4 bits.");
        if (Data.Length > MaxDataBytes)
            throw new InvalidOperationException($"Data exceeds {MaxDataBytes}-byte cap.");

        writer.Write(ElementInstanceTag, 4);
        writer.Write(DataByteAlignFlag ? 1u : 0u, 1);

        if (Data.Length < 255)
        {
            writer.Write((uint)Data.Length, 8);
        }
        else
        {
            writer.Write(255u, 8);
            writer.Write((uint)(Data.Length - 255), 8);
        }

        if (DataByteAlignFlag) writer.AlignToByte();

        ReadOnlySpan<byte> span = Data.Span;
        for (int i = 0; i < span.Length; i++) writer.Write(span[i], 8);
    }
}
