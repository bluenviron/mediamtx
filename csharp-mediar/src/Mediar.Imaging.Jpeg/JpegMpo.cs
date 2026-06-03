using System.Buffers.Binary;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Multi-Picture Format (MPO) read+write helpers per CIPA DC-007:2009.
/// </summary>
/// <remarks>
/// <para>
/// An MPO file is two or more independent JPEG bitstreams concatenated
/// back-to-back. The first JPEG carries an APP2 segment beginning
/// <c>"MPF\0"</c> followed by a TIFF stream that lists every sub-image
/// in an MP Index IFD (tag 0xB002, MPEntry table). Each MPEntry is a
/// 16-byte record with an attribute, a 4-byte size, a 4-byte offset
/// (relative to the MP Endian header), and two dependency indices.
/// </para>
/// <para>
/// This class implements the writer side. Reading is provided by
/// <c>Mediar.Imaging.Mpo.MpoReader</c>; the symmetric <see cref="ReadOffsets"/>
/// helper here returns just the per-sub-image byte ranges so that
/// downstream code can stream individual JPEGs without taking the full
/// MPO dependency.
/// </para>
/// </remarks>
public static class JpegMpo
{
    /// <summary>
    /// One sub-image's JPEG bytes plus an MPO image-type classifier.
    /// </summary>
    public sealed record SubImage(ReadOnlyMemory<byte> JpegBytes, uint MpType = 0x030000u);

