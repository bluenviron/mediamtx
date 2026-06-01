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

    [Fact]
    public void Open_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => Jpeg2000Reader.Open((Stream)null!));
    }

    [Fact]
    public void Raw_J2k_SingleComponent_Greyscale()
    {
        byte[] cs = BuildCodestream(width: 100, height: 50, components: 1, bitDepth: 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.Single(r.Components);
        Assert.Equal(8, r.Info.BitsPerPixel);
        Assert.Equal(1, r.Info.ChannelCount);
    }

    [Fact]
    public void Raw_J2k_HighBitDepth_PreservedPerComponent()
    {
        byte[] cs = BuildCodestream(width: 16, height: 16, components: 3, bitDepth: 16);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.Equal(48, r.Info.BitsPerPixel); // 3 * 16
        Assert.All(r.Components, c => Assert.Equal(16, c.BitDepth));
    }

    [Fact]
    public void Raw_J2k_NoSoc_Returns_Empty_Geometry()
    {
        // Just put junk before SOC; the reader gives up and returns default geometry.
        byte[] cs = new byte[] { 0x00, 0x00, 0x00, 0x00 };
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.Empty(r.Components);
        Assert.Equal(0, r.Info.Width);
        Assert.Equal(0, r.Info.Height);
    }

    [Fact]
    public void Jp2_Wrapper_Greyscale_ColourSpace_Recognised()
    {
        byte[] cs = BuildCodestream(32, 32, 1, 8);
        byte[] file = WrapInJp2(cs, enumColorSpace: 17 /* Greyscale */);
        using var r = Jpeg2000Reader.Open(new MemoryStream(file), ImageFormat.Jp2, ownsStream: true);
        Assert.Equal("Greyscale", r.ColourSpace);
    }

    [Fact]
    public void Jp2_Wrapper_sYCC_ColourSpace_Recognised()
    {
        byte[] cs = BuildCodestream(32, 32, 3, 8);
        byte[] file = WrapInJp2(cs, enumColorSpace: 18 /* sYCC */);
        using var r = Jpeg2000Reader.Open(new MemoryStream(file), ImageFormat.Jp2, ownsStream: true);
        Assert.Equal("sYCC", r.ColourSpace);
    }

    [Fact]
    public void Jp2_Wrapper_Unknown_Enum_Falls_Back_To_Enum_Tag()
    {
        byte[] cs = BuildCodestream(32, 32, 3, 8);
        byte[] file = WrapInJp2(cs, enumColorSpace: 9999);
        using var r = Jpeg2000Reader.Open(new MemoryStream(file), ImageFormat.Jp2, ownsStream: true);
        Assert.Equal("Enum:9999", r.ColourSpace);
    }

    [Fact]
    public void Raw_J2k_HasNoColourSpace_NoBoxes()
    {
        byte[] cs = BuildCodestream(32, 32, 3, 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.Empty(r.Boxes);
        Assert.Equal("", r.ColourSpace);
        Assert.Null(r.Info.ColorSpace);
    }

    [Fact]
    public void Format_Is_Set_From_Hint_Parameter()
    {
        byte[] cs = BuildCodestream(8, 8, 1, 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2c, ownsStream: true);
        Assert.Equal(ImageFormat.J2c, r.Format);
        Assert.Equal(ImageFormat.J2c, r.Info.Format);
    }

    [Fact]
    public void CanDecodePixels_Is_False()
    {
        byte[] cs = BuildCodestream(8, 8, 1, 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.False(r.CanDecodePixels);
        Assert.Equal(ImageMetadata.Empty, r.Metadata);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        byte[] cs = BuildCodestream(8, 8, 1, 8);
        var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void OwnsStream_False_Leaves_Source_Open()
    {
        byte[] cs = BuildCodestream(8, 8, 1, 8);
        var ms = new MemoryStream(cs);
        using (var r = Jpeg2000Reader.Open(ms, ImageFormat.J2k, ownsStream: false))
        {
            Assert.NotNull(r);
        }
        Assert.True(ms.CanRead);
    }

    [Fact]
    public void Size_Properties_PopulatedFromSiz()
    {
        byte[] cs = BuildCodestream(width: 320, height: 240, components: 3, bitDepth: 8);
        using var r = Jpeg2000Reader.Open(new MemoryStream(cs), ImageFormat.J2k, ownsStream: true);
        Assert.Equal(320u, r.Size.Xsiz);
        Assert.Equal(240u, r.Size.Ysiz);
        Assert.Equal(0u, r.Size.XOsiz);
        Assert.Equal(0u, r.Size.YOsiz);
    }
}
