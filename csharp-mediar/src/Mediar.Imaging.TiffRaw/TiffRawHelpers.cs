using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.TiffRaw;

/// <summary>
/// Static byte-level helpers shared by every TIFF-RAW reader. These were
/// previously duplicated verbatim across 15 reader projects (~450 LOC
/// each); centralising them removes ~4 kLOC of duplicate code and makes
/// the IFD walk a single point of audit for byte-bounds checking.
/// </summary>
public static class TiffRawHelpers
{
    /// <summary>
    /// Parse the TIFF IFD at <paramref name="offset"/> into an array of
    /// <see cref="IfdEntry"/>. Throws <see cref="ImageFormatException"/>
    /// on out-of-bounds offsets or truncated IFDs.
    /// </summary>
    public static IfdEntry[] ParseIfd(byte[] b, bool le, int offset)
    {
        if (offset < 0 || offset + 2 > b.Length) throw new ImageFormatException("Bad IFD offset.");
        int n = ReadU16(b, offset, le);
        if (offset + 2 + n * 12 > b.Length) throw new ImageFormatException("IFD truncated.");
        var arr = new IfdEntry[n];
        for (int i = 0; i < n; i++)
        {
            int o = offset + 2 + i * 12;
            arr[i] = new IfdEntry(
                ReadU16(b, o, le),
                ReadU16(b, o + 2, le),
                ReadU32(b, o + 4, le),
                ReadU32(b, o + 8, le));
        }
        return arr;
    }

