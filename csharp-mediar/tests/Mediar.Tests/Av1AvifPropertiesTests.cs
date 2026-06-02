using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class Av1AvifPropertiesTests
{
    // ---------- a1op ----------

    [Fact]
    public void Av1OperatingPoint_TryParse_Decodes_Base()
    {
        Assert.True(HeifAv1OperatingPoint.TryParse(new byte[] { 0 }, out var op));
        Assert.Equal((byte)0, op.OpIndex);
    }

    [Fact]
    public void Av1OperatingPoint_TryParse_Decodes_NonZero_Index()
    {
        Assert.True(HeifAv1OperatingPoint.TryParse(new byte[] { 7 }, out var op));
        Assert.Equal((byte)7, op.OpIndex);
    }

    [Fact]
    public void Av1OperatingPoint_TryParse_Rejects_Empty_Payload()
    {
        Assert.False(HeifAv1OperatingPoint.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void HeifReader_Resolves_A1op_Via_Ipma()
    {
        var bytes = BuildHeifWithProperty("a1op", new byte[] { 3 });
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetAv1OperatingPoint(1, out var op));
        Assert.Equal((byte)3, op.OpIndex);

        Assert.False(r.TryGetAv1OperatingPoint(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_A1op()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetAv1OperatingPoint(1, out _));
    }

    // ---------- a1lx ----------

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Decodes_Small_Sizes()
    {
        // large_size=0, three uint16 layer sizes: 1000, 2000, 3000
        var payload = new byte[7];
        payload[0] = 0;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(1, 2), 1000);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(3, 2), 2000);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(5, 2), 3000);

        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.False(rec.LargeSize);
        Assert.Equal(new uint[] { 1000, 2000, 3000 }, rec.LayerSizes);
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Decodes_Large_Sizes()
    {
        // large_size=1, three uint32 layer sizes: 100000, 200000, 300000
        var payload = new byte[13];
        payload[0] = 1;
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(1, 4), 100000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(5, 4), 200000);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(9, 4), 300000);

        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.True(rec.LargeSize);
        Assert.Equal(new uint[] { 100000, 200000, 300000 }, rec.LayerSizes);
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Decodes_Missing_Trailing_Layers()
    {
        // Layer 0 carries the only data; layers 1+2 absent (size = 0).
        var payload = new byte[7];
        payload[0] = 0;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(1, 2), 4096);
        // layers 1 + 2 left as zero
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.Equal(new uint[] { 4096, 0, 0 }, rec.LayerSizes);
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Ignores_Reserved_Bits()
    {
        // Top 7 bits set; large_size = 0
        var payload = new byte[7];
        payload[0] = 0xFE;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(1, 2), 1);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(3, 2), 2);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(5, 2), 3);

        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.False(rec.LargeSize);
        Assert.Equal(new uint[] { 1, 2, 3 }, rec.LayerSizes);
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Rejects_Empty_Payload()
    {
        Assert.False(HeifAv1LayeredImageIndexing.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Rejects_Truncated_Small()
    {
        // large_size=0 needs 7 bytes; only 6 supplied.
        var payload = new byte[6];
        Assert.False(HeifAv1LayeredImageIndexing.TryParse(payload, out _));
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Rejects_Truncated_Large()
    {
        // large_size=1 needs 13 bytes; only 12 supplied.
        var payload = new byte[12];
        payload[0] = 1;
        Assert.False(HeifAv1LayeredImageIndexing.TryParse(payload, out _));
    }

    [Fact]
    public void HeifReader_Resolves_A1lx_Via_Ipma()
    {
        var payload = new byte[7];
        payload[0] = 0;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(1, 2), 512);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(3, 2), 1024);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(5, 2), 2048);
        var bytes = BuildHeifWithProperty("a1lx", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetAv1LayeredImageIndexing(1, out var rec));
        Assert.False(rec.LargeSize);
        Assert.Equal(new uint[] { 512, 1024, 2048 }, rec.LayerSizes);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_A1lx()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetAv1LayeredImageIndexing(1, out _));
    }

    [Fact]
    public void Av1OperatingPoint_TryParse_Decodes_Max_Index()
    {
        Assert.True(HeifAv1OperatingPoint.TryParse(new byte[] { 0xFF }, out var op));
        Assert.Equal(byte.MaxValue, op.OpIndex);
    }

    [Fact]
    public void Av1OperatingPoint_TryParse_Ignores_Trailing_Bytes()
    {
        Assert.True(HeifAv1OperatingPoint.TryParse(new byte[] { 5, 0xAA, 0xBB }, out var op));
        Assert.Equal((byte)5, op.OpIndex);
    }

    [Fact]
    public void Av1OperatingPoint_Record_Equality_Works()
    {
        var a = new HeifAv1OperatingPoint { OpIndex = 3 };
        var b = new HeifAv1OperatingPoint { OpIndex = 3 };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
        Assert.NotEqual(a, a with { OpIndex = 4 });
    }

    [Fact]
    public void Av1LayeredImageIndexing_LargeSize_Bit_IsExactlyLowBit()
    {
        // 0xFE = 11111110 → low bit clear → LargeSize false
        var payload = new byte[7];
        payload[0] = 0xFE;
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec1));
        Assert.False(rec1.LargeSize);

        // 0x01 = 00000001 → low bit set → LargeSize true
        var payload2 = new byte[13];
        payload2[0] = 0x01;
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload2, out var rec2));
        Assert.True(rec2.LargeSize);

        // 0xFF = 11111111 → low bit set → LargeSize true
        var payload3 = new byte[13];
        payload3[0] = 0xFF;
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload3, out var rec3));
        Assert.True(rec3.LargeSize);
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_AllZeroSizes()
    {
        var payload = new byte[7];
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.False(rec.LargeSize);
        Assert.Equal(3, rec.LayerSizes.Length);
        Assert.All(rec.LayerSizes, sz => Assert.Equal(0u, sz));
    }

    [Fact]
    public void Av1LayeredImageIndexing_TryParse_Large_MaxUInt32_Layer()
    {
        var payload = new byte[13];
        payload[0] = 1;
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(1, 4), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(5, 4), 1u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(9, 4), 2u);
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.Equal(uint.MaxValue, rec.LayerSizes[0]);
        Assert.Equal(1u, rec.LayerSizes[1]);
        Assert.Equal(2u, rec.LayerSizes[2]);
    }

    [Fact]
    public void Av1LayeredImageIndexing_LayerSizes_IsImmutableArray_NotDefault()
    {
        var payload = new byte[7];
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.False(rec.LayerSizes.IsDefault);
    }

    [Fact]
    public void Av1LayeredImageIndexing_OnlyLowBit_IsLargeSize()
    {
        // 0x02 (binary 00000010) → LargeSize false (low bit is 0).
        var payload = new byte[7];
        payload[0] = 0x02;
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(1, 2), 42);
        Assert.True(HeifAv1LayeredImageIndexing.TryParse(payload, out var rec));
        Assert.False(rec.LargeSize);
        Assert.Equal(42u, rec.LayerSizes[0]);
    }

    // ---------- helpers ----------

    private static byte[] BuildIspePayload(uint width, uint height)
    {
        byte[] payload = new byte[12];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), width);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), height);
        return payload;
    }

    private static byte[] BuildHeifWithProperty(string propertyType, byte[] propertyPayload)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write("avif"u8);
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write("mif1"u8);
            w.Write("avif"u8);
        });
        WriteBox(ms, "meta", meta =>
        {
            Span<byte> vf = stackalloc byte[4];
            meta.Write(vf);
            WriteBox(meta, "hdlr", h =>
            {
                Span<byte> b = stackalloc byte[25];
                Encoding.ASCII.GetBytes("pict").CopyTo(b.Slice(8));
                h.Write(b);
            });
            WriteBox(meta, "pitm", h =>
            {
                Span<byte> b = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), 1);
                h.Write(b);
            });
            WriteBox(meta, "iinf", h =>
            {
                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), 1);
                h.Write(hdr);
                WriteBox(h, "infe", inf =>
                {
                    Span<byte> data = stackalloc byte[15];
                    data[0] = 2;
                    BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), 1);
                    Encoding.ASCII.GetBytes("av01").CopyTo(data.Slice(8));
                    inf.Write(data);
                });
            });
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 64);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), 64);
                        isp.Write(data);
                    });
                    if (propertyType != "ispe")
                    {
                        WriteBox(ipco, propertyType, p => p.Write(propertyPayload));
                    }
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    int assocCount = propertyType == "ispe" ? 1 : 2;
                    Span<byte> entry = stackalloc byte[3 + assocCount];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), 1);
                    entry[2] = (byte)assocCount;
                    entry[3] = 1;
                    if (assocCount == 2) entry[4] = 2;
                    ipma.Write(entry);
                });
            });
        });
        return ms.ToArray();
    }

    private static void WriteBox(Stream s, string type, Action<MemoryStream> writePayload)
    {
        using var inner = new MemoryStream();
        writePayload(inner);
        var payload = inner.ToArray();
        int total = payload.Length + 8;
        Span<byte> hdr = stackalloc byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(0, 4), (uint)total);
        Encoding.ASCII.GetBytes(type).CopyTo(hdr.Slice(4, 4));
        s.Write(hdr);
        s.Write(payload);
    }
}
