using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifReaderTests
{
    [Fact]
    public void Parses_Ftyp_Major_Brand_And_Sets_Format()
    {
        var bytes = BuildMinimalHeif(brand: "heic", widthDim: 320, heightDim: 240);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal("heic", r.MajorBrand);
        Assert.Equal(ImageFormat.Heic, r.Format);
        Assert.Equal(320, r.Info.Width);
        Assert.Equal(240, r.Info.Height);
    }

    [Fact]
    public void Recognises_Avif_Brand()
    {
        var bytes = BuildMinimalHeif(brand: "avif", widthDim: 64, heightDim: 64);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.Equal("avif", r.MajorBrand);
        Assert.Equal(ImageFormat.Avif, r.Format);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_For_Hevc_Decode()
    {
        var bytes = BuildMinimalHeif(brand: "heic", widthDim: 8, heightDim: 8);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public void Exposes_Primary_Item()
    {
        var bytes = BuildMinimalHeif("heic", 100, 100, primaryItemId: 1);
        using var r = HeifReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(1u, r.PrimaryItemId);
    }

    // ---- fixture builder ----
    private static byte[] BuildMinimalHeif(string brand, int widthDim, int heightDim, uint primaryItemId = 1)
    {
        using var ms = new MemoryStream();

        // ftyp box
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes(brand));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write(Encoding.ASCII.GetBytes("mif1"));
            w.Write(Encoding.ASCII.GetBytes(brand));
        });

        // meta box (FullBox: version=0 flags=0)
        WriteBox(ms, "meta", meta =>
        {
            Span<byte> vf = stackalloc byte[4];
            meta.Write(vf);

            // hdlr
            WriteBox(meta, "hdlr", h =>
            {
                Span<byte> b = stackalloc byte[25];
                Encoding.ASCII.GetBytes("pict").CopyTo(b.Slice(8));
                h.Write(b);
            });

            // pitm version=0
            WriteBox(meta, "pitm", h =>
            {
                Span<byte> b = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), (ushort)primaryItemId);
                h.Write(b);
            });

            // iinf v0 with one infe v2
            WriteBox(meta, "iinf", h =>
            {
                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), 1);
                h.Write(hdr);
                WriteBox(h, "infe", inf =>
                {
                    Span<byte> data = stackalloc byte[15];
                    data[0] = 2;  // version
                    BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), (ushort)primaryItemId);
                    Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
                    // name: empty NUL
                    inf.Write(data);
                });
            });

            // iprp / ipco / ispe
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), (uint)widthDim);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), (uint)heightDim);
                        isp.Write(data);
                    });
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    Span<byte> entry = stackalloc byte[4];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), (ushort)primaryItemId);
                    entry[2] = 1;  // assoc count
                    entry[3] = 1;  // property index 1
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
