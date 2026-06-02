using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class VvcSequenceParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_1080p_Main_Profile_With_Ptl_No_Gci()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: false, levelIdc: 51,
            frameOnly: true, multilayerEnabled: false,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.NotNull(sps);
        Assert.Equal((byte)1, sps!.GeneralProfileIdc);
        Assert.Equal((byte)51, sps.GeneralLevelIdc);
        Assert.False(sps.GeneralTierFlag);
        Assert.Equal((byte)1, sps.ChromaFormatIdc);
        Assert.Equal("4:2:0", sps.ChromaFormat);
        Assert.Equal((byte)2, sps.Log2CtuSizeMinus5);
        Assert.Equal(128u, sps.CtuSize);
        Assert.Equal(1920u, sps.PictureWidthMaxInLumaSamples);
        Assert.Equal(1080u, sps.PictureHeightMaxInLumaSamples);
        Assert.Equal(1920u, sps.DecodedWidth);
        Assert.Equal(1080u, sps.DecodedHeight);
        Assert.Equal(8u, sps.BitDepth);
    }

    [Fact]
    public void TryParse_Decodes_4K_Main10_With_High_Tier()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: true, levelIdc: 60,
            frameOnly: true, multilayerEnabled: false,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 3840, picHeightMax: 2160,
            confWinFlag: false,
            bitDepthMinus8: 2);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.GeneralTierFlag);
        Assert.Equal(10u, sps.BitDepth);
        Assert.Equal(3840u, sps.DecodedWidth);
        Assert.Equal(2160u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Applies_Conformance_Window_With_Chroma_Aware_Crop()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1088,
            confWinFlag: true,
            confLeft: 0, confRight: 0, confTop: 0, confBottom: 2,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.ConformanceWindowFlag);
        // 4:2:0: SubHeightC=2 -> crop = 2*2 = 4 luma samples bottom
        Assert.Equal(1920u, sps.DecodedWidth);
        Assert.Equal(1084u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_4_4_4_Uses_Both_Sub_Sample_Factors_1()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 3, log2CtuSizeMinus5: 1,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: true,
            confLeft: 0, confRight: 4, confTop: 0, confBottom: 2,
            bitDepthMinus8: 4);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)3, sps!.ChromaFormatIdc);
        Assert.Equal("4:4:4", sps.ChromaFormat);
        Assert.Equal(12u, sps.BitDepth);
        // 4:4:4: SubWidthC=SubHeightC=1
        Assert.Equal(1916u, sps.DecodedWidth);
        Assert.Equal(1078u, sps.DecodedHeight);
    }

    [Fact]
    public void TryParse_Decodes_Without_Ptl_Block()
    {
        var nalu = SpsBuilder.Build(
            spsId: 1, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 640, picHeightMax: 360,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.False(sps!.PtlDpbHrdParamsPresentFlag);
        Assert.Null(sps.GeneralProfileIdc);
        Assert.Null(sps.GeneralLevelIdc);
        Assert.Equal((byte)1, sps.SequenceParameterSetId);
    }

    [Fact]
    public void TryParse_Decodes_Ref_Pic_Resampling_Path()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: true,
            refPicResamplingEnabled: true,
            resChangeInClvsAllowed: true,
            picWidthMax: 1280, picHeightMax: 720,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.GdrEnabledFlag);
        Assert.True(sps.RefPicResamplingEnabledFlag);
        Assert.True(sps.ResChangeInClvsAllowedFlag);
    }

    [Fact]
    public void TryParse_Rejects_Sps_With_Gci_Present()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: false, levelIdc: 51,
            frameOnly: true, multilayerEnabled: false,
            gciPresent: true, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Sps_With_Sub_Profiles()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: false, levelIdc: 51,
            frameOnly: true, multilayerEnabled: false,
            gciPresent: false, numSubProfiles: 1,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Sps_With_Subpic_Info_Present()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            subpicInfoPresent: true,
            bitDepthMinus8: 0);

        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Non_Sps_Nal_Unit()
    {
        // nal_unit_type = 16 (PPS) instead of 15 (SPS)
        byte[] nalu = [0x00, 0x80, 0x00, 0x00];
        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Zero_Bit_Set()
    {
        // First byte has forbidden_zero_bit = 1
        byte[] nalu = [0x80, 0x78, 0x00, 0x00];
        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Nal_Header()
    {
        byte[] nalu = [0x00, 0x78];
        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        // 2-byte NAL header for SPS_NUT=15 followed by only one byte of payload.
        byte[] nalu = [0x00, 0x78, 0x00];
        Assert.False(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Null(sps);
    }

    [Theory]
    [InlineData((byte)0, 32u)]
    [InlineData((byte)1, 64u)]
    [InlineData((byte)2, 128u)]
    [InlineData((byte)3, 256u)]
    public void CtuSize_Matches_Log2CtuSizeMinus5(byte log2Minus5, uint expected)
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: log2Minus5,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 640, picHeightMax: 360,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(log2Minus5, sps!.Log2CtuSizeMinus5);
        Assert.Equal(expected, sps.CtuSize);
    }

    [Theory]
    [InlineData((byte)0, "4:0:0")]
    [InlineData((byte)1, "4:2:0")]
    [InlineData((byte)2, "4:2:2")]
    [InlineData((byte)3, "4:4:4")]
    public void ChromaFormat_String_Matches_ChromaFormatIdc(byte idc, string expected)
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: idc, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 640, picHeightMax: 360,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal(expected, sps!.ChromaFormat);
    }

    [Fact]
    public void ChromaFormat_4_2_2_Applies_Width_Crop_Only()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 2, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1088,
            confWinFlag: true,
            confLeft: 0, confRight: 2, confTop: 0, confBottom: 4,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.ChromaFormatIdc);
        // 4:2:2 -> SubWidthC=2, SubHeightC=1 -> crop = (2*2, 1*4) = (4, 4)
        Assert.Equal(1916u, sps.DecodedWidth);
        Assert.Equal(1084u, sps.DecodedHeight);
    }

    [Fact]
    public void ChromaFormat_4_0_0_Monochrome_Applies_No_Sub_Sampling()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 0, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1088,
            confWinFlag: true,
            confLeft: 0, confRight: 4, confTop: 0, confBottom: 8,
            bitDepthMinus8: 2);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        // 4:0:0 -> SubWidthC=SubHeightC=1 -> crop = (1*4, 1*8) = (4, 8)
        Assert.Equal(1916u, sps!.DecodedWidth);
        Assert.Equal(1080u, sps.DecodedHeight);
    }

    [Fact]
    public void DecodedWidth_Returns_Pic_Max_When_ConformanceWindowFlag_False()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 320, picHeightMax: 240,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.False(sps!.ConformanceWindowFlag);
        Assert.Equal(320u, sps.DecodedWidth);
        Assert.Equal(240u, sps.DecodedHeight);
        Assert.Equal(0u, sps.ConformanceWindowLeftOffset);
        Assert.Equal(0u, sps.ConformanceWindowRightOffset);
    }

    [Fact]
    public void Decoded_Dimensions_Clamp_When_Crop_Exceeds_Picture_Size()
    {
        // crop budget exceeds picture dimensions - the getter returns picWidthMax.
        var sps = new VvcSequenceParameterSet
        {
            SequenceParameterSetId = 0,
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            ChromaFormatIdc = 1,
            Log2CtuSizeMinus5 = 2,
            PtlDpbHrdParamsPresentFlag = false,
            GdrEnabledFlag = false,
            RefPicResamplingEnabledFlag = false,
            ResChangeInClvsAllowedFlag = false,
            PictureWidthMaxInLumaSamples = 16,
            PictureHeightMaxInLumaSamples = 16,
            ConformanceWindowFlag = true,
            ConformanceWindowLeftOffset = 100,
            ConformanceWindowRightOffset = 100,
            ConformanceWindowTopOffset = 100,
            ConformanceWindowBottomOffset = 100,
            SubpicInfoPresentFlag = false,
            BitDepthMinus8 = 0,
        };
        Assert.Equal(16u, sps.DecodedWidth);
        Assert.Equal(16u, sps.DecodedHeight);
    }

    [Fact]
    public void RefPicResampling_With_ResChange_False_Is_Preserved()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: true,
            resChangeInClvsAllowed: false,
            picWidthMax: 1280, picHeightMax: 720,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.RefPicResamplingEnabledFlag);
        Assert.False(sps.ResChangeInClvsAllowedFlag);
    }

    [Fact]
    public void Ptl_FrameOnly_False_Multilayer_True_Preserved()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: false, levelIdc: 51,
            frameOnly: false, multilayerEnabled: true,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.True(sps!.PtlDpbHrdParamsPresentFlag);
        Assert.False(sps.PtlFrameOnlyConstraintFlag);
        Assert.True(sps.PtlMultilayerEnabledFlag);
    }

    [Fact]
    public void MaxSubLayersMinus1_Nonzero_Skips_Sublayer_Level_Present_Bits()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 2,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: true,
            profileIdc: 1, tierFlag: false, levelIdc: 51,
            frameOnly: true, multilayerEnabled: false,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var sps));
        Assert.Equal((byte)2, sps!.MaxSubLayersMinus1);
    }

    [Fact]
    public void Record_Equality_And_With_Expression()
    {
        var nalu = SpsBuilder.Build(
            spsId: 0, vpsId: 0, maxSubLayersMinus1: 0,
            chromaFormatIdc: 1, log2CtuSizeMinus5: 2,
            ptlDpbHrdParamsPresent: false,
            profileIdc: null, tierFlag: null, levelIdc: null,
            frameOnly: null, multilayerEnabled: null,
            gciPresent: false, numSubProfiles: 0,
            gdrEnabled: false,
            refPicResamplingEnabled: false,
            picWidthMax: 1920, picHeightMax: 1080,
            confWinFlag: false,
            bitDepthMinus8: 0);

        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var a));
        Assert.True(VvcSequenceParameterSet.TryParse(nalu, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a!.GetHashCode(), b!.GetHashCode());

        var c = a with { BitDepthMinus8 = 4 };
        Assert.NotEqual(a, c);
        Assert.Equal(12u, c.BitDepth);
    }

    [Fact]
    public void SpsNalUnitType_Constant_Equals_15()
    {
        Assert.Equal(15, VvcSequenceParameterSet.SpsNalUnitType);
    }

    [Fact]
    public void ChromaFormat_Reserved_Returns_Bracketed_String_For_Out_Of_Range_Direct_Construction()
    {
        // 2-bit field can never decode to >=4 from parse, but direct construction may.
        var sps = new VvcSequenceParameterSet
        {
            SequenceParameterSetId = 0,
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            ChromaFormatIdc = 5,
            Log2CtuSizeMinus5 = 2,
            PtlDpbHrdParamsPresentFlag = false,
            GdrEnabledFlag = false,
            RefPicResamplingEnabledFlag = false,
            ResChangeInClvsAllowedFlag = false,
            PictureWidthMaxInLumaSamples = 16,
            PictureHeightMaxInLumaSamples = 16,
            ConformanceWindowFlag = false,
            ConformanceWindowLeftOffset = 0,
            ConformanceWindowRightOffset = 0,
            ConformanceWindowTopOffset = 0,
            ConformanceWindowBottomOffset = 0,
            SubpicInfoPresentFlag = false,
            BitDepthMinus8 = 0,
        };
        Assert.Equal("reserved(5)", sps.ChromaFormat);
    }

    // ---------- SPS bitstream builder ----------

    private static class SpsBuilder
    {
        public static byte[] Build(
            byte spsId, byte vpsId, byte maxSubLayersMinus1,
            byte chromaFormatIdc, byte log2CtuSizeMinus5,
            bool ptlDpbHrdParamsPresent,
            byte? profileIdc, bool? tierFlag, byte? levelIdc,
            bool? frameOnly, bool? multilayerEnabled,
            bool gciPresent, byte numSubProfiles,
            bool gdrEnabled,
            bool refPicResamplingEnabled,
            uint picWidthMax, uint picHeightMax,
            bool confWinFlag,
            uint bitDepthMinus8,
            uint confLeft = 0, uint confRight = 0, uint confTop = 0, uint confBottom = 0,
            bool subpicInfoPresent = false,
            bool resChangeInClvsAllowed = false)
        {
            var bw = new BitWriter();
            bw.WriteBits(spsId, 4);
            bw.WriteBits(vpsId, 4);
            bw.WriteBits(maxSubLayersMinus1, 3);
            bw.WriteBits(chromaFormatIdc, 2);
            bw.WriteBits(log2CtuSizeMinus5, 2);
            bw.WriteBit(ptlDpbHrdParamsPresent);

            if (ptlDpbHrdParamsPresent)
            {
                bw.WriteBits(profileIdc!.Value, 7);
                bw.WriteBit(tierFlag!.Value);
                bw.WriteBits(levelIdc!.Value, 8);
                bw.WriteBit(frameOnly!.Value);
                bw.WriteBit(multilayerEnabled!.Value);
                bw.WriteBit(gciPresent);
                // Skip GCI fields when absent (parser only supports gciPresent=false).
                bw.AlignToByte();
                for (int i = maxSubLayersMinus1 - 1; i >= 0; i--)
                {
                    bw.WriteBit(false);
                }
                bw.AlignToByte();
                bw.WriteBits(numSubProfiles, 8);
                for (int i = 0; i < numSubProfiles; i++)
                {
                    bw.WriteBits(0, 32);
                }
            }

            bw.WriteBit(gdrEnabled);
            bw.WriteBit(refPicResamplingEnabled);
            if (refPicResamplingEnabled) bw.WriteBit(resChangeInClvsAllowed);

            bw.WriteUe(picWidthMax);
            bw.WriteUe(picHeightMax);
            bw.WriteBit(confWinFlag);
            if (confWinFlag)
            {
                bw.WriteUe(confLeft);
                bw.WriteUe(confRight);
                bw.WriteUe(confTop);
                bw.WriteUe(confBottom);
            }
            bw.WriteBit(subpicInfoPresent);
            bw.WriteUe(bitDepthMinus8);

            // Trailing rbsp_trailing_bits().
            bw.WriteBit(true);
            bw.AlignToByte();

            byte[] rbsp = bw.ToArray();
            byte[] nalu = new byte[2 + rbsp.Length];
            // NAL header: forbidden=0, nuh_reserved=0, nuh_layer_id=0,
            // nal_unit_type=15, nuh_temporal_id_plus1=1.
            nalu[0] = 0x00;
            nalu[1] = (byte)((15 << 3) | 0); // type=15, temporal_id_plus1=0 (then we'd OR 1 conventionally)
            // Some parsers care about temporal_id_plus1; ours doesn't so leave as 0.
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
