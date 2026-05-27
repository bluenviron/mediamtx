using System.Buffers.Binary;

namespace Mediar.Tests.Dng;

/// <summary>
/// Test-only DNG (TIFF-based) byte-stream synthesiser. Emits a
/// minimal little-endian TIFF 6.0 container that includes a DNGVersion
/// tag (0xC612) and optional SubIFDs (tag 0x014A), so the
/// <see cref="Mediar.Imaging.Dng.DngReader"/> can be exercised against
/// realistic-looking files without shipping real RAW captures.
/// </summary>
internal static class TestDngBuilder
{
    internal sealed class IfdSpec
    {
        public required int Width { get; init; }
        public required int Height { get; init; }
        public required int BitsPerSample { get; init; }
        public required int SamplesPerPixel { get; init; }
        public required int Compression { get; init; }
        public required int Photometric { get; init; }
        public required int NewSubFileType { get; init; }
        public required byte[] StripPayload { get; init; }

        // DNG-only tags. Only IFD0 typically carries these.
        public byte[]? DngVersion { get; init; }
        public byte[]? DngBackwardVersion { get; init; }
        public string? UniqueCameraModel { get; init; }
        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
        public byte[]? CfaPattern { get; init; }
        public uint[]? BlackLevel { get; init; }
        public uint[]? WhiteLevel { get; init; }

        public IReadOnlyList<IfdSpec>? SubIfds { get; init; }
    }