    /// <summary>
    /// Look up the scalar value of the IFD entry with the given tag. For
    /// SHORT (type 3) entries the low 16 bits of the ValueOffset slot are
    /// returned; for LONG (type 4) the full 32-bit value. Returns
    /// <paramref name="def"/> if the tag is missing.
    /// </summary>
    public static uint GetScalar(IfdEntry[] ifd, int tag, uint def = 0)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            if (e.Type == 3) return e.ValueOffset & 0xFFFF;
            return e.ValueOffset;
        }
        return def;
    }

    /// <summary>
    /// Total byte length of the value attached to <paramref name="tag"/>
    /// (i.e. the Count field of the IFD entry), or 0 if the tag is absent.
    /// </summary>
    public static int GetTagByteLength(IfdEntry[] ifd, int tag)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            return (int)e.Count;
        }
        return 0;
    }

    /// <summary>
    /// Read a LONG (type 4) array. If Count == 1 the value is the
    /// ValueOffset slot itself; otherwise it points to <c>n * 4</c> bytes
    /// at that offset.
    /// </summary>
    public static uint[] ReadLongArray(IfdEntry e, byte[] b, bool le)
    {
        int n = (int)e.Count;
        if (n == 0) return [];
        if (n == 1) return [e.ValueOffset];
        var arr = new uint[n];
        for (int k = 0; k < n; k++)
        {
            arr[k] = ReadU32(b, (int)e.ValueOffset + k * 4, le);
        }
        return arr;
    }

    /// <summary>
    /// Read a SHORT (type 3) array attached to <paramref name="tag"/>.
    /// If the total byte length is &lt;= 4 the values are packed inline
    /// in the ValueOffset slot (TIFF 6.0 spec); otherwise the slot is an
    /// absolute file offset pointing at <c>n * 2</c> bytes.
    /// </summary>
    public static ushort[] GetShortArray(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            var arr = new ushort[n];
            if (n == 0) return arr;
            if (n * 2 <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                if (le) BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                else BinaryPrimitives.WriteUInt32BigEndian(tmp, e.ValueOffset);
                for (int k = 0; k < n; k++)
                {
                    arr[k] = le
                        ? BinaryPrimitives.ReadUInt16LittleEndian(tmp[(k * 2)..])
                        : BinaryPrimitives.ReadUInt16BigEndian(tmp[(k * 2)..]);
                }
            }
            else
            {
                for (int k = 0; k < n; k++)
                {
                    arr[k] = ReadU16(b, (int)e.ValueOffset + k * 2, le);
                }
            }
            return arr;
        }
        return [];
    }

    /// <summary>
    /// Read an ASCII (type 2) tag. The TIFF 6.0 spec packs strings whose
    /// total byte length is &lt;= 4 inline in the ValueOffset slot;
    /// longer strings live at the absolute offset. The trailing NUL
    /// terminator is stripped before the string is decoded. Returns
    /// <c>null</c> if the tag is absent.
    /// </summary>
    public static string? ReadAsciiTag(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return string.Empty;
            string raw;
            if (n <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                if (le) BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                else BinaryPrimitives.WriteUInt32BigEndian(tmp, e.ValueOffset);
                while (n > 0 && tmp[n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(tmp[..n]);
            }
            else
            {
                if (e.ValueOffset + n > b.Length) return null;
                while (n > 0 && b[e.ValueOffset + n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(b, (int)e.ValueOffset, n);
            }
            return raw;
        }
        return null;
    }

    /// <summary>Read a little-endian or big-endian unsigned 16-bit value from <paramref name="b"/> at <paramref name="o"/>.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static ushort ReadU16(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    /// <summary>Read a little-endian or big-endian unsigned 32-bit value from <paramref name="b"/> at <paramref name="o"/>.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static uint ReadU32(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));

    /// <summary>
    /// Walks a TIFF IFD chain starting at <paramref name="ifdOffset"/> and
    /// recursively descends into any SubIFD pointers (tag <c>0x014A</c>),
    /// invoking <paramref name="build"/> for each discovered IFD to materialise
    /// a per-format sub-image descriptor into <paramref name="sink"/>.
    /// </summary>
    /// <typeparam name="T">Per-format sub-image descriptor type.</typeparam>
    /// <param name="bytes">The whole file buffer.</param>
    /// <param name="le">True for little-endian byte order.</param>
    /// <param name="ifdOffset">Starting IFD offset (typically the value read
    /// from the TIFF header next-IFD slot at offset 4).</param>
    /// <param name="parentSubIfdLevel">Recursion depth (0 at the top-level
    /// chain; incremented for each SubIFD descent).</param>
    /// <param name="sink">Destination list for the built sub-image
    /// descriptors. One entry is appended per discovered IFD.</param>
    /// <param name="visited">Cycle-guard set of already-walked offsets;
    /// pass an empty <see cref="HashSet{T}"/> at the top-level call.</param>
    /// <param name="build">Per-format builder delegate. Receives the parsed
    /// IFD entry table, the file buffer, the byte-order flag, and the
    /// current SubIFD recursion depth.</param>
    /// <remarks>
    /// <para>
    /// This is the canonical IFD-walk for TIFF-based RAW readers (DNG / ORF /
    /// RW2 etc). It walks the next-IFD chain only at the top level
    /// (<paramref name="parentSubIfdLevel"/> == 0), matching the TIFF 6.0
    /// convention that SubIFDs themselves do not carry meaningful next-IFD
    /// pointers.
    /// </para>
    /// <para>
    /// Out-of-bounds offsets are silently treated as end-of-chain rather
    /// than thrown - this matches the behaviour of every consumer that
    /// previously inlined this loop.
    /// </para>
    /// </remarks>
    public static void WalkIfdsRecursive<T>(
        byte[] bytes,
        bool le,
        uint ifdOffset,
        int parentSubIfdLevel,
        List<T> sink,
        HashSet<uint> visited,
        Func<IfdEntry[], byte[], bool, int, T> build)
    {
        ArgumentNullException.ThrowIfNull(sink);
        ArgumentNullException.ThrowIfNull(visited);
        ArgumentNullException.ThrowIfNull(build);

        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset)) return;
            if (ifdOffset + 2 > bytes.Length) return;
            var entries = ParseIfd(bytes, le, (int)ifdOffset);
            sink.Add(build(entries, bytes, le, parentSubIfdLevel));

            foreach (var e in entries)
            {
                if (e.Tag != 0x014A) continue;
                var subOffsets = ReadLongArray(e, bytes, le);
                foreach (uint sub in subOffsets)
                {
                    WalkIfdsRecursive(bytes, le, sub, parentSubIfdLevel + 1, sink, visited, build);
                }
            }

            if (parentSubIfdLevel != 0) return;

            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) return;
            ifdOffset = ReadU32(bytes, nextSlot, le);
        }
    }
}
