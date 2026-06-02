using System.Collections.Immutable;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class VvcPictureParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_Minimal_Pps()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.NotNull(pps);
        Assert.Equal(0, pps!.PicParameterSetId);
        Assert.Equal(0, pps.SeqParameterSetId);
        Assert.False(pps.MixedNaluTypesInPicFlag);
        Assert.Equal(1920u, pps.PicWidthInLumaSamples);
        Assert.Equal(1080u, pps.PicHeightInLumaSamples);
        Assert.False(pps.ConformanceWindowFlag);
        Assert.Null(pps.ConfWinLeftOffset);
        Assert.False(pps.ScalingWindowExplicitSignallingFlag);
        Assert.Null(pps.ScalingWinLeftOffset);
        Assert.True(pps.NoPicPartitionFlag);
        Assert.False(pps.SubpicIdMappingPresentFlag);
        Assert.False(pps.RefWraparoundEnabledFlag);
        Assert.Null(pps.PicWidthMinusWraparoundOffset);
        Assert.Equal(0, pps.InitQpMinus26);
        Assert.False(pps.ChromaToolOffsetsPresentFlag);
        Assert.Null(pps.CbQpOffset);
        Assert.Empty(pps.CbQpOffsetList);
        Assert.False(pps.DeblockingFilterControlPresentFlag);
        Assert.Null(pps.DeblockingFilterOverrideEnabledFlag);
        Assert.False(pps.ExtensionFlag);
    }

    [Fact]
    public void TryParse_Decodes_PicSet_Ids_And_Picture_Size()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 0x2A,
            SeqParameterSetId = 0xB,
            MixedNaluTypesInPicFlag = true,
            PicWidthInLumaSamples = 3840,
            PicHeightInLumaSamples = 2160,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(0x2A, pps!.PicParameterSetId);
        Assert.Equal(0xB, pps.SeqParameterSetId);
        Assert.True(pps.MixedNaluTypesInPicFlag);
        Assert.Equal(3840u, pps.PicWidthInLumaSamples);
        Assert.Equal(2160u, pps.PicHeightInLumaSamples);
    }

    [Fact]
    public void TryParse_Decodes_Conformance_Window()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1088,
            ConformanceWindowFlag = true,
            ConfWinLeftOffset = 0,
            ConfWinRightOffset = 0,
            ConfWinTopOffset = 0,
            ConfWinBottomOffset = 4,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.ConformanceWindowFlag);
        Assert.Equal(0u, pps.ConfWinLeftOffset);
        Assert.Equal(0u, pps.ConfWinRightOffset);
        Assert.Equal(0u, pps.ConfWinTopOffset);
        Assert.Equal(4u, pps.ConfWinBottomOffset);
    }

    [Fact]
    public void TryParse_Decodes_Scaling_Window()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1280,
            PicHeightInLumaSamples = 720,
            ScalingWindowExplicitSignallingFlag = true,
            ScalingWinLeftOffset = -2,
            ScalingWinRightOffset = 4,
            ScalingWinTopOffset = -1,
            ScalingWinBottomOffset = 3,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.ScalingWindowExplicitSignallingFlag);
        Assert.Equal(-2, pps.ScalingWinLeftOffset);
        Assert.Equal(4, pps.ScalingWinRightOffset);
        Assert.Equal(-1, pps.ScalingWinTopOffset);
        Assert.Equal(3, pps.ScalingWinBottomOffset);
    }

    [Fact]
    public void TryParse_Decodes_Reference_Lists_And_Init_Qp()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            CabacInitPresentFlag = true,
            NumRefIdxL0DefaultActiveMinus1 = 2,
            NumRefIdxL1DefaultActiveMinus1 = 1,
            Rpl1IdxPresentFlag = true,
            WeightedPredFlag = true,
            WeightedBipredFlag = true,
            InitQpMinus26 = -8,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.CabacInitPresentFlag);
        Assert.Equal(2u, pps.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(1u, pps.NumRefIdxL1DefaultActiveMinus1);
        Assert.True(pps.Rpl1IdxPresentFlag);
        Assert.True(pps.WeightedPredFlag);
        Assert.True(pps.WeightedBipredFlag);
        Assert.Equal(-8, pps.InitQpMinus26);
    }

    [Fact]
    public void TryParse_Decodes_Reference_Wraparound()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            RefWraparoundEnabledFlag = true,
            PicWidthMinusWraparoundOffset = 13,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.RefWraparoundEnabledFlag);
        Assert.Equal(13u, pps.PicWidthMinusWraparoundOffset);
    }

    [Fact]
    public void TryParse_Decodes_Chroma_Tool_Offsets_With_Qp_Offset_List()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ChromaToolOffsetsPresentFlag = true,
            CbQpOffset = -3,
            CrQpOffset = 4,
            JointCbCrQpOffsetPresentFlag = false,
            SliceChromaQpOffsetsPresentFlag = true,
            CuChromaQpOffsetListEnabledFlag = true,
            CbQpOffsetList = ImmutableArray.Create(-2, 5),
            CrQpOffsetList = ImmutableArray.Create(1, -4),
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.ChromaToolOffsetsPresentFlag);
        Assert.Equal(-3, pps.CbQpOffset);
        Assert.Equal(4, pps.CrQpOffset);
        Assert.Equal(false, pps.JointCbCrQpOffsetPresentFlag);
        Assert.Null(pps.JointCbCrQpOffsetValue);
        Assert.Equal(true, pps.SliceChromaQpOffsetsPresentFlag);
        Assert.Equal(true, pps.CuChromaQpOffsetListEnabledFlag);
        Assert.Equal(2, pps.CbQpOffsetList.Length);
        Assert.Equal(-2, pps.CbQpOffsetList[0]);
        Assert.Equal(5, pps.CbQpOffsetList[1]);
        Assert.Equal(2, pps.CrQpOffsetList.Length);
        Assert.Equal(1, pps.CrQpOffsetList[0]);
        Assert.Equal(-4, pps.CrQpOffsetList[1]);
        Assert.Empty(pps.JointCbCrQpOffsetList);
    }

    [Fact]
    public void TryParse_Decodes_Joint_CbCr_Qp_Offset_With_List()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ChromaToolOffsetsPresentFlag = true,
            CbQpOffset = 0,
            CrQpOffset = 0,
            JointCbCrQpOffsetPresentFlag = true,
            JointCbCrQpOffsetValue = 7,
            CuChromaQpOffsetListEnabledFlag = true,
            CbQpOffsetList = ImmutableArray.Create(1),
            CrQpOffsetList = ImmutableArray.Create(-1),
            JointCbCrQpOffsetList = ImmutableArray.Create(2),
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(true, pps!.JointCbCrQpOffsetPresentFlag);
        Assert.Equal(7, pps.JointCbCrQpOffsetValue);
        Assert.Single(pps.CbQpOffsetList);
        Assert.Equal(1, pps.CbQpOffsetList[0]);
        Assert.Single(pps.CrQpOffsetList);
        Assert.Equal(-1, pps.CrQpOffsetList[0]);
        Assert.Single(pps.JointCbCrQpOffsetList);
        Assert.Equal(2, pps.JointCbCrQpOffsetList[0]);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_Filter_Control()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            DeblockingFilterControlPresentFlag = true,
            DeblockingFilterOverrideEnabledFlag = true,
            DeblockingFilterDisabledFlag = false,
            LumaBetaOffsetDiv2 = -3,
            LumaTcOffsetDiv2 = 5,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DeblockingFilterControlPresentFlag);
        Assert.Equal(true, pps.DeblockingFilterOverrideEnabledFlag);
        Assert.Equal(false, pps.DeblockingFilterDisabledFlag);
        Assert.Equal(-3, pps.LumaBetaOffsetDiv2);
        Assert.Equal(5, pps.LumaTcOffsetDiv2);
        Assert.Null(pps.CbBetaOffsetDiv2);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_With_Chroma_Offsets()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ChromaToolOffsetsPresentFlag = true,
            CbQpOffset = 0,
            CrQpOffset = 0,
            JointCbCrQpOffsetPresentFlag = false,
            DeblockingFilterControlPresentFlag = true,
            DeblockingFilterOverrideEnabledFlag = false,
            DeblockingFilterDisabledFlag = false,
            LumaBetaOffsetDiv2 = 1,
            LumaTcOffsetDiv2 = 2,
            CbBetaOffsetDiv2 = -1,
            CbTcOffsetDiv2 = 3,
            CrBetaOffsetDiv2 = -2,
            CrTcOffsetDiv2 = 4,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(1, pps!.LumaBetaOffsetDiv2);
        Assert.Equal(2, pps.LumaTcOffsetDiv2);
        Assert.Equal(-1, pps.CbBetaOffsetDiv2);
        Assert.Equal(3, pps.CbTcOffsetDiv2);
        Assert.Equal(-2, pps.CrBetaOffsetDiv2);
        Assert.Equal(4, pps.CrTcOffsetDiv2);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_When_Disabled_Omits_Offsets()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            DeblockingFilterControlPresentFlag = true,
            DeblockingFilterOverrideEnabledFlag = false,
            DeblockingFilterDisabledFlag = true,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DeblockingFilterControlPresentFlag);
        Assert.Equal(true, pps.DeblockingFilterDisabledFlag);
        Assert.Null(pps.LumaBetaOffsetDiv2);
        Assert.Null(pps.LumaTcOffsetDiv2);
    }

    [Fact]
    public void TryParse_Decodes_Extension_Flags()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            PictureHeaderExtensionPresentFlag = true,
            SliceHeaderExtensionPresentFlag = true,
            ExtensionFlag = true,
        });

        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PictureHeaderExtensionPresentFlag);
        Assert.True(pps.SliceHeaderExtensionPresentFlag);
        Assert.True(pps.ExtensionFlag);
    }

    [Fact]
    public void TryParse_Rejects_NonPps_NalType()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            NalUnitTypeOverride = 15, // SPS_NUT
        });

        Assert.False(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Or_Reserved_Bit_Set()
    {
        var withForbidden = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ForbiddenZeroBit = true,
        });
        Assert.False(VvcPictureParameterSet.TryParse(withForbidden, out _));

        var withReserved = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ReservedZeroBit = true,
        });
        Assert.False(VvcPictureParameterSet.TryParse(withReserved, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(VvcPictureParameterSet.TryParse(ReadOnlySpan<byte>.Empty, out _));
        Assert.False(VvcPictureParameterSet.TryParse(new byte[] { 0x00 }, out _));
        Assert.False(VvcPictureParameterSet.TryParse(new byte[] { 0x00, 0x81 }, out _));
    }

    [Fact]
    public void TryParse_Rejects_Pps_With_Pic_Partition()
    {
        // pps_no_pic_partition_flag = 0 is outside the bounded scope of
        // this parser; the picture-partition sub-stream is not decoded.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            NoPicPartitionFlag = false,
        });

        Assert.False(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Pps_With_Subpic_Id_Mapping()
    {
        // Explicit subpic id mapping requires the SPS to derive
        // pps_num_subpics_minus1 when pic-partition is disabled; rejected.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            SubpicIdMappingPresentFlag = true,
        });

        Assert.False(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Roundtrips_Through_Emulation_Prevention_Encoding()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 5,
            SeqParameterSetId = 2,
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            InitQpMinus26 = 3,
        });

        // Walk the RBSP bytes (after the 2-byte NAL header) and inject
        // an emulation prevention 0x03 byte after every 00 00 sequence.
        var encoded = new List<byte> { nalu[0], nalu[1] };
        int zeros = 0;
        for (int i = 2; i < nalu.Length; i++)
        {
            byte b = nalu[i];
            if (zeros >= 2 && b <= 0x03)
            {
                encoded.Add(0x03);
                zeros = 0;
            }
            encoded.Add(b);
            zeros = b == 0 ? zeros + 1 : 0;
        }

        Assert.True(VvcPictureParameterSet.TryParse(encoded.ToArray(), out var pps));
        Assert.Equal(5, pps!.PicParameterSetId);
        Assert.Equal(2, pps.SeqParameterSetId);
        Assert.Equal(3, pps.InitQpMinus26);
    }

    [Theory]
    [InlineData(320u, 180u)]
    [InlineData(1280u, 720u)]
    [InlineData(7680u, 4320u)]
    public void TryParse_Various_Resolutions(uint width, uint height)
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = width,
            PicHeightInLumaSamples = height,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(width, pps!.PicWidthInLumaSamples);
        Assert.Equal(height, pps.PicHeightInLumaSamples);
    }

    [Theory]
    [InlineData(-26)]
    [InlineData(-15)]
    [InlineData(0)]
    [InlineData(15)]
    [InlineData(25)]
    public void TryParse_InitQp_BoundaryValues(int initQp)
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            InitQpMinus26 = initQp,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(initQp, pps!.InitQpMinus26);
    }

    [Fact]
    public void TryParse_OutputFlagPresent_Flag_Recorded()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            OutputFlagPresentFlag = true,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.OutputFlagPresentFlag);
    }

    [Fact]
    public void TryParse_CuQpDeltaEnabled_Flag_Recorded()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            CuQpDeltaEnabledFlag = true,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.CuQpDeltaEnabledFlag);
    }

    [Fact]
    public void TryParse_ChromaTool_Without_QpOffsetList_NoLists()
    {
        // ChromaToolOffsetsPresentFlag=true but CuChromaQpOffsetListEnabledFlag=false
        // should yield empty CbQpOffsetList/CrQpOffsetList.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ChromaToolOffsetsPresentFlag = true,
            CbQpOffset = -2,
            CrQpOffset = 3,
            CuChromaQpOffsetListEnabledFlag = false,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.ChromaToolOffsetsPresentFlag);
        Assert.Equal(-2, pps.CbQpOffset);
        Assert.Equal(3, pps.CrQpOffset);
        Assert.Empty(pps.CbQpOffsetList);
        Assert.Empty(pps.CrQpOffsetList);
        Assert.Empty(pps.JointCbCrQpOffsetList);
    }

    [Fact]
    public void TryParse_NumRefIdx_NonZero_Values_Preserved()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            NumRefIdxL0DefaultActiveMinus1 = 14,
            NumRefIdxL1DefaultActiveMinus1 = 7,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(14u, pps!.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(7u, pps.NumRefIdxL1DefaultActiveMinus1);
    }

    [Fact]
    public void TryParse_Conformance_Window_AllSides_NonZero()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ConformanceWindowFlag = true,
            ConfWinLeftOffset = 4,
            ConfWinRightOffset = 8,
            ConfWinTopOffset = 2,
            ConfWinBottomOffset = 6,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(4u, pps!.ConfWinLeftOffset);
        Assert.Equal(8u, pps.ConfWinRightOffset);
        Assert.Equal(2u, pps.ConfWinTopOffset);
        Assert.Equal(6u, pps.ConfWinBottomOffset);
    }

    [Fact]
    public void TryParse_RefWraparound_With_Zero_Offset()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            RefWraparoundEnabledFlag = true,
            PicWidthMinusWraparoundOffset = 0,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.RefWraparoundEnabledFlag);
        Assert.Equal(0u, pps.PicWidthMinusWraparoundOffset);
    }

    [Fact]
    public void TryParse_PicParameterSetId_MaxValue_63()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 63,
            SeqParameterSetId = 15,
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(63, pps!.PicParameterSetId);
        Assert.Equal(15, pps.SeqParameterSetId);
    }

    [Theory]
    [InlineData((byte)0)]   // TRAIL_NUT
    [InlineData((byte)15)]  // SPS_NUT
    [InlineData((byte)17)]  // APS_NUT
    [InlineData((byte)31)]  // RESERVED
    public void TryParse_Rejects_AllOther_NalUnitTypes(byte nut)
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            NalUnitTypeOverride = nut,
        });
        Assert.False(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_ScalingWindow_NegativeAndPositive_Mix()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1280,
            PicHeightInLumaSamples = 720,
            ScalingWindowExplicitSignallingFlag = true,
            ScalingWinLeftOffset = -100,
            ScalingWinRightOffset = 200,
            ScalingWinTopOffset = -50,
            ScalingWinBottomOffset = 75,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(-100, pps!.ScalingWinLeftOffset);
        Assert.Equal(200, pps.ScalingWinRightOffset);
        Assert.Equal(-50, pps.ScalingWinTopOffset);
        Assert.Equal(75, pps.ScalingWinBottomOffset);
    }

    [Fact]
    public void TryParse_ScalingWindow_All_Zero_Offsets_Stored_As_Zero()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicWidthInLumaSamples = 1920,
            PicHeightInLumaSamples = 1080,
            ScalingWindowExplicitSignallingFlag = true,
        });
        Assert.True(VvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.ScalingWindowExplicitSignallingFlag);
        Assert.Equal(0, pps.ScalingWinLeftOffset);
        Assert.Equal(0, pps.ScalingWinRightOffset);
        Assert.Equal(0, pps.ScalingWinTopOffset);
        Assert.Equal(0, pps.ScalingWinBottomOffset);
    }

    // -----------------------------------------------------------------
    // Test helpers
    // -----------------------------------------------------------------

    private sealed record PpsSpec
    {
        public byte PicParameterSetId { get; init; }
        public byte SeqParameterSetId { get; init; }
        public bool MixedNaluTypesInPicFlag { get; init; }
        public uint PicWidthInLumaSamples { get; init; } = 1920;
        public uint PicHeightInLumaSamples { get; init; } = 1080;

        public bool ConformanceWindowFlag { get; init; }
        public uint ConfWinLeftOffset { get; init; }
        public uint ConfWinRightOffset { get; init; }
        public uint ConfWinTopOffset { get; init; }
        public uint ConfWinBottomOffset { get; init; }

        public bool ScalingWindowExplicitSignallingFlag { get; init; }
        public int ScalingWinLeftOffset { get; init; }
        public int ScalingWinRightOffset { get; init; }
        public int ScalingWinTopOffset { get; init; }
        public int ScalingWinBottomOffset { get; init; }

        public bool OutputFlagPresentFlag { get; init; }
        public bool NoPicPartitionFlag { get; init; } = true;
        public bool SubpicIdMappingPresentFlag { get; init; }

        public bool CabacInitPresentFlag { get; init; }
        public uint NumRefIdxL0DefaultActiveMinus1 { get; init; }
        public uint NumRefIdxL1DefaultActiveMinus1 { get; init; }
        public bool Rpl1IdxPresentFlag { get; init; }
        public bool WeightedPredFlag { get; init; }
        public bool WeightedBipredFlag { get; init; }
        public bool RefWraparoundEnabledFlag { get; init; }
        public uint PicWidthMinusWraparoundOffset { get; init; }

        public int InitQpMinus26 { get; init; }
        public bool CuQpDeltaEnabledFlag { get; init; }
        public bool ChromaToolOffsetsPresentFlag { get; init; }
        public int CbQpOffset { get; init; }
        public int CrQpOffset { get; init; }
        public bool JointCbCrQpOffsetPresentFlag { get; init; }
        public int JointCbCrQpOffsetValue { get; init; }
        public bool SliceChromaQpOffsetsPresentFlag { get; init; }
        public bool CuChromaQpOffsetListEnabledFlag { get; init; }
        public ImmutableArray<int> CbQpOffsetList { get; init; } = ImmutableArray<int>.Empty;
        public ImmutableArray<int> CrQpOffsetList { get; init; } = ImmutableArray<int>.Empty;
        public ImmutableArray<int> JointCbCrQpOffsetList { get; init; } = ImmutableArray<int>.Empty;

        public bool DeblockingFilterControlPresentFlag { get; init; }
        public bool DeblockingFilterOverrideEnabledFlag { get; init; }
        public bool DeblockingFilterDisabledFlag { get; init; }
        public int LumaBetaOffsetDiv2 { get; init; }
        public int LumaTcOffsetDiv2 { get; init; }
        public int CbBetaOffsetDiv2 { get; init; }
        public int CbTcOffsetDiv2 { get; init; }
        public int CrBetaOffsetDiv2 { get; init; }
        public int CrTcOffsetDiv2 { get; init; }

        public bool PictureHeaderExtensionPresentFlag { get; init; }
        public bool SliceHeaderExtensionPresentFlag { get; init; }
        public bool ExtensionFlag { get; init; }

        public bool ForbiddenZeroBit { get; init; }
        public bool ReservedZeroBit { get; init; }
        public byte NalUnitTypeOverride { get; init; } = 16; // PPS_NUT
    }

    private static class PpsBuilder
    {
        public static byte[] Build(PpsSpec spec)
        {
            var w = new BitWriter();
            w.WriteBits(spec.PicParameterSetId, 6);
            w.WriteBits(spec.SeqParameterSetId, 4);
            w.WriteBit(spec.MixedNaluTypesInPicFlag);
            w.WriteUe(spec.PicWidthInLumaSamples);
            w.WriteUe(spec.PicHeightInLumaSamples);

            w.WriteBit(spec.ConformanceWindowFlag);
            if (spec.ConformanceWindowFlag)
            {
                w.WriteUe(spec.ConfWinLeftOffset);
                w.WriteUe(spec.ConfWinRightOffset);
                w.WriteUe(spec.ConfWinTopOffset);
                w.WriteUe(spec.ConfWinBottomOffset);
            }

            w.WriteBit(spec.ScalingWindowExplicitSignallingFlag);
            if (spec.ScalingWindowExplicitSignallingFlag)
            {
                w.WriteSe(spec.ScalingWinLeftOffset);
                w.WriteSe(spec.ScalingWinRightOffset);
                w.WriteSe(spec.ScalingWinTopOffset);
                w.WriteSe(spec.ScalingWinBottomOffset);
            }

            w.WriteBit(spec.OutputFlagPresentFlag);
            w.WriteBit(spec.NoPicPartitionFlag);
            w.WriteBit(spec.SubpicIdMappingPresentFlag);

            // The test fixtures never combine SubpicIdMappingPresentFlag = true
            // with NoPicPartitionFlag = true in a successful parse path, so
            // we emit the smallest valid follow-up here only when needed for
            // the rejection tests (the parser exits before reading these bits
            // because it rejects either flag mismatch up-front).

            w.WriteBit(spec.CabacInitPresentFlag);
            w.WriteUe(spec.NumRefIdxL0DefaultActiveMinus1);
            w.WriteUe(spec.NumRefIdxL1DefaultActiveMinus1);
            w.WriteBit(spec.Rpl1IdxPresentFlag);
            w.WriteBit(spec.WeightedPredFlag);
            w.WriteBit(spec.WeightedBipredFlag);
            w.WriteBit(spec.RefWraparoundEnabledFlag);
            if (spec.RefWraparoundEnabledFlag) w.WriteUe(spec.PicWidthMinusWraparoundOffset);

            w.WriteSe(spec.InitQpMinus26);
            w.WriteBit(spec.CuQpDeltaEnabledFlag);
            w.WriteBit(spec.ChromaToolOffsetsPresentFlag);
            if (spec.ChromaToolOffsetsPresentFlag)
            {
                w.WriteSe(spec.CbQpOffset);
                w.WriteSe(spec.CrQpOffset);
                w.WriteBit(spec.JointCbCrQpOffsetPresentFlag);
                if (spec.JointCbCrQpOffsetPresentFlag) w.WriteSe(spec.JointCbCrQpOffsetValue);
                w.WriteBit(spec.SliceChromaQpOffsetsPresentFlag);
                w.WriteBit(spec.CuChromaQpOffsetListEnabledFlag);
                if (spec.CuChromaQpOffsetListEnabledFlag)
                {
                    int len = spec.CbQpOffsetList.Length;
                    w.WriteUe((uint)(len - 1));
                    for (int i = 0; i < len; i++)
                    {
                        w.WriteSe(spec.CbQpOffsetList[i]);
                        w.WriteSe(spec.CrQpOffsetList[i]);
                        if (spec.JointCbCrQpOffsetPresentFlag)
                            w.WriteSe(spec.JointCbCrQpOffsetList[i]);
                    }
                }
            }

            w.WriteBit(spec.DeblockingFilterControlPresentFlag);
            if (spec.DeblockingFilterControlPresentFlag)
            {
                w.WriteBit(spec.DeblockingFilterOverrideEnabledFlag);
                w.WriteBit(spec.DeblockingFilterDisabledFlag);
                // pps_dbf_info_in_ph_flag is gated by !no_pic_partition; skipped.
                if (!spec.DeblockingFilterDisabledFlag)
                {
                    w.WriteSe(spec.LumaBetaOffsetDiv2);
                    w.WriteSe(spec.LumaTcOffsetDiv2);
                    if (spec.ChromaToolOffsetsPresentFlag)
                    {
                        w.WriteSe(spec.CbBetaOffsetDiv2);
                        w.WriteSe(spec.CbTcOffsetDiv2);
                        w.WriteSe(spec.CrBetaOffsetDiv2);
                        w.WriteSe(spec.CrTcOffsetDiv2);
                    }
                }
            }

            w.WriteBit(spec.PictureHeaderExtensionPresentFlag);
            w.WriteBit(spec.SliceHeaderExtensionPresentFlag);
            w.WriteBit(spec.ExtensionFlag);

            // rbsp_trailing_bits(): rbsp_stop_one_bit + alignment zero bits
            w.WriteBit(true);
            w.AlignToByte();

            byte[] rbsp = w.ToArray();
            var nalu = new byte[rbsp.Length + 2];
            int forbidden = spec.ForbiddenZeroBit ? 1 : 0;
            int reserved = spec.ReservedZeroBit ? 1 : 0;
            // Byte 0: forbidden (1) + reserved_zero (1) + nuh_layer_id (6, = 0)
            nalu[0] = (byte)((forbidden << 7) | (reserved << 6));
            // Byte 1: nal_unit_type (5) + temporal_id_plus1 (3, = 1)
            nalu[1] = (byte)(((spec.NalUnitTypeOverride & 0x1F) << 3) | 0x01);
            Array.Copy(rbsp, 0, nalu, 2, rbsp.Length);
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

        public void WriteSe(int value)
        {
            uint codeNum = value > 0
                ? (uint)(2 * value - 1)
                : (uint)(-2 * value);
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
