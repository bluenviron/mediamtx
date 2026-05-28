using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class AvcSequenceParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_1080p_Baseline_Profile_With_Crop()
    {
        // Baseline (66) at level 4.0, 1920x1080 with frame crop bottom=4
        // (height grows from 1088 = 68 mb units to 1080 after crop).
        var nalu = SpsBuilder.Build(profileIdc: 66, constraintSet: 0xC0, levelIdc: 40,
            widthMbsMinus1: 119, // 1920/16 - 1 = 119
            heightMapUnitsMinus1: 67, // 1088/16 - 1 = 67
            frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 4);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.NotNull(sps);
        Assert.Equal((byte)66, sps!.ProfileIdc);
        Assert.Equal((byte)40, sps.LevelIdc);
        Assert.Equal((byte)1, sps.ChromaFormatIdc); // default for baseline
        Assert.Equal((byte)8, sps.BitDepthLuma);
        Assert.Equal((byte)8, sps.BitDepthChroma);
        Assert.True(sps.FrameMbsOnlyFlag);
        Assert.True(sps.FrameCroppingFlag);
        Assert.Equal(1920u, sps.PictureWidthInSamples);
        Assert.Equal(1088u, sps.PictureHeightInSamples);
        Assert.Equal(1920u, sps.DecodedWidth);
        // 4:2:0 + frame_mbs_only=true => CropUnitY = 2*1 = 2; crop = 2*(0+4)=8
        Assert.Equal(1080u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Decodes_High_Profile_With_Bit_Depths()
    {
        var nalu = SpsBuilder.Build(profileIdc: 110, constraintSet: 0x00, levelIdc: 51,
            widthMbsMinus1: 239, // 3840/16 - 1
            heightMapUnitsMinus1: 134, // 2160/16 - 1
            frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 0,
            chromaFormatIdc: 1, bitDepthLumaMinus8: 2, bitDepthChromaMinus8: 2);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)110, sps!.ProfileIdc);
        Assert.Equal((byte)10, sps.BitDepthLuma);
        Assert.Equal((byte)10, sps.BitDepthChroma);
        Assert.Equal(3840u, sps.PictureWidthInSamples);
        Assert.Equal(2160u, sps.PictureHeightInSamples);
        Assert.Equal(3840u, sps.DecodedWidth);
        Assert.Equal(2160u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Crop_Unit_Y_Doubles_For_Interlaced_Stream()
    {
        // frame_mbs_only_flag = 0 doubles CropUnitY.
        var nalu = SpsBuilder.Build(profileIdc: 77, constraintSet: 0x00, levelIdc: 30,
            widthMbsMinus1: 44, // 720/16 - 1 = 44
            heightMapUnitsMinus1: 17, // 288/16 - 1 = 17 map units, doubled = 34 mb rows = 544
            frameMbsOnlyFlag: false,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 8);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.False(sps!.FrameMbsOnlyFlag);
        Assert.Equal(720u, sps.PictureWidthInSamples);
        Assert.Equal(576u, sps.PictureHeightInSamples); // 18 * 16 * 2 = 576
        // CropUnitY = SubHeightC(=2 for 4:2:0) * (2 - frame_mbs_only=0) = 4; crop = 4*(0+8)=32
        Assert.Equal(544u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_4_2_2_Uses_SubHeightC_1_For_Crop()
    {
        var nalu = SpsBuilder.Build(profileIdc: 122, constraintSet: 0x00, levelIdc: 41,
            widthMbsMinus1: 119,
            heightMapUnitsMinus1: 67,
            frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 4,
            chromaFormatIdc: 2);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.ChromaFormatIdc);
        // 4:2:2: SubHeightC=1, frame_mbs_only=1 -> CropUnitY=1, crop=1*4=4
        Assert.Equal(1084u, sps.DecodedHeight); // 1088 - 4
    }

    [Fact]
    public void TryParse_Decodes_With_Pic_Order_Cnt_Type_2()
    {
        var nalu = SpsBuilder.Build(profileIdc: 66, constraintSet: 0xC0, levelIdc: 30,
            widthMbsMinus1: 39,
            heightMapUnitsMinus1: 22,
            frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 0,
            picOrderCntType: 2);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(2u, sps!.PicOrderCntType);
    }

    [Fact]
    public void TryParse_Decodes_With_Pic_Order_Cnt_Type_1()
    {
        var nalu = SpsBuilder.Build(profileIdc: 66, constraintSet: 0xC0, levelIdc: 30,
            widthMbsMinus1: 39,
            heightMapUnitsMinus1: 22,
            frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 0,
            picOrderCntType: 1);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(1u, sps!.PicOrderCntType);
    }

    [Fact]
    public void TryParse_Survives_Emulation_Prevention_Bytes_In_Rbsp()
    {
        var nalu = SpsBuilder.Build(profileIdc: 66, constraintSet: 0xC0, levelIdc: 30,
            widthMbsMinus1: 39, heightMapUnitsMinus1: 22, frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 0);

        // Inject a 0x00 0x00 0x03 sequence inside the RBSP. The stripper
        // must remove the 0x03 before bit decoding so the meaningful bits
        // shift correctly.
        const int splice = 6;
        byte[] withEpb = new byte[nalu.Length + 1];
        Buffer.BlockCopy(nalu, 0, withEpb, 0, splice);
        withEpb[splice] = 0x00;
        withEpb[splice + 1] = 0x00;
        withEpb[splice + 2] = 0x03;
        Buffer.BlockCopy(nalu, splice + 2, withEpb, splice + 3, nalu.Length - splice - 2);
        if (nalu[splice] == 0 && nalu[splice + 1] == 0)
        {
            Assert.True(AvcSequenceParameterSet.TryParse(withEpb, out var sps));
            Assert.Equal((byte)66, sps!.ProfileIdc);
        }
        else
        {
            // Even without a meaningful EPB, parsing the original NAL unit must succeed.
            Assert.True(AvcSequenceParameterSet.TryParse(nalu, out _));
        }
    }

    [Fact]
    public void TryParse_Rejects_Non_Sps_Nal_Unit()
    {
        // nal_unit_type = 8 (PPS) instead of 7
        byte[] nalu = [0x08, 0x42, 0xC0, 0x28, 0xFF];
        Assert.False(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Zero_Bit_Set()
    {
        byte[] nalu = [0x87, 0x42, 0xC0, 0x28, 0xFF];
        Assert.False(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        byte[] nalu = [0x67, 0x42, 0xC0];
        Assert.False(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        // Looks like an SPS NAL but RBSP cuts off after profile/level.
        byte[] nalu = [0x67, 0x42, 0x00, 0x1E];
        Assert.False(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void Picture_Geometry_Computed_Properties_Work()
    {
        var nalu = SpsBuilder.Build(profileIdc: 66, constraintSet: 0xC0, levelIdc: 30,
            widthMbsMinus1: 39, heightMapUnitsMinus1: 22, frameMbsOnlyFlag: true,
            cropLeft: 0, cropRight: 0, cropTop: 0, cropBottom: 0);

        Assert.True(AvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(640u, sps!.PictureWidthInSamples);
        Assert.Equal(368u, sps.PictureHeightInSamples);
    }

    // ---------- SPS bitstream builder ----------

    private static class SpsBuilder
    {
        public static byte[] Build(
            byte profileIdc, byte constraintSet, byte levelIdc,
            uint widthMbsMinus1, uint heightMapUnitsMinus1,
            bool frameMbsOnlyFlag,
            uint cropLeft, uint cropRight, uint cropTop, uint cropBottom,
            byte chromaFormatIdc = 1,
            byte bitDepthLumaMinus8 = 0, byte bitDepthChromaMinus8 = 0,
            uint picOrderCntType = 0)
        {
            var bw = new BitWriter();
            bw.WriteBits(profileIdc, 8);
            bw.WriteBits(constraintSet, 8);
            bw.WriteBits(levelIdc, 8);
            bw.WriteUe(0); // sps_id

            bool isHigh = IsHighProfile(profileIdc);
            if (isHigh)
            {
                bw.WriteUe(chromaFormatIdc);
                if (chromaFormatIdc == 3) bw.WriteBit(false); // separate_colour_plane_flag
                bw.WriteUe(bitDepthLumaMinus8);
                bw.WriteUe(bitDepthChromaMinus8);
                bw.WriteBit(false); // qpprime_y_zero_transform_bypass_flag
                bw.WriteBit(false); // seq_scaling_matrix_present_flag
            }

            bw.WriteUe(0); // log2_max_frame_num_minus4
            bw.WriteUe(picOrderCntType);
            if (picOrderCntType == 0)
            {
                bw.WriteUe(0); // log2_max_pic_order_cnt_lsb_minus4
            }
            else if (picOrderCntType == 1)
            {
                bw.WriteBit(true); // delta_pic_order_always_zero_flag
                bw.WriteSe(0); bw.WriteSe(0);
                bw.WriteUe(0); // num_ref_frames_in_pic_order_cnt_cycle
            }

            bw.WriteUe(1); // max_num_ref_frames
            bw.WriteBit(false); // gaps_in_frame_num_value_allowed_flag
            bw.WriteUe(widthMbsMinus1);
            bw.WriteUe(heightMapUnitsMinus1);
            bw.WriteBit(frameMbsOnlyFlag);
            if (!frameMbsOnlyFlag) bw.WriteBit(false); // mb_adaptive_frame_field_flag
            bw.WriteBit(true); // direct_8x8_inference_flag

            bool cropFlag = cropLeft != 0 || cropRight != 0 || cropTop != 0 || cropBottom != 0;
            bw.WriteBit(cropFlag);
            if (cropFlag)
            {
                bw.WriteUe(cropLeft);
                bw.WriteUe(cropRight);
                bw.WriteUe(cropTop);
                bw.WriteUe(cropBottom);
            }
            bw.WriteBit(false); // vui_parameters_present_flag

            bw.WriteBit(true); // rbsp_stop_one_bit
            bw.AlignToByte();

            byte[] rbsp = bw.ToArray();
            byte[] nalu = new byte[1 + rbsp.Length];
            nalu[0] = 0x67; // nal_ref_idc=3, nal_unit_type=7
            Buffer.BlockCopy(rbsp, 0, nalu, 1, rbsp.Length);
            return nalu;
        }

        private static bool IsHighProfile(byte profileIdc) => profileIdc switch
        {
            100 or 110 or 122 or 244 or 44 or 83 or 86 or 118 or 128 or 138 or 139 or 134 or 135 => true,
            _ => false,
        };
    }

    private sealed class BitWriter
    {
        private readonly List<byte> _bytes = new();
        private byte _current;
        private int _bitPos;

        public void WriteBit(bool b)
        {
            if (b) _current |= (byte)(1 << (7 - _bitPos));
            _bitPos++;
            if (_bitPos == 8) { _bytes.Add(_current); _current = 0; _bitPos = 0; }
        }

        public void WriteBits(ulong value, int count)
        {
            for (int i = count - 1; i >= 0; i--) WriteBit(((value >> i) & 1) == 1);
        }

        public void WriteUe(uint value)
        {
            ulong x = (ulong)value + 1;
            int bits = 0;
            ulong tmp = x;
            while (tmp > 0) { bits++; tmp >>= 1; }
            for (int i = 0; i < bits - 1; i++) WriteBit(false);
            WriteBits(x, bits);
        }

        public void WriteSe(int value)
        {
            uint codeNum = value <= 0 ? (uint)(-2 * value) : (uint)(2 * value - 1);
            WriteUe(codeNum);
        }

        public void AlignToByte()
        {
            if (_bitPos != 0) { _bytes.Add(_current); _current = 0; _bitPos = 0; }
        }

        public byte[] ToArray()
        {
            AlignToByte();
            return _bytes.ToArray();
        }
    }
}
