using System.Buffers.Binary;
using System.Text;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Writes EXIF (APP1), ICC profile (APP2), and XMP (APP1) metadata
/// segments after the JFIF APP0 marker emitted by
/// <see cref="JpegBaselineEncoder"/>.
/// </summary>
/// <remarks>
/// <list type="bullet">
///   <item><description>
///     EXIF is serialised as a minimal TIFF stream (II byte-order,
///     magic 42, single IFD0). The same stream round-trips through
///     <see cref="Mediar.Imaging.Metadata.ExifParser"/> so tag values
///     survive a full encode→decode cycle.
///   </description></item>
///   <item><description>
///     ICC profiles longer than the 65 533-byte APP2 payload limit are
///     chunked per ICC.1:2010 Annex B.4: each segment carries an
///     "ICC_PROFILE\0" identifier, a 1-based chunk number and the
///     total chunk count.
///   </description></item>
///   <item><description>
///     XMP packets are wrapped in a single APP1 segment whose payload
///     begins with the namespace URI <c>http://ns.adobe.com/xap/1.0/\0</c>
///     (Adobe XMP Specification Part 3, 2012).
///   </description></item>
/// </list>
/// </remarks>
public static class JpegMetadataWriter
{
    /// <summary>Maximum APP segment payload size (65 533 = 65 535 − 2 length bytes).</summary>
    public const int MaxSegmentPayload = 65_533;

    /// <summary>
    /// Emit any of EXIF / ICC / XMP that the caller supplied, in the
    /// canonical JFIF marker order.
    /// </summary>
    public static void WriteOptionalMetadata(
        Stream output,
        IReadOnlyDictionary<string, string>? exif,
        ReadOnlyMemory<byte> iccProfile,
        string? xmp)
    {
        if (exif is { Count: > 0 })
        {
            WriteExifApp1(output, exif);
        }
        if (!iccProfile.IsEmpty)
        {
            WriteIccApp2Chunks(output, iccProfile.Span);
        }
        if (!string.IsNullOrEmpty(xmp))
        {
            WriteXmpApp1(output, xmp);
        }
    }

    // ---- EXIF ----

    /// <summary>Build a minimal TIFF/EXIF payload from a tag map.</summary>
    public static byte[] BuildExifPayload(IReadOnlyDictionary<string, string> exif)
    {
        ArgumentNullException.ThrowIfNull(exif);
        // Collect IFD0:* tags whose numeric tag id and ASCII value are stable.
        var entries = new List<(ushort Tag, string Value)>();
        foreach (var (key, value) in exif)
        {
            if (!key.StartsWith("IFD0:", StringComparison.Ordinal)) continue;
            string name = key[5..];
            if (!s_ifd0Tags.TryGetValue(name, out ushort tagId)) continue;
            entries.Add((tagId, value));
        }
        entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

        // Layout: "II" 0x002A LE, uint32 offset (8), then IFD = uint16 count + N*12 + uint32 nextOffset.
        // ASCII values longer than 4 bytes are spilled into the heap after the IFD.
        int ifdSize = 2 + entries.Count * 12 + 4;
        var heap = new List<byte>();
        var ms = new MemoryStream();
        ms.Write([0x49, 0x49, 0x2A, 0x00]); // II 42
        Span<byte> u32 = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 8); ms.Write(u32);
        Span<byte> u16 = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(u16, (ushort)entries.Count); ms.Write(u16);

        int heapBase = 8 + ifdSize;
        foreach (var (tag, value) in entries)
        {
            byte[] ascii = Encoding.ASCII.GetBytes(value + "\0");
            BinaryPrimitives.WriteUInt16LittleEndian(u16, tag); ms.Write(u16);
            BinaryPrimitives.WriteUInt16LittleEndian(u16, 2);   ms.Write(u16); // ASCII
            BinaryPrimitives.WriteUInt32LittleEndian(u32, (uint)ascii.Length); ms.Write(u32);
            if (ascii.Length <= 4)
            {
                Span<byte> inline = stackalloc byte[4];
                ascii.AsSpan().CopyTo(inline);
                ms.Write(inline);
            }
            else
            {
                BinaryPrimitives.WriteUInt32LittleEndian(u32, (uint)(heapBase + heap.Count));
                ms.Write(u32);
                heap.AddRange(ascii);
            }
        }
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 0); ms.Write(u32); // next IFD = 0
        ms.Write(heap.ToArray());
        return ms.ToArray();
    }

    private static void WriteExifApp1(Stream s, IReadOnlyDictionary<string, string> exif)
    {
        var payload = BuildExifPayload(exif);
        int total = 6 + payload.Length;
        if (total > MaxSegmentPayload)
        {
            throw new InvalidOperationException(
                $"EXIF payload {total} bytes exceeds APP1 segment limit ({MaxSegmentPayload}).");
        }
        WriteSegmentHeader(s, 0xE1, total);
        s.Write("Exif\0\0"u8);
        s.Write(payload);
    }

    // ---- ICC ----

    private static void WriteIccApp2Chunks(Stream s, ReadOnlySpan<byte> icc)
    {
        const int idLen = 12; // "ICC_PROFILE\0"
        int per = MaxSegmentPayload - idLen - 2; // 2 bytes for chunk# + total
        int chunks = (icc.Length + per - 1) / per;
        if (chunks > 255)
        {
            throw new InvalidOperationException(
                $"ICC profile too large to fit in 255 APP2 chunks (got {chunks}).");
        }

        ReadOnlySpan<byte> id = "ICC_PROFILE\0"u8;
        for (int i = 0; i < chunks; i++)
        {
            int off = i * per;
            int len = Math.Min(per, icc.Length - off);
            int payload = idLen + 2 + len;
            WriteSegmentHeader(s, 0xE2, payload);
            s.Write(id);
            s.WriteByte((byte)(i + 1));
            s.WriteByte((byte)chunks);
            s.Write(icc.Slice(off, len));
        }
    }

    // ---- XMP ----

    private static void WriteXmpApp1(Stream s, string xmp)
    {
        ReadOnlySpan<byte> ns = "http://ns.adobe.com/xap/1.0/\0"u8;
        byte[] packet = Encoding.UTF8.GetBytes(xmp);
        int total = ns.Length + packet.Length;
        if (total > MaxSegmentPayload)
        {
            throw new InvalidOperationException(
                $"XMP packet {total} bytes exceeds APP1 segment limit ({MaxSegmentPayload}).");
        }
        WriteSegmentHeader(s, 0xE1, total);
        s.Write(ns);
        s.Write(packet);
    }

    // ---- Helpers ----

    private static void WriteSegmentHeader(Stream s, byte marker, int payloadLength)
    {
        s.WriteByte(0xFF);
        s.WriteByte(marker);
        Span<byte> len = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(len, (ushort)(payloadLength + 2));
        s.Write(len);
    }

    // ---- IFD0 tag mapping ----

    private static readonly Dictionary<string, ushort> s_ifd0Tags = new(StringComparer.Ordinal)
    {
        ["Make"] = 0x010F,
        ["Model"] = 0x0110,
        ["Orientation"] = 0x0112,
        ["XResolution"] = 0x011A,
        ["YResolution"] = 0x011B,
        ["ResolutionUnit"] = 0x0128,
        ["Software"] = 0x0131,
        ["DateTime"] = 0x0132,
        ["Artist"] = 0x013B,
        ["Copyright"] = 0x8298,
        ["ImageDescription"] = 0x010E,
    };
}
