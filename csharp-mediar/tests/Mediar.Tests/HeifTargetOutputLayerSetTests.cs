using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifTargetOutputLayerSetTests
{
    [Fact]
    public void TryParse_Accepts_Zero_Index()
    {
        // FullBox header (version=0, flags=0) + 2-byte target_ols = 0
        byte[] payload = [0, 0, 0, 0, 0, 0];
        Assert.True(HeifTargetOutputLayerSet.TryParse(payload, out var result));
        Assert.NotNull(result);
        Assert.Equal((ushort)0, result!.TargetOlsIndex);
    }

    [Fact]
    public void TryParse_Accepts_Non_Zero_Index()
    {
        // target_ols = 0x1234
        byte[] payload = [0, 0, 0, 0, 0x12, 0x34];
        Assert.True(HeifTargetOutputLayerSet.TryParse(payload, out var result));
        Assert.NotNull(result);
        Assert.Equal((ushort)0x1234, result!.TargetOlsIndex);
    }

    [Fact]
    public void TryParse_Accepts_Max_Index()
    {
        byte[] payload = [0, 0, 0, 0, 0xFF, 0xFF];
        Assert.True(HeifTargetOutputLayerSet.TryParse(payload, out var result));
        Assert.Equal(ushort.MaxValue, result!.TargetOlsIndex);
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        byte[] payload = [1, 0, 0, 0, 0, 5]; // version = 1
        Assert.False(HeifTargetOutputLayerSet.TryParse(payload, out var result));
        Assert.Null(result);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Payload()
    {
        byte[] payload = [0, 0, 0, 0, 0]; // only 5 bytes
        Assert.False(HeifTargetOutputLayerSet.TryParse(payload, out var result));
        Assert.Null(result);
    }

    [Fact]
    public void TryParse_Rejects_Empty_Payload()
    {
        Assert.False(HeifTargetOutputLayerSet.TryParse(ReadOnlySpan<byte>.Empty, out var result));
        Assert.Null(result);
    }

    [Fact]
    public void HeifReader_TryGetTargetOutputLayerSet_Roundtrips_Through_Container()
    {
        var bytes = BuildHeifWithProperty("tols", [0, 0, 0, 0, 0x00, 0x07]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetTargetOutputLayerSet(1, out var tols));
        Assert.NotNull(tols);
        Assert.Equal((ushort)7, tols.TargetOlsIndex);
    }

    [Fact]
    public void HeifReader_TryGetTargetOutputLayerSet_Returns_False_When_Property_Missing()
    {
        var bytes = BuildHeifWithProperty(propertyType: null, propertyPayload: null);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.False(r.TryGetTargetOutputLayerSet(1, out var tols));
        Assert.Null(tols);
    }

    // ---------- helpers ----------

    private static byte[] BuildHeifWithProperty(string? propertyType, byte[]? propertyPayload)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes("heic"));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write(Encoding.ASCII.GetBytes("mif1"));
            w.Write(Encoding.ASCII.GetBytes("heic"));
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
                    Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
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
                    if (propertyType is not null && propertyPayload is not null)
                    {
                        WriteBox(ipco, propertyType, p => p.Write(propertyPayload));
                    }
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    int assocCount = propertyType is null ? 1 : 2;
                    Span<byte> entry = stackalloc byte[3 + assocCount];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), 1);
                    entry[2] = (byte)assocCount;
                    for (int i = 0; i < assocCount; i++) entry[3 + i] = (byte)(i + 1);
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
