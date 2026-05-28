namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Typed view of an AAC FIL <c>EXT_FILL_DATA</c> payload (extension_type
/// 0x1) per ISO/IEC 14496-3 §4.5.2.13.3 Table 4.58. After the 4-bit
/// <c>extension_type</c> field already split off by
/// <see cref="AacFillExtensionPayload"/>, the body is a 4-bit
/// <c>fill_nibble</c> followed by <c>cnt - 1</c> 8-bit
/// <c>fill_byte</c> values. The spec specifies <c>fill_nibble = 0x0</c>
/// and <c>fill_byte = 0xA5</c>; <see cref="IsConformant"/> reports
/// whether the observed payload matches that pattern.
/// </summary>
public sealed record AacFillDataExtension
{
    /// <summary>Spec-mandated value of <c>fill_nibble</c>.</summary>
    public const byte ExpectedFillNibble = 0x0;

    /// <summary>Spec-mandated value of each <c>fill_byte</c>.</summary>
    public const byte ExpectedFillByte = 0xA5;

    /// <summary>4-bit <c>fill_nibble</c> field.</summary>
    public required byte FillNibble { get; init; }

    /// <summary><c>fill_byte[]</c> values (one per FIL <c>cnt &gt; 1</c>).</summary>
    public required ReadOnlyMemory<byte> FillBytes { get; init; }

    /// <summary>
    /// True when every observed value matches the spec-mandated pattern
    /// (<see cref="ExpectedFillNibble"/> = 0x0 and every
    /// <see cref="FillBytes"/> entry = 0xA5).
    /// </summary>
    public bool IsConformant
    {
        get
        {
            if (FillNibble != ExpectedFillNibble) return false;
            ReadOnlySpan<byte> span = FillBytes.Span;
            for (int i = 0; i < span.Length; i++)
            {
                if (span[i] != ExpectedFillByte) return false;
            }
            return true;
        }
    }

    /// <summary>
    /// Parse a FIL <c>EXT_FILL_DATA</c> body. The expected bit shape is
    /// <c>4 + 8 * (cnt - 1)</c> with <c>cnt &gt;= 1</c>, so
    /// <paramref name="bodyBitLength"/> must satisfy
    /// <c>bodyBitLength &gt;= 4</c> and
    /// <c>(bodyBitLength - 4) % 8 == 0</c>. Returns false on any other
    /// shape or on truncation.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> body,
        int bodyBitLength,
        out AacFillDataExtension? data)
    {
        data = null;
        if (bodyBitLength < 4) return false;
        if (body.Length * 8 < bodyBitLength) return false;

        int remaining = bodyBitLength - 4;
        if ((remaining & 7) != 0) return false;
        int byteCount = remaining >> 3;

        try
        {
            var reader = new BitReader(body);
            byte nibble = (byte)reader.ReadBits(4);
            byte[] fillBytes = byteCount == 0 ? Array.Empty<byte>() : new byte[byteCount];
            for (int i = 0; i < byteCount; i++) fillBytes[i] = (byte)reader.ReadBits(8);
            data = new AacFillDataExtension
            {
                FillNibble = nibble,
                FillBytes = fillBytes,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            data = null;
            return false;
        }
    }
}
