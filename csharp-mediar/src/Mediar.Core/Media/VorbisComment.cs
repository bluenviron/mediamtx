using System.Buffers.Binary;
using System.Text;

namespace Mediar;

/// <summary>
/// Reads a Vorbis comment payload — used by FLAC's <c>VORBIS_COMMENT</c>
/// metadata block (RFC 9639), Ogg Vorbis (header packet 2), Ogg Opus
/// (the <c>OpusTags</c> packet which shares the byte layout from the vendor
/// field onwards), and Theora.
/// </summary>
/// <remarks>
/// Payload layout:
/// <c>[u32_le vendor_len][vendor_utf8][u32_le user_count]([u32_le c_len][c_utf8] …)</c>.
/// Each user comment is a UTF-8 string of the form <c>"FIELD=value"</c>.
/// </remarks>
public static class VorbisComment
{
    /// <summary>
    /// Push every parsed tag (and the vendor string) into <paramref name="meta"/>.
    /// Returns the number of tags actually written.
    /// </summary>
    public static int ReadInto(ReadOnlySpan<byte> payload, MediaMetadataBuilder meta)
    {
        ArgumentNullException.ThrowIfNull(meta);
        if (payload.Length < 8) return 0;
        int p = 0;
        uint vendorLen = BinaryPrimitives.ReadUInt32LittleEndian(payload[p..]);
        p += 4;
        if (vendorLen > (uint)(payload.Length - p)) return 0;
        if (vendorLen > 0)
        {
            string vendor = Encoding.UTF8.GetString(payload.Slice(p, (int)vendorLen));
            meta.SetVendor(vendor);
            p += (int)vendorLen;
        }
        if (p + 4 > payload.Length) return 0;

        uint count = BinaryPrimitives.ReadUInt32LittleEndian(payload[p..]);
        p += 4;
        int written = 0;
        for (uint i = 0; i < count; i++)
        {
            if (p + 4 > payload.Length) break;
            uint len = BinaryPrimitives.ReadUInt32LittleEndian(payload[p..]);
            p += 4;
            if (len > (uint)(payload.Length - p)) break;
            if (len > 0)
            {
                string entry = Encoding.UTF8.GetString(payload.Slice(p, (int)len));
                int eq = entry.IndexOf('=');
                if (eq > 0)
                {
                    string key = entry[..eq];
                    string value = entry[(eq + 1)..];
                    meta.Set(key, value);
                    written++;
                }
                p += (int)len;
            }
        }
        return written;
    }
}
