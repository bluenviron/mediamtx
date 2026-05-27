using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Orf;

/// <summary>
/// Test-only synthesiser for Olympus ORF byte streams. Lays out a
/// little-endian TIFF 6.0 container with a configurable magic word
/// at offset 2 (0x002A standard, 0x4F52 = Olympus 'RO', 0x5253 =
/// Olympus 'RS', 0x524F = big-endian 'OR' when MM is used). Supports
/// the EXIF strings + an optional MakerNote payload, with optional
/// strip data for a real round-trip.
/// </summary>
internal static class TestOrfBuilder
{
    public sealed record OrfSpec
    {
        public bool LittleEndian { get; init; } = true;
        public ushort Magic { get; init; } = 0x4F52;

        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
        public string? DateTime { get; init; }
        public string? Artist { get; init; }
        public string? Copyright { get; init; }
        public byte[]? MakerNote { get; init; }

        public uint TiffWidth { get; init; }
        public uint TiffHeight { get; init; }
        public uint Compression { get; init; } = 1;
        public uint BitsPerSample { get; init; } = 12;
        public uint SamplesPerPixel { get; init; } = 1;
        public uint Photometric { get; init; } = 32803;

        public bool IncludeStripData { get; init; }
        public byte[]? StripBytes { get; init; }
    }

    public static byte[] Build(OrfSpec spec)
    {
        bool le = spec.LittleEndian;
        var pool = new List<byte>();
        var entries = new List<(ushort Tag, ushort Type, uint Count, byte[]? OutOfLine, uint InlineValue)>();

        void AddAscii(ushort tag, string? value)
        {
            if (value is null) return;
            byte[] bytes = Encoding.ASCII.GetBytes(value + "\0");
            if (bytes.Length <= 4)
            {
                uint packed = 0;
                for (int i = 0; i < bytes.Length; i++)
                    packed |= (uint)bytes[i] << (i * 8);
                if (!le) packed = BinaryPrimitives.ReverseEndianness(packed);
                entries.Add((tag, 2, (uint)bytes.Length, null, packed));
            }
            else
            {
                entries.Add((tag, 2, (uint)bytes.Length, bytes, 0));
            }
        }

        void AddRaw(ushort tag, byte[]? value, ushort type)
        {
            if (value is null) return;
            if (value.Length <= 4)
            {
                uint packed = 0;
                for (int i = 0; i < value.Length; i++)
                    packed |= (uint)value[i] << (i * 8);
                if (!le) packed = BinaryPrimitives.ReverseEndianness(packed);
                entries.Add((tag, type, (uint)value.Length, null, packed));
            }
            else
            {
                entries.Add((tag, type, (uint)value.Length, value, 0));
            }
        }

        void AddLong(ushort tag, uint value) => entries.Add((tag, 4, 1u, null, value));
        void AddShort(ushort tag, uint value) => entries.Add((tag, 3, 1u, null, value));

        if (spec.TiffWidth > 0) AddLong(0x0100, spec.TiffWidth);
        if (spec.TiffHeight > 0) AddLong(0x0101, spec.TiffHeight);
        AddShort(0x0102, spec.BitsPerSample);
        AddShort(0x0103, spec.Compression);
        AddShort(0x0106, spec.Photometric);
        AddShort(0x0115, spec.SamplesPerPixel);

        if (spec.IncludeStripData && spec.StripBytes is not null)
        {
            entries.Add((Tag: 0x0111, Type: 4, Count: 1u, OutOfLine: null, InlineValue: 0));
            AddLong(0x0116, spec.TiffHeight);
            AddLong(0x0117, (uint)spec.StripBytes.Length);
        }

        AddAscii(0x010F, spec.Make);
        AddAscii(0x0110, spec.Model);
        AddAscii(0x0131, spec.Software);
        AddAscii(0x0132, spec.DateTime);
        AddAscii(0x013B, spec.Artist);
        AddAscii(0x8298, spec.Copyright);
        if (spec.MakerNote is not null) AddRaw(0x927C, spec.MakerNote, type: 7);

        entries.Sort((a, b) => a.Tag.CompareTo(b.Tag));

        int ifdSize = 2 + entries.Count * 12 + 4;
        int headerSize = 8;
        int poolStart = headerSize + ifdSize;

        var offsets = new int[entries.Count];
        for (int i = 0; i < entries.Count; i++)
        {
            if (entries[i].OutOfLine is not null)
            {
                offsets[i] = poolStart + pool.Count;
                pool.AddRange(entries[i].OutOfLine!);
            }
            else
            {
                offsets[i] = -1;
            }
        }

        int stripBytesOffset = -1;
        if (spec.IncludeStripData && spec.StripBytes is not null)
        {
            stripBytesOffset = poolStart + pool.Count;
            pool.AddRange(spec.StripBytes);
        }

        using var ms = new MemoryStream();
        Span<byte> header = stackalloc byte[8];
        if (le) { header[0] = 0x49; header[1] = 0x49; }
        else { header[0] = 0x4D; header[1] = 0x4D; }
        if (le) BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(2, 2), spec.Magic);
        else BinaryPrimitives.WriteUInt16BigEndian(header.Slice(2, 2), spec.Magic);
        if (le) BinaryPrimitives.WriteUInt32LittleEndian(header.Slice(4, 4), (uint)headerSize);
        else BinaryPrimitives.WriteUInt32BigEndian(header.Slice(4, 4), (uint)headerSize);
        ms.Write(header);

        Span<byte> u16 = stackalloc byte[2];
        if (le) BinaryPrimitives.WriteUInt16LittleEndian(u16, (ushort)entries.Count);
        else BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)entries.Count);
        ms.Write(u16);

        Span<byte> entry = stackalloc byte[12];
        for (int i = 0; i < entries.Count; i++)
        {
            entry.Clear();
            if (le)
            {
                BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(0, 2), entries[i].Tag);
                BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), entries[i].Type);
                BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), entries[i].Count);
            }
            else
            {
                BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), entries[i].Tag);
                BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(2, 2), entries[i].Type);
                BinaryPrimitives.WriteUInt32BigEndian(entry.Slice(4, 4), entries[i].Count);
            }
            uint valueOrOffset = entries[i].InlineValue;
            if (offsets[i] >= 0) valueOrOffset = (uint)offsets[i];
            if (entries[i].Tag == 0x0111 && spec.IncludeStripData && spec.StripBytes is not null)
            {
                valueOrOffset = (uint)stripBytesOffset;
            }
            if (le) BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), valueOrOffset);
            else BinaryPrimitives.WriteUInt32BigEndian(entry.Slice(8, 4), valueOrOffset);
            ms.Write(entry);
        }

        Span<byte> next = stackalloc byte[4];
        ms.Write(next);
        ms.Write(pool.ToArray());

        return ms.ToArray();
    }
}
