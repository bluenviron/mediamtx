using System.Buffers.Binary;

namespace Mediar.Tests.Dcr;

/// <summary>
/// Test-only DCR (TIFF-based Kodak RAW) byte-stream synthesiser. Emits a
/// little-endian TIFF 6.0 container with an EXIF Make tag set to a Kodak
/// brand string (the marker by which
/// <see cref="Mediar.Imaging.Dcr.DcrReader"/> identifies a DCR file).
/// SubIFDs are emitted via tag 0x014A.
/// </summary>
internal static class TestDcrBuilder
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

        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
        public string? DateTime { get; init; }
        public string? Artist { get; init; }
        public string? Copyright { get; init; }
        public byte[]? MakerNote { get; init; }

        public IReadOnlyList<IfdSpec>? SubIfds { get; init; }
    }

    public static byte[] Build(IfdSpec root)
    {
        ArgumentNullException.ThrowIfNull(root);
        using var ms = new MemoryStream();
        var w = new BinaryWriter(ms);

        w.Write((byte)'I'); w.Write((byte)'I');
        w.Write((ushort)42);
        long headerIfdOffsetSlot = ms.Position;
        w.Write((uint)0);

        var flat = new List<(IfdSpec Spec, int ParentIndex)>();
        FlattenIfds(root, parentIndex: -1, flat);

        var stripOffset = new uint[flat.Count];
        var stripByteCount = new uint[flat.Count];
        var ifdPosition = new uint[flat.Count];

        for (int i = 0; i < flat.Count; i++)
        {
            AlignToEven(ms, w);
            stripOffset[i] = (uint)ms.Position;
            stripByteCount[i] = (uint)flat[i].Spec.StripPayload.Length;
            w.Write(flat[i].Spec.StripPayload);
        }

        long prevNextIfdSlot = headerIfdOffsetSlot;
        var subIfdEntryValueSlot = new long[flat.Count];
        var subIfdArrayOffsets = new long[flat.Count];

        for (int i = 0; i < flat.Count; i++)
        {
            AlignToEven(ms, w);
            var spec = flat[i].Spec;

            uint makeOffset = 0; int makeLen = 0;
            uint modelOffset = 0; int modelLen = 0;
            uint softwareOffset = 0; int softwareLen = 0;
            uint dateTimeOffset = 0; int dateTimeLen = 0;
            uint artistOffset = 0; int artistLen = 0;
            uint copyrightOffset = 0; int copyrightLen = 0;
            uint makerNoteOffset = 0; int makerNoteLen = 0;

            if (!string.IsNullOrEmpty(spec.Make))
                (makeOffset, makeLen) = EmitAscii(ms, w, spec.Make);
            if (!string.IsNullOrEmpty(spec.Model))
                (modelOffset, modelLen) = EmitAscii(ms, w, spec.Model);
            if (!string.IsNullOrEmpty(spec.Software))
                (softwareOffset, softwareLen) = EmitAscii(ms, w, spec.Software);
            if (!string.IsNullOrEmpty(spec.DateTime))
                (dateTimeOffset, dateTimeLen) = EmitAscii(ms, w, spec.DateTime);
            if (!string.IsNullOrEmpty(spec.Artist))
                (artistOffset, artistLen) = EmitAscii(ms, w, spec.Artist);
            if (!string.IsNullOrEmpty(spec.Copyright))
                (copyrightOffset, copyrightLen) = EmitAscii(ms, w, spec.Copyright);
            if (spec.MakerNote is { Length: > 0 } mn)
            {
                AlignToEven(ms, w);
                makerNoteOffset = (uint)ms.Position;
                makerNoteLen = mn.Length;
                w.Write(mn);
            }

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
                uint bpsArrayOffset = (uint)ms.Position;
                foreach (ushort v in bps) w.Write(v);
                bpsValueOrOffset = bpsArrayOffset;
            }

            int numChildren = spec.SubIfds?.Count ?? 0;
            if (numChildren > 1)
            {
                AlignToEven(ms, w);
                subIfdArrayOffsets[i] = ms.Position;
                for (int k = 0; k < numChildren; k++) w.Write((uint)0);
            }

            AlignToEven(ms, w);
            ifdPosition[i] = (uint)ms.Position;

            var entries = new List<(ushort Tag, ushort Type, uint Count, uint ValueOrOffset, long MarkerPatch)>();
            void Add(ushort tag, ushort type, uint count, uint value)
                => entries.Add((tag, type, count, value, 0));

            Add(0x00FE, 4, 1, (uint)spec.NewSubFileType);
            Add(0x0100, 4, 1, (uint)spec.Width);
            Add(0x0101, 4, 1, (uint)spec.Height);
            Add(0x0102, 3, (uint)spec.SamplesPerPixel, bpsValueOrOffset);
            Add(0x0103, 3, 1, PackShort((ushort)spec.Compression));
            Add(0x0106, 3, 1, PackShort((ushort)spec.Photometric));
            if (makeLen > 0)
                Add(0x010F, 2, (uint)makeLen, makeOffset);
            if (modelLen > 0)
                Add(0x0110, 2, (uint)modelLen, modelOffset);
            Add(0x0111, 4, 1, stripOffset[i]);
            Add(0x0115, 3, 1, PackShort((ushort)spec.SamplesPerPixel));
            Add(0x0116, 4, 1, (uint)spec.Height);
            Add(0x0117, 4, 1, stripByteCount[i]);
            if (softwareLen > 0)
                Add(0x0131, 2, (uint)softwareLen, softwareOffset);
            if (dateTimeLen > 0)
                Add(0x0132, 2, (uint)dateTimeLen, dateTimeOffset);
            if (artistLen > 0)
                Add(0x013B, 2, (uint)artistLen, artistOffset);

            if (numChildren == 1)
            {
                entries.Add((0x014A, 4, 1, 0u, -1L));
            }
            else if (numChildren > 1)
            {
                Add(0x014A, 4, (uint)numChildren, (uint)subIfdArrayOffsets[i]);
            }

            if (copyrightLen > 0)
                Add(0x8298, 2, (uint)copyrightLen, copyrightOffset);
            if (makerNoteLen > 0)
                Add(0x927C, 7, (uint)makerNoteLen, makerNoteOffset);

            entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

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
                if (entry.MarkerPatch == -1L && entry.Tag == 0x014A)
                {
                    subIfdEntryValueSlot[i] = valSlot;
                }
            }

            if (flat[i].ParentIndex == -1)
            {
                prevNextIfdSlot = ms.Position;
                w.Write((uint)0);
            }
            else
            {
                w.Write((uint)0);
            }
        }

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
        w.Write((byte)0);
        return (off, bytes.Length + 1);
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
