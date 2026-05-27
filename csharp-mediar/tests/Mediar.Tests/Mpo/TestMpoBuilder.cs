using System.Buffers.Binary;

namespace Mediar.Tests.Mpo;

/// <summary>
/// Test-only synthesiser for MPO (Multi-Picture Object) byte streams
/// that conform to CIPA DC-007. Each spec lists the per-sub-image JPEG
/// payloads plus the MPF index attributes; the builder constructs a
/// valid first-image JPEG with an APP2 MPF segment, then concatenates
/// the remaining JPEG payloads, patching the MPEntry table to point at
/// each one.
/// </summary>
internal static class TestMpoBuilder
{
    public sealed record MpoEntrySpec
    {
        public byte[] JpegBytes { get; init; } = [];
        public uint Attribute { get; init; }
        public ushort DependentImage1 { get; init; }
        public ushort DependentImage2 { get; init; }
    }

    public sealed record MpoSpec
    {
        /// <summary>Sub-image JPEG payloads. The first one becomes the parent.</summary>
        public IReadOnlyList<MpoEntrySpec> Entries { get; init; } = [];

        /// <summary>If true, omit the MPF APP2 segment (used to test rejection).</summary>
        public bool OmitMpfSegment { get; init; }

        /// <summary>If non-null, force MPFVersion to this 4-byte ASCII string (default "0100").</summary>
        public string? OverrideVersion { get; init; }

        /// <summary>If non-empty, append a 33-byte ImageUID per entry under tag 0xB003.</summary>
        public IReadOnlyList<string>? ImageUids { get; init; }

        /// <summary>If true, declare NumberOfImages = Entries.Count + 1 (deliberate mismatch).</summary>
        public bool DeclareMismatchedImageCount { get; init; }
    }