    /// <summary>
    /// Build a DNG file whose IFD0 chain consists of <paramref name="root"/>.
    /// SubIFDs inside <paramref name="root"/> are emitted at the same time
    /// and pointed at via tag 0x014A.
    /// </summary>
    public static byte[] Build(IfdSpec root)
    {
        ArgumentNullException.ThrowIfNull(root);
        using var ms = new MemoryStream();
        var w = new BinaryWriter(ms);

        // Header: II 42 ifd0
        w.Write((byte)'I'); w.Write((byte)'I');
        w.Write((ushort)42);
        long headerIfdOffsetSlot = ms.Position;
        w.Write((uint)0);

        // Plan: walk every IFD (root + SubIFDs recursively), emit all
        // payloads + out-of-line value blocks first, then place the IFDs
        // back-to-back, patching SubIFD pointer arrays after each IFD is
        // written.
        var flat = new List<(IfdSpec Spec, int ParentIndex)>();
        FlattenIfds(root, parentIndex: -1, flat);

        // Per-IFD state.
        var stripOffset = new uint[flat.Count];
        var stripByteCount = new uint[flat.Count];
        var ifdPosition = new uint[flat.Count];
        // pointers from parent IFD to its sub-IFD slot (per child IFD): slot in source bytes.
        var subIfdPointerPatchSites = new List<(int ParentIndex, long PatchSiteStart, int NumChildren)>();

        // Step 1 — emit all strip payloads first (so we know strip offsets when we write the IFDs).
        for (int i = 0; i < flat.Count; i++)
        {
            AlignToEven(ms, w);
            stripOffset[i] = (uint)ms.Position;
            stripByteCount[i] = (uint)flat[i].Spec.StripPayload.Length;
            w.Write(flat[i].Spec.StripPayload);
        }

        // Step 2 — emit IFDs. For each IFD, also stash long out-of-line
        // values (ASCII strings > 4 chars, BYTE arrays > 4 elems, etc.)
        // at the tail and patch the entry's value/offset slot.
        long prevNextIfdSlot = headerIfdOffsetSlot;

        // We need to remember sub-IFD pointer arrays: each parent IFD that
        // has children needs a tag 0x014A whose value/offset slot points at
        // a uint32[N] array. We can't know children's IFD positions until
        // they're written, so we record the patch sites and resolve at the
        // end.
        var subIfdEntryValueSlot = new long[flat.Count];  // slot inside the parent IFD entry record where it stores either the inline single child or the offset to the array
        var subIfdArrayOffsets = new long[flat.Count];    // offset of the out-of-line uint32[] when N > 1
        var subIfdCounts = new int[flat.Count];

        for (int i = 0; i < flat.Count; i++)
        {
            AlignToEven(ms, w);
            var spec = flat[i].Spec;

            // First emit out-of-line values we'll need, capturing their offsets.
            uint dngVersionValue = 0;
            uint dngBackwardVersionValue = 0;
            uint uniqueCameraModelOffset = 0;
            int uniqueCameraModelLen = 0;
            uint makeOffset = 0; int makeLen = 0;
            uint modelOffset = 0; int modelLen = 0;
            uint softwareOffset = 0; int softwareLen = 0;
            uint cfaPatternOffset = 0; int cfaPatternLen = 0;
            uint blackLevelOffset = 0; int blackLevelLen = 0;
            uint whiteLevelOffset = 0; int whiteLevelLen = 0;
            uint bpsArrayOffset = 0;

            // DNGVersion (BYTE[4]) fits inline.
            if (spec.DngVersion is { Length: > 0 })
            {
                dngVersionValue = PackInlineBytes(spec.DngVersion);
            }
            if (spec.DngBackwardVersion is { Length: > 0 })
            {
                dngBackwardVersionValue = PackInlineBytes(spec.DngBackwardVersion);
            }
            if (!string.IsNullOrEmpty(spec.UniqueCameraModel))
            {
                (uniqueCameraModelOffset, uniqueCameraModelLen) = EmitAscii(ms, w, spec.UniqueCameraModel);
            }
            if (!string.IsNullOrEmpty(spec.Make))
                (makeOffset, makeLen) = EmitAscii(ms, w, spec.Make);
            if (!string.IsNullOrEmpty(spec.Model))
                (modelOffset, modelLen) = EmitAscii(ms, w, spec.Model);
            if (!string.IsNullOrEmpty(spec.Software))
                (softwareOffset, softwareLen) = EmitAscii(ms, w, spec.Software);
            if (spec.CfaPattern is { Length: > 0 } cfa)
            {
                if (cfa.Length > 4)
                {
                    AlignToEven(ms, w);
                    cfaPatternOffset = (uint)ms.Position;
                    cfaPatternLen = cfa.Length;
                    w.Write(cfa);
                }
                else
                {
                    cfaPatternOffset = PackInlineBytes(cfa);
                    cfaPatternLen = cfa.Length;
                }
            }
            if (spec.BlackLevel is { Length: > 0 } bl)
            {
                AlignToEven(ms, w);
                blackLevelOffset = (uint)ms.Position;
                blackLevelLen = bl.Length;
                foreach (uint v in bl) w.Write(v);
            }
            if (spec.WhiteLevel is { Length: > 0 } wl)
            {
                AlignToEven(ms, w);
                whiteLevelOffset = (uint)ms.Position;
                whiteLevelLen = wl.Length;
                foreach (uint v in wl) w.Write(v);
            }

            // BitsPerSample (SHORT[SamplesPerPixel]). Fits inline when <=2 samples.
            ushort[] bps = new ushort[spec.SamplesPerPixel];
            for (int s = 0; s < spec.SamplesPerPixel; s++) bps[s] = (ushort)spec.BitsPerSample;
            uint bpsValueOrOffset;
            if (spec.SamplesPerPixel <= 2)
            {
                bpsValueOrOffset = PackInlineShorts(bps, spec.SamplesPerPixel);
            }
            else
            {
                AlignToEven(ms, w);
                bpsArrayOffset = (uint)ms.Position;
                foreach (ushort v in bps) w.Write(v);
                bpsValueOrOffset = bpsArrayOffset;
            }

            // Sub-IFD pointer array — leave placeholder, patch at end.
            int numChildren = spec.SubIfds?.Count ?? 0;
            if (numChildren > 1)
            {
                AlignToEven(ms, w);
                subIfdArrayOffsets[i] = ms.Position;
                subIfdCounts[i] = numChildren;
                for (int k = 0; k < numChildren; k++) w.Write((uint)0);  // patched later
            }

            // Now write the IFD itself.
            AlignToEven(ms, w);
            ifdPosition[i] = (uint)ms.Position;

            var entries = new List<(ushort Tag, ushort Type, uint Count, uint ValueOrOffset, long PatchOffset)>();
            void Add(ushort tag, ushort type, uint count, uint value)
                => entries.Add((tag, type, count, value, 0));

            Add(0x00FE, 4, 1, (uint)spec.NewSubFileType);
            Add(0x0100, 4, 1, (uint)spec.Width);
            Add(0x0101, 4, 1, (uint)spec.Height);
            Add(0x0102, 3, (uint)spec.SamplesPerPixel, bpsValueOrOffset);
            Add(0x0103, 3, 1, PackShort((ushort)spec.Compression));
            Add(0x0106, 3, 1, PackShort((ushort)spec.Photometric));
            Add(0x0111, 4, 1, stripOffset[i]);
            Add(0x0115, 3, 1, PackShort((ushort)spec.SamplesPerPixel));
            Add(0x0116, 4, 1, (uint)spec.Height);
            Add(0x0117, 4, 1, stripByteCount[i]);

            // SubIFDs (0x014A). For 1 child, the entry holds the child's IFD
            // offset inline; for >1 children, the entry holds an offset to
            // the placeholder uint32[] we already emitted.
            if (numChildren == 1)
            {
                // Will be patched after children are written: record patch via tag entry index.
                entries.Add((0x014A, 4, 1, /*placeholder*/ 0u, /*marker*/ -1L));
            }
            else if (numChildren > 1)
            {
                Add(0x014A, 4, (uint)numChildren, (uint)subIfdArrayOffsets[i]);
            }

            // DNG-specific.
            if (spec.DngVersion is { Length: > 0 } dv)
                Add(0xC612, 1, (uint)dv.Length, dngVersionValue);
            if (spec.DngBackwardVersion is { Length: > 0 } dbv)
                Add(0xC613, 1, (uint)dbv.Length, dngBackwardVersionValue);
            if (uniqueCameraModelLen > 0)
                Add(0xC614, 2, (uint)uniqueCameraModelLen, uniqueCameraModelOffset);
            if (makeLen > 0)
                Add(0x010F, 2, (uint)makeLen, makeOffset);
            if (modelLen > 0)
                Add(0x0110, 2, (uint)modelLen, modelOffset);
            if (softwareLen > 0)
                Add(0x0131, 2, (uint)softwareLen, softwareOffset);
            if (cfaPatternLen > 0)
                Add(0x828E, 1, (uint)cfaPatternLen, cfaPatternOffset);
            if (blackLevelLen > 0)
                Add(0xC61A, 4, (uint)blackLevelLen, blackLevelOffset);
            if (whiteLevelLen > 0)
                Add(0xC61D, 4, (uint)whiteLevelLen, whiteLevelOffset);

            entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

            // Patch the previous IFD's next-pointer to point at this IFD.
            long savePos = ms.Position;
            ms.Position = prevNextIfdSlot;
            w.Write(ifdPosition[i]);
            ms.Position = savePos;

            w.Write((ushort)entries.Count);
            for (int e = 0; e < entries.Count; e++)
            {
                var entry = entries[e];
                w.Write(entry.Tag);
                w.Write(entry.Type);
                w.Write(entry.Count);
                long valSlot = ms.Position;
                w.Write(entry.ValueOrOffset);
                if (entry.PatchOffset == -1L && entry.Tag == 0x014A)
                {
                    subIfdEntryValueSlot[i] = valSlot;
                    subIfdCounts[i] = 1;
                }
            }

            // Sub-IFDs are NOT chained from their parent's next-IFD slot.
            // Only the top-level IFD chain participates.
            if (flat[i].ParentIndex == -1)
            {
                prevNextIfdSlot = ms.Position;
                w.Write((uint)0);
            }
            else
            {
                // SubIFD doesn't expose a chain; emit a 0 next-IFD anyway for
                // compatibility with naive readers.
                long discard = ms.Position;
                w.Write((uint)0);
                _ = discard;
            }
        }

        // Step 3 — patch every parent IFD's sub-IFD pointers now that all
        // children have been positioned.
        for (int i = 0; i < flat.Count; i++)
        {
            int numChildren = flat[i].Spec.SubIfds?.Count ?? 0;
            if (numChildren == 0) continue;

            if (numChildren == 1)
            {
                int childIndex = FindFirstChildIndex(flat, i);
                long save = ms.Position;
                ms.Position = subIfdEntryValueSlot[i];
                w.Write(ifdPosition[childIndex]);
                ms.Position = save;
            }
            else
            {
                long save = ms.Position;
                ms.Position = subIfdArrayOffsets[i];
                int childIdx = FindFirstChildIndex(flat, i);
                for (int c = 0; c < numChildren; c++)
                {
                    w.Write(ifdPosition[childIdx + c]);
                }
                ms.Position = save;
            }
        }

        return ms.ToArray();
    }

