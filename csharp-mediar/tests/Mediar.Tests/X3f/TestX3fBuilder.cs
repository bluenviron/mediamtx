using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.X3f;

/// <summary>
/// Test-only synthesiser for valid Sigma Foveon X3F byte streams. Lays out
/// a "FOVb" header, optional extended (v2.1+) header fields, configurable
/// number of sections (image, properties, camera metadata), and the
/// trailing "SECd" directory + u32 directory-offset pointer.
/// </summary>
internal sealed class TestX3fBuilder
{
    public ushort VersionMajor { get; set; } = 2;
    public ushort VersionMinor { get; set; }
    public byte[] FileId { get; set; } = new byte[16];
    public uint FileMark { get; set; }
    public uint? Rotation { get; set; }
    public string? WhiteBalanceLabel { get; set; }

    public List<Section> Sections { get; } = new();

    public uint? DirectoryOffsetOverride { get; set; }

    public TestX3fBuilder AddJpegPreview(byte[] jpegBytes, uint width, uint height)
    {
        Sections.Add(new Section
        {
            DirectoryId = "IMA2",
            SectionMagic = "SECi",
            ImageType = 1,
            DataFormat = 3,
            Width = width,
            Height = height,
            RowStride = (uint)jpegBytes.Length,
            Payload = jpegBytes,
        });
        return this;
    }

    public TestX3fBuilder AddRawMosaic(byte[] payload, uint width, uint height,
                                       uint imageType = 2, uint dataFormat = 11,
                                       uint rowStride = 0)
    {
        Sections.Add(new Section
        {
            DirectoryId = "IMA2",
            SectionMagic = "SECi",
            ImageType = imageType,
            DataFormat = dataFormat,
            Width = width,
            Height = height,
            RowStride = rowStride == 0 ? (uint)payload.Length / Math.Max(1u, height) : rowStride,
            Payload = payload,
        });
        return this;
    }

    public TestX3fBuilder AddProperties(IEnumerable<KeyValuePair<string, string>> properties)
    {
        var list = properties as IList<KeyValuePair<string, string>> ?? new List<KeyValuePair<string, string>>(properties);
        Sections.Add(new Section
        {
            DirectoryId = "PROP",
            SectionMagic = "SECp",
            Properties = list,
        });
        return this;
    }

    public TestX3fBuilder AddCameraMetadata(byte[] payload)
    {
        Sections.Add(new Section
        {
            DirectoryId = "CAMF",
            SectionMagic = "SECc",
            Payload = payload,
        });
        return this;
    }

    public TestX3fBuilder AddUnknown(string id, byte[] payload)
    {
        Sections.Add(new Section
        {
            DirectoryId = id,
            SectionMagic = "????",
            Payload = payload,
        });
        return this;
    }

    public byte[] Build()
    {
        using var ms = new MemoryStream();

        // Header: "FOVb"
        ms.Write("FOVb"u8);
        WriteU16(ms, VersionMinor);
        WriteU16(ms, VersionMajor);
        ms.Write(FileId, 0, 16);
        WriteU32(ms, FileMark);

        // Extended header (v >= 2.1).
        if (VersionMajor >= 2 && VersionMinor >= 1)
        {
            WriteU32(ms, Rotation ?? 0u);
            var wbBytes = new byte[32];
            if (WhiteBalanceLabel is not null)
            {
                var src = Encoding.ASCII.GetBytes(WhiteBalanceLabel);
                Array.Copy(src, wbBytes, Math.Min(src.Length, 31));
            }
            ms.Write(wbBytes, 0, 32);
        }

        // Section payloads.
        var sectionOffsets = new long[Sections.Count];
        var sectionLengths = new long[Sections.Count];
        for (int i = 0; i < Sections.Count; i++)
        {
            sectionOffsets[i] = ms.Position;
            WriteSection(ms, Sections[i]);
            sectionLengths[i] = ms.Position - sectionOffsets[i];
        }

        // Directory.
        long directoryStart = ms.Position;
        ms.Write("SECd"u8);
        WriteU16(ms, 0); // minor
        WriteU16(ms, 2); // major
        WriteU32(ms, (uint)Sections.Count);
        for (int i = 0; i < Sections.Count; i++)
        {
            WriteU32(ms, (uint)sectionOffsets[i]);
            WriteU32(ms, (uint)sectionLengths[i]);
            var idBytes = Encoding.ASCII.GetBytes(Sections[i].DirectoryId);
            if (idBytes.Length != 4) throw new InvalidOperationException("DirectoryId must be 4 chars");
            ms.Write(idBytes, 0, 4);
        }

        // Trailing directory-offset pointer (u32).
        uint dirOff = DirectoryOffsetOverride ?? (uint)directoryStart;
        WriteU32(ms, dirOff);

        return ms.ToArray();
    }

