using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class VvcCodecConfigurationRecordTests
{
    [Fact]
    public void TryParse_Decodes_PtlAbsent_Minimal()
    {
        // version=1, lengthSizeMinusOne=3, ptl_present=0, num_of_arrays=0.
        byte[] payload = new byte[]
        {
            0x01,        // version
            0xC3,        // 0b11 reserved | 0b000011 lengthSizeMinusOne=3
            0xC0,        // 0b11 reserved | 0 ptl_present | 0b00000 padding
            0x00,        // num_of_arrays=0
        };

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)1, rec.ConfigurationVersion);
        Assert.Equal((byte)3, rec.LengthSizeMinusOne);
        Assert.Equal(4, rec.NalUnitLengthBytes);
        Assert.False(rec.PtlPresentFlag);
        Assert.Null(rec.OlsIdx);
        Assert.Null(rec.NumSublayers);
        Assert.Null(rec.TrackPtl);
        Assert.Null(rec.MaxPictureWidth);
        Assert.Null(rec.ChromaFormat);
        Assert.Null(rec.BitDepth);
        Assert.Empty(rec.Arrays);
    }

    [Fact]
    public void TryParse_Decodes_PtlPresent_Main10_420_Sublayer1()
    {
        // ptl_present=1, ols_idx=0, num_sublayers=1, constant_frame_rate=0,
        // chroma_format_idc=1 (4:2:0), bit_depth_minus8=2 (10-bit).
        // PTL: num_bytes_constraint_info=1, general_profile_idc=1 (Main 10),
        // general_tier_flag=0, general_level_idc=80, frame_only=1,
        // multi_layer=0, ptl_num_sub_profiles=0.
        // max_pic_width=1920, max_pic_height=1080, avg_frame_rate=0,
        // num_of_arrays=0.
        byte[] payload = BuildPtlPresentHeader(
            olsIdx: 0,
            numSublayers: 1,
            constantFrameRate: 0,
            chromaFormatIdc: 1,
            bitDepthMinus8: 2,
            ptl: BuildPtl(
                numBytesConstraintInfo: 1,
                generalProfileIdc: 1,
                generalTierFlag: false,
                generalLevelIdc: 80,
                frameOnly: true,
                multiLayer: false,
                constraintInfoTail: Array.Empty<byte>(),
                sublayerLevelPresentFlags: Array.Empty<bool>(),
                sublayerLevelIdcs: Array.Empty<byte>(),
                subProfileIdcs: Array.Empty<uint>()),
            maxPicWidth: 1920,
            maxPicHeight: 1080,
            avgFrameRate: 0,
            arrays: Array.Empty<(bool, byte, byte[][])>());

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.True(rec.PtlPresentFlag);
        Assert.Equal((ushort)0, rec.OlsIdx);
        Assert.Equal((byte)1, rec.NumSublayers);
        Assert.Equal((byte)0, rec.ConstantFrameRate);
        Assert.Equal((byte)1, rec.ChromaFormatIdc);
        Assert.Equal("4:2:0", rec.ChromaFormat);
        Assert.Equal((byte)2, rec.BitDepthMinus8);
        Assert.Equal(10, rec.BitDepth);
        Assert.Equal((ushort)1920, rec.MaxPictureWidth);
        Assert.Equal((ushort)1080, rec.MaxPictureHeight);
        Assert.Equal((ushort)0, rec.AvgFrameRate);
        Assert.NotNull(rec.TrackPtl);
        Assert.Equal((byte)1, rec.TrackPtl!.NumBytesConstraintInfo);
        Assert.Equal((byte)1, rec.TrackPtl.GeneralProfileIdc);
        Assert.False(rec.TrackPtl.GeneralTierFlag);
        Assert.Equal((byte)80, rec.TrackPtl.GeneralLevelIdc);
        Assert.True(rec.TrackPtl.PtlFrameOnlyConstraintFlag);
        Assert.False(rec.TrackPtl.PtlMultiLayerEnabledFlag);
        Assert.Empty(rec.TrackPtl.SublayerLevelIdcs);
        Assert.Empty(rec.TrackPtl.GeneralSubProfileIdcs);
        Assert.Empty(rec.Arrays);
    }

    [Fact]
    public void TryParse_Decodes_PtlPresent_444_12bit_HighTier()
    {
        byte[] payload = BuildPtlPresentHeader(
            olsIdx: 0x123,
            numSublayers: 1,
            constantFrameRate: 1,
            chromaFormatIdc: 3,
            bitDepthMinus8: 4,
            ptl: BuildPtl(
                numBytesConstraintInfo: 1,
                generalProfileIdc: 2,
                generalTierFlag: true,
                generalLevelIdc: 100,
                frameOnly: true,
                multiLayer: false,
                constraintInfoTail: Array.Empty<byte>(),
                sublayerLevelPresentFlags: Array.Empty<bool>(),
                sublayerLevelIdcs: Array.Empty<byte>(),
                subProfileIdcs: Array.Empty<uint>()),
            maxPicWidth: 7680,
            maxPicHeight: 4320,
            avgFrameRate: 0,
            arrays: Array.Empty<(bool, byte, byte[][])>());

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((ushort)0x123, rec.OlsIdx);
        Assert.Equal((byte)1, rec.ConstantFrameRate);
        Assert.Equal("4:4:4", rec.ChromaFormat);
        Assert.Equal(12, rec.BitDepth);
        Assert.True(rec.TrackPtl!.GeneralTierFlag);
        Assert.Equal((byte)100, rec.TrackPtl.GeneralLevelIdc);
        Assert.Equal((ushort)7680, rec.MaxPictureWidth);
        Assert.Equal((ushort)4320, rec.MaxPictureHeight);
    }

    [Fact]
    public void TryParse_Decodes_MultiSublayer_With_Level_Idcs_And_SubProfiles()
    {
        // num_sublayers=3 -> 2 sublayer level present flags ([1]=true, [0]=false).
        byte[] constraintTail = [0x55];
        bool[] sublayerFlags = [false, true]; // index 0=false, index 1=true (i = numSublayers-2..0)
        byte[] sublayerIdcs = [70]; // only one set, value=70 for sublayer index 1
        uint[] subProfileIdcs = [0xAABBCCDDu, 0x11223344u];
        byte[] payload = BuildPtlPresentHeader(
            olsIdx: 0,
            numSublayers: 3,
            constantFrameRate: 0,
            chromaFormatIdc: 1,
            bitDepthMinus8: 0,
            ptl: BuildPtl(
                numBytesConstraintInfo: 2,
                generalProfileIdc: 1,
                generalTierFlag: false,
                generalLevelIdc: 80,
                frameOnly: true,
                multiLayer: false,
                constraintInfoTail: constraintTail,
                sublayerLevelPresentFlags: sublayerFlags,
                sublayerLevelIdcs: sublayerIdcs,
                subProfileIdcs: subProfileIdcs),
            maxPicWidth: 1920,
            maxPicHeight: 1080,
            avgFrameRate: 0,
            arrays: Array.Empty<(bool, byte, byte[][])>());

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal((byte)3, rec.NumSublayers);
        Assert.Equal((byte)2, rec.TrackPtl!.NumBytesConstraintInfo);
        Assert.Single(rec.TrackPtl.SublayerLevelIdcs);
        Assert.Equal((byte)70, rec.TrackPtl.SublayerLevelIdcs[0]);
        Assert.Equal(2, rec.TrackPtl.GeneralSubProfileIdcs.Length);
        Assert.Equal(0xAABBCCDDu, rec.TrackPtl.GeneralSubProfileIdcs[0]);
        Assert.Equal(0x11223344u, rec.TrackPtl.GeneralSubProfileIdcs[1]);
        // Constraint info: first byte after masking out top 2 flag bits + tail.
        Assert.Equal(2, rec.TrackPtl.GeneralConstraintInfo.Length);
        Assert.Equal((byte)0x55, rec.TrackPtl.GeneralConstraintInfo[1]);
    }

    [Fact]
    public void TryParse_Decodes_PtlAbsent_With_Sps_Array()
    {
        var sps = new byte[] { 0x42, 0x01, 0xAA };
        byte[] payload = BuildPtlAbsentWithArrays(
            lengthSizeMinusOne: 3,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 15, new[] { sps }),
            });

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.False(rec.PtlPresentFlag);
        Assert.Single(rec.Arrays);
        Assert.True(rec.Arrays[0].ArrayCompleteness);
        Assert.Equal((byte)15, rec.Arrays[0].NalUnitType);
        Assert.Single(rec.Arrays[0].NalUnits);
        Assert.Equal(sps, rec.Arrays[0].NalUnits[0].ToArray());
    }

    [Fact]
    public void TryParse_Decodes_OPI_DCI_Implicit_Single_Nalu()
    {
        // OPI (12) and DCI (13) omit the 16-bit num_nalus field.
        var opi = new byte[] { 0x18, 0x01, 0xCC };
        var dci = new byte[] { 0x1A, 0x02, 0xDD };
        byte[] payload = BuildPtlAbsentWithArrays(
            lengthSizeMinusOne: 3,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 12, new[] { opi }),
                (true, 13, new[] { dci }),
            });

        Assert.True(VvcCodecConfigurationRecord.TryParse(payload, out var rec));
        Assert.Equal(2, rec.Arrays.Length);
        Assert.Equal((byte)12, rec.Arrays[0].NalUnitType);
        Assert.Single(rec.Arrays[0].NalUnits);
        Assert.Equal(opi, rec.Arrays[0].NalUnits[0].ToArray());
        Assert.Equal((byte)13, rec.Arrays[1].NalUnitType);
        Assert.Equal(dci, rec.Arrays[1].NalUnits[0].ToArray());
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        byte[] payload = new byte[] { 99, 0xC3, 0xC0, 0x00 };
        Assert.False(VvcCodecConfigurationRecord.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(VvcCodecConfigurationRecord.TryParse(new byte[2], out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_When_PtlPresent_Missing_PicSize()
    {
        // ptl_present=1 but payload ends right after PTL.
        byte[] payload = BuildPtlPresentHeader(
            olsIdx: 0, numSublayers: 1, constantFrameRate: 0,
            chromaFormatIdc: 1, bitDepthMinus8: 0,
            ptl: BuildPtl(1, 1, false, 80, true, false,
                Array.Empty<byte>(), Array.Empty<bool>(),
                Array.Empty<byte>(), Array.Empty<uint>()),
            maxPicWidth: 1920, maxPicHeight: 1080, avgFrameRate: 0,
            arrays: Array.Empty<(bool, byte, byte[][])>());
        // Truncate before max_pic_width.
        // PTL ends at byte index 6 + 3 (ptl hdr) + 1 (constraintInfo) + 1 (numSubProfiles) = 11.
        byte[] truncated = payload[..11];
        Assert.False(VvcCodecConfigurationRecord.TryParse(truncated, out _));
    }

    [Fact]
    public void HeifReader_Resolves_VvcC_Via_Ipma()
    {
        var sps = new byte[] { 0x42, 0x01, 0xAA };
        var payload = BuildPtlPresentHeader(
            olsIdx: 0, numSublayers: 1, constantFrameRate: 0,
            chromaFormatIdc: 1, bitDepthMinus8: 2,
            ptl: BuildPtl(1, 1, false, 80, true, false,
                Array.Empty<byte>(), Array.Empty<bool>(),
                Array.Empty<byte>(), Array.Empty<uint>()),
            maxPicWidth: 1920, maxPicHeight: 1080, avgFrameRate: 0,
            arrays: new (bool, byte, byte[][])[]
            {
                (true, 15, new[] { sps }),
            });
        var bytes = BuildHeifWithProperty("vvcC", payload);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetVvcCodecConfiguration(1, out var rec));
        Assert.True(rec.PtlPresentFlag);
        Assert.Equal((byte)1, rec.TrackPtl!.GeneralProfileIdc);
        Assert.Equal((ushort)1920, rec.MaxPictureWidth);
        Assert.Equal("4:2:0", rec.ChromaFormat);
        Assert.Equal(10, rec.BitDepth);
        Assert.Single(rec.Arrays);
        Assert.Equal((byte)15, rec.Arrays[0].NalUnitType);

        Assert.False(r.TryGetVvcCodecConfiguration(99, out _));
    }

    [Fact]
    public void HeifReader_Rejects_Missing_VvcC()
    {
        var bytes = BuildHeifWithProperty("ispe", BuildIspePayload(64, 64));
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetVvcCodecConfiguration(1, out _));
    }

    private static byte[] BuildPtl(
        byte numBytesConstraintInfo,
        byte generalProfileIdc,
        bool generalTierFlag,
        byte generalLevelIdc,
        bool frameOnly,
        bool multiLayer,
        byte[] constraintInfoTail,
        bool[] sublayerLevelPresentFlags, // index i corresponds to sublayer i (low-to-high)
        byte[] sublayerLevelIdcs,
        uint[] subProfileIdcs)
    {
        using var ms = new MemoryStream();
        ms.WriteByte((byte)(numBytesConstraintInfo & 0x3F));
        ms.WriteByte((byte)(((generalProfileIdc & 0x7F) << 1) | (generalTierFlag ? 1 : 0)));
        ms.WriteByte(generalLevelIdc);

        // Constraint info: top 2 bits of first byte = frame_only + multi_layer.
        byte ci0 = (byte)(((frameOnly ? 1 : 0) << 7) | ((multiLayer ? 1 : 0) << 6));
        ms.WriteByte(ci0);
        if (constraintInfoTail.Length != numBytesConstraintInfo - 1)
            throw new ArgumentException("constraintInfoTail length must equal numBytesConstraintInfo - 1");
        ms.Write(constraintInfoTail);

        // Sublayer level present flags (only if num_sublayers > 1).
        int numSublayers = sublayerLevelPresentFlags.Length + 1;
        if (numSublayers > 1)
        {
            byte sb = 0;
            // flags packed from MSB: flag[num_sublayers-2], flag[num_sublayers-3], ..., flag[0]
            for (int i = numSublayers - 2; i >= 0; i--)
            {
                int bitFromTop = (numSublayers - 2 - i);
                if (sublayerLevelPresentFlags[i])
                    sb |= (byte)(1 << (7 - bitFromTop));
            }
            ms.WriteByte(sb);
        }

        // Sublayer level idcs (one byte per set flag, in same order).
        ms.Write(sublayerLevelIdcs);

        // ptl_num_sub_profiles
        ms.WriteByte((byte)subProfileIdcs.Length);
        Span<byte> u32 = stackalloc byte[4];
        foreach (var idc in subProfileIdcs)
        {
            BinaryPrimitives.WriteUInt32BigEndian(u32, idc);
            ms.Write(u32);
        }
        return ms.ToArray();
    }

    private static byte[] BuildPtlPresentHeader(
        ushort olsIdx, byte numSublayers, byte constantFrameRate,
        byte chromaFormatIdc, byte bitDepthMinus8,
        byte[] ptl,
        ushort maxPicWidth, ushort maxPicHeight, ushort avgFrameRate,
        (bool ArrayCompleteness, byte NalUnitType, byte[][] NalUnits)[] arrays)
    {
        using var ms = new MemoryStream();
        ms.WriteByte(0x01);              // version
        ms.WriteByte((byte)(0xC0 | 3));  // reserved+lengthSizeMinusOne=3

        // byte 2: 0b11 reserved | 1 ptl_present | top 5 bits of ols_idx
        byte b2 = (byte)(0xC0 | 0x20 | ((olsIdx >> 8) & 0x1F));
        ms.WriteByte(b2);
        ms.WriteByte((byte)(olsIdx & 0xFF));

        // byte 4: num_sublayers(3) + constant_frame_rate(2) + chroma_format_idc(3)
        byte b4 = (byte)(((numSublayers & 0x7) << 5) | ((constantFrameRate & 0x3) << 3) | (chromaFormatIdc & 0x7));
        ms.WriteByte(b4);

        // byte 5: bit_depth_minus8(3) + reserved(5)
        byte b5 = (byte)(((bitDepthMinus8 & 0x7) << 5) | 0x1F);
        ms.WriteByte(b5);

        ms.Write(ptl);

        Span<byte> u16 = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(u16, maxPicWidth); ms.Write(u16);
        BinaryPrimitives.WriteUInt16BigEndian(u16, maxPicHeight); ms.Write(u16);
        BinaryPrimitives.WriteUInt16BigEndian(u16, avgFrameRate); ms.Write(u16);

        WriteArrays(ms, arrays);
        return ms.ToArray();
    }

    private static byte[] BuildPtlAbsentWithArrays(
        byte lengthSizeMinusOne,
        (bool ArrayCompleteness, byte NalUnitType, byte[][] NalUnits)[] arrays)
    {
        using var ms = new MemoryStream();
        ms.WriteByte(0x01);
        ms.WriteByte((byte)(0xC0 | (lengthSizeMinusOne & 0x3F)));
        ms.WriteByte(0xC0); // ptl_present=0
        WriteArrays(ms, arrays);
        return ms.ToArray();
    }

    private static void WriteArrays(
        MemoryStream ms,
        (bool ArrayCompleteness, byte NalUnitType, byte[][] NalUnits)[] arrays)
    {
        ms.WriteByte((byte)arrays.Length);
        Span<byte> u16 = stackalloc byte[2];
        foreach (var (arrayCompleteness, nalUnitType, nalUnits) in arrays)
        {
            byte aByte = (byte)(((arrayCompleteness ? 1 : 0) << 7) | (nalUnitType & 0x1F));
            ms.WriteByte(aByte);
            if (nalUnitType != 12 && nalUnitType != 13)
            {
                BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)nalUnits.Length);
                ms.Write(u16);
            }
            foreach (var nalu in nalUnits)
            {
                BinaryPrimitives.WriteUInt16BigEndian(u16, (ushort)nalu.Length);
                ms.Write(u16);
                ms.Write(nalu);
            }
        }
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
            w.Write("vvic"u8);
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
                    Encoding.ASCII.GetBytes("vvc1").CopyTo(data.Slice(8));
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
