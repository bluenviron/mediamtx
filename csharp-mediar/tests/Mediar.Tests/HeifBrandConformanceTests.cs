using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifBrandConformanceTests
{
    // ---------- AVIF ----------

    [Fact]
    public void ValidateAvif_Accepts_Well_Formed_Container()
    {
        var bytes = BuildHeif(
            majorBrand: "avif",
            compatibleBrands: ["mif1", "avif"],
            itemType: "av01",
            properties: [("ispe", BuildIspe(64, 64)), ("av1C", BuildAv1C())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Avif, ownsStream: true);

        var result = HeifBrandConformance.ValidateAvif(r);
        Assert.True(result.IsConformant);
        Assert.Empty(result.Issues);
        Assert.Equal("AVIF", result.ProfileName);
    }

    [Fact]
    public void ValidateAvif_Reports_Missing_Brand()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "av01",
            properties: [("ispe", BuildIspe(64, 64)), ("av1C", BuildAv1C())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateAvif(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("'avif'"));
    }

    [Fact]
    public void ValidateAvif_Reports_Missing_Av1C()
    {
        var bytes = BuildHeif(
            majorBrand: "avif",
            compatibleBrands: ["mif1", "avif"],
            itemType: "av01",
            properties: [("ispe", BuildIspe(64, 64))]); // missing av1C
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Avif, ownsStream: true);

        var result = HeifBrandConformance.ValidateAvif(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("av1C"));
    }

    [Fact]
    public void ValidateAvif_Reports_Wrong_Primary_Item_Type()
    {
        var bytes = BuildHeif(
            majorBrand: "avif",
            compatibleBrands: ["mif1", "avif"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64))]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Avif, ownsStream: true);

        var result = HeifBrandConformance.ValidateAvif(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("Primary item type"));
    }

    // ---------- HEIC ----------

    [Fact]
    public void ValidateHeic_Accepts_Well_Formed_Container()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64)), ("hvcC", BuildHvcC())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateHeic(r);
        Assert.True(result.IsConformant);
        Assert.Empty(result.Issues);
        Assert.Equal("HEIC Main", result.ProfileName);
    }

    [Fact]
    public void ValidateHeic_Accepts_Hev1_Item_Type()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "hev1",
            properties: [("ispe", BuildIspe(64, 64)), ("hvcC", BuildHvcC())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateHeic(r);
        Assert.True(result.IsConformant);
    }

    [Fact]
    public void ValidateHeic_Reports_Missing_Brand()
    {
        var bytes = BuildHeif(
            majorBrand: "avif",
            compatibleBrands: ["avif"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64)), ("hvcC", BuildHvcC())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Avif, ownsStream: true);

        var result = HeifBrandConformance.ValidateHeic(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("'heic'"));
    }

    [Fact]
    public void ValidateHeic_Reports_Missing_HvcC()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64))]); // missing hvcC
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateHeic(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("hvcC"));
    }

    [Fact]
    public void ValidateHeic_Reports_Missing_Ispe()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "hvc1",
            properties: [("hvcC", BuildHvcC())]); // missing ispe
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateHeic(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("ispe"));
    }

    // ---------- MIAF ----------

    [Fact]
    public void ValidateMiaf_Accepts_Pixi_For_Colour_Info()
    {
        var bytes = BuildHeif(
            majorBrand: "mif2",
            compatibleBrands: ["mif2", "heic", "miaf"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64)), ("pixi", BuildPixi())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateMiaf(r);
        Assert.True(result.IsConformant);
        Assert.Equal("MIAF", result.ProfileName);
    }

    [Fact]
    public void ValidateMiaf_Accepts_Colr_For_Colour_Info()
    {
        var bytes = BuildHeif(
            majorBrand: "mif2",
            compatibleBrands: ["mif2", "heic"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64)), ("colr", BuildColrNclx())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateMiaf(r);
        Assert.True(result.IsConformant);
    }

    [Fact]
    public void ValidateMiaf_Reports_Missing_Colour_Info()
    {
        var bytes = BuildHeif(
            majorBrand: "mif2",
            compatibleBrands: ["mif2", "heic"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64))]); // no pixi nor colr
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateMiaf(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("colour info"));
    }

    [Fact]
    public void ValidateMiaf_Reports_Missing_Brand()
    {
        var bytes = BuildHeif(
            majorBrand: "heic",
            compatibleBrands: ["mif1", "heic"],
            itemType: "hvc1",
            properties: [("ispe", BuildIspe(64, 64)), ("pixi", BuildPixi())]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        var result = HeifBrandConformance.ValidateMiaf(r);
        Assert.False(result.IsConformant);
        Assert.Contains(result.Issues, i => i.Contains("'miaf'"));
    }

    [Fact]
    public void Validators_Throw_On_Null_Reader()
    {
        Assert.Throws<ArgumentNullException>(() => HeifBrandConformance.ValidateAvif(null!));
        Assert.Throws<ArgumentNullException>(() => HeifBrandConformance.ValidateHeic(null!));
        Assert.Throws<ArgumentNullException>(() => HeifBrandConformance.ValidateMiaf(null!));
    }

    // ---------- helpers ----------

    private static byte[] BuildIspe(uint width, uint height)
    {
        byte[] payload = new byte[12];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), width);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), height);
        return payload;
    }

    private static byte[] BuildAv1C()
    {
        // Minimal valid av1C: marker=1 | version=1, profile=0 level=0, 4:2:0, no IPD.
        return [0x81, 0x00, 0x0C, 0x00];
    }

    private static byte[] BuildHvcC()
    {
        // 23-byte minimum hvcC stub: version=1, then zeroes, lengthSize=4 (lsm = 3) -> last header byte 0xFC | 0x03.
        var b = new byte[23];
        b[0] = 1;
        b[21] = 0xFC | 0x03; // numTempLayers=0/tempIdNested=0/lengthSizeMinusOne=3 + reserved
        b[22] = 0; // numOfArrays = 0
        return b;
    }

    private static byte[] BuildPixi()
    {
        return [0, 0, 0, 0, 3, 8, 8, 8];
    }

    private static byte[] BuildColrNclx()
    {
        // 'nclx' colour info: 4-char type + colour_primaries(2) + transfer(2) + matrix(2) + full_range_flag(1)
        var b = new byte[11];
        Encoding.ASCII.GetBytes("nclx").CopyTo(b.AsSpan(0));
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(4, 2), 1);
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(6, 2), 13);
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(8, 2), 6);
        b[10] = 0x80;
        return b;
    }

    private static byte[] BuildHeif(
        string majorBrand,
        string[] compatibleBrands,
        string itemType,
        (string Type, byte[] Payload)[] properties)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes(majorBrand));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            foreach (var b in compatibleBrands)
            {
                w.Write(Encoding.ASCII.GetBytes(b));
            }
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
                    Encoding.ASCII.GetBytes(itemType).CopyTo(data.Slice(8));
                    inf.Write(data);
                });
            });
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    foreach (var (type, payload) in properties)
                    {
                        WriteBox(ipco, type, p => p.Write(payload));
                    }
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    int assocCount = properties.Length;
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
