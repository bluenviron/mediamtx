using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Crw;

/// <summary>
/// Synthesises minimal but spec-conforming Canon CIFF v1 (CRW) byte
/// streams used by <see cref="CrwReaderTests"/>. Layout per the Canon
/// CIFF v1.0R specification: 26-byte header ("II"/"MM" + u32 header
/// length + "HEAPCCDR" + u32 version + 8 reserved), heap body (entry
/// payloads back-to-back), heap directory (u16 entry count + N x 10
/// byte (tag, size, payload-offset) entries), trailing u32 directory
/// offset (relative to the start of the heap body).
/// </summary>
internal static class TestCrwBuilder
{
    internal sealed record CrwSpec
    {
        /// <summary>True for little-endian ("II"), false for big-endian ("MM"). Default little-endian.</summary>
        public bool LittleEndian { get; init; } = true;

        /// <summary>Optional override for the byte-order mark (used for rejection testing).</summary>
        public byte[]? OverrideByteOrderMark { get; init; }

        /// <summary>Optional override for the 8-byte HEAPCCDR signature (used for rejection testing).</summary>
        public byte[]? OverrideSignature { get; init; }

        /// <summary>CIFF version field (e.g. 0x00010002 = v1.2).</summary>
        public uint Version { get; init; } = 0x00010002;

        /// <summary>Top-level entries. The builder lays out their payloads in declaration order before the directory.</summary>
        public IReadOnlyList<EntrySpec> Entries { get; init; } = Array.Empty<EntrySpec>();

        /// <summary>If true, the produced byte array is truncated to <see cref="TruncateTo"/> bytes.</summary>
        public bool Truncate { get; init; }
        public int TruncateTo { get; init; }
    }

    internal sealed record EntrySpec
    {
        /// <summary>Full 16-bit CIFF tag (high nibble = category, low 12 bits = id).</summary>
        public required ushort Tag { get; init; }

        /// <summary>Payload bytes for non-sub-heap entries.</summary>
        public byte[]? Payload { get; init; }

        /// <summary>Child entries if this is a sub-heap (tag must be 0x3xxx).</summary>
        public IReadOnlyList<EntrySpec>? Children { get; init; }
    }

    internal static byte[] Build(CrwSpec spec)
    {
        bool le = spec.LittleEndian;

        // First lay out every payload (recursively materialising sub-heaps)
        // into a heap-body buffer, recording (offset, size) for each entry.
        var heap = new MemoryStream();
        var topLevelOffsets = new List<(uint offset, uint size)>();
        foreach (var e in spec.Entries)
        {
            var (off, size) = MaterialiseEntry(heap, e, le);
            topLevelOffsets.Add((off, size));
        }

        // Top-level directory at the end of the heap body (current position).
        uint topDirOffset = (uint)heap.Position;
        WriteDirectory(heap, spec.Entries, topLevelOffsets, le);

        byte[] heapBytes = heap.ToArray();

        // Assemble: 26-byte header + heap body + trailing u32 directory offset.
        var ms = new MemoryStream();
        var bom = spec.OverrideByteOrderMark
            ?? (le ? new byte[] { (byte)'I', (byte)'I' } : new byte[] { (byte)'M', (byte)'M' });
        ms.Write(bom, 0, bom.Length);
        WriteU32(ms, 26u, le); // header length
        var sig = spec.OverrideSignature ?? Encoding.ASCII.GetBytes("HEAPCCDR");
        ms.Write(sig, 0, sig.Length);
        WriteU32(ms, spec.Version, le); // version
        ms.Write(new byte[8], 0, 8); // reserved

        ms.Write(heapBytes, 0, heapBytes.Length);
        WriteU32(ms, topDirOffset, le); // trailing directory offset (relative to heap base)

        var bytes = ms.ToArray();
        if (spec.Truncate && spec.TruncateTo < bytes.Length)
        {
            bytes = bytes.AsSpan(0, spec.TruncateTo).ToArray();
        }
        return bytes;
    }

