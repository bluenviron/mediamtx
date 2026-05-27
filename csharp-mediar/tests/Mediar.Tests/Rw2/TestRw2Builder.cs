using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Rw2;

/// <summary>
/// Test-only synthesiser for Panasonic RW2 byte streams. Lays out a
/// little-endian TIFF 6.0 container with RW2's signature magic 0x0055
/// in place of TIFF's 0x002A. Supports IFD 0 with arbitrary tag set
/// including Panasonic-specific tags 0x0001..0x0017 and the EXIF
/// strings, plus SubIFD chains through tag 0x014A.
/// </summary>
internal static class TestRw2Builder
{
    public sealed record Rw2Spec
    {
        /// <summary>The magic value to emit. Default 0x0055 (correct).</summary>
        public ushort Magic { get; init; } = 0x0055;

        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
        public string? DateTime { get; init; }
        public string? Artist { get; init; }
        public string? Copyright { get; init; }

        public string? PanasonicRawVersion { get; init; }
        public uint SensorWidth { get; init; }
        public uint SensorHeight { get; init; }
        public uint SensorTopBorder { get; init; }
        public uint SensorLeftBorder { get; init; }
        public uint SensorBottomBorder { get; init; }
        public uint SensorRightBorder { get; init; }
        public uint CfaPattern { get; init; }
        public uint Iso { get; init; }

        /// <summary>Set this for the primary image dimensions (TIFF tags 0x0100/0x0101).</summary>
        public uint TiffWidth { get; init; }

        /// <summary>Set this for the primary image dimensions (TIFF tags 0x0100/0x0101).</summary>
        public uint TiffHeight { get; init; }

        /// <summary>TIFF compression tag value (default 34316 = Panasonic proprietary).</summary>
        public uint Compression { get; init; } = 34316;

        public uint BitsPerSample { get; init; } = 12;
        public uint SamplesPerPixel { get; init; } = 1;
        public uint Photometric { get; init; } = 32803;

        public bool IncludeStripData { get; init; }
        public byte[]? StripBytes { get; init; }
    }

    public static byte[] Build(Rw2Spec spec)
    {
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
                {
                    packed |= (uint)bytes[i] << (i * 8);
                }
                entries.Add((tag, Type: 2, (uint)bytes.Length, null, packed));
            }
            else
            {
                entries.Add((tag, Type: 2, (uint)bytes.Length, bytes, 0));
            }
        }

        void AddLong(ushort tag, uint value)
        {
            entries.Add((tag, Type: 4, 1u, null, value));
        }

        void AddShort(ushort tag, uint value)
        {
            entries.Add((tag, Type: 3, 1u, null, value));
        }

        if (spec.PanasonicRawVersion is not null) AddAscii(0x0001, spec.PanasonicRawVersion);
        if (spec.SensorWidth > 0) AddLong(0x0002, spec.SensorWidth);
        if (spec.SensorHeight > 0) AddLong(0x0003, spec.SensorHeight);
        if (spec.SensorTopBorder > 0) AddLong(0x0004, spec.SensorTopBorder);
        if (spec.SensorLeftBorder > 0) AddLong(0x0005, spec.SensorLeftBorder);
        if (spec.SensorBottomBorder > 0) AddLong(0x0006, spec.SensorBottomBorder);
        if (spec.SensorRightBorder > 0) AddLong(0x0007, spec.SensorRightBorder);
        if (spec.CfaPattern > 0) AddShort(0x0009, spec.CfaPattern);
        if (spec.Iso > 0) AddShort(0x0017, spec.Iso);

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
        header[0] = 0x49; header[1] = 0x49;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(2, 2), spec.Magic);
        BinaryPrimitives.WriteUInt32LittleEndian(header.Slice(4, 4), (uint)headerSize);
        ms.Write(header);

        Span<byte> u16 = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(u16, (ushort)entries.Count);
        ms.Write(u16);

        Span<byte> entry = stackalloc byte[12];
        for (int i = 0; i < entries.Count; i++)
        {
            entry.Clear();
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(0, 2), entries[i].Tag);
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), entries[i].Type);
            BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), entries[i].Count);
            uint valueOrOffset = entries[i].InlineValue;
            if (offsets[i] >= 0) valueOrOffset = (uint)offsets[i];
            if (entries[i].Tag == 0x0111 && spec.IncludeStripData && spec.StripBytes is not null)
            {
                valueOrOffset = (uint)stripBytesOffset;
            }
            BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), valueOrOffset);
            ms.Write(entry);
        }

        Span<byte> next = stackalloc byte[4];
        ms.Write(next);

        ms.Write(pool.ToArray());

        return ms.ToArray();
    }
}
