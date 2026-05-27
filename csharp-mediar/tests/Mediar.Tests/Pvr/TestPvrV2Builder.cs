using System.Buffers.Binary;
using Mediar.Imaging.Pvr;

namespace Mediar.Tests.Pvr;

/// <summary>
/// Test-only synthesiser for valid PVR v2 (legacy) byte streams. Lays
/// out the 52-byte little-endian header followed by the pixel payload
/// per surface and per mip-level.
/// </summary>
internal sealed class TestPvrV2Builder
{
    public uint HeaderSize { get; set; } = 52;
    public uint Height { get; set; } = 4;
    public uint Width { get; set; } = 4;
    public uint MipMapCount { get; set; }
    public PvrV2FormatId FormatId { get; set; } = PvrV2FormatId.Argb8888;
    public PvrV2Flags Flags { get; set; }
    public uint DataLength { get; set; }
    public uint BitsPerPixel { get; set; } = 32;
    public uint RedMask { get; set; }
    public uint GreenMask { get; set; }
    public uint BlueMask { get; set; }
    public uint AlphaMask { get; set; }
    public uint Magic { get; set; } = 0x21525650u;
    public uint NumSurfaces { get; set; } = 1;

    public List<byte[]> Payloads { get; } = new();

    public byte[] Build()
    {
        using var ms = new MemoryStream();
        WriteU32(ms, HeaderSize);
        WriteU32(ms, Height);
        WriteU32(ms, Width);
        WriteU32(ms, MipMapCount);
        uint pfWord = ((uint)Flags & 0xFFFFFF00u) | (byte)FormatId;
        WriteU32(ms, pfWord);
        WriteU32(ms, DataLength);
        WriteU32(ms, BitsPerPixel);
        WriteU32(ms, RedMask);
        WriteU32(ms, GreenMask);
        WriteU32(ms, BlueMask);
        WriteU32(ms, AlphaMask);
        WriteU32(ms, Magic);
        WriteU32(ms, NumSurfaces);
        foreach (var p in Payloads) ms.Write(p, 0, p.Length);
        return ms.ToArray();
    }

    private static void WriteU32(Stream s, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        s.Write(b);
    }
}
