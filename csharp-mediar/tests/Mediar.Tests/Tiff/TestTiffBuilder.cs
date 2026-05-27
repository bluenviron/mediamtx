using System.Buffers.Binary;

namespace Mediar.Tests.Tiff;

/// <summary>
/// Test-only helper that produces minimal-but-valid little-endian TIFF 6.0
/// byte streams for the Mediar TIFF reader test suite. Supports both
/// strip-based and tile-based layouts and arbitrary compression codes
/// (the byte stream is opaque — the caller is responsible for producing
/// the strip/tile payload bytes that match the declared compression).
/// </summary>
internal static class TestTiffBuilder
{
    internal sealed class TiffSpec
    {
        public required int Width { get; init; }
        public required int Height { get; init; }
        public required int BitsPerSample { get; init; }
        public required int SamplesPerPixel { get; init; }
        public required int Compression { get; init; }
        public required int Photometric { get; init; }

        // Strip layout (mutually exclusive with tile fields)
        public int? RowsPerStrip { get; init; }
        public byte[][]? StripPayloads { get; init; }

        // Tile layout (mutually exclusive with strip fields)
        public int? TileWidth { get; init; }
        public int? TileHeight { get; init; }
        public byte[][]? TilePayloads { get; init; }
    }

    public static byte[] Build(TiffSpec spec) => Build([spec]);

    /// <summary>
    /// Build a multi-page TIFF from an ordered list of page specs.
    /// </summary>
    public static byte[] Build(IReadOnlyList<TiffSpec> specs)
    {
        ArgumentOutOfRangeException.ThrowIfZero(specs.Count);
        using var ms = new MemoryStream();
        var w = new BinaryWriter(ms);

        // --- TIFF header ---
        w.Write((byte)'I'); w.Write((byte)'I');
        w.Write((ushort)42);
        long ifdOffsetSlot = ms.Position;
        w.Write((uint)0);  // patched once we know IFD0's position

        // For each page: write payload + offset/count arrays, then defer IFD layout.
        var perPagePayloadOffsets = new uint[specs.Count][];
        var perPagePayloadByteCounts = new uint[specs.Count][];
        var perPageOffsetsArrayPos = new uint[specs.Count];
        var perPageCountsArrayPos = new uint[specs.Count];
        for (int p = 0; p < specs.Count; p++)
        {
            var spec = specs[p];
            bool tiled = spec.TilePayloads is not null;
            byte[][] payloads = tiled ? spec.TilePayloads! : spec.StripPayloads!;

            var offs = new uint[payloads.Length];
            var counts = new uint[payloads.Length];
            for (int i = 0; i < payloads.Length; i++)
            {
                offs[i] = (uint)ms.Position;
                counts[i] = (uint)payloads[i].Length;
                w.Write(payloads[i]);
            }
            perPagePayloadOffsets[p] = offs;
            perPagePayloadByteCounts[p] = counts;

            if ((ms.Position & 1) == 1) w.Write((byte)0);

            if (payloads.Length > 1)
            {
                perPageOffsetsArrayPos[p] = (uint)ms.Position;
                foreach (uint o in offs) w.Write(o);
                perPageCountsArrayPos[p] = (uint)ms.Position;
                foreach (uint c in counts) w.Write(c);
            }
        }

        // Now write IFDs back-to-back, patching the previous page's next-IFD slot.
        long prevNextSlot = ifdOffsetSlot;
        for (int p = 0; p < specs.Count; p++)
        {
            var spec = specs[p];
            bool tiled = spec.TilePayloads is not null;
            byte[][] payloads = tiled ? spec.TilePayloads! : spec.StripPayloads!;
            var offs = perPagePayloadOffsets[p];
            var counts = perPagePayloadByteCounts[p];

            var entries = new List<(ushort Tag, ushort Type, uint Count, uint ValueOrOffset)>
            {
                (0x0100, 4, 1, (uint)spec.Width),
                (0x0101, 4, 1, (uint)spec.Height),
                (0x0102, 3, 1, PackShort((ushort)spec.BitsPerSample)),
                (0x0103, 3, 1, PackShort((ushort)spec.Compression)),
                (0x0106, 3, 1, PackShort((ushort)spec.Photometric)),
                (0x0115, 3, 1, PackShort((ushort)spec.SamplesPerPixel)),
            };

            if (tiled)
            {
                entries.Add((0x0142, 4, 1, (uint)spec.TileWidth!.Value));
                entries.Add((0x0143, 4, 1, (uint)spec.TileHeight!.Value));
                entries.Add((0x0144, 4, (uint)payloads.Length,
                    payloads.Length == 1 ? offs[0] : perPageOffsetsArrayPos[p]));
                entries.Add((0x0145, 4, (uint)payloads.Length,
                    payloads.Length == 1 ? counts[0] : perPageCountsArrayPos[p]));
            }
            else
            {
                entries.Add((0x0116, 4, 1, (uint)(spec.RowsPerStrip ?? spec.Height)));
                entries.Add((0x0111, 4, (uint)payloads.Length,
                    payloads.Length == 1 ? offs[0] : perPageOffsetsArrayPos[p]));
                entries.Add((0x0117, 4, (uint)payloads.Length,
                    payloads.Length == 1 ? counts[0] : perPageCountsArrayPos[p]));
            }

            entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

            uint ifdPos = (uint)ms.Position;
            // Patch the previous IFD's next-pointer (or the header slot for page 0).
            long savePos = ms.Position;
            ms.Position = prevNextSlot;
            w.Write(ifdPos);
            ms.Position = savePos;

            w.Write((ushort)entries.Count);
            foreach (var e in entries)
            {
                w.Write(e.Tag);
                w.Write(e.Type);
                w.Write(e.Count);
                w.Write(e.ValueOrOffset);
            }
            prevNextSlot = ms.Position;
            w.Write((uint)0);
        }

        return ms.ToArray();
    }

    /// <summary>Packs a single ushort into the little-endian 4-byte value-or-offset slot.</summary>
    private static uint PackShort(ushort v)
    {
        Span<byte> tmp = stackalloc byte[4];
        BinaryPrimitives.WriteUInt16LittleEndian(tmp, v);
        return BinaryPrimitives.ReadUInt32LittleEndian(tmp);
    }
}
