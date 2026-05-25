using System.Buffers;
using System.Buffers.Binary;

namespace Mediar.Bitstream;

/// <summary>
/// Convert between Annex-B (start-code prefixed) and AVCC / HVCC
/// (length-prefixed) NAL formats for H.264 / H.265.
/// </summary>
/// <remarks>
/// MP4 / Matroska expect length-prefixed NAL units in sample data with
/// the prefix length stored in the codec configuration record's
/// <c>lengthSizeMinusOne</c> field (typically 4). RTP / TS / raw .h264
/// elementary streams use Annex-B start codes <c>00 00 00 01</c>
/// or <c>00 00 01</c>.
/// </remarks>
public static class AnnexBAvccConverter
{
    /// <summary>
    /// Convert an Annex-B buffer to AVCC / HVCC with the given
    /// length-prefix size (1, 2, or 4 bytes; 4 is recommended).
    /// </summary>
    public static byte[] AnnexBToLengthPrefixed(ReadOnlySpan<byte> annexB, int lengthSize = 4)
    {
        if (lengthSize is not (1 or 2 or 4))
            throw new ArgumentOutOfRangeException(nameof(lengthSize), "lengthSize must be 1, 2 or 4.");
        var nals = AnnexBScanner.FindNalUnits(annexB);
        int total = 0;
        foreach (var n in nals) total += lengthSize + n.Length;
        byte[] result = new byte[total];
        int off = 0;
        foreach (var n in nals)
        {
            WriteLength(result.AsSpan(off, lengthSize), (uint)n.Length, lengthSize);
            off += lengthSize;
            annexB.Slice(n.Offset, n.Length).CopyTo(result.AsSpan(off, n.Length));
            off += n.Length;
        }
        return result;
    }

    /// <summary>
    /// Convert a length-prefixed AVCC / HVCC buffer to Annex-B with
    /// <c>00 00 00 01</c> start codes.
    /// </summary>
    public static byte[] LengthPrefixedToAnnexB(ReadOnlySpan<byte> avcc, int lengthSize = 4)
    {
        if (lengthSize is not (1 or 2 or 4))
            throw new ArgumentOutOfRangeException(nameof(lengthSize));
        using var owner = MemoryPool<byte>.Shared.Rent(avcc.Length + 16);
        var writer = new ArrayBufferWriter<byte>(avcc.Length + 16);
        int pos = 0;
        while (pos + lengthSize <= avcc.Length)
        {
            uint len = ReadLength(avcc.Slice(pos, lengthSize), lengthSize);
            pos += lengthSize;
            if (pos + len > avcc.Length) throw new InvalidDataException("Truncated AVCC NAL.");
            var span = writer.GetSpan(4 + (int)len);
            span[0] = 0; span[1] = 0; span[2] = 0; span[3] = 1;
            avcc.Slice(pos, (int)len).CopyTo(span[4..]);
            writer.Advance(4 + (int)len);
            pos += (int)len;
        }
        return writer.WrittenSpan.ToArray();
    }

    private static void WriteLength(Span<byte> dst, uint value, int lengthSize)
    {
        switch (lengthSize)
        {
            case 1:
                dst[0] = (byte)value;
                break;
            case 2:
                BinaryPrimitives.WriteUInt16BigEndian(dst, (ushort)value);
                break;
            case 4:
                BinaryPrimitives.WriteUInt32BigEndian(dst, value);
                break;
        }
    }

    private static uint ReadLength(ReadOnlySpan<byte> src, int lengthSize) => lengthSize switch
    {
        1 => src[0],
        2 => BinaryPrimitives.ReadUInt16BigEndian(src),
        4 => BinaryPrimitives.ReadUInt32BigEndian(src),
        _ => throw new ArgumentOutOfRangeException(nameof(lengthSize)),
    };

    /// <summary>
    /// Strip H.264 / H.265 emulation prevention bytes (0x03 inserted after
    /// any 0x00 0x00 sequence) from a NAL unit's RBSP payload. Returns
    /// the SODB / EBSP-cleaned bytes.
    /// </summary>
    public static byte[] RemoveEmulationPrevention(ReadOnlySpan<byte> ebsp)
    {
        var sb = new System.Collections.Generic.List<byte>(ebsp.Length);
        for (int i = 0; i < ebsp.Length; i++)
        {
            if (i + 2 < ebsp.Length && ebsp[i] == 0 && ebsp[i + 1] == 0 && ebsp[i + 2] == 0x03)
            {
                sb.Add(0);
                sb.Add(0);
                i += 2;
            }
            else
            {
                sb.Add(ebsp[i]);
            }
        }
        return sb.ToArray();
    }

    /// <summary>
    /// Insert H.264 / H.265 emulation prevention bytes (0x03 after any
    /// 0x00 0x00 followed by a byte ≤ 0x03) into a RBSP payload to
    /// produce a valid EBSP for Annex-B framing.
    /// </summary>
    public static byte[] AddEmulationPrevention(ReadOnlySpan<byte> rbsp)
    {
        var sb = new System.Collections.Generic.List<byte>(rbsp.Length + (rbsp.Length >> 4));
        int zeros = 0;
        for (int i = 0; i < rbsp.Length; i++)
        {
            byte b = rbsp[i];
            if (zeros >= 2 && b <= 0x03)
            {
                sb.Add(0x03);
                zeros = 0;
            }
            sb.Add(b);
            zeros = (b == 0) ? zeros + 1 : 0;
        }
        return sb.ToArray();
    }
}