    /// <summary>
    /// Build a complete MPO byte stream from the spec. The first entry's
    /// JPEG receives an injected APP2 MPF segment; subsequent entries
    /// are concatenated unmodified.
    /// </summary>
    public static byte[] Build(MpoSpec spec)
    {
        if (spec.Entries.Count == 0)
        {
            throw new ArgumentException("MPO must contain at least one entry.");
        }

        // Step 1: build the MP Index IFD payload (little-endian).
        // Tag entries (12 bytes each):
        //   0xB000 UNDEFINED count=4   value=ASCII "0100" (inline)
        //   0xB001 LONG       count=1   value=NumberOfImages (inline)
        //   0xB002 UNDEFINED  count=16*N value-offset = pointer relative to MP Endian header
        //   0xB003 UNDEFINED  count=33*N value-offset = pointer (only if UIDs present)
        // Followed by next-IFD offset (4 bytes, 0 = no next).
        bool emitUids = spec.ImageUids is { Count: > 0 };
        int tagCount = emitUids ? 4 : 3;
        int ifdSize = 2 + tagCount * 12 + 4; // count + entries + next-IFD slot

        // MP Endian header is 8 bytes (II/MM + magic + first IFD offset).
        // The MP Index IFD lives at offset 8 (right after the header).
        // The MPEntry table follows the IFD; UIDs (if any) follow the MPEntry table.
        int mpEntryRelOffset = 8 + ifdSize;
        int mpEntryLength = spec.Entries.Count * 16;
        int uidsRelOffset = mpEntryRelOffset + mpEntryLength;
        int uidsLength = emitUids ? spec.ImageUids!.Count * 33 : 0;

        int mpEndianSize = 8 + ifdSize + mpEntryLength + uidsLength;

        // Now build the MP Endian + IFD + MPEntry + UIDs block.
        byte[] mpEndian = new byte[mpEndianSize];
        // II + magic + first-IFD offset = 8
        mpEndian[0] = 0x49; mpEndian[1] = 0x49;
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(2), 42);
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(4), 8u);

        // IFD entry count.
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(8), (ushort)tagCount);

        int entryAt = 10;
        // MPFVersion (0xB000)
        string version = spec.OverrideVersion ?? "0100";
        if (version.Length != 4) throw new ArgumentException("MPFVersion must be 4 ASCII chars.");
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt), 0xB000);
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt + 2), 7); // UNDEFINED
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 4), 4); // count
        mpEndian[entryAt + 8] = (byte)version[0];
        mpEndian[entryAt + 9] = (byte)version[1];
        mpEndian[entryAt + 10] = (byte)version[2];
        mpEndian[entryAt + 11] = (byte)version[3];
        entryAt += 12;

        // NumberOfImages (0xB001)
        uint declaredCount = (uint)(spec.DeclareMismatchedImageCount ? spec.Entries.Count + 1 : spec.Entries.Count);
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt), 0xB001);
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt + 2), 4); // LONG
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 4), 1);
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 8), declaredCount);
        entryAt += 12;

        // MPEntry (0xB002)
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt), 0xB002);
        BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt + 2), 7); // UNDEFINED
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 4), (uint)mpEntryLength);
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 8), (uint)mpEntryRelOffset);
        entryAt += 12;

        if (emitUids)
        {
            BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt), 0xB003);
            BinaryPrimitives.WriteUInt16LittleEndian(mpEndian.AsSpan(entryAt + 2), 7);
            BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 4), (uint)uidsLength);
            BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt + 8), (uint)uidsRelOffset);
            entryAt += 12;
        }

        // next-IFD offset = 0 (we don't chain).
        BinaryPrimitives.WriteUInt32LittleEndian(mpEndian.AsSpan(entryAt), 0u);

        // MPEntry table will be written after we know absolute offsets.
        // Placeholder zeros for now; we patch after assembling the file.

        // Optional UIDs.
        if (emitUids)
        {
            for (int i = 0; i < spec.ImageUids!.Count; i++)
            {
                string uid = spec.ImageUids[i];
                int dst = uidsRelOffset + i * 33;
                int n = Math.Min(uid.Length, 33);
                for (int k = 0; k < n; k++) mpEndian[dst + k] = (byte)uid[k];
            }
        }

        // Step 2: build the first JPEG (with the MPF APP2 segment injected
        // right after SOI).
        byte[] firstJpeg = spec.Entries[0].JpegBytes;
        if (firstJpeg.Length < 4 || firstJpeg[0] != 0xFF || firstJpeg[1] != 0xD8)
        {
            throw new ArgumentException("First entry JpegBytes must start with SOI marker.");
        }

        byte[] mpfPayload = new byte[s_mpfId.Length + mpEndian.Length];
        Buffer.BlockCopy(s_mpfId, 0, mpfPayload, 0, s_mpfId.Length);
        Buffer.BlockCopy(mpEndian, 0, mpfPayload, s_mpfId.Length, mpEndian.Length);

        byte[] app2 = spec.OmitMpfSegment ? [] : BuildApp2(mpfPayload);

        // Stitch: SOI + APP2 + (firstJpeg without leading SOI).
        var firstWithMpf = new List<byte>(2 + app2.Length + firstJpeg.Length - 2);
        firstWithMpf.Add(0xFF);
        firstWithMpf.Add(0xD8);
        firstWithMpf.AddRange(app2);
        firstWithMpf.AddRange(firstJpeg.AsSpan(2).ToArray());

        // Step 3: figure out absolute file offsets for each sub-image
        // and patch the MPEntry table inside the assembled byte stream.
        int firstLength = firstWithMpf.Count;
        var allBytes = new List<byte>(firstWithMpf);
        long[] subOffsets = new long[spec.Entries.Count];
        uint[] subSizes = new uint[spec.Entries.Count];
        subOffsets[0] = 0;
        subSizes[0] = (uint)firstLength;
        long cursor = firstLength;
        for (int i = 1; i < spec.Entries.Count; i++)
        {
            subOffsets[i] = cursor;
            subSizes[i] = (uint)spec.Entries[i].JpegBytes.Length;
            allBytes.AddRange(spec.Entries[i].JpegBytes);
            cursor += spec.Entries[i].JpegBytes.Length;
        }

        if (spec.OmitMpfSegment)
        {
            return [.. allBytes];
        }

        // Locate the MP Endian base inside the assembled file:
        //   = 2 (SOI) + 4 (APP2 header: marker + length) + 4 (MPF\0) = 10.
        const int mpEndianBaseInFile = 2 + 4 + 4;

        // Patch the MPEntry table inside the assembled file.
        int mpEntryAbsInFile = mpEndianBaseInFile + mpEntryRelOffset;
        for (int i = 0; i < spec.Entries.Count; i++)
        {
            int rec = mpEntryAbsInFile + i * 16;
            uint attr = spec.Entries[i].Attribute;
            BinaryPrimitives.WriteUInt32LittleEndian(allBytes.AsBytesSpan(rec), attr);
            BinaryPrimitives.WriteUInt32LittleEndian(allBytes.AsBytesSpan(rec + 4), subSizes[i]);
            uint relOffset = i == 0 ? 0u : (uint)(subOffsets[i] - mpEndianBaseInFile);
            BinaryPrimitives.WriteUInt32LittleEndian(allBytes.AsBytesSpan(rec + 8), relOffset);
            BinaryPrimitives.WriteUInt16LittleEndian(allBytes.AsBytesSpan(rec + 12), spec.Entries[i].DependentImage1);
            BinaryPrimitives.WriteUInt16LittleEndian(allBytes.AsBytesSpan(rec + 14), spec.Entries[i].DependentImage2);
        }

        return [.. allBytes];
    }

    private static readonly byte[] s_mpfId = "MPF\0"u8.ToArray();

    private static byte[] BuildApp2(byte[] payload)
    {
        // FF E2  + 2-byte length (includes the 2 length bytes) + payload.
        int len = 2 + payload.Length;
        if (len > 65535) throw new ArgumentException("APP2 segment too large.");
        var bytes = new byte[2 + 2 + payload.Length];
        bytes[0] = 0xFF;
        bytes[1] = 0xE2;
        BinaryPrimitives.WriteUInt16BigEndian(bytes.AsSpan(2), (ushort)len);
        Buffer.BlockCopy(payload, 0, bytes, 4, payload.Length);
        return bytes;
    }
}

internal static class ListByteSpanExtensions
{
    /// <summary>
    /// Materialise a writable span over the backing storage of a
    /// <see cref="List{T}"/> of bytes by copying through an array.
    /// (We can't get a writable Span directly from List, so we maintain
    /// a parallel array. Used only by TestMpoBuilder.)
    /// </summary>
    public static Span<byte> AsBytesSpan(this List<byte> list, int offset)
    {
        // List<byte>.AsSpan() is not available, so we use a helper that
        // exposes the internal _items array via CollectionsMarshal.
        var items = System.Runtime.InteropServices.CollectionsMarshal.AsSpan(list);
        return items.Slice(offset);
    }
}
