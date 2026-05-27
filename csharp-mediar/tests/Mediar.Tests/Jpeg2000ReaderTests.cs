using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Jpeg2000;
using Xunit;

namespace Mediar.Tests;

public class Jpeg2000ReaderTests
{
    [Fact]
    public void Parses_Raw_J2k_Codestream_With_Siz()
    {
        byte[] cs = BuildCodestream(width: 256, height: 128, components: 3, bitDepth: 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);

        Assert.False(r.HasJp2Wrapper);
        Assert.Equal(256, r.Info.Width);
        Assert.Equal(128, r.Info.Height);
        Assert.Equal(3, r.Components.Length);
        Assert.Equal(8, r.Components[0].BitDepth);
        Assert.False(r.Components[0].IsSigned);
    }

    [Fact]
    public void Parses_Jp2_Wrapper_Boxes()
    {
        byte[] cs = BuildCodestream(64, 64, 3, 8);
        byte[] file = WrapInJp2(cs, enumColorSpace: 16 /* sRGB */);
        using var r = Jpeg2000Reader.Open(new MemoryStream(file), ImageFormat.Jp2, ownsStream: true);

        Assert.True(r.HasJp2Wrapper);
        Assert.Contains(r.Boxes, b => b.Type == "jP  ");
        Assert.Contains(r.Boxes, b => b.Type == "ftyp");
        Assert.Contains(r.Boxes, b => b.Type == "jp2h");
        Assert.Contains(r.Boxes, b => b.Type == "jp2c");
        Assert.Equal("sRGB", r.ColourSpace);
        Assert.Equal(64, r.Info.Width);
        Assert.Equal(64, r.Info.Height);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws()
    {
        byte[] cs = BuildCodestream(8, 8, 1, 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    private static byte[] BuildCodestream(int width, int height, int components, int bitDepth)
    {
        using var ms = new MemoryStream();
        ms.WriteByte(0xFF); ms.WriteByte(0x4F);  // SOC
        // SIZ marker: FF 51 + len + 36 fixed bytes + 3 per component
        int contentLen = 36 + 3 * components;
        int segLen = 2 + contentLen;
        ms.WriteByte(0xFF); ms.WriteByte(0x51);
        Span<byte> len = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(len, (ushort)segLen);
        ms.Write(len);
        Span<byte> seg = stackalloc byte[36];
        BinaryPrimitives.WriteUInt16BigEndian(seg.Slice(0, 2), 0);  // Rsiz
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(2, 4), (uint)width);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(6, 4), (uint)height);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(10, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(14, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(18, 4), (uint)width);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(22, 4), (uint)height);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(26, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(seg.Slice(30, 4), 0);
        BinaryPrimitives.WriteUInt16BigEndian(seg.Slice(34, 2), (ushort)components);
        ms.Write(seg);
        for (int i = 0; i < components; i++)
        {
            ms.WriteByte((byte)(bitDepth - 1));  // Ssiz
            ms.WriteByte(1); ms.WriteByte(1);  // XRsiz / YRsiz
        }
        // EOC
        ms.WriteByte(0xFF); ms.WriteByte(0xD9);
        return ms.ToArray();
    }

    private static byte[] WrapInJp2(byte[] codestream, uint enumColorSpace)
    {
        using var ms = new MemoryStream();
        // jP signature box
        WriteBox(ms, "jP  ", w => w.Write(new byte[] { 0x0D, 0x0A, 0x87, 0x0A }));
        // ftyp
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes("jp2 "));
            w.Write(new byte[4]);
            w.Write(Encoding.ASCII.GetBytes("jp2 "));
        });
        // jp2h contains ihdr + colr
        WriteBox(ms, "jp2h", jp2h =>
        {
            WriteBox(jp2h, "ihdr", w =>
            {
                Span<byte> data = stackalloc byte[14];
                BinaryPrimitives.WriteUInt32BigEndian(data.Slice(0, 4), 64);  // height
                BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 64);  // width
                BinaryPrimitives.WriteUInt16BigEndian(data.Slice(8, 2), 3);
                w.Write(data);
            });
            WriteBox(jp2h, "colr", w =>
            {
                Span<byte> data = stackalloc byte[7];
                data[0] = 1;  // METH = enum
                BinaryPrimitives.WriteUInt32BigEndian(data.Slice(3, 4), enumColorSpace);
                w.Write(data);
            });
        });
        // jp2c (codestream)
        WriteBox(ms, "jp2c", w => w.Write(codestream));
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
