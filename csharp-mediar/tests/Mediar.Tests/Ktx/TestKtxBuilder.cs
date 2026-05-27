using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Test-only synthesiser for valid KTX 1.x byte streams. Writes the
/// 12-byte Khronos identifier, a 13-u32 little-endian header, an optional
/// key-value pool, and the mip pyramid with per-mip imageSize prefixes
/// and 4-byte padding between mips.
/// </summary>
internal sealed class TestKtxBuilder
{
    public uint GlType { get; set; }
    public uint GlTypeSize { get; set; } = 1;
    public uint GlFormat { get; set; }
    public uint GlInternalFormat { get; set; }
    public uint GlBaseInternalFormat { get; set; }
    public uint PixelWidth { get; set; } = 4;
    public uint PixelHeight { get; set; } = 4;
    public uint PixelDepth { get; set; }
    public uint ArrayElems { get; set; }
    public uint FaceCount { get; set; } = 1;
    public uint MipLevels { get; set; } = 1;
    public List<KeyValuePair<string, string>> KeyValues { get; } = new();
    public List<byte[]> MipPayloads { get; } = new();

    public byte[] Build()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0xAB, 0x4B, 0x54, 0x58, 0x20, 0x31, 0x31, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A });
        WriteU32(ms, 0x04030201u); // endianness
        WriteU32(ms, GlType);
        WriteU32(ms, GlTypeSize);
        WriteU32(ms, GlFormat);
        WriteU32(ms, GlInternalFormat);
        WriteU32(ms, GlBaseInternalFormat);
        WriteU32(ms, PixelWidth);
        WriteU32(ms, PixelHeight);
        WriteU32(ms, PixelDepth);
        WriteU32(ms, ArrayElems);
        WriteU32(ms, FaceCount);
        WriteU32(ms, MipLevels);

        var kvPool = BuildKeyValuePool(KeyValues);
        WriteU32(ms, (uint)kvPool.Length);
        ms.Write(kvPool, 0, kvPool.Length);

        for (int i = 0; i < MipPayloads.Count; i++)
        {
            WriteU32(ms, (uint)MipPayloads[i].Length);
            ms.Write(MipPayloads[i], 0, MipPayloads[i].Length);
            int pad = (-(int)ms.Position) & 3;
            for (int p = 0; p < pad; p++) ms.WriteByte(0);
        }
        return ms.ToArray();
    }

    private static byte[] BuildKeyValuePool(List<KeyValuePair<string, string>> kvs)
    {
        if (kvs.Count == 0) return Array.Empty<byte>();
        using var ms = new MemoryStream();
        foreach (var kv in kvs)
        {
            var key = Encoding.UTF8.GetBytes(kv.Key);
            var value = Encoding.UTF8.GetBytes(kv.Value);
            int totalLen = key.Length + 1 + value.Length + 1;
            WriteU32(ms, (uint)totalLen);
            ms.Write(key, 0, key.Length);
            ms.WriteByte(0);
            ms.Write(value, 0, value.Length);
            ms.WriteByte(0);
            int pad = (-(int)ms.Position) & 3;
            for (int p = 0; p < pad; p++) ms.WriteByte(0);
        }
        return ms.ToArray();
    }

    private static void WriteU32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        ms.Write(b);
    }
}

/// <summary>
/// Test-only synthesiser for valid KTX 2.x byte streams. Writes the 12-byte
/// Khronos identifier, the 68-byte fixed header (vkFormat + dims +
/// supercompression + index), the per-mip level index (3 × u64), the
/// kvd section, and the mip payload data.
/// </summary>
internal sealed class TestKtx2Builder
{
    public uint VkFormat { get; set; }
    public uint TypeSize { get; set; } = 1;
    public uint PixelWidth { get; set; } = 4;
    public uint PixelHeight { get; set; } = 4;
    public uint PixelDepth { get; set; }
    public uint LayerCount { get; set; }
    public uint FaceCount { get; set; } = 1;
    public uint SupercompressionScheme { get; set; }
    public List<KeyValuePair<string, string>> KeyValues { get; } = new();
    public List<byte[]> MipPayloads { get; } = new();
    public List<ulong>? UncompressedSizes { get; set; }
    public uint? LevelCountOverride { get; set; }

