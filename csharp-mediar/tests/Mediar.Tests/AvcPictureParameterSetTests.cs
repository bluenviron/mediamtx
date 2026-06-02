using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class AvcPictureParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_Minimal_Pps_Without_PostTail()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 0,
            SeqParameterSetId = 0,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.NotNull(pps);
        Assert.Equal(0u, pps!.PicParameterSetId);
        Assert.Equal(0u, pps.SeqParameterSetId);
        Assert.False(pps.EntropyCodingModeFlag);
        Assert.False(pps.BottomFieldPicOrderInFramePresentFlag);
        Assert.Equal(0u, pps.NumSliceGroupsMinus1);
        Assert.Equal(0u, pps.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(0u, pps.NumRefIdxL1DefaultActiveMinus1);
        Assert.False(pps.WeightedPredFlag);
        Assert.Equal(0, pps.WeightedBipredIdc);
        Assert.Equal(0, pps.PicInitQpMinus26);
        Assert.Equal(0, pps.PicInitQsMinus26);
        Assert.Equal(0, pps.ChromaQpIndexOffset);
        Assert.False(pps.DeblockingFilterControlPresentFlag);
        Assert.False(pps.ConstrainedIntraPredFlag);
        Assert.False(pps.RedundantPicCntPresentFlag);
        Assert.Null(pps.Transform8x8ModeFlag);
        Assert.Null(pps.PicScalingMatrixPresentFlag);
        Assert.Null(pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_Ids_And_Ref_Counts_And_Qp_Fields()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 4,
            SeqParameterSetId = 2,
            NumRefIdxL0DefaultActiveMinus1 = 3,
            NumRefIdxL1DefaultActiveMinus1 = 1,
            PicInitQpMinus26 = -5,
            PicInitQsMinus26 = 7,
            ChromaQpIndexOffset = -3,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(4u, pps!.PicParameterSetId);
        Assert.Equal(2u, pps.SeqParameterSetId);
        Assert.Equal(3u, pps.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(1u, pps.NumRefIdxL1DefaultActiveMinus1);
        Assert.Equal(-5, pps.PicInitQpMinus26);
        Assert.Equal(7, pps.PicInitQsMinus26);
        Assert.Equal(-3, pps.ChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_Cabac_Entropy_Coding_Mode()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            EntropyCodingModeFlag = true,
            BottomFieldPicOrderInFramePresentFlag = true,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.EntropyCodingModeFlag);
        Assert.True(pps.BottomFieldPicOrderInFramePresentFlag);
    }

    [Fact]
    public void TryParse_Decodes_Weighted_Pred_And_Bipred_Idc()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            WeightedPredFlag = true,
            WeightedBipredIdc = 2,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.WeightedPredFlag);
        Assert.Equal(2, pps.WeightedBipredIdc);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_Filter_Control_And_Other_Flags()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            DeblockingFilterControlPresentFlag = true,
            ConstrainedIntraPredFlag = true,
            RedundantPicCntPresentFlag = true,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DeblockingFilterControlPresentFlag);
        Assert.True(pps.ConstrainedIntraPredFlag);
        Assert.True(pps.RedundantPicCntPresentFlag);
    }

    [Fact]
    public void TryParse_Decodes_PostTail_Without_Scaling_Matrix()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = false,
            SecondChromaQpIndexOffset = -7,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.Transform8x8ModeFlag);
        Assert.False(pps.PicScalingMatrixPresentFlag);
        Assert.Equal(-7, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_PostTail_With_Scaling_Matrix_And_Transform_8x8()
    {
        // Scaling matrix is emitted with pic_scaling_list_present_flag = 0
        // for every list (6 + 2 = 8 lists for transform_8x8 + chroma_format != 3).
        // Tests the bit-accurate skip path.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = true,
            SecondChromaQpIndexOffset = 5,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.Transform8x8ModeFlag);
        Assert.True(pps.PicScalingMatrixPresentFlag);
        Assert.Equal(5, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_PostTail_With_Scaling_Matrix_Without_Transform_8x8()
    {
        // transform_8x8_mode_flag = 0 means only the 6 4x4 scaling lists are
        // present (no 8x8 lists). Verifies the conditional list-count math.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = false,
            PicScalingMatrixPresentFlag = true,
            SecondChromaQpIndexOffset = -2,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.False(pps!.Transform8x8ModeFlag);
        Assert.True(pps.PicScalingMatrixPresentFlag);
        Assert.Equal(-2, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Accepts_Various_NalRefIdc_Values()
    {
        // PPS NAL units typically carry nal_ref_idc = 3 ("important") but
        // the parser must accept any value of that field.
        foreach (byte refIdc in new byte[] { 0, 1, 2, 3 })
        {
            var nalu = PpsBuilder.Build(new PpsSpec
            {
                PicParameterSetId = 1,
                NalRefIdc = refIdc,
            });
            Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps),
                $"Should accept nal_ref_idc = {refIdc}");
            Assert.Equal(1u, pps!.PicParameterSetId);
        }
    }

    [Fact]
    public void TryParse_Rejects_NonPps_Nal_Type()
    {
        var nalu = PpsBuilder.Build(new PpsSpec { NalUnitTypeOverride = 7 }); // 7 = SPS
        Assert.False(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_ForbiddenZero_Bit_Set()
    {
        var nalu = PpsBuilder.Build(new PpsSpec { ForbiddenZeroBit = true });
        Assert.False(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(AvcPictureParameterSet.TryParse(ReadOnlySpan<byte>.Empty, out _));
        Assert.False(AvcPictureParameterSet.TryParse(new byte[] { 0x68 }, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            SecondChromaQpIndexOffset = 1,
        });

        // Strip enough bytes that the baseline fields cannot fit.
        // (Just trimming the trailing byte is recovered by
        // more_rbsp_data() returning false; truncating below the
        // baseline minimum forces the bit reader to overrun.)
        var truncated = nalu.AsSpan(0, 2).ToArray();
        Assert.False(AvcPictureParameterSet.TryParse(truncated, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Surfaces_PostTail_As_Null_When_Posttail_Bytes_Are_Stripped()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            SecondChromaQpIndexOffset = 1,
        });

        // Strip only the trailing post-tail byte. The baseline bits still
        // fit, so more_rbsp_data() correctly returns false and the parser
        // surfaces the post-tail fields as null rather than raising.
        var trimmed = nalu.AsSpan(0, nalu.Length - 1).ToArray();
        Assert.True(AvcPictureParameterSet.TryParse(trimmed, out var pps));
        Assert.NotNull(pps);
        Assert.Null(pps!.Transform8x8ModeFlag);
        Assert.Null(pps.PicScalingMatrixPresentFlag);
        Assert.Null(pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Rejects_MultiSliceGroup_Pps()
    {
        // Multi-slice-group PPSes (num_slice_groups_minus1 > 0) are outside
        // the bounded subset implemented by this parser. They must be
        // rejected deterministically rather than mis-parsed.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            NumSliceGroupsMinus1 = 1,
        });

        Assert.False(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Roundtrips_Through_Emulation_Prevention_Encoding()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 11,
            SeqParameterSetId = 3,
            PicInitQpMinus26 = 4,
        });

        // Walk the RBSP bytes (after NAL header at offset 0) and inject
        // an emulation-prevention 0x03 byte after every 00 00 pair so the
        // parser's StripEmulationPreventionBytes path is exercised end-to-end.
        var encoded = new List<byte> { nalu[0] };
        int zeros = 0;
        for (int i = 1; i < nalu.Length; i++)
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

        Assert.True(AvcPictureParameterSet.TryParse(encoded.ToArray(), out var pps));
        Assert.Equal(11u, pps!.PicParameterSetId);
        Assert.Equal(3u, pps.SeqParameterSetId);
        Assert.Equal(4, pps.PicInitQpMinus26);
    }

    [Fact]
    public void PpsNalUnitType_Constant_Is_Eight()
    {
        Assert.Equal(8, AvcPictureParameterSet.PpsNalUnitType);
    }

    [Theory]
    [InlineData(1)]  // slice_layer_without_partitioning_rbsp
    [InlineData(5)]  // IDR
    [InlineData(6)]  // SEI
    [InlineData(7)]  // SPS
    [InlineData(9)]  // access_unit_delimiter
    [InlineData(10)] // end_of_sequence
    [InlineData(12)] // filler_data
    public void TryParse_Rejects_Various_NonPps_Nal_Types(int nalType)
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 0,
            NalUnitTypeOverride = (byte)nalType,
        });
        Assert.False(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Record_Equality_For_Identical_Bytes()
    {
        var spec = new PpsSpec
        {
            PicParameterSetId = 2,
            SeqParameterSetId = 1,
            EntropyCodingModeFlag = true,
            PicInitQpMinus26 = -3,
        };
        var nalu = PpsBuilder.Build(spec);

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var a));
        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var b));

        Assert.Equal(a, b);
        Assert.Equal(a!.GetHashCode(), b!.GetHashCode());
    }

    [Fact]
    public void TryParse_With_Expression_Returns_Modified_Copy()
    {
        var nalu = PpsBuilder.Build(new PpsSpec { PicParameterSetId = 3 });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        var modified = pps! with { PicParameterSetId = 99 };

        Assert.Equal(99u, modified.PicParameterSetId);
        Assert.Equal(3u, pps.PicParameterSetId); // original unchanged
        Assert.NotSame(pps, modified);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    public void TryParse_Decodes_All_WeightedBipredIdc_Values(byte idc)
    {
        var nalu = PpsBuilder.Build(new PpsSpec { WeightedBipredIdc = idc });
        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(idc, pps!.WeightedBipredIdc);
    }

    [Fact]
    public void TryParse_Decodes_ChromaFormatIdc3_With_Transform8x8_TwelveLists()
    {
        // chroma_format_idc == 3 (4:4:4) with transform_8x8_mode_flag set
        // expands the scaling list count from 6+2=8 to 6+6=12. Verify the
        // parser walks all 12 list_present_flag bits and still finds
        // second_chroma_qp_index_offset where expected.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = true,
            BuilderChromaFormatIdc = 3,
            SecondChromaQpIndexOffset = 8,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps, chromaFormatIdc: 3));
        Assert.True(pps!.Transform8x8ModeFlag);
        Assert.True(pps.PicScalingMatrixPresentFlag);
        Assert.Equal(8, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_ChromaFormatIdc3_Without_Transform8x8_SixLists()
    {
        // chroma_format_idc == 3 but transform_8x8_mode_flag = 0:
        // still only 6 lists (the per-format extras gate on t8x8).
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = false,
            PicScalingMatrixPresentFlag = true,
            BuilderChromaFormatIdc = 3,
            SecondChromaQpIndexOffset = -4,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps, chromaFormatIdc: 3));
        Assert.False(pps!.Transform8x8ModeFlag);
        Assert.True(pps.PicScalingMatrixPresentFlag);
        Assert.Equal(-4, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_ScalingMatrix_With_NonZero_ListPresent_4x4_AllZeroDeltas()
    {
        // Exercise the SkipScalingList path with a real 4x4 list. All
        // delta_scale=0 means nextScale stays at 8 and the parser reads
        // all 16 deltas, then continues to the remaining 7 lists
        // (flag=0 each) and second_chroma_qp_index_offset.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = true,
            ScalingListIndexWithDeltas = 0,
            ScalingListDeltas = new int[16],
            SecondChromaQpIndexOffset = 6,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PicScalingMatrixPresentFlag);
        Assert.Equal(6, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_ScalingMatrix_With_NonZero_ListPresent_8x8_AllZeroDeltas()
    {
        // Same path as above but for an 8x8 list at index 6 (the first
        // 8x8 list). Exercises the size=64 branch of SkipScalingList.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = true,
            ScalingListIndexWithDeltas = 6,
            ScalingListDeltas = new int[64],
            SecondChromaQpIndexOffset = 0,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PicScalingMatrixPresentFlag);
        Assert.Equal(0, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_ScalingList_EarlyTermination_When_NextScale_Hits_Zero()
    {
        // delta_scale = -8 at j=0 gives nextScale = (8 + (-8) + 256) & 0xFF = 0.
        // After that the loop stops reading further deltas. The remaining
        // 7 list_present_flags (all 0) and second_chroma_qp_index_offset
        // must still align to where the parser expects them.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            IncludePostTail = true,
            Transform8x8ModeFlag = true,
            PicScalingMatrixPresentFlag = true,
            ScalingListIndexWithDeltas = 0,
            ScalingListDeltas = new[] { -8 }, // only one delta needs to be present
            SecondChromaQpIndexOffset = -1,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PicScalingMatrixPresentFlag);
        Assert.Equal(-1, pps.SecondChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_Large_Ids()
    {
        // pic_parameter_set_id is uint, but the typical encoder range is
        // small. Sanity-check that a few non-trivial values roundtrip.
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 200,
            SeqParameterSetId = 16,
        });
        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(200u, pps!.PicParameterSetId);
        Assert.Equal(16u, pps.SeqParameterSetId);
    }

    [Fact]
    public void TryParse_Decodes_Large_NumRefIdx_Values()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            NumRefIdxL0DefaultActiveMinus1 = 30,
            NumRefIdxL1DefaultActiveMinus1 = 30,
        });
        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(30u, pps!.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(30u, pps.NumRefIdxL1DefaultActiveMinus1);
    }

    [Fact]
    public void TryParse_Decodes_All_Baseline_Flags_On_Combined()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 1,
            SeqParameterSetId = 1,
            EntropyCodingModeFlag = true,
            BottomFieldPicOrderInFramePresentFlag = true,
            NumRefIdxL0DefaultActiveMinus1 = 4,
            NumRefIdxL1DefaultActiveMinus1 = 4,
            WeightedPredFlag = true,
            WeightedBipredIdc = 2,
            PicInitQpMinus26 = -10,
            PicInitQsMinus26 = 10,
            ChromaQpIndexOffset = -5,
            DeblockingFilterControlPresentFlag = true,
            ConstrainedIntraPredFlag = true,
            RedundantPicCntPresentFlag = true,
        });

        Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.EntropyCodingModeFlag);
        Assert.True(pps.BottomFieldPicOrderInFramePresentFlag);
        Assert.True(pps.WeightedPredFlag);
        Assert.Equal(2, pps.WeightedBipredIdc);
        Assert.True(pps.DeblockingFilterControlPresentFlag);
        Assert.True(pps.ConstrainedIntraPredFlag);
        Assert.True(pps.RedundantPicCntPresentFlag);
        Assert.Equal(-10, pps.PicInitQpMinus26);
        Assert.Equal(10, pps.PicInitQsMinus26);
        Assert.Equal(-5, pps.ChromaQpIndexOffset);
    }

    [Fact]
    public void TryParse_Decodes_Range_Of_Signed_Qp_Values()
    {
        foreach (int qp in new[] { -26, -1, 0, 1, 25 })
        {
            var nalu = PpsBuilder.Build(new PpsSpec
            {
                PicInitQpMinus26 = qp,
                PicInitQsMinus26 = qp,
                ChromaQpIndexOffset = qp,
            });
            Assert.True(AvcPictureParameterSet.TryParse(nalu, out var pps),
                $"PPS with QP fields = {qp} must parse");
            Assert.Equal(qp, pps!.PicInitQpMinus26);
            Assert.Equal(qp, pps.PicInitQsMinus26);
            Assert.Equal(qp, pps.ChromaQpIndexOffset);
        }
    }

    // -----------------------------------------------------------------
    // Test helpers
    // -----------------------------------------------------------------

    private sealed record PpsSpec
    {
        public uint PicParameterSetId { get; init; }
        public uint SeqParameterSetId { get; init; }
        public bool EntropyCodingModeFlag { get; init; }
        public bool BottomFieldPicOrderInFramePresentFlag { get; init; }
        public uint NumSliceGroupsMinus1 { get; init; }
        public uint NumRefIdxL0DefaultActiveMinus1 { get; init; }
        public uint NumRefIdxL1DefaultActiveMinus1 { get; init; }
        public bool WeightedPredFlag { get; init; }
        public byte WeightedBipredIdc { get; init; }
        public int PicInitQpMinus26 { get; init; }
        public int PicInitQsMinus26 { get; init; }
        public int ChromaQpIndexOffset { get; init; }
        public bool DeblockingFilterControlPresentFlag { get; init; }
        public bool ConstrainedIntraPredFlag { get; init; }
        public bool RedundantPicCntPresentFlag { get; init; }

        public bool IncludePostTail { get; init; }
        public bool Transform8x8ModeFlag { get; init; }
        public bool PicScalingMatrixPresentFlag { get; init; }
        public int SecondChromaQpIndexOffset { get; init; }

        public byte NalRefIdc { get; init; } = 3;
        public bool ForbiddenZeroBit { get; init; }
        public byte NalUnitTypeOverride { get; init; } = 8;

        // Test-only: chroma_format_idc the parser will be told to use.
        // The builder uses it to write the right number of
        // pic_scaling_list_present_flag bits when
        // pic_scaling_matrix_present_flag = 1.
        public int BuilderChromaFormatIdc { get; init; } = 1;

        // Test-only: when set, emit pic_scaling_list_present_flag = 1
        // for the list at this index and write the supplied deltas
        // (zero-padded to the list's size).
        public int? ScalingListIndexWithDeltas { get; init; }
        public int[]? ScalingListDeltas { get; init; }
    }

    private static class PpsBuilder
    {
        public static byte[] Build(PpsSpec spec)
        {
            var w = new BitWriter();
            w.WriteUe(spec.PicParameterSetId);
            w.WriteUe(spec.SeqParameterSetId);
            w.WriteBit(spec.EntropyCodingModeFlag);
            w.WriteBit(spec.BottomFieldPicOrderInFramePresentFlag);
            w.WriteUe(spec.NumSliceGroupsMinus1);

            // When num_slice_groups_minus1 > 0 the spec emits a
            // slice_group_map_type-driven sub-stream we don't model here;
            // the parser rejects this configuration up-front so the bits
            // after num_slice_groups_minus1 are interpreted as the
            // num_ref_idx_l0_default_active_minus1 etc. regardless.
            w.WriteUe(spec.NumRefIdxL0DefaultActiveMinus1);
            w.WriteUe(spec.NumRefIdxL1DefaultActiveMinus1);
            w.WriteBit(spec.WeightedPredFlag);
            w.WriteBits(spec.WeightedBipredIdc, 2);
            w.WriteSe(spec.PicInitQpMinus26);
            w.WriteSe(spec.PicInitQsMinus26);
            w.WriteSe(spec.ChromaQpIndexOffset);
            w.WriteBit(spec.DeblockingFilterControlPresentFlag);
            w.WriteBit(spec.ConstrainedIntraPredFlag);
            w.WriteBit(spec.RedundantPicCntPresentFlag);

            if (spec.IncludePostTail)
            {
                w.WriteBit(spec.Transform8x8ModeFlag);
                w.WriteBit(spec.PicScalingMatrixPresentFlag);
                if (spec.PicScalingMatrixPresentFlag)
                {
                    int extraLists = (spec.BuilderChromaFormatIdc == 3 ? 6 : 2)
                                     * (spec.Transform8x8ModeFlag ? 1 : 0);
                    int numLists = 6 + extraLists;
                    for (int i = 0; i < numLists; i++)
                    {
                        bool listPresent = spec.ScalingListIndexWithDeltas == i;
                        w.WriteBit(listPresent);
                        if (listPresent && spec.ScalingListDeltas != null)
                        {
                            // Mirror the parser's SkipScalingList loop: it only
                            // reads delta_scale while nextScale != 0 (per
                            // ITU-T H.264 7.3.2.1.1.1). Emit exactly that many
                            // deltas so the bit stream stays aligned with what
                            // the parser will consume.
                            int size = i < 6 ? 16 : 64;
                            int lastScale = 8;
                            int nextScale = 8;
                            for (int j = 0; j < size; j++)
                            {
                                if (nextScale != 0)
                                {
                                    int delta = j < spec.ScalingListDeltas.Length
                                        ? spec.ScalingListDeltas[j]
                                        : 0;
                                    w.WriteSe(delta);
                                    nextScale = (lastScale + delta + 256) & 0xFF;
                                }
                                if (nextScale != 0)
                                {
                                    lastScale = nextScale;
                                }
                            }
                        }
                    }
                }
                w.WriteSe(spec.SecondChromaQpIndexOffset);
            }

            // rbsp_trailing_bits(): rbsp_stop_one_bit + alignment zero bits
            w.WriteBit(true);
            w.AlignToByte();

            byte[] rbsp = w.ToArray();
            var nalu = new byte[rbsp.Length + 1];
            int forbidden = spec.ForbiddenZeroBit ? 1 : 0;
            nalu[0] = (byte)((forbidden << 7) | ((spec.NalRefIdc & 0x3) << 5) | (spec.NalUnitTypeOverride & 0x1F));
            Array.Copy(rbsp, 0, nalu, 1, rbsp.Length);
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
