using System.Buffers.Binary;

namespace Mediar.Tests.Cr2;

/// <summary>
/// Test-only Canon Raw v2 (CR2) byte-stream synthesiser. Produces a
/// minimal-but-valid little-endian TIFF 6.0 container with the CR2
/// sentinel + raw-IFD pointer at bytes 8-15.
/// </summary>
internal static class TestCr2Builder
{
    internal sealed class IfdSpec
    {
        public required int Width { get; init; }
        public required int Height { get; init; }
        public required int BitsPerSample { get; init; }
        public required int SamplesPerPixel { get; init; }
        public required int Compression { get; init; }
        public required int Photometric { get; init; }
        public required byte[] StripPayload { get; init; }

        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
    }

    /// <summary>
    /// Build a CR2 with the given chained IFDs (typically 3: thumbnail,
    /// alternate, preview) plus a separate <paramref name="raw"/> IFD whose
    /// offset is recorded in the CR2 header.
    /// </summary>
    public static byte[] Build(IReadOnlyList<IfdSpec> chain, IfdSpec? raw,
                               int majorVersion = 2, int minorVersion = 0)
    {
        ArgumentNullException.ThrowIfNull(chain);
        ArgumentOutOfRangeException.ThrowIfZero(chain.Count);

        using var ms = new MemoryStream();
        var w = new BinaryWriter(ms);

        // TIFF header + CR2 extension (16 bytes total).
        w.Write((byte)'I'); w.Write((byte)'I');
        w.Write((ushort)42);
        long ifd0OffsetSlot = ms.Position;
        w.Write((uint)0);             // patched below
        w.Write((byte)'C'); w.Write((byte)'R');
        w.Write((byte)majorVersion);
        w.Write((byte)minorVersion);
        long rawIfdOffsetSlot = ms.Position;
        w.Write((uint)0);             // patched below if raw is provided

        // Per-IFD: write strip payload + ascii out-of-line bytes first, then the IFD itself.
        var allIfds = new List<IfdSpec>(chain);
        if (raw is not null) allIfds.Add(raw);

        var stripOffsets = new uint[allIfds.Count];
        var stripCounts = new uint[allIfds.Count];
        var asciiOffsets = new (uint Make, int MakeLen, uint Model, int ModelLen, uint Software, int SoftwareLen)[allIfds.Count];

        // Pass 1: emit strip payloads + ASCII metadata strings.
        for (int i = 0; i < allIfds.Count; i++)
        {
            AlignEven(ms, w);
            stripOffsets[i] = (uint)ms.Position;
            stripCounts[i] = (uint)allIfds[i].StripPayload.Length;
            w.Write(allIfds[i].StripPayload);

            (uint, int) makeAt = (0, 0), modelAt = (0, 0), swAt = (0, 0);
            if (!string.IsNullOrEmpty(allIfds[i].Make)) makeAt = EmitAscii(ms, w, allIfds[i].Make!);
            if (!string.IsNullOrEmpty(allIfds[i].Model)) modelAt = EmitAscii(ms, w, allIfds[i].Model!);
            if (!string.IsNullOrEmpty(allIfds[i].Software)) swAt = EmitAscii(ms, w, allIfds[i].Software!);
            asciiOffsets[i] = (makeAt.Item1, makeAt.Item2, modelAt.Item1, modelAt.Item2, swAt.Item1, swAt.Item2);
        }

        // Pass 2: emit IFDs. Chain only the first `chain.Count` IFDs;
        // the raw IFD (last entry) is referenced separately via the CR2
        // header pointer at bytes 12-15.
        var ifdPositions = new uint[allIfds.Count];
        long prevNextSlot = ifd0OffsetSlot;
        for (int i = 0; i < allIfds.Count; i++)
        {
            AlignEven(ms, w);
            ifdPositions[i] = (uint)ms.Position;

            // Patch the chain's prev-next slot (skip for the raw IFD; the
            // raw IFD's offset is patched into the CR2 header below).
            bool isChained = i < chain.Count;
            if (isChained)
            {
                long save = ms.Position;
                ms.Position = prevNextSlot;
                w.Write(ifdPositions[i]);
                ms.Position = save;
            }

            var spec = allIfds[i];
            var entries = new List<(ushort Tag, ushort Type, uint Count, uint Value)>
            {
                (0x0100, 4, 1, (uint)spec.Width),
                (0x0101, 4, 1, (uint)spec.Height),
                (0x0102, 3, 1, PackShort((ushort)spec.BitsPerSample)),
                (0x0103, 3, 1, PackShort((ushort)spec.Compression)),
                (0x0106, 3, 1, PackShort((ushort)spec.Photometric)),
                (0x0111, 4, 1, stripOffsets[i]),
                (0x0115, 3, 1, PackShort((ushort)spec.SamplesPerPixel)),
                (0x0116, 4, 1, (uint)spec.Height),
                (0x0117, 4, 1, stripCounts[i]),
            };
            var asc = asciiOffsets[i];
            if (asc.MakeLen > 0) entries.Add((0x010F, 2, (uint)asc.MakeLen, asc.Make));
            if (asc.ModelLen > 0) entries.Add((0x0110, 2, (uint)asc.ModelLen, asc.Model));
            if (asc.SoftwareLen > 0) entries.Add((0x0131, 2, (uint)asc.SoftwareLen, asc.Software));
            entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

            w.Write((ushort)entries.Count);
            foreach (var e in entries)
            {
                w.Write(e.Tag);
                w.Write(e.Type);
                w.Write(e.Count);
                w.Write(e.Value);
            }
            if (isChained)
            {
                prevNextSlot = ms.Position;
                w.Write((uint)0);
            }
            else
            {
                // RAW IFD: still write a next-IFD slot for sane parsers,
                // but it is not chained-from-anywhere, and we patch the
                // CR2 header's raw-IFD-pointer slot instead.
                w.Write((uint)0);
                long save = ms.Position;
                ms.Position = rawIfdOffsetSlot;
                w.Write(ifdPositions[i]);
                ms.Position = save;
            }
        }

        return ms.ToArray();
    }

    private static (uint Offset, int Length) EmitAscii(MemoryStream ms, BinaryWriter w, string s)
    {
        AlignEven(ms, w);
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

    private static void AlignEven(MemoryStream ms, BinaryWriter w)
    {
        if ((ms.Position & 1) == 1) w.Write((byte)0);
    }
}
