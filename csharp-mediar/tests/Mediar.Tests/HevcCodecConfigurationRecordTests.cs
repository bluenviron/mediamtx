using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HevcCodecConfigurationRecordTests
{
    [Fact]
    public void TryParse_Decodes_Main_Profile_8bit_420_Level4()
    {
        // configurationVersion=1, profile_space=0, tier=0, profile_idc=1 (Main),
        // profile_compat=0x60000000 (Main+Main10),
        // constraints=0, level_idc=120 (level 4.0),
        // min_seg=0, parallelism=0, chroma_format_idc=1 (4:2:0),
        // bd_luma_minus8=0, bd_chroma_minus8=0, avgFps=0,
        // const_fps=0, num_temp_layers=1, temp_id_nested=1, length_size_minus_one=3,
        // numArrays=0.
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0x60000000u,
            constraints: 0UL,
            levelIdc: 120,
            minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)1, rec.ConfigurationVersion);
        Assert.Equal((byte)0, rec.GeneralProfileSpace);
        Assert.False(rec.GeneralTierFlag);
        Assert.Equal((byte)1, rec.GeneralProfileIdc);
        Assert.Equal(0x60000000u, rec.GeneralProfileCompatibilityFlags);
        Assert.Equal(0UL, rec.GeneralConstraintIndicatorFlags);
        Assert.Equal((byte)120, rec.GeneralLevelIdc);
        Assert.Equal((ushort)0, rec.MinSpatialSegmentationIdc);
        Assert.Equal((byte)0, rec.ParallelismType);
        Assert.Equal((byte)1, rec.ChromaFormatIdc);
        Assert.Equal("4:2:0", rec.ChromaFormat);
        Assert.Equal(8, rec.BitDepthLuma);
        Assert.Equal(8, rec.BitDepthChroma);
        Assert.Equal((byte)1, rec.NumTemporalLayers);
        Assert.True(rec.TemporalIdNested);
        Assert.Equal((byte)3, rec.LengthSizeMinusOne);
        Assert.Equal(4, rec.NalUnitLengthBytes);
        Assert.Empty(rec.Arrays);
    }

    [Fact]
    public void TryParse_Decodes_Main10_HighTier_444_10bit()
    {
        // Main10 profile_idc=2, tier=1, 4:4:4, 10-bit, level 5.0 = 150
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: true, profileIdc: 2,
            profileCompat: 0x40000000u,
            constraints: 0UL,
            levelIdc: 150,
            minSeg: 0, parallelism: 0,
            chromaFormatIdc: 3, bdLumaM8: 2, bdChromaM8: 2,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.True(rec.GeneralTierFlag);
        Assert.Equal((byte)2, rec.GeneralProfileIdc);
        Assert.Equal((byte)150, rec.GeneralLevelIdc);
        Assert.Equal((byte)3, rec.ChromaFormatIdc);
        Assert.Equal("4:4:4", rec.ChromaFormat);
        Assert.Equal(10, rec.BitDepthLuma);
        Assert.Equal(10, rec.BitDepthChroma);
    }

    [Fact]
    public void TryParse_Decodes_Monochrome_12bit()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 4,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120,
            minSeg: 0, parallelism: 0,
            chromaFormatIdc: 0, bdLumaM8: 4, bdChromaM8: 4,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)0, rec.ChromaFormatIdc);
        Assert.Equal("4:0:0", rec.ChromaFormat);
        Assert.Equal(12, rec.BitDepthLuma);
        Assert.Equal(12, rec.BitDepthChroma);
    }

    [Fact]
    public void TryParse_Decodes_422_Chroma_Format()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 2, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal("4:2:2", rec.ChromaFormat);
    }

    [Fact]
    public void TryParse_Decodes_Constraint_Flags_And_MinSeg()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0x60000000u,
            constraints: 0x900000000000UL,
            levelIdc: 120,
            minSeg: 0x123, parallelism: 2,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0x1234, constFps: 1, numTemporalLayers: 3,
            temporalIdNested: false, lengthSizeMinusOne: 1,
            arrays: []);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(0x900000000000UL, rec.GeneralConstraintIndicatorFlags);
        Assert.Equal((ushort)0x123, rec.MinSpatialSegmentationIdc);
        Assert.Equal((byte)2, rec.ParallelismType);
        Assert.Equal((ushort)0x1234, rec.AvgFrameRate);
        Assert.Equal((byte)1, rec.ConstantFrameRate);
        Assert.Equal((byte)3, rec.NumTemporalLayers);
        Assert.False(rec.TemporalIdNested);
        Assert.Equal((byte)1, rec.LengthSizeMinusOne);
        Assert.Equal(2, rec.NalUnitLengthBytes);
    }

    [Fact]
    public void TryParse_Decodes_Multiple_Parameter_Set_Arrays()
    {
        // 3 arrays: VPS (NAL=32, complete), SPS (33), PPS (34).
        var vpsBytes = new byte[] { 0x40, 0x01, 0x0C, 0x01 };
        var spsBytes = new byte[] { 0x42, 0x01, 0x01, 0x02, 0x03 };
        var ppsBytes = new byte[] { 0x44, 0x01, 0xC1 };

        var arrays = new (bool, byte, byte[][])[]
        {
            (true,  32, new[] { vpsBytes }),
            (true,  33, new[] { spsBytes }),
            (false, 34, new[] { ppsBytes }),
        };
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0x60000000u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: arrays);

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(3, rec.Arrays.Length);

        Assert.True(rec.Arrays[0].ArrayCompleteness);
        Assert.Equal((byte)32, rec.Arrays[0].NalUnitType);
        Assert.Single(rec.Arrays[0].NalUnits);
        Assert.Equal(vpsBytes, rec.Arrays[0].NalUnits[0].ToArray());

        Assert.True(rec.Arrays[1].ArrayCompleteness);
        Assert.Equal((byte)33, rec.Arrays[1].NalUnitType);
        Assert.Equal(spsBytes, rec.Arrays[1].NalUnits[0].ToArray());

        Assert.False(rec.Arrays[2].ArrayCompleteness);
        Assert.Equal((byte)34, rec.Arrays[2].NalUnitType);
        Assert.Equal(ppsBytes, rec.Arrays[2].NalUnits[0].ToArray());
    }

    [Fact]
    public void TryParse_Decodes_Array_With_Multiple_NalUnits()
    {
        var sps1 = new byte[] { 0x42, 0x01, 0xAA };
        var sps2 = new byte[] { 0x42, 0x01, 0xBB, 0xCC };

        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 33, new[] { sps1, sps2 }),
            });

        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Single(rec.Arrays);
        Assert.Equal(2, rec.Arrays[0].NalUnits.Length);
        Assert.Equal(sps1, rec.Arrays[0].NalUnits[0].ToArray());
        Assert.Equal(sps2, rec.Arrays[0].NalUnits[1].ToArray());
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        payload[0] = 99; // bogus version
        Assert.False(HevcCodecConfigurationRecord.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(HevcCodecConfigurationRecord.TryParse(new byte[22], out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Array_Header()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        // Force numArrays=1 with no payload.
        payload[22] = 1;
        Assert.False(HevcCodecConfigurationRecord.TryParse(payload, out _));
    }

    [Fact]
    public void HeifReader_Resolves_HvcC_Via_Ipma()
    {
        var hvcc = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0x60000000u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 33, new[] { new byte[] { 0x42, 0x01, 0xAA } }),
            });

        var bytes = BuildHeifWithProperty("hvcC", hvcc);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetHevcCodecConfiguration(1, out var rec));
        Assert.Equal((byte)1, rec.GeneralProfileIdc);
        Assert.Equal((byte)120, rec.GeneralLevelIdc);
        Assert.Equal("4:2:0", rec.ChromaFormat);
        Assert.Equal(8, rec.BitDepthLuma);
        Assert.Single(rec.Arrays);
        Assert.Equal((byte)33, rec.Arrays[0].NalUnitType);

        Assert.False(r.TryGetHevcCodecConfiguration(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_HvcC()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.False(r.TryGetHevcCodecConfiguration(1, out _));
    }

    private static byte[] BuildHvcCHeader(
        byte profileSpace, bool tierFlag, byte profileIdc,
        uint profileCompat, ulong constraints, byte levelIdc,
        ushort minSeg, byte parallelism, byte chromaFormatIdc,
        byte bdLumaM8, byte bdChromaM8, ushort avgFps,
        byte constFps, byte numTemporalLayers, bool temporalIdNested,
        byte lengthSizeMinusOne,
        (bool ArrayCompleteness, byte NalUnitType, byte[][] NalUnits)[] arrays)
    {
        using var ms = new MemoryStream();
        ms.WriteByte(0x01); // configurationVersion
        byte b1 = (byte)(((profileSpace & 0x3) << 6) | ((tierFlag ? 1 : 0) << 5) | (profileIdc & 0x1F));
        ms.WriteByte(b1);
        Span<byte> u32 = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(u32, profileCompat);
        ms.Write(u32);
        Span<byte> u48 = stackalloc byte[6];
        for (int i = 0; i < 6; i++)
            u48[i] = (byte)((constraints >> (8 * (5 - i))) & 0xFF);
        ms.Write(u48);
        ms.WriteByte(levelIdc);
        // 4 reserved bits (1111) + 12-bit min_seg.
        ushort minSegPacked = (ushort)((0xF << 12) | (minSeg & 0x0FFF));
        Span<byte> u16 = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(u16, minSegPacked);
        ms.Write(u16);
        ms.WriteByte((byte)(0xFC | (parallelism & 0x3))); // 6 reserved + parallelism
        ms.WriteByte((byte)(0xFC | (chromaFormatIdc & 0x3)));
        ms.WriteByte((byte)(0xF8 | (bdLumaM8 & 0x7)));
        ms.WriteByte((byte)(0xF8 | (bdChromaM8 & 0x7)));
        BinaryPrimitives.WriteUInt16BigEndian(u16, avgFps);
        ms.Write(u16);
        byte b21 = (byte)(((constFps & 0x3) << 6) | ((numTemporalLayers & 0x7) << 3) | ((temporalIdNested ? 1 : 0) << 2) | (lengthSizeMinusOne & 0x3));
        ms.WriteByte(b21);
        ms.WriteByte((byte)arrays.Length);

        foreach (var (arrayCompleteness, nalUnitType, nalUnits) in arrays)
        {
            byte aByte = (byte)(((arrayCompleteness ? 1 : 0) << 7) | (nalUnitType & 0x3F));
            ms.WriteByte(aByte);
            BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)nalUnits.Length);
            ms.Write(u16);
            foreach (var nalu in nalUnits)
            {
                BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)nalu.Length);
                ms.Write(u16);
                ms.Write(nalu);
            }
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