    private static void WriteSection(MemoryStream ms, Section s)
    {
        var magicBytes = Encoding.ASCII.GetBytes(s.SectionMagic);
        ms.Write(magicBytes, 0, 4);

        if (s.DirectoryId == "PROP" && s.Properties is not null)
        {
            WritePropertiesSection(ms, s.Properties);
            return;
        }
        if (s.DirectoryId is "IMA2" or "IMAG")
        {
            WriteU16(ms, 0); // minor
            WriteU16(ms, 2); // major
            WriteU32(ms, s.ImageType);
            WriteU32(ms, s.DataFormat);
            WriteU32(ms, s.Width);
            WriteU32(ms, s.Height);
            WriteU32(ms, s.RowStride);
            if (s.Payload is not null) ms.Write(s.Payload, 0, s.Payload.Length);
            return;
        }

        // CAMF or unknown: header (4 bytes) + version + payload.
        WriteU16(ms, 0);
        WriteU16(ms, 2);
        if (s.Payload is not null) ms.Write(s.Payload, 0, s.Payload.Length);
    }

    private static void WritePropertiesSection(MemoryStream ms, IList<KeyValuePair<string, string>> props)
    {
        // SECp body (24-byte header total including the SECp magic already written).
        WriteU16(ms, 0); // minor
        WriteU16(ms, 2); // major
        WriteU32(ms, (uint)props.Count);
        WriteU32(ms, 0); // char format: 0 = UTF-16
        WriteU32(ms, 0); // reserved

        // Build the UTF-16 pool: for each property, append name + nul + value + nul.
        var pool = new MemoryStream();
        var entryOffsets = new (uint nameCharOff, uint valueCharOff)[props.Count];
        for (int i = 0; i < props.Count; i++)
        {
            uint nameCharOff = (uint)(pool.Position / 2);
            var nameBytes = Encoding.Unicode.GetBytes(props[i].Key);
            pool.Write(nameBytes, 0, nameBytes.Length);
            pool.WriteByte(0); pool.WriteByte(0);

            uint valueCharOff = (uint)(pool.Position / 2);
            var valueBytes = Encoding.Unicode.GetBytes(props[i].Value);
            pool.Write(valueBytes, 0, valueBytes.Length);
            pool.WriteByte(0); pool.WriteByte(0);

            entryOffsets[i] = (nameCharOff, valueCharOff);
        }

        uint poolChars = (uint)(pool.Position / 2);
        WriteU32(ms, poolChars); // pool char count

        // Entry table (entry_count × 8 bytes).
        for (int i = 0; i < props.Count; i++)
        {
            WriteU32(ms, entryOffsets[i].nameCharOff);
            WriteU32(ms, entryOffsets[i].valueCharOff);
        }

        // UTF-16 pool.
        pool.WriteTo(ms);
    }

    private static void WriteU16(MemoryStream ms, ushort value)
    {
        Span<byte> buf = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(buf, value);
        ms.Write(buf);
    }

    private static void WriteU32(MemoryStream ms, uint value)
    {
        Span<byte> buf = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(buf, value);
        ms.Write(buf);
    }

    internal sealed class Section
    {
        public string DirectoryId { get; set; } = "????";
        public string SectionMagic { get; set; } = "????";
        public uint ImageType { get; set; }
        public uint DataFormat { get; set; }
        public uint Width { get; set; }
        public uint Height { get; set; }
        public uint RowStride { get; set; }
        public byte[]? Payload { get; set; }
        public IList<KeyValuePair<string, string>>? Properties { get; set; }
    }
}
