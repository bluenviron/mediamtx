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

    [Fact]
    public void TryParse_Rejects_Truncated_NalUnit_LengthPrefix()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        // Inject numArrays=1, then a complete array header but no length prefix.
        var bytes = new byte[payload.Length + 3];
        Buffer.BlockCopy(payload, 0, bytes, 0, payload.Length);
        bytes[22] = 1; // numArrays
        bytes[^3] = 0x80 | 33; // arrayCompleteness=1, nalUnitType=33
        bytes[^2] = 0;
        bytes[^1] = 1; // 1 NAL unit declared but no length prefix follows
        Assert.False(HevcCodecConfigurationRecord.TryParse(bytes, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_NalUnit_Payload()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        var bytes = new byte[payload.Length + 5];
        Buffer.BlockCopy(payload, 0, bytes, 0, payload.Length);
        bytes[22] = 1;
        bytes[^5] = 0x80 | 33;
        bytes[^4] = 0; bytes[^3] = 1; // numNalus=1
        bytes[^2] = 0; bytes[^1] = 10; // naluLength=10, but no body
        Assert.False(HevcCodecConfigurationRecord.TryParse(bytes, out _));
    }

    [Fact]
    public void TryParse_Accepts_Array_With_Zero_NalUnits()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 33, Array.Empty<byte[]>()),
            });
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Single(rec.Arrays);
        Assert.Empty(rec.Arrays[0].NalUnits);
        Assert.True(rec.Arrays[0].ArrayCompleteness);
        Assert.Equal((byte)33, rec.Arrays[0].NalUnitType);
    }

    [Fact]
    public void TryParse_MaxProfileSpace_And_MaxNumTemporalLayers()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 3, tierFlag: true, profileIdc: 31,
            profileCompat: 0xFFFFFFFFu, constraints: 0xFFFFFFFFFFFFUL,
            levelIdc: 255, minSeg: 0xFFF, parallelism: 3,
            chromaFormatIdc: 3, bdLumaM8: 7, bdChromaM8: 7,
            avgFps: 0xFFFF, constFps: 3, numTemporalLayers: 7,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)3, rec.GeneralProfileSpace);
        Assert.True(rec.GeneralTierFlag);
        Assert.Equal((byte)31, rec.GeneralProfileIdc);
        Assert.Equal(0xFFFFFFFFu, rec.GeneralProfileCompatibilityFlags);
        Assert.Equal(0xFFFFFFFFFFFFUL, rec.GeneralConstraintIndicatorFlags);
        Assert.Equal((byte)255, rec.GeneralLevelIdc);
        Assert.Equal((ushort)0xFFF, rec.MinSpatialSegmentationIdc);
        Assert.Equal((byte)3, rec.ParallelismType);
        Assert.Equal((byte)3, rec.ChromaFormatIdc);
        Assert.Equal(15, rec.BitDepthLuma);
        Assert.Equal(15, rec.BitDepthChroma);
        Assert.Equal((byte)7, rec.NumTemporalLayers);
        Assert.Equal((ushort)0xFFFF, rec.AvgFrameRate);
        Assert.Equal((byte)3, rec.ConstantFrameRate);
    }

    [Theory]
    [InlineData((byte)0, 1)]
    [InlineData((byte)1, 2)]
    [InlineData((byte)2, 3)]
    [InlineData((byte)3, 4)]
    public void LengthSizeMinusOne_Maps_To_NalUnitLengthBytes(byte lsm1, int expected)
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: lsm1,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(lsm1, rec.LengthSizeMinusOne);
        Assert.Equal(expected, rec.NalUnitLengthBytes);
    }

    [Theory]
    [InlineData((byte)0, "4:0:0")]
    [InlineData((byte)1, "4:2:0")]
    [InlineData((byte)2, "4:2:2")]
    [InlineData((byte)3, "4:4:4")]
    public void ChromaFormat_String_Matches_ChromaFormatIdc(byte idc, string expected)
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: idc, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(expected, rec.ChromaFormat);
    }

    [Fact]
    public void ChromaFormat_String_Returns_Unknown_For_Out_Of_Range_Idc()
    {
        // Parser masks to 2 bits so cannot produce idc>3 from bytes;
        // exercise the switch's default arm by constructing the record directly.
        var rec = new HevcCodecConfigurationRecord
        {
            ConfigurationVersion = 1,
            GeneralProfileSpace = 0,
            GeneralTierFlag = false,
            GeneralProfileIdc = 1,
            GeneralProfileCompatibilityFlags = 0,
            GeneralConstraintIndicatorFlags = 0,
            GeneralLevelIdc = 120,
            MinSpatialSegmentationIdc = 0,
            ParallelismType = 0,
            ChromaFormatIdc = 4, // out of spec range
            BitDepthLumaMinus8 = 0,
            BitDepthChromaMinus8 = 0,
            AvgFrameRate = 0,
            ConstantFrameRate = 0,
            NumTemporalLayers = 1,
            TemporalIdNested = true,
            LengthSizeMinusOne = 3,
            Arrays = System.Collections.Immutable.ImmutableArray<HevcParameterSetArray>.Empty,
        };
        Assert.Equal("unknown", rec.ChromaFormat);
    }

    [Theory]
    [InlineData(22)]
    [InlineData(20)]
    [InlineData(10)]
    [InlineData(1)]
    [InlineData(0)]
    public void TryParse_Rejects_Header_Less_Than_23_Bytes(int length)
    {
        Assert.False(HevcCodecConfigurationRecord.TryParse(new byte[length], out _));
    }

    [Fact]
    public void Record_Equality_And_With_Expression()
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var a));
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { GeneralLevelIdc = 150 };
        Assert.NotEqual(a, c);
        Assert.Equal((byte)150, c.GeneralLevelIdc);
        Assert.Equal((byte)120, a.GeneralLevelIdc);
    }

    [Fact]
    public void HevcParameterSetArray_Record_Equality()
    {
        var nalu = System.Collections.Immutable.ImmutableArray.Create<byte>(0x40, 0x01);
        var a = new HevcParameterSetArray
        {
            ArrayCompleteness = true,
            NalUnitType = 32,
            NalUnits = System.Collections.Immutable.ImmutableArray.Create(nalu),
        };
        var b = a with { ArrayCompleteness = false };
        Assert.NotEqual(a, b);
        Assert.False(b.ArrayCompleteness);
        Assert.True(a.ArrayCompleteness);
    }

    [Theory]
    [InlineData((byte)0)]
    [InlineData((byte)1)]
    [InlineData((byte)2)]
    [InlineData((byte)3)]
    public void ParallelismType_RoundTrips_All_Values(byte parallelism)
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: parallelism,
            chromaFormatIdc: 1, bdLumaM8: 0, bdChromaM8: 0,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(parallelism, rec.ParallelismType);
    }

    [Theory]
    [InlineData((byte)0, (byte)0, 8, 8)]
    [InlineData((byte)2, (byte)2, 10, 10)]
    [InlineData((byte)4, (byte)4, 12, 12)]
    [InlineData((byte)6, (byte)6, 14, 14)]
    [InlineData((byte)7, (byte)0, 15, 8)] // mismatched luma/chroma
    public void BitDepth_Computed_Properties_Match_Encoded_Values(
        byte bdLuma, byte bdChroma, int expectedLuma, int expectedChroma)
    {
        var payload = BuildHvcCHeader(
            profileSpace: 0, tierFlag: false, profileIdc: 1,
            profileCompat: 0u, constraints: 0UL,
            levelIdc: 120, minSeg: 0, parallelism: 0,
            chromaFormatIdc: 1, bdLumaM8: bdLuma, bdChromaM8: bdChroma,
            avgFps: 0, constFps: 0, numTemporalLayers: 1,
            temporalIdNested: true, lengthSizeMinusOne: 3,
            arrays: []);
        Assert.True(HevcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(expectedLuma, rec.BitDepthLuma);
        Assert.Equal(expectedChroma, rec.BitDepthChroma);
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