    public byte[] Build()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0xAB, 0x4B, 0x54, 0x58, 0x20, 0x32, 0x30, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A });
        WriteU32(ms, VkFormat);
        WriteU32(ms, TypeSize);
        WriteU32(ms, PixelWidth);
        WriteU32(ms, PixelHeight);
        WriteU32(ms, PixelDepth);
        WriteU32(ms, LayerCount);
        WriteU32(ms, FaceCount);
        uint levels = LevelCountOverride ?? (uint)MipPayloads.Count;
        WriteU32(ms, levels);
        WriteU32(ms, SupercompressionScheme);

        // Index placeholders (filled later).
        long indexFieldsStart = ms.Position;
        for (int i = 0; i < 4; i++) WriteU32(ms, 0); // dfd/kvd offset+length
        for (int i = 0; i < 2; i++) WriteU64(ms, 0); // sgd offset+length

        // Level index.
        long levelIndexStart = ms.Position;
        for (int i = 0; i < levels; i++)
        {
            WriteU64(ms, 0); WriteU64(ms, 0); WriteU64(ms, 0);
        }

        // Key-value pool.
        var kvPool = BuildKeyValuePool(KeyValues);
        long kvOffset = ms.Position;
        ms.Write(kvPool, 0, kvPool.Length);
        long kvEnd = ms.Position;

        int pad = (-(int)ms.Position) & 7;
        for (int p = 0; p < pad; p++) ms.WriteByte(0);

        // Mip payloads.
        var mipOffsets = new long[MipPayloads.Count];
        var mipLengths = new long[MipPayloads.Count];
        for (int i = 0; i < MipPayloads.Count; i++)
        {
            mipOffsets[i] = ms.Position;
            mipLengths[i] = MipPayloads[i].Length;
            ms.Write(MipPayloads[i], 0, MipPayloads[i].Length);
        }

        byte[] bytes = ms.ToArray();

        // Patch index fields.
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan((int)indexFieldsStart + 0, 4), 0); // dfdByteOffset (unused)
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan((int)indexFieldsStart + 4, 4), 0); // dfdByteLength
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan((int)indexFieldsStart + 8, 4), (uint)kvOffset);
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan((int)indexFieldsStart + 12, 4), (uint)(kvEnd - kvOffset));

        // Patch level index.
        for (int i = 0; i < MipPayloads.Count && i < levels; i++)
        {
            long e = levelIndexStart + i * 24;
            ulong uncompressedLen = UncompressedSizes is { } us && i < us.Count
                ? us[i]
                : (ulong)mipLengths[i];
            BinaryPrimitives.WriteUInt64LittleEndian(bytes.AsSpan((int)e + 0, 8), (ulong)mipOffsets[i]);
            BinaryPrimitives.WriteUInt64LittleEndian(bytes.AsSpan((int)e + 8, 8), (ulong)mipLengths[i]);
            BinaryPrimitives.WriteUInt64LittleEndian(bytes.AsSpan((int)e + 16, 8), uncompressedLen);
        }

        return bytes;
    }

    private static byte[] BuildKeyValuePool(List<KeyValuePair<string, string>> kvs)
    {
        if (kvs.Count == 0) return Array.Empty<byte>();
        using var ms = new MemoryStream();
        foreach (var kv in kvs)
        {
            var key = Encoding.UTF8.GetBytes(kv.Key);
            var value = Encoding.UTF8.GetBytes(kv.Value);
            int totalLen = key.Length + 1 + value.Length + 1;
            WriteU32(ms, (uint)totalLen);
            ms.Write(key, 0, key.Length);
            ms.WriteByte(0);
            ms.Write(value, 0, value.Length);
            ms.WriteByte(0);
            int pad = (-(int)ms.Position) & 3;
            for (int p = 0; p < pad; p++) ms.WriteByte(0);
        }
        return ms.ToArray();
    }

    private static void WriteU32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        ms.Write(b);
    }

    private static void WriteU64(MemoryStream ms, ulong v)
    {
        Span<byte> b = stackalloc byte[8];
        BinaryPrimitives.WriteUInt64LittleEndian(b, v);
        ms.Write(b);
    }
}
