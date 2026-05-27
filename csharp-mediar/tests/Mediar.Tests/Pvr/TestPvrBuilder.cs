using System.Buffers.Binary;

namespace Mediar.Tests.Pvr;

/// <summary>
/// Test-only synthesiser for valid PVR v3 byte streams. Writes the
/// 52-byte fixed header followed by an optional metadata block (FourCC +
/// Key + Data triples) and then the mip / surface / face payloads laid
/// out back-to-back, mip 0 first.
/// </summary>
internal sealed class TestPvrBuilder
{
    public bool LittleEndian { get; set; } = true;
    public uint Flags { get; set; }
    public ulong PixelFormatWord { get; set; }
    public uint ColourSpace { get; set; }
    public uint ChannelType { get; set; }
    public uint Height { get; set; } = 4;
    public uint Width { get; set; } = 4;
    public uint Depth { get; set; } = 1;
    public uint NumSurfaces { get; set; } = 1;
    public uint NumFaces { get; set; } = 1;
    public uint NumMipMaps { get; set; } = 1;
    public List<(uint FourCc, uint Key, byte[] Data)> MetaEntries { get; } = new();

    /// <summary>
    /// Per-level / surface / face payloads, in iteration order:
    /// for each mip level, for each surface, for each face.
    /// </summary>
    public List<byte[]> Payloads { get; } = new();

    public byte[] Build()
    {
        using var ms = new MemoryStream();
        WriteU32(ms, 0x03525650u);
        WriteU32(ms, Flags);
        WriteU64(ms, PixelFormatWord);
        WriteU32(ms, ColourSpace);
        WriteU32(ms, ChannelType);
        WriteU32(ms, Height);
        WriteU32(ms, Width);
        WriteU32(ms, Depth);
        WriteU32(ms, NumSurfaces);
        WriteU32(ms, NumFaces);
        WriteU32(ms, NumMipMaps);

        byte[] meta = BuildMetaBlock();
        WriteU32(ms, (uint)meta.Length);
        ms.Write(meta, 0, meta.Length);

        foreach (var p in Payloads) ms.Write(p, 0, p.Length);
        return ms.ToArray();
    }

    private byte[] BuildMetaBlock()
    {
        using var ms = new MemoryStream();
        foreach (var (cc, key, data) in MetaEntries)
        {
            WriteU32(ms, cc);
            WriteU32(ms, key);
            WriteU32(ms, (uint)data.Length);
            ms.Write(data, 0, data.Length);
        }
        return ms.ToArray();
    }

    private void WriteU32(Stream s, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        if (LittleEndian) BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        else BinaryPrimitives.WriteUInt32BigEndian(b, v);
        s.Write(b);
    }

    private void WriteU64(Stream s, ulong v)
    {
        Span<byte> b = stackalloc byte[8];
        if (LittleEndian) BinaryPrimitives.WriteUInt64LittleEndian(b, v);
        else BinaryPrimitives.WriteUInt64BigEndian(b, v);
        s.Write(b);
    }

    /// <summary>
    /// Pack a (channel-name, bit-width) descriptor for the upper 32 bits +
    /// lower 32 bits of the pixel format word. Channel names are ASCII
    /// bytes (lowercase) and bit widths are unsigned 8-bit values.
    /// </summary>
    public static ulong PackChannelDescriptor(string channelNames, byte[] bitWidths)
    {
        if (channelNames.Length != 4) throw new ArgumentException("4 chars", nameof(channelNames));
        if (bitWidths.Length != 4) throw new ArgumentException("4 widths", nameof(bitWidths));
        ulong v = 0;
        for (int i = 0; i < 4; i++) v |= ((ulong)(byte)channelNames[i]) << (i * 8);
        for (int i = 0; i < 4; i++) v |= ((ulong)bitWidths[i]) << ((i + 4) * 8);
        return v;
    }
}