    private static int FindFirstChildIndex(List<(IfdSpec Spec, int ParentIndex)> flat, int parentIndex)
    {
        for (int i = parentIndex + 1; i < flat.Count; i++)
        {
            if (flat[i].ParentIndex == parentIndex) return i;
        }
        throw new InvalidOperationException("Parent has children but none found in flat list.");
    }

    private static void FlattenIfds(IfdSpec node, int parentIndex, List<(IfdSpec Spec, int ParentIndex)> sink)
    {
        sink.Add((node, parentIndex));
        int myIndex = sink.Count - 1;
        if (node.SubIfds is null) return;
        foreach (var c in node.SubIfds)
        {
            FlattenIfds(c, myIndex, sink);
        }
    }

    private static (uint Offset, int Length) EmitAscii(MemoryStream ms, BinaryWriter w, string s)
    {
        AlignToEven(ms, w);
        var off = (uint)ms.Position;
        var bytes = System.Text.Encoding.ASCII.GetBytes(s);
        w.Write(bytes);
        w.Write((byte)0);  // NUL terminator per TIFF spec
        return (off, bytes.Length + 1);
    }

    private static uint PackInlineBytes(byte[] b)
    {
        Span<byte> tmp = stackalloc byte[4];
        b.AsSpan(0, Math.Min(4, b.Length)).CopyTo(tmp);
        return BinaryPrimitives.ReadUInt32LittleEndian(tmp);
    }

    private static uint PackShort(ushort v)
    {
        Span<byte> tmp = stackalloc byte[4];
        BinaryPrimitives.WriteUInt16LittleEndian(tmp, v);
        return BinaryPrimitives.ReadUInt32LittleEndian(tmp);
    }

    private static uint PackInlineShorts(ushort[] values, int count)
    {
        Span<byte> tmp = stackalloc byte[4];
        for (int s = 0; s < count; s++)
            BinaryPrimitives.WriteUInt16LittleEndian(tmp[(s * 2)..], values[s]);
        return BinaryPrimitives.ReadUInt32LittleEndian(tmp);
    }

    private static void AlignToEven(MemoryStream ms, BinaryWriter w)
    {
        if ((ms.Position & 1) == 1) w.Write((byte)0);
    }
}
