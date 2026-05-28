using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifTransformPropertiesTests
{
    // ---------- irot ----------

    [Theory]
    [InlineData(0, HeifImageRotation.None)]
    [InlineData(1, HeifImageRotation.Rotate90Ccw)]
    [InlineData(2, HeifImageRotation.Rotate180)]
    [InlineData(3, HeifImageRotation.Rotate270Ccw)]
    public void HeifReader_Resolves_Irot_Via_Ipma(byte rotByte, HeifImageRotation expected)
    {
        var bytes = BuildHeifWithProperty("irot", new byte[] { rotByte });
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetImageRotation(1, out var rot));
        Assert.Equal(expected, rot);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Irot()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetImageRotation(1, out var rot));
        Assert.Equal(HeifImageRotation.None, rot);
    }

    // ---------- imir ----------

    [Theory]
    [InlineData(0, HeifImageMirrorAxis.Vertical)]
    [InlineData(1, HeifImageMirrorAxis.Horizontal)]
    public void HeifReader_Resolves_Imir_Via_Ipma(byte axisByte, HeifImageMirrorAxis expected)
    {
        var bytes = BuildHeifWithProperty("imir", new byte[] { axisByte });
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetImageMirror(1, out var axis));
        Assert.Equal(expected, axis);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Imir()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetImageMirror(1, out _));
    }

    // ---------- pasp ----------

    [Fact]
    public void HeifReader_Resolves_Pasp_Via_Ipma()
    {
        byte[] payload = new byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(0, 4), 40);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4, 4), 33);
        var bytes = BuildHeifWithProperty("pasp", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetPixelAspectRatio(1, out var aspect));
        Assert.Equal(40u, aspect.HorizontalSpacing);
        Assert.Equal(33u, aspect.VerticalSpacing);
        Assert.Equal(40.0 / 33.0, aspect.Ratio, precision: 12);
    }

    [Fact]
    public void HeifPixelAspectRatio_Ratio_Is_NaN_When_VerticalSpacing_Is_Zero()
    {
        var aspect = new HeifPixelAspectRatio { HorizontalSpacing = 5, VerticalSpacing = 0 };
        Assert.True(double.IsNaN(aspect.Ratio));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Pasp()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetPixelAspectRatio(1, out _));
    }

    // ---------- pixi ----------

    [Fact]
    public void HeifPixelInformation_TryParse_Decodes_8Bit_Rgb()
    {
        // FullBox header + numChannels=3 + 3*8
        var payload = new byte[] { 0, 0, 0, 0, 3, 8, 8, 8 };
        Assert.True(HeifPixelInformation.TryParse(payload, out var info));
        Assert.Equal(3, info.NumberOfChannels);
        Assert.Equal(new byte[] { 8, 8, 8 }, info.BitDepthsPerChannel);
    }

    [Fact]
    public void HeifPixelInformation_TryParse_Decodes_10Bit_Yuv()
    {
        var payload = new byte[] { 0, 0, 0, 0, 3, 10, 10, 10 };
        Assert.True(HeifPixelInformation.TryParse(payload, out var info));
        Assert.Equal(new byte[] { 10, 10, 10 }, info.BitDepthsPerChannel);
    }

    [Fact]
    public void HeifPixelInformation_TryParse_Decodes_Monochrome()
    {
        var payload = new byte[] { 0, 0, 0, 0, 1, 16 };
        Assert.True(HeifPixelInformation.TryParse(payload, out var info));
        Assert.Equal(1, info.NumberOfChannels);
        Assert.Equal((byte)16, info.BitDepthsPerChannel[0]);
    }

    [Fact]
    public void HeifPixelInformation_TryParse_Rejects_Truncated_Payload()
    {
        // declares 4 channels but only carries 2 bytes.
        var payload = new byte[] { 0, 0, 0, 0, 4, 8, 8 };
        Assert.False(HeifPixelInformation.TryParse(payload, out _));
    }

    [Fact]
    public void HeifPixelInformation_TryParse_Rejects_Short_Header()
    {
        Assert.False(HeifPixelInformation.TryParse(new byte[4], out _));
    }

    [Fact]
    public void HeifReader_Resolves_Pixi_Via_Ipma()
    {
        // 3-channel 10-bit Rgb.
        var payload = new byte[] { 0, 0, 0, 0, 3, 10, 10, 10 };
        var bytes = BuildHeifWithProperty("pixi", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetPixelInformation(1, out var info));
        Assert.Equal(3, info.NumberOfChannels);
        Assert.Equal(new byte[] { 10, 10, 10 }, info.BitDepthsPerChannel);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_Pixi()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetPixelInformation(1, out _));
    }

    // ---------- auxC ----------

    [Fact]
    public void HeifAuxiliaryType_TryParse_Decodes_Alpha_Urn_With_No_Subtype()
    {
        var payload = BuildAuxCPayload("urn:mpeg:mpegB:cicp:systems:auxiliary:alpha", []);
        Assert.True(HeifAuxiliaryType.TryParse(payload, out var aux));
        Assert.Equal("urn:mpeg:mpegB:cicp:systems:auxiliary:alpha", aux.AuxTypeUrn);
        Assert.True(aux.IsAlpha);
        Assert.False(aux.IsDepth);
        Assert.False(aux.IsGainMap);
        Assert.True(aux.AuxSubtype.IsEmpty);
    }

    [Fact]
    public void HeifAuxiliaryType_TryParse_Decodes_Depth_Urn()
    {
        var payload = BuildAuxCPayload("urn:mpeg:mpegB:cicp:systems:auxiliary:depth", []);
        Assert.True(HeifAuxiliaryType.TryParse(payload, out var aux));
        Assert.True(aux.IsDepth);
    }

    [Fact]
    public void HeifAuxiliaryType_TryParse_Decodes_GainMap_Urn()
    {
        var payload = BuildAuxCPayload("urn:iso:std:iso:ts:21496:-1:gainmap", []);
        Assert.True(HeifAuxiliaryType.TryParse(payload, out var aux));
        Assert.True(aux.IsGainMap);
    }

    [Fact]
    public void HeifAuxiliaryType_TryParse_Decodes_Subtype_Bytes()
    {
        byte[] subtype = [0xAA, 0xBB, 0xCC, 0xDD];
        var payload = BuildAuxCPayload("urn:custom:my:aux", subtype);
        Assert.True(HeifAuxiliaryType.TryParse(payload, out var aux));
        Assert.Equal("urn:custom:my:aux", aux.AuxTypeUrn);
        Assert.Equal(subtype, aux.AuxSubtype);
    }

    [Fact]
    public void HeifAuxiliaryType_TryParse_Rejects_Short_Header()
    {
        Assert.False(HeifAuxiliaryType.TryParse(new byte[3], out _));
    }

    [Fact]
    public void HeifReader_Resolves_AuxC_Via_Ipma()
    {
        var payload = BuildAuxCPayload("urn:mpeg:mpegB:cicp:systems:auxiliary:alpha", []);
        var bytes = BuildHeifWithProperty("auxC", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetAuxiliaryType(1, out var aux));
        Assert.True(aux.IsAlpha);
    }

    [Fact]
    public void HeifReader_Resolves_AuxC_With_Subtype_Via_Ipma()
    {
        byte[] subtype = [0x01, 0x02, 0x03];
        var payload = BuildAuxCPayload("urn:custom:test", subtype);
        var bytes = BuildHeifWithProperty("auxC", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetAuxiliaryType(1, out var aux));
        Assert.Equal("urn:custom:test", aux.AuxTypeUrn);
        Assert.Equal(subtype, aux.AuxSubtype);
    }

    [Fact]
    public void HeifReader_Rejects_Missing_AuxC()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetAuxiliaryType(1, out _));
    }

    // ---------- helpers ----------

    private static byte[] BuildAuxCPayload(string urn, byte[] subtype)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { 0, 0, 0, 0 }); // version + flags
        ms.Write(Encoding.UTF8.GetBytes(urn));
        ms.WriteByte(0);
        ms.Write(subtype);
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
