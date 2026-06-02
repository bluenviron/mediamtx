using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifImagePropertiesTests
{
    [Fact]
    public void Clli_TryParse_Decodes_4_Bytes()
    {
        byte[] payload = new byte[4];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0), 1200);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 400);

        Assert.True(HeifContentLightLevel.TryParse(payload, out var info));
        Assert.Equal((ushort)1200, info.MaxContentLightLevel);
        Assert.Equal((ushort)400, info.MaxPicAverageLightLevel);
    }

    [Fact]
    public void Clli_TryParse_Rejects_Truncated()
    {
        Assert.False(HeifContentLightLevel.TryParse(new byte[3], out _));
    }

    [Fact]
    public void Mdcv_TryParse_Decodes_24_Bytes()
    {
        // Rec.2020 primaries scaled by 50000:
        //   R=(0.708, 0.292) -> (35400, 14600)
        //   G=(0.170, 0.797) -> (8500, 39850)
        //   B=(0.131, 0.046) -> (6550, 2300)
        //   W=(0.3127, 0.3290) -> (15635, 16450) (D65)
        byte[] payload = new byte[24];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0), 35400);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 14600);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(4), 8500);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(6), 39850);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(8), 6550);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10), 2300);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12), 15635);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(14), 16450);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(16), 10000_0000u);  // 1000 cd/m^2
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(20), 1u);            // 0.0001 cd/m^2

        Assert.True(HeifMasteringDisplayColourVolume.TryParse(payload, out var info));
        Assert.Equal(((ushort)35400, (ushort)14600), info.DisplayPrimaryR);
        Assert.Equal(((ushort)8500, (ushort)39850), info.DisplayPrimaryG);
        Assert.Equal(((ushort)6550, (ushort)2300), info.DisplayPrimaryB);
        Assert.Equal(((ushort)15635, (ushort)16450), info.WhitePoint);
        Assert.Equal(10000_0000u, info.MaxDisplayMasteringLuminance);
        Assert.Equal(1u, info.MinDisplayMasteringLuminance);
    }

    [Fact]
    public void Mdcv_TryParse_Rejects_Truncated()
    {
        Assert.False(HeifMasteringDisplayColourVolume.TryParse(new byte[20], out _));
    }

    [Fact]
    public void Clap_TryParse_Decodes_32_Bytes_With_Negative_Offsets()
    {
        // width=1920/1, height=1080/1, hOff=-10/1, vOff=5/1
        byte[] payload = new byte[32];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(0), 1920);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), 1);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), 1080);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(12), 1);
        // -10 as signed int (two's complement) -> 0xFFFFFFF6
        BinaryPrimitives.WriteInt32BigEndian(payload.AsSpan(16), -10);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(20), 1);
        BinaryPrimitives.WriteInt32BigEndian(payload.AsSpan(24), 5);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(28), 1);

        Assert.True(HeifCleanAperture.TryParse(payload, out var info));
        Assert.Equal((1920, 1u), info.Width);
        Assert.Equal((1080, 1u), info.Height);
        Assert.Equal((-10, 1u), info.HorizontalOffset);
        Assert.Equal((5, 1u), info.VerticalOffset);
    }

    [Fact]
    public void Clap_TryParse_Rejects_Truncated()
    {
        Assert.False(HeifCleanAperture.TryParse(new byte[28], out _));
    }

    [Fact]
    public void HeifReader_Resolves_Clli_Via_Ipma_Association()
    {
        byte[] clli = new byte[4];
        BinaryPrimitives.WriteUInt16BigEndian(clli.AsSpan(0), 1500);
        BinaryPrimitives.WriteUInt16BigEndian(clli.AsSpan(2), 600);

        var bytes = BuildHeifWithProperty(propertyType: "clli", propertyPayload: clli);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetContentLightLevel(1, out var info));
        Assert.Equal((ushort)1500, info.MaxContentLightLevel);
        Assert.Equal((ushort)600, info.MaxPicAverageLightLevel);

        Assert.False(r.TryGetContentLightLevel(99, out _));
    }

    [Fact]
    public void HeifReader_Resolves_Mdcv_Via_Ipma_Association()
    {
        byte[] mdcv = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(mdcv.AsSpan(16), 50000000u);
        BinaryPrimitives.WriteUInt32BigEndian(mdcv.AsSpan(20), 10u);

        var bytes = BuildHeifWithProperty(propertyType: "mdcv", propertyPayload: mdcv);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetMasteringDisplayColourVolume(1, out var info));
        Assert.Equal(50000000u, info.MaxDisplayMasteringLuminance);
        Assert.Equal(10u, info.MinDisplayMasteringLuminance);
    }

    [Fact]
    public void HeifReader_Resolves_Clap_Via_Ipma_Association()
    {
        byte[] clap = new byte[32];
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(0), 800);
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(4), 1);
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(8), 600);
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(12), 1);
        BinaryPrimitives.WriteInt32BigEndian(clap.AsSpan(16), 0);
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(20), 1);
        BinaryPrimitives.WriteInt32BigEndian(clap.AsSpan(24), 0);
        BinaryPrimitives.WriteUInt32BigEndian(clap.AsSpan(28), 1);

        var bytes = BuildHeifWithProperty(propertyType: "clap", propertyPayload: clap);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetCleanAperture(1, out var info));
        Assert.Equal((800, 1u), info.Width);
        Assert.Equal((600, 1u), info.Height);
        Assert.Equal((0, 1u), info.HorizontalOffset);
        Assert.Equal((0, 1u), info.VerticalOffset);
    }

    [Fact]
    public void HeifReader_Returns_False_For_Missing_Property()
    {
        // ispe-only fixture.
        var bytes = BuildHeifWithProperty(propertyType: "ispe", propertyPayload: BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.False(r.TryGetContentLightLevel(1, out _));
        Assert.False(r.TryGetMasteringDisplayColourVolume(1, out _));
        Assert.False(r.TryGetCleanAperture(1, out _));
    }

    [Fact]
    public void HeifReader_Returns_False_For_Truncated_Property_Payload()
    {
        // 2-byte clli (truncated) - ParseProperty stores nothing because the case guard requires >= 4 bytes.
        var bytes = BuildHeifWithProperty(propertyType: "clli", propertyPayload: new byte[2]);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.False(r.TryGetContentLightLevel(1, out _));
    }

    [Fact]
    public void Clli_TryParse_MaxUshortValues_PreservedExactly()
    {
        byte[] payload = new byte[4];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0), ushort.MaxValue);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), ushort.MaxValue);
        Assert.True(HeifContentLightLevel.TryParse(payload, out var info));
        Assert.Equal(ushort.MaxValue, info.MaxContentLightLevel);
        Assert.Equal(ushort.MaxValue, info.MaxPicAverageLightLevel);
    }

    [Fact]
    public void Clli_TryParse_AllZero_Yields_Zeros()
    {
        Assert.True(HeifContentLightLevel.TryParse(new byte[4], out var info));
        Assert.Equal((ushort)0, info.MaxContentLightLevel);
        Assert.Equal((ushort)0, info.MaxPicAverageLightLevel);
    }

    [Fact]
    public void Clli_TryParse_TrailingBytes_Ignored()
    {
        byte[] payload = new byte[8];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0), 100);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 50);
        payload[4] = 0xDE; payload[5] = 0xAD; payload[6] = 0xBE; payload[7] = 0xEF;
        Assert.True(HeifContentLightLevel.TryParse(payload, out var info));
        Assert.Equal((ushort)100, info.MaxContentLightLevel);
        Assert.Equal((ushort)50, info.MaxPicAverageLightLevel);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(3)]
    public void Clli_TryParse_TooShort_Rejected(int length)
    {
        Assert.False(HeifContentLightLevel.TryParse(new byte[length], out _));
    }

    [Fact]
    public void Clli_Record_Equality_AcrossIdenticalParses()
    {
        byte[] payload = new byte[4];
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0), 800);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2), 200);
        Assert.True(HeifContentLightLevel.TryParse(payload, out var a));
        Assert.True(HeifContentLightLevel.TryParse(payload, out var b));
        Assert.Equal(a, b);
    }

    [Fact]
    public void Mdcv_TryParse_MaxUint32Luminance_Preserved()
    {
        byte[] payload = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(16), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(20), uint.MaxValue);
        Assert.True(HeifMasteringDisplayColourVolume.TryParse(payload, out var info));
        Assert.Equal(uint.MaxValue, info.MaxDisplayMasteringLuminance);
        Assert.Equal(uint.MaxValue, info.MinDisplayMasteringLuminance);
    }

    [Fact]
    public void Mdcv_Record_With_Expression_Mutates_OnlyChosenField()
    {
        byte[] payload = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(16), 1000);
        Assert.True(HeifMasteringDisplayColourVolume.TryParse(payload, out var rec));
        var mutated = rec with { MinDisplayMasteringLuminance = 42u };
        Assert.Equal(42u, mutated.MinDisplayMasteringLuminance);
        Assert.Equal(rec.MaxDisplayMasteringLuminance, mutated.MaxDisplayMasteringLuminance);
        Assert.Equal(rec.DisplayPrimaryR, mutated.DisplayPrimaryR);
    }

    [Fact]
    public void Mdcv_TryParse_TrailingBytes_Ignored()
    {
        byte[] payload = new byte[28];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(16), 1234u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(20), 5u);
        payload[24] = 0xAA; payload[25] = 0xBB; payload[26] = 0xCC; payload[27] = 0xDD;
        Assert.True(HeifMasteringDisplayColourVolume.TryParse(payload, out var info));
        Assert.Equal(1234u, info.MaxDisplayMasteringLuminance);
        Assert.Equal(5u, info.MinDisplayMasteringLuminance);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(23)]
    public void Mdcv_TryParse_TooShort_Rejected(int length)
    {
        Assert.False(HeifMasteringDisplayColourVolume.TryParse(new byte[length], out _));
    }

    [Fact]
    public void Clap_TryParse_NumeratorWraps_Past_IntMaxValue_To_Negative()
    {
        // 0x80000000 as uint32 (2_147_483_648) → (int)cast is int.MinValue.
        byte[] payload = new byte[32];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(0), 0x80000000u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), 1u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), 100u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(12), 1u);
        Assert.True(HeifCleanAperture.TryParse(payload, out var info));
        Assert.Equal(int.MinValue, info.Width.Numerator);
        Assert.Equal(1u, info.Width.Denominator);
    }

    [Fact]
    public void Clap_TryParse_MaxDenominator_Preserved()
    {
        byte[] payload = new byte[32];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(12), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(20), uint.MaxValue);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(28), uint.MaxValue);
        Assert.True(HeifCleanAperture.TryParse(payload, out var info));
        Assert.Equal(uint.MaxValue, info.Width.Denominator);
        Assert.Equal(uint.MaxValue, info.Height.Denominator);
        Assert.Equal(uint.MaxValue, info.HorizontalOffset.Denominator);
        Assert.Equal(uint.MaxValue, info.VerticalOffset.Denominator);
    }

    [Fact]
    public void Clap_Record_Equality_AcrossIdenticalParses()
    {
        byte[] payload = new byte[32];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(0), 640u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), 1u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), 480u);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(12), 1u);
        Assert.True(HeifCleanAperture.TryParse(payload, out var a));
        Assert.True(HeifCleanAperture.TryParse(payload, out var b));
        Assert.Equal(a, b);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(15)]
    [InlineData(31)]
    public void Clap_TryParse_TooShort_Rejected(int length)
    {
        Assert.False(HeifCleanAperture.TryParse(new byte[length], out _));
    }

    private static byte[] BuildIspePayload(uint width, uint height)
    {
        byte[] payload = new byte[12];
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(4), width);
        BinaryPrimitives.WriteUInt32BigEndian(payload.AsSpan(8), height);
        return payload;
    }

    // ---- fixture builder ----
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
                    // Property index 1 = always ispe so that ImageInfo derives correctly.
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 64);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), 64);
                        isp.Write(data);
                    });
                    // Property index 2 = the property under test (unless it IS ispe, in which case
                    // index 1 already covers it and we skip the second slot).
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
                    entry[3] = 1; // property index 1 (ispe), not essential
                    if (assocCount == 2)
                    {
                        entry[4] = 2; // property index 2 (the test property), not essential
                    }
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
