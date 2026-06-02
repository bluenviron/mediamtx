using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HevcSequenceParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_1080p_Main_Profile()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.NotNull(sps);
        Assert.Equal((byte)1, sps!.GeneralProfileIdc);
        Assert.False(sps.GeneralTierFlag);
        Assert.Equal((byte)120, sps.GeneralLevelIdc);
        Assert.Equal((byte)1, sps.ChromaFormatIdc);
        Assert.Equal(1920u, sps.PictureWidthInLumaSamples);
        Assert.Equal(1080u, sps.PictureHeightInLumaSamples);
        Assert.Equal((byte)8, sps.BitDepthLuma);
        Assert.Equal((byte)8, sps.BitDepthChroma);
        Assert.False(sps.ConformanceWindowFlag);
        Assert.Equal(1920u, sps.DecodedWidth);
        Assert.Equal(1080u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Decodes_4K_Main10()
    {
        var nalu = SpsBuilder.Build(profileIdc: 2, tierFlag: true, levelIdc: 153,
            chromaFormat: 1, width: 3840, height: 2160,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 2, bitDepthChromaMinus8: 2,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.GeneralProfileIdc);
        Assert.True(sps.GeneralTierFlag);
        Assert.Equal((byte)153, sps.GeneralLevelIdc);
        Assert.Equal(3840u, sps.PictureWidthInLumaSamples);
        Assert.Equal(2160u, sps.PictureHeightInLumaSamples);
        Assert.Equal((byte)10, sps.BitDepthLuma);
        Assert.Equal((byte)10, sps.BitDepthChroma);
    }

    [Fact]
    public void TryParse_Applies_Conformance_Window_Cropping_For_4_2_0()
    {
        // 4:2:0 has SubWidthC=2, SubHeightC=2 so a left+right offset of 4+4
        // crops 16 luma samples horizontally; top+bottom 0+5 crops 10 vertically.
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1088,
            confLeft: 4, confRight: 4, confTop: 0, confBottom: 5,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.ConformanceWindowFlag);
        Assert.Equal(1904u, sps.DecodedWidth);  // 1920 - 2*(4+4) = 1904
        Assert.Equal(1078u, sps.DecodedHeight); // 1088 - 2*(0+5) = 1078
    }

    [Fact]
    public void TryParse_4_2_2_Uses_SubHeight_1_For_Crop()
    {
        // 4:2:2 has SubWidthC=2, SubHeightC=1.
        var nalu = SpsBuilder.Build(profileIdc: 4, tierFlag: false, levelIdc: 120,
            chromaFormat: 2, width: 1920, height: 1080,
            confLeft: 0, confRight: 2, confTop: 0, confBottom: 4,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.ChromaFormatIdc);
        Assert.Equal(1916u, sps.DecodedWidth);  // 1920 - 2*(0+2) = 1916
        Assert.Equal(1076u, sps.DecodedHeight); // 1080 - 1*(0+4) = 1076
    }

    [Fact]
    public void TryParse_4_4_4_Uses_SubWidth_1_For_Crop()
    {
        var nalu = SpsBuilder.Build(profileIdc: 4, tierFlag: false, levelIdc: 120,
            chromaFormat: 3, width: 1920, height: 1080,
            confLeft: 1, confRight: 1, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)3, sps!.ChromaFormatIdc);
        Assert.False(sps.SeparateColourPlaneFlag);
        Assert.Equal(1918u, sps.DecodedWidth);  // 1920 - 1*(1+1) = 1918
        Assert.Equal(1080u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Survives_Emulation_Prevention_Bytes()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        // Inject a 0x00 0x00 0x03 sequence in the middle of the RBSP that would
        // otherwise contain a 0x00 0x00 bit pattern. The stripper must remove
        // the 0x03 before decoding, leaving the bit positions unchanged.
        byte[] withEpb = new byte[nalu.Length + 1];
        // Copy NAL hdr (2 bytes) + a few RBSP bytes so the EPB lands deep enough
        // not to disturb header bits.
        const int splice = 10;
        Buffer.BlockCopy(nalu, 0, withEpb, 0, splice);
        withEpb[splice] = 0x00;
        withEpb[splice + 1] = 0x00;
        withEpb[splice + 2] = 0x03;
        Buffer.BlockCopy(nalu, splice + 2, withEpb, splice + 3, nalu.Length - splice - 2);
        // Only meaningful if the spliced original bytes were 0x00 0x00; if not,
        // we still expect successful parsing because the EPB stripper looks
        // only at the byte pattern 00 00 03.
        if (nalu[splice] == 0 && nalu[splice + 1] == 0)
        {
            Assert.True(HevcSequenceParameterSet.TryParse(withEpb, out var sps));
            Assert.Equal(1920u, sps!.PictureWidthInLumaSamples);
        }

        // Always validate the standalone EPB stripper function directly.
        ReadOnlySpan<byte> sample = stackalloc byte[] { 0x00, 0x00, 0x03, 0xAA, 0x00, 0x00, 0x03, 0xBB };
        byte[] stripped = HevcSequenceParameterSet.StripEmulationPreventionBytes(sample);
        byte[] expected = [0x00, 0x00, 0xAA, 0x00, 0x00, 0xBB];
        Assert.Equal(expected, stripped);
    }

    [Fact]
    public void TryParse_Rejects_Non_Sps_Nal_Unit()
    {
        // nal_unit_type = 32 (VPS_NUT) instead of 33
        byte[] nalu = [(byte)((32 << 1) & 0x7E), 0x01, 0xFF, 0xFF];
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Zero_Bit_Set()
    {
        byte[] nalu = [0x80 | ((33 << 1) & 0x7E), 0x01, 0xFF, 0xFF];
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        byte[] nalu = [(byte)((33 << 1) & 0x7E), 0x01];
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        // Valid header, valid first byte (vps_id=0, max_sub_layers=0, temp_id_nesting=1)
        // but cuts off in the middle of profile_tier_level.
        byte[] nalu = [(byte)((33 << 1) & 0x7E), 0x01, 0x01, 0x00, 0x00];
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Decodes_With_Sub_Layers()
    {
        // maxSubLayersMinus1=2 stresses the sub-layer flag and skip loop.
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1280, height: 720,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 2);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.MaxSubLayersMinus1);
        Assert.Equal(1280u, sps.PictureWidthInLumaSamples);
        Assert.Equal(720u, sps.PictureHeightInLumaSamples);
    }

    [Fact]
    public void TryParse_Rejects_ChromaFormat_Above_3()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 4, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_BitDepth_Above_8()
    {
        // bit_depth_luma_minus8 > 8 means > 16 bpc → rejected.
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 9, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Chroma_BitDepth_Above_8()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 9,
            maxSubLayersMinus1: 0);
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Zero_Picture_Width()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 0, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Zero_Picture_Height()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 0,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Monochrome_ChromaFormatIdc_Zero_HasUnitySubSampling()
    {
        // Monochrome (4:0:0) → SubWidthC=SubHeightC=1, no crop reduction.
        var nalu = SpsBuilder.Build(profileIdc: 4, tierFlag: false, levelIdc: 120,
            chromaFormat: 0, width: 640, height: 480,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)0, sps!.ChromaFormatIdc);
        Assert.Equal(640u, sps.DecodedWidth);
        Assert.Equal(480u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_TooShort_3Bytes_Rejected()
    {
        byte[] nalu = [(byte)((33 << 1) & 0x7E), 0x01, 0xFF];
        Assert.False(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Empty_Nalu_Rejected()
    {
        Assert.False(HevcSequenceParameterSet.TryParse(ReadOnlySpan<byte>.Empty, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void DecodedWidth_Returns_Zero_When_Crop_Exceeds_Picture_Width()
    {
        // 4:2:0 has SubWidthC=2, so crop=2*(100+100)=400 > picture width 200.
        // SpsBuilder uses ue(v) which can encode large offsets without issue,
        // but practical safer test: build directly with crop offsets equal to
        // half the picture width plus one.
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 30,
            chromaFormat: 1, width: 200, height: 200,
            confLeft: 51, confRight: 50, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        // 2*(51+50)=202 > 200 → DecodedWidth clamps to 0.
        Assert.Equal(0u, sps!.DecodedWidth);
    }

    [Fact]
    public void DecodedHeight_Returns_Zero_When_Crop_Exceeds_Picture_Height()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 30,
            chromaFormat: 1, width: 200, height: 200,
            confLeft: 0, confRight: 0, confTop: 50, confBottom: 51,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(0u, sps!.DecodedHeight);
    }

    [Fact]
    public void Record_Equality_Holds_For_Two_Identical_Parses()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var a));
        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a!.GetHashCode(), b!.GetHashCode());
    }

    [Fact]
    public void Record_With_Expression_Mutates_PictureWidth_Only()
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1920, height: 1080,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: 0, bitDepthChromaMinus8: 0,
            maxSubLayersMinus1: 0);
        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        var mutated = sps! with { PictureWidthInLumaSamples = 1280 };
        Assert.Equal(1280u, mutated.PictureWidthInLumaSamples);
        Assert.Equal(1080u, mutated.PictureHeightInLumaSamples);
        Assert.Equal(1920u, sps.PictureWidthInLumaSamples);
    }

    [Fact]
    public void StripEmulationPreventionBytes_Empty_Returns_Empty()
    {
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(ReadOnlySpan<byte>.Empty);
        Assert.Empty(result);
    }

    [Fact]
    public void StripEmulationPreventionBytes_NoEpbs_Preserves_All_Bytes()
    {
        ReadOnlySpan<byte> src = stackalloc byte[] { 0x01, 0x02, 0x03, 0xFF, 0x80 };
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(src);
        Assert.Equal((byte[])[0x01, 0x02, 0x03, 0xFF, 0x80], result);
    }

    [Fact]
    public void StripEmulationPreventionBytes_Removes_Multiple_Epbs()
    {
        // Two EPB sequences: each "00 00 03 xx" → "00 00 xx".
        ReadOnlySpan<byte> src = stackalloc byte[]
        {
            0x00, 0x00, 0x03, 0xAA,
            0xFF,
            0x00, 0x00, 0x03, 0xBB,
        };
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(src);
        Assert.Equal((byte[])[0x00, 0x00, 0xAA, 0xFF, 0x00, 0x00, 0xBB], result);
    }

    [Fact]
    public void StripEmulationPreventionBytes_DoesNot_Strip_Without_Two_Leading_Zeros()
    {
        // Only the 0x03 after 0x00 0x00 is an EPB; 0x01 0x00 0x03 is left intact.
        ReadOnlySpan<byte> src = stackalloc byte[] { 0x01, 0x00, 0x03, 0xAA, 0xBB };
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(src);
        Assert.Equal((byte[])[0x01, 0x00, 0x03, 0xAA, 0xBB], result);
    }

    [Fact]
    public void StripEmulationPreventionBytes_Trailing_00_00_03_At_End_Is_Stripped()
    {
        // i+2 < src.Length is true for i=0 in a 3-byte buffer, so the trailing
        // 0x03 still qualifies as an emulation prevention byte and is removed.
        ReadOnlySpan<byte> src = stackalloc byte[] { 0x00, 0x00, 0x03 };
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(src);
        Assert.Equal((byte[])[0x00, 0x00], result);
    }

    [Fact]
    public void StripEmulationPreventionBytes_Trailing_00_00_Without_03_Not_Touched()
    {
        // Two-byte tail of zeros with nothing after — must remain intact.
        ReadOnlySpan<byte> src = stackalloc byte[] { 0xAA, 0x00, 0x00 };
        var result = HevcSequenceParameterSet.StripEmulationPreventionBytes(src);
        Assert.Equal((byte[])[0xAA, 0x00, 0x00], result);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(4)]
    [InlineData(6)]
    [InlineData(8)]
    public void TryParse_Accepts_All_Legal_BitDepth_Minus8_Values(int minus8)
    {
        var nalu = SpsBuilder.Build(profileIdc: 1, tierFlag: false, levelIdc: 120,
            chromaFormat: 1, width: 1280, height: 720,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 0,
            bitDepthLumaMinus8: (byte)minus8, bitDepthChromaMinus8: (byte)minus8,
            maxSubLayersMinus1: 0);

        Assert.True(HevcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)(minus8 + 8), sps!.BitDepthLuma);
        Assert.Equal((byte)(minus8 + 8), sps.BitDepthChroma);
    }

    // ---------- SPS bitstream builder ----------

    private static class SpsBuilder
    {
        public static byte[] Build(
            byte profileIdc, bool tierFlag, byte levelIdc,
            byte chromaFormat, uint width, uint height,
            uint confLeft, uint confRight, uint confTop, uint confBottom,
            byte bitDepthLumaMinus8, byte bitDepthChromaMinus8,
            byte maxSubLayersMinus1)
        {
            var bw = new BitWriter();
            // sps_video_parameter_set_id u(4) = 0
            bw.WriteBits(0, 4);
            // sps_max_sub_layers_minus1 u(3)
            bw.WriteBits(maxSubLayersMinus1, 3);
            // sps_temporal_id_nesting_flag u(1) = 1
            bw.WriteBit(true);

            // profile_tier_level fixed part (profilePresentFlag=1).
            bw.WriteBits(0, 2); // general_profile_space
            bw.WriteBit(tierFlag);
            bw.WriteBits(profileIdc, 5);
            bw.WriteBits(0, 32); // general_profile_compatibility_flags
            bw.WriteBits(0, 48); // general source + constraint flags
            bw.WriteBits(levelIdc, 8);

            // Sub-layer presence flags + reserved zeros.
            for (int i = 0; i < maxSubLayersMinus1; i++)
            {
                bw.WriteBit(false); // sub_layer_profile_present_flag = 0
                bw.WriteBit(false); // sub_layer_level_present_flag = 0
            }
            if (maxSubLayersMinus1 > 0)
            {
                for (int i = maxSubLayersMinus1; i < 8; i++) bw.WriteBits(0, 2);
            }

            // sps_seq_parameter_set_id ue(v) = 0
            bw.WriteUe(0);
            // chroma_format_idc ue(v)
            bw.WriteUe(chromaFormat);
            if (chromaFormat == 3) bw.WriteBit(false); // separate_colour_plane_flag = 0

            bw.WriteUe(width);
            bw.WriteUe(height);

            bool confFlag = confLeft != 0 || confRight != 0 || confTop != 0 || confBottom != 0;
            bw.WriteBit(confFlag);
            if (confFlag)
            {
                bw.WriteUe(confLeft);
                bw.WriteUe(confRight);
                bw.WriteUe(confTop);
                bw.WriteUe(confBottom);
            }

            bw.WriteUe(bitDepthLumaMinus8);
            bw.WriteUe(bitDepthChromaMinus8);

            // Append trailing rbsp_stop_one_bit + zero bits to byte-align.
            bw.WriteBit(true);
            bw.AlignToByte();

            byte[] rbsp = bw.ToArray();
            byte[] nalu = new byte[2 + rbsp.Length];
            nalu[0] = (byte)((33 << 1) & 0x7E);
            nalu[1] = 0x01;
            Buffer.BlockCopy(rbsp, 0, nalu, 2, rbsp.Length);
            return nalu;
        }
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