    /// <summary>
    /// Compose an MPO from a non-empty list of independently-encoded
    /// JPEGs. The MPF APP2 segment is inserted into the first JPEG
    /// (immediately after its SOI marker) and every MPEntry's
    /// <c>DataOffset</c> is patched relative to the resulting MP Endian
    /// header position. The output preserves the original byte content
    /// of every sub-image except for the MPF segment splice in image 0.
    /// </summary>
    public static void Write(Stream output, IReadOnlyList<SubImage> images)
    {
        ArgumentNullException.ThrowIfNull(output);
        ArgumentNullException.ThrowIfNull(images);
        if (images.Count < 2)
        {
            throw new ArgumentException("MPO requires at least 2 sub-images.", nameof(images));
        }

        // Build the MPF APP2 payload (without the FF E2 length prefix).
        // Sub-image sizes after splicing are known once we know the payload size.
        // The payload size depends only on number of images (no UIDs), so it's deterministic.
        int n = images.Count;
        int mpfPayloadSize = BuildMpfPayloadSize(n);
        int mpfSegmentLength = mpfPayloadSize + 2; // +2 for marker-segment length bytes
        int splicedImage0Length = images[0].JpegBytes.Length + 2 /* FF E2 */ + mpfSegmentLength;

        // MP Endian header position in the spliced file:
        //   2 (SOI) + 2 (FF E2) + 2 (seg length) + 4 ("MPF\0") = byte 10
        const int mpEndianBase = 10;

        // First MPEntry has offset 0; subsequent offsets are file_offset_of_subN - mpEndianBase.
        // Build offsets for each sub-image first.
        var subOffsets = new long[n];
        subOffsets[0] = 0;
        long cursor = splicedImage0Length;
        for (int i = 1; i < n; i++)
        {
            subOffsets[i] = cursor;
            cursor += images[i].JpegBytes.Length;
        }

        // Now build the MPF payload with concrete entries.
        byte[] mpfPayload = BuildMpfPayload(images, subOffsets, mpEndianBase);
        if (mpfPayload.Length != mpfPayloadSize)
        {
            throw new InvalidOperationException("MPO payload size mismatch (BUG).");
        }

        // Emit spliced image 0: SOI, APP2(MPF), rest of original (after SOI).
        var img0 = images[0].JpegBytes.Span;
        if (img0.Length < 2 || img0[0] != 0xFF || img0[1] != 0xD8)
        {
            throw new ArgumentException("MPO sub-image 0 is not a JPEG (missing SOI).", nameof(images));
        }
        output.WriteByte(0xFF); output.WriteByte(0xD8);
        output.WriteByte(0xFF); output.WriteByte(0xE2);
        Span<byte> u16 = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)mpfSegmentLength);
        output.Write(u16);
        output.Write(mpfPayload);
        output.Write(img0[2..]);

        // Emit the remaining sub-images verbatim.
        for (int i = 1; i < n; i++)
        {
            output.Write(images[i].JpegBytes.Span);
        }
    }

    /// <summary>
    /// Return the per-sub-image absolute byte ranges declared by the
    /// MPF Index IFD in <paramref name="mpoBytes"/>. Throws
    /// <see cref="System.IO.InvalidDataException"/> if the MPF segment
    /// is missing or malformed.
    /// </summary>
    public static IReadOnlyList<(long Offset, long Length)> ReadOffsets(ReadOnlySpan<byte> mpoBytes)
    {
        if (!TryFindMpf(mpoBytes, out int mpfPayloadStart, out int mpfPayloadLength))
        {
            throw new InvalidDataException("MPO: no MPF APP2 segment found in sub-image 0.");
        }
        int mpEndianBase = mpfPayloadStart + 4; // skip "MPF\0"
        int mpEndianEnd = mpfPayloadStart + mpfPayloadLength;
        if (mpEndianBase + 8 > mpEndianEnd)
        {
            throw new InvalidDataException("MPO: MPF segment truncated before MP Endian header.");
        }
        bool le;
        if (mpoBytes[mpEndianBase] == 'I' && mpoBytes[mpEndianBase + 1] == 'I') le = true;
        else if (mpoBytes[mpEndianBase] == 'M' && mpoBytes[mpEndianBase + 1] == 'M') le = false;
        else throw new InvalidDataException("MPO: bad MP Endian byte order.");

        ushort magic = ReadU16(mpoBytes, mpEndianBase + 2, le);
        if (magic != 42) throw new InvalidDataException("MPO: bad TIFF magic.");
        uint firstIfd = ReadU32(mpoBytes, mpEndianBase + 4, le);
        int ifdAbs = mpEndianBase + (int)firstIfd;
        if (ifdAbs + 2 > mpEndianEnd) throw new InvalidDataException("MPO: IFD truncated.");
        ushort tagCount = ReadU16(mpoBytes, ifdAbs, le);

        int mpEntryOffsetRel = -1, mpEntryLength = 0;
        uint numberOfImages = 0;
        for (int i = 0; i < tagCount; i++)
        {
            int entry = ifdAbs + 2 + i * 12;
            ushort tag = ReadU16(mpoBytes, entry, le);
            ushort type = ReadU16(mpoBytes, entry + 2, le);
            uint count = ReadU32(mpoBytes, entry + 4, le);
            int valueAt = entry + 8;
            int byteCount = TypeByteSize(type) * (int)count;
            switch (tag)
            {
                case 0xB001:
                    if (type == 4 && count == 1) numberOfImages = ReadU32(mpoBytes, valueAt, le);
                    break;
                case 0xB002:
                    mpEntryLength = byteCount;
                    mpEntryOffsetRel = byteCount <= 4
                        ? valueAt - mpEndianBase
                        : (int)ReadU32(mpoBytes, valueAt, le);
                    break;
            }
        }
        if (mpEntryOffsetRel < 0 || numberOfImages == 0 || mpEntryLength != (int)numberOfImages * 16)
        {
            throw new InvalidDataException("MPO: missing or malformed MPEntry table.");
        }
        int mpEntryAbs = mpEndianBase + mpEntryOffsetRel;
        if (mpEntryAbs + mpEntryLength > mpoBytes.Length)
        {
            throw new InvalidDataException("MPO: MPEntry table extends past EOF.");
        }
        var list = new List<(long, long)>((int)numberOfImages);
        for (int i = 0; i < (int)numberOfImages; i++)
        {
            int rec = mpEntryAbs + i * 16;
            uint size = ReadU32(mpoBytes, rec + 4, le);
            uint relOff = ReadU32(mpoBytes, rec + 8, le);
            long absOff = i == 0 ? 0 : mpEndianBase + (long)relOff;
            if (absOff < 0 || absOff + size > mpoBytes.Length)
            {
                throw new InvalidDataException($"MPO: MPEntry[{i}] points outside file bounds.");
            }
            list.Add((absOff, size));
        }
        return list;
    }

    private static int BuildMpfPayloadSize(int n)
    {
        // "MPF\0" + 8 (MP Endian header) + 2 (IFD count) + 3*12 (tags) + 4 (next IFD) + 16*n (MPEntry)
        return 4 + 8 + 2 + 3 * 12 + 4 + 16 * n;
    }

    private static byte[] BuildMpfPayload(IReadOnlyList<SubImage> images, long[] absOffsets, int mpEndianBase)
    {
        int n = images.Count;
        var ms = new MemoryStream();
        ms.Write("MPF\0"u8);
        ms.Write([0x49, 0x49, 0x2A, 0x00]); // II 42
        Span<byte> u32 = stackalloc byte[4];
        Span<byte> u16 = stackalloc byte[2];

        // MP Endian header: byte 0 of MP Endian = byte 4 of MPF payload. Offsets in
        // the IFD are relative to byte 0 of the MP Endian header.
        // First IFD offset = 8 (immediately after MP Endian header).
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 8);
        ms.Write(u32);

        // IFD with 3 tags: 0xB000 Version, 0xB001 NumberOfImages, 0xB002 MPEntry.
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 3); ms.Write(u16);

        // MPEntry table sits after the IFD body (count + 3*12 + 4 nextIfd) = at relative offset 8+2+36+4 = 50.
        const int mpEntryRelOff = 8 + 2 + 36 + 4;

        // 0xB000 MPFVersion, UNDEFINED count=4, inline "0100"
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 0xB000); ms.Write(u16);
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 7);      ms.Write(u16);
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 4);      ms.Write(u32);
        ms.Write("0100"u8);

        // 0xB001 NumberOfImages, LONG count=1, inline
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 0xB001); ms.Write(u16);
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 4);      ms.Write(u16);
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 1);      ms.Write(u32);
        BinaryPrimitives.WriteUInt32LittleEndian(u32, (uint)n); ms.Write(u32);

        // 0xB002 MPEntry, UNDEFINED count=16*n, offset
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 0xB002); ms.Write(u16);
        BinaryPrimitives.WriteUInt16LittleEndian(u16, 7);      ms.Write(u16);
        BinaryPrimitives.WriteUInt32LittleEndian(u32, (uint)(16 * n)); ms.Write(u32);
        BinaryPrimitives.WriteUInt32LittleEndian(u32, (uint)mpEntryRelOff); ms.Write(u32);

        // Next IFD offset = 0.
        BinaryPrimitives.WriteUInt32LittleEndian(u32, 0); ms.Write(u32);

        // MPEntry records.
        for (int i = 0; i < n; i++)
        {
            uint mpType = images[i].MpType;
            uint attr = mpType & 0x00FFFFFFu;
            if (i == 0) attr |= 0x20000000u; // representative
            uint size = (uint)images[i].JpegBytes.Length;
            // For i == 0 the splice adds an APP2 inside image 0; size must reflect post-splice length.
            // The caller computed offsets that include the splice, but the size stored here should
            // remain the size of the bytes that will be at that offset.
            uint relOff = i == 0 ? 0u : (uint)(absOffsets[i] - mpEndianBase);
            if (i == 0)
            {
                // Image 0 size = pre-splice size + 4 (FF E2 + 2 bytes len) + 4 ("MPF\0") + mpf body length.
                // mpfPayload is what we're currently building; image 0 length = original + 2 (FF E2) + 2 (len)
                //   + 4 ("MPF\0") + (payload_size - 4 (MPF identifier already counted)).
                // Simplified: image 0 size = orig + 4 + (payload size including "MPF\0")
                size = (uint)(images[0].JpegBytes.Length + 4 + BuildMpfPayloadSize(n));
            }
            BinaryPrimitives.WriteUInt32LittleEndian(u32, attr); ms.Write(u32);
            BinaryPrimitives.WriteUInt32LittleEndian(u32, size); ms.Write(u32);
            BinaryPrimitives.WriteUInt32LittleEndian(u32, relOff); ms.Write(u32);
            BinaryPrimitives.WriteUInt16LittleEndian(u16, 0); ms.Write(u16);
            BinaryPrimitives.WriteUInt16LittleEndian(u16, 0); ms.Write(u16);
        }

        return ms.ToArray();
    }

    private static bool TryFindMpf(ReadOnlySpan<byte> b, out int payloadStart, out int payloadLength)
    {
        payloadStart = 0;
        payloadLength = 0;
        if (b.Length < 4 || b[0] != 0xFF || b[1] != 0xD8) return false;
        int i = 2;
        while (i + 4 <= b.Length)
        {
            if (b[i] != 0xFF) return false;
            byte m = b[i + 1];
            if (m == 0xFF) { i++; continue; }
            if (m == 0xDA || m == 0xD9) return false;
            if (m == 0x01 || (m >= 0xD0 && m <= 0xD7)) { i += 2; continue; }
            int segLen = (b[i + 2] << 8) | b[i + 3];
            if (segLen < 2 || i + 2 + segLen > b.Length) return false;
            int dataAt = i + 4;
            int dataLen = segLen - 2;
            if (m == 0xE2 && dataLen >= 4 &&
                b[dataAt] == 'M' && b[dataAt + 1] == 'P' && b[dataAt + 2] == 'F' && b[dataAt + 3] == 0)
            {
                payloadStart = dataAt;
                payloadLength = dataLen;
                return true;
            }
            i = dataAt + dataLen;
        }
        return false;
    }

    private static ushort ReadU16(ReadOnlySpan<byte> b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt16LittleEndian(b[o..])
        : BinaryPrimitives.ReadUInt16BigEndian(b[o..]);

    private static uint ReadU32(ReadOnlySpan<byte> b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt32LittleEndian(b[o..])
        : BinaryPrimitives.ReadUInt32BigEndian(b[o..]);

    private static int TypeByteSize(ushort t) => t switch
    {
        1 or 2 or 7 => 1, 3 => 2, 4 or 9 => 4, 5 or 10 => 8, _ => 1,
    };
}