    private static (uint offset, uint size) MaterialiseEntry(MemoryStream heap, EntrySpec e, bool le)
    {
        uint offset = (uint)heap.Position;
        if (e.Children is { } children)
        {
            // Sub-heap: lay out children, then their directory, then a
            // trailing u32 pointing at the directory (relative to the
            // sub-heap's start, i.e. this offset).
            var subStart = heap.Position;
            var childOffsets = new List<(uint offset, uint size)>();
            foreach (var c in children)
            {
                long childStart = heap.Position;
                var (childOff, childSize) = MaterialiseEntry(heap, c, le);
                // childOff is absolute in heap; convert to sub-heap-relative
                // by subtracting subStart.
                childOffsets.Add(((uint)(childOff - subStart), childSize));
            }
            uint subDirOffset = (uint)(heap.Position - subStart);
            WriteDirectory(heap, children, childOffsets, le);
            WriteU32(heap, subDirOffset, le); // trailing offset, sub-heap-relative
            uint size = (uint)(heap.Position - subStart);
            return (offset, size);
        }
        var payload = e.Payload ?? Array.Empty<byte>();
        heap.Write(payload, 0, payload.Length);
        return (offset, (uint)payload.Length);
    }

    private static void WriteDirectory(
        MemoryStream heap,
        IReadOnlyList<EntrySpec> entries,
        List<(uint offset, uint size)> placements,
        bool le)
    {
        WriteU16(heap, (ushort)entries.Count, le);
        for (int i = 0; i < entries.Count; i++)
        {
            WriteU16(heap, entries[i].Tag, le);
            WriteU32(heap, placements[i].size, le);
            WriteU32(heap, placements[i].offset, le);
        }
    }

    private static void WriteU16(Stream s, ushort v, bool le)
    {
        Span<byte> b = stackalloc byte[2];
        if (le) BinaryPrimitives.WriteUInt16LittleEndian(b, v);
        else BinaryPrimitives.WriteUInt16BigEndian(b, v);
        s.Write(b);
    }

    private static void WriteU32(Stream s, uint v, bool le)
    {
        Span<byte> b = stackalloc byte[4];
        if (le) BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        else BinaryPrimitives.WriteUInt32BigEndian(b, v);
        s.Write(b);
    }

    // ---------- helpers for canonical tag payloads ----------

    /// <summary>Build a NUL-terminated ASCII payload (camera type / firmware / owner-name style).</summary>
    internal static byte[] AsciiPayload(string text)
    {
        var bytes = Encoding.ASCII.GetBytes(text);
        var buf = new byte[bytes.Length + 1];
        Buffer.BlockCopy(bytes, 0, buf, 0, bytes.Length);
        return buf;
    }

    /// <summary>Build a "Make\0Model\0" payload for tag 0x080A.</summary>
    internal static byte[] CameraTypePayload(string make, string model)
    {
        var makeBytes = Encoding.ASCII.GetBytes(make);
        var modelBytes = Encoding.ASCII.GetBytes(model);
        var buf = new byte[makeBytes.Length + 1 + modelBytes.Length + 1];
        Buffer.BlockCopy(makeBytes, 0, buf, 0, makeBytes.Length);
        Buffer.BlockCopy(modelBytes, 0, buf, makeBytes.Length + 1, modelBytes.Length);
        return buf;
    }

    /// <summary>Build a 28-byte ImageSpec payload (tag 0x1810) per Canon CIFF v1.0R.</summary>
    internal static byte[] ImageSpecPayload(uint width, uint height,
                                            uint aspectNum, uint aspectDen,
                                            uint rotation, uint componentBitDepth,
                                            uint colorBitDepth, bool le)
    {
        var buf = new byte[28];
        var s = buf.AsSpan();
        if (le)
        {
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(0, 4), width);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(4, 4), height);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(8, 4), aspectNum);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(12, 4), aspectDen);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(16, 4), rotation);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(20, 4), componentBitDepth);
            BinaryPrimitives.WriteUInt32LittleEndian(s.Slice(24, 4), colorBitDepth);
        }
        else
        {
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(0, 4), width);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(4, 4), height);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(8, 4), aspectNum);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(12, 4), aspectDen);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(16, 4), rotation);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(20, 4), componentBitDepth);
            BinaryPrimitives.WriteUInt32BigEndian(s.Slice(24, 4), colorBitDepth);
        }
        return buf;
    }

    /// <summary>Build a u32 capture-time payload (tag 0x180E).</summary>
    internal static byte[] CaptureTimePayload(uint epochSeconds, bool le)
    {
        var buf = new byte[4];
        if (le) BinaryPrimitives.WriteUInt32LittleEndian(buf, epochSeconds);
        else BinaryPrimitives.WriteUInt32BigEndian(buf, epochSeconds);
        return buf;
    }
}
