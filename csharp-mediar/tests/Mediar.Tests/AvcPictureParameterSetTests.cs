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
                    // chroma_format_idc != 3: 6 + 2 * transform_8x8 lists.
                    int numLists = 6 + (spec.Transform8x8ModeFlag ? 2 : 0);
                    for (int i = 0; i < numLists; i++)
                    {
                        w.WriteBit(false); // pic_scaling_list_present_flag = 0 — skipped
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
