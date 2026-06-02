using System.Buffers.Binary;
using System.Collections.Immutable;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifLayerAndReferencePropertiesTests
{
    // ---------- lsel ----------

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(255)]
    [InlineData(65535)]
    public void LayerSelector_TryParse_Decodes_LayerId(ushort layerId)
    {
        var payload = new byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(payload, layerId);
        Assert.True(HeifLayerSelector.TryParse(payload, out var rec));
        Assert.Equal(layerId, rec.LayerId);
    }

    [Fact]
    public void LayerSelector_TryParse_Rejects_Truncated_Payload()
    {
        Assert.False(HeifLayerSelector.TryParse(new byte[1], out _));
        Assert.False(HeifLayerSelector.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void HeifReader_Resolves_Lsel_Via_Ipma()
    {
        var payload = new byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(payload, 5);
        var bytes = BuildHeifWithProperty("lsel", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetLayerSelector(1, out var rec));
        Assert.Equal((ushort)5, rec.LayerId);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Lsel()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetLayerSelector(1, out _));
    }

    // ---------- rref ----------

    [Fact]
    public void RequiredReference_TryParse_Decodes_Empty_List()
    {
        // FullBox header + count=0
        var payload = new byte[] { 0, 0, 0, 0, 0 };
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        Assert.True(rec.ReferenceTypes.IsEmpty);
        Assert.False(rec.Requires("dimg"));
    }

    [Fact]
    public void RequiredReference_TryParse_Decodes_Single_Reference()
    {
        var payload = BuildRrefPayload("dimg");
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        string[] expected = ["dimg"];
        Assert.Equal(expected, rec.ReferenceTypes);
        Assert.True(rec.Requires("dimg"));
        Assert.False(rec.Requires("thmb"));
        Assert.False(rec.Requires(""));
    }

    [Fact]
    public void RequiredReference_TryParse_Decodes_Multiple_References()
    {
        var payload = BuildRrefPayload("dimg", "thmb", "auxl");
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        string[] expected = ["dimg", "thmb", "auxl"];
        Assert.Equal(expected, rec.ReferenceTypes);
        Assert.True(rec.Requires("auxl"));
    }

    [Fact]
    public void RequiredReference_TryParse_Preserves_Order()
    {
        var payload = BuildRrefPayload("auxl", "dimg", "thmb");
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        string[] expected = ["auxl", "dimg", "thmb"];
        Assert.Equal(expected, rec.ReferenceTypes);
    }

    [Fact]
    public void RequiredReference_TryParse_Rejects_Wrong_Version()
    {
        var payload = new byte[] { 1, 0, 0, 0, 0 };
        Assert.False(HeifRequiredReference.TryParse(payload, out _));
    }

    [Fact]
    public void RequiredReference_TryParse_Rejects_Truncated_Header()
    {
        Assert.False(HeifRequiredReference.TryParse(new byte[4], out _));
    }

    [Fact]
    public void RequiredReference_TryParse_Rejects_Truncated_Entries()
    {
        // declares count=2 but only carries 1 entry (4 bytes) of payload.
        var payload = new byte[5 + 4];
        payload[4] = 2; // count
        Encoding.ASCII.GetBytes("dimg").CopyTo(payload.AsSpan(5));
        Assert.False(HeifRequiredReference.TryParse(payload, out _));
    }

    [Fact]
    public void HeifReader_Resolves_Rref_Via_Ipma()
    {
        var payload = BuildRrefPayload("dimg", "auxl");
        var bytes = BuildHeifWithProperty("rref", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetRequiredReference(1, out var rec));
        string[] expected = ["dimg", "auxl"];
        Assert.Equal(expected, rec.ReferenceTypes);
        Assert.True(rec.Requires("dimg"));
        Assert.True(rec.Requires("auxl"));
        Assert.False(rec.Requires("thmb"));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Rref()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetRequiredReference(1, out _));
    }

    [Fact]
    public void LayerSelector_TryParse_Decodes_MaxLayerId()
    {
        // 0xFFFF means "delegated to embedded operating-point info"
        Assert.True(HeifLayerSelector.TryParse(new byte[] { 0xFF, 0xFF }, out var rec));
        Assert.Equal(ushort.MaxValue, rec.LayerId);
    }

    [Fact]
    public void LayerSelector_TryParse_Ignores_Trailing_Bytes()
    {
        // Reader honours [..2] slice; trailing bytes are not consumed.
        byte[] payload = [0x00, 0x05, 0xDE, 0xAD, 0xBE, 0xEF];
        Assert.True(HeifLayerSelector.TryParse(payload, out var rec));
        Assert.Equal((ushort)5, rec.LayerId);
    }

    [Fact]
    public void LayerSelector_Record_Equality_HoldsForSameLayerId()
    {
        var a = new HeifLayerSelector { LayerId = 7 };
        var b = new HeifLayerSelector { LayerId = 7 };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
        Assert.NotEqual(a, a with { LayerId = 8 });
    }

    [Fact]
    public void RequiredReference_Requires_ReturnsFalse_ForNullString()
    {
        var rec = new HeifRequiredReference
        {
            ReferenceTypes = ImmutableArray.Create("dimg", "thmb"),
        };
        Assert.False(rec.Requires(null!));
        Assert.False(rec.Requires(""));
        Assert.True(rec.Requires("dimg"));
    }

    [Fact]
    public void RequiredReference_TryParse_ZeroCount_PreservesEmptyImmutableArray()
    {
        var payload = new byte[] { 0, 0, 0, 0, 0 };
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        Assert.False(rec.ReferenceTypes.IsDefault);
        Assert.True(rec.ReferenceTypes.IsEmpty);
    }

    [Theory]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(255)]
    public void RequiredReference_TryParse_Rejects_UnknownVersions(byte version)
    {
        var payload = new byte[] { version, 0, 0, 0, 0 };
        Assert.False(HeifRequiredReference.TryParse(payload, out _));
    }

    [Fact]
    public void RequiredReference_TryParse_ReadsExactlyDeclaredBytes_IgnoresTrailing()
    {
        // count=1 → 9 byte payload, but we tack on 8 trailing bytes.
        var payload = new byte[5 + 4 + 8];
        payload[4] = 1;
        Encoding.ASCII.GetBytes("dimg").CopyTo(payload.AsSpan(5));
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        Assert.Single(rec.ReferenceTypes);
        Assert.Equal("dimg", rec.ReferenceTypes[0]);
    }

    [Fact]
    public void RequiredReference_TryParse_DuplicateReferenceTypes_AllPreserved()
    {
        var payload = BuildRrefPayload("dimg", "dimg", "thmb");
        Assert.True(HeifRequiredReference.TryParse(payload, out var rec));
        Assert.Equal(3, rec.ReferenceTypes.Length);
        Assert.Equal("dimg", rec.ReferenceTypes[0]);
        Assert.Equal("dimg", rec.ReferenceTypes[1]);
        Assert.Equal("thmb", rec.ReferenceTypes[2]);
        Assert.True(rec.Requires("dimg"));
    }

    [Fact]
    public void RequiredReference_TryParse_RejectsEmpty()
    {
        Assert.False(HeifRequiredReference.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void RequiredReference_Record_Equality_StrictByReferenceArray()
    {
        var arr = ImmutableArray.Create("dimg");
        var a = new HeifRequiredReference { ReferenceTypes = arr };
        var b = new HeifRequiredReference { ReferenceTypes = arr };
        Assert.Equal(a, b);
    }

    // ---------- helpers ----------

    private static byte[] BuildRrefPayload(params string[] referenceTypes)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 }); // FullBox header
        ms.WriteByte((byte)referenceTypes.Length);
        foreach (var rt in referenceTypes)
        {
            if (rt.Length != 4) throw new ArgumentException("reference type must be 4 ASCII chars", nameof(referenceTypes));
            ms.Write(Encoding.ASCII.GetBytes(rt));
        }
        return ms.ToArray();
    }

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
            w.Write("heic"u8);
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write("mif1"u8);
            w.Write("heic"u8);
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
