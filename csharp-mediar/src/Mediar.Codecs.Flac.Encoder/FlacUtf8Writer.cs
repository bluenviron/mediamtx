namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Writes the UTF-8-encoded frame number used by FLAC frame headers
/// (RFC 9639 §10.2). FLAC re-uses the UTF-8 envelope for up to 36 bits
/// of payload, not the 21-bit Unicode codepoint range.
/// </summary>
internal static class FlacUtf8Writer
{
    /// <summary>
    /// Encode <paramref name="value"/> as a 1–7-byte UTF-8 sequence and write
    /// it into <paramref name="dest"/>. Returns the number of bytes written.
    /// </summary>
    public static int Write(ulong value, Span<byte> dest)
    {
        if (value < 0x80UL)
        {
            dest[0] = (byte)value;
            return 1;
        }
        if (value < 0x800UL)
        {
            dest[0] = (byte)(0xC0 | (value >> 6));
            dest[1] = (byte)(0x80 | (value & 0x3F));
            return 2;
        }
        if (value < 0x10000UL)
        {
            dest[0] = (byte)(0xE0 | (value >> 12));
            dest[1] = (byte)(0x80 | ((value >> 6) & 0x3F));
            dest[2] = (byte)(0x80 | (value & 0x3F));
            return 3;
        }
        if (value < 0x200000UL)
        {
            dest[0] = (byte)(0xF0 | (value >> 18));
            dest[1] = (byte)(0x80 | ((value >> 12) & 0x3F));
            dest[2] = (byte)(0x80 | ((value >> 6) & 0x3F));
            dest[3] = (byte)(0x80 | (value & 0x3F));
            return 4;
        }
        if (value < 0x4000000UL)
        {
            dest[0] = (byte)(0xF8 | (value >> 24));
            dest[1] = (byte)(0x80 | ((value >> 18) & 0x3F));
            dest[2] = (byte)(0x80 | ((value >> 12) & 0x3F));
            dest[3] = (byte)(0x80 | ((value >> 6) & 0x3F));
            dest[4] = (byte)(0x80 | (value & 0x3F));
            return 5;
        }
        if (value < 0x80000000UL)
        {
            dest[0] = (byte)(0xFC | (value >> 30));
            dest[1] = (byte)(0x80 | ((value >> 24) & 0x3F));
            dest[2] = (byte)(0x80 | ((value >> 18) & 0x3F));
            dest[3] = (byte)(0x80 | ((value >> 12) & 0x3F));
            dest[4] = (byte)(0x80 | ((value >> 6) & 0x3F));
            dest[5] = (byte)(0x80 | (value & 0x3F));
            return 6;
        }
        // 7-byte form: leading 0xFE, six continuation bytes.
        dest[0] = 0xFE;
        dest[1] = (byte)(0x80 | ((value >> 30) & 0x3F));
        dest[2] = (byte)(0x80 | ((value >> 24) & 0x3F));
        dest[3] = (byte)(0x80 | ((value >> 18) & 0x3F));
        dest[4] = (byte)(0x80 | ((value >> 12) & 0x3F));
        dest[5] = (byte)(0x80 | ((value >> 6) & 0x3F));
        dest[6] = (byte)(0x80 | (value & 0x3F));
        return 7;
    }

    /// <summary>Maximum bytes that <see cref="Write"/> may emit.</summary>
    public const int MaxBytes = 7;
}
