using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HevcPictureParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_Minimal_Pps_Without_Tiles_Or_Scaling_Lists()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 0,
            SeqParameterSetId = 0,
            InitQpMinus26 = 0,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.NotNull(pps);
        Assert.Equal(0u, pps!.PicParameterSetId);
        Assert.Equal(0u, pps.SeqParameterSetId);
        Assert.False(pps.TilesEnabledFlag);
        Assert.Equal(1u, pps.NumTileColumns);
        Assert.Equal(1u, pps.NumTileRows);
        Assert.Null(pps.UniformSpacingFlag);
        Assert.Empty(pps.ColumnWidthsMinus1);
        Assert.Empty(pps.RowHeightsMinus1);
        Assert.Null(pps.LoopFilterAcrossTilesEnabledFlag);
        Assert.False(pps.DeblockingFilterControlPresentFlag);
        Assert.Null(pps.DeblockingFilterOverrideEnabledFlag);
        Assert.Null(pps.PpsDeblockingFilterDisabledFlag);
        Assert.Null(pps.PpsBetaOffsetDiv2);
        Assert.Null(pps.PpsTcOffsetDiv2);
        Assert.False(pps.PpsScalingListDataPresentFlag);
        Assert.False(pps.PpsExtensionPresentFlag);
    }

    [Fact]
    public void TryParse_Decodes_Ids_And_Default_Ref_Counts()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PicParameterSetId = 3,
            SeqParameterSetId = 5,
            NumRefIdxL0DefaultActiveMinus1 = 1,
            NumRefIdxL1DefaultActiveMinus1 = 2,
            InitQpMinus26 = -8,
            PpsCbQpOffset = -3,
            PpsCrQpOffset = 4,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Equal(3u, pps!.PicParameterSetId);
        Assert.Equal(5u, pps.SeqParameterSetId);
        Assert.Equal(1u, pps.NumRefIdxL0DefaultActiveMinus1);
        Assert.Equal(2u, pps.NumRefIdxL1DefaultActiveMinus1);
        Assert.Equal(-8, pps.InitQpMinus26);
        Assert.Equal(-3, pps.PpsCbQpOffset);
        Assert.Equal(4, pps.PpsCrQpOffset);
    }

    [Fact]
    public void TryParse_Decodes_CuQpDelta_With_DiffDepth()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            CuQpDeltaEnabledFlag = true,
            DiffCuQpDeltaDepth = 2,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.CuQpDeltaEnabledFlag);
        Assert.Equal(2u, pps.DiffCuQpDeltaDepth);
    }

    [Fact]
    public void TryParse_Decodes_Tiles_With_Uniform_Spacing()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            TilesEnabledFlag = true,
            NumTileColumnsMinus1 = 3,
            NumTileRowsMinus1 = 1,
            UniformSpacingFlag = true,
            LoopFilterAcrossTilesEnabledFlag = true,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.TilesEnabledFlag);
        Assert.Equal(4u, pps.NumTileColumns);
        Assert.Equal(2u, pps.NumTileRows);
        Assert.Equal(true, pps.UniformSpacingFlag);
        Assert.Empty(pps.ColumnWidthsMinus1);
        Assert.Empty(pps.RowHeightsMinus1);
        Assert.Equal(true, pps.LoopFilterAcrossTilesEnabledFlag);
    }

    [Fact]
    public void TryParse_Decodes_Tiles_With_Explicit_Spacing()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            TilesEnabledFlag = true,
            NumTileColumnsMinus1 = 2,
            NumTileRowsMinus1 = 1,
            UniformSpacingFlag = false,
            ColumnWidthsMinus1 = new uint[] { 30, 30 },
            RowHeightsMinus1 = new uint[] { 16 },
            LoopFilterAcrossTilesEnabledFlag = false,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.TilesEnabledFlag);
        Assert.Equal(3u, pps.NumTileColumns);
        Assert.Equal(2u, pps.NumTileRows);
        Assert.Equal(false, pps.UniformSpacingFlag);
        Assert.Equal(new uint[] { 30, 30 }, pps.ColumnWidthsMinus1);
        Assert.Equal(new uint[] { 16 }, pps.RowHeightsMinus1);
        Assert.Equal(false, pps.LoopFilterAcrossTilesEnabledFlag);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_Filter_Control_With_Offsets()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            DeblockingFilterControlPresentFlag = true,
            DeblockingFilterOverrideEnabledFlag = true,
            PpsDeblockingFilterDisabledFlag = false,
            PpsBetaOffsetDiv2 = -2,
            PpsTcOffsetDiv2 = 3,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DeblockingFilterControlPresentFlag);
        Assert.Equal(true, pps.DeblockingFilterOverrideEnabledFlag);
        Assert.Equal(false, pps.PpsDeblockingFilterDisabledFlag);
        Assert.Equal(-2, pps.PpsBetaOffsetDiv2);
        Assert.Equal(3, pps.PpsTcOffsetDiv2);
    }

    [Fact]
    public void TryParse_Decodes_Deblocking_Filter_Control_Disabled_Skips_Offsets()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            DeblockingFilterControlPresentFlag = true,
            DeblockingFilterOverrideEnabledFlag = false,
            PpsDeblockingFilterDisabledFlag = true,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DeblockingFilterControlPresentFlag);
        Assert.Equal(false, pps.DeblockingFilterOverrideEnabledFlag);
        Assert.Equal(true, pps.PpsDeblockingFilterDisabledFlag);
        Assert.Null(pps.PpsBetaOffsetDiv2);
        Assert.Null(pps.PpsTcOffsetDiv2);
    }

    [Fact]
    public void TryParse_Skips_Inline_Scaling_List_Data()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PpsScalingListDataPresentFlag = true,
            ListsModificationPresentFlag = true,
            Log2ParallelMergeLevelMinus2 = 1,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PpsScalingListDataPresentFlag);
        Assert.True(pps.ListsModificationPresentFlag);
        Assert.Equal(1u, pps.Log2ParallelMergeLevelMinus2);
    }

    [Fact]
    public void TryParse_Decodes_Extension_Flag_Without_Decoding_Extension_Body()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            PpsExtensionPresentFlag = true,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.PpsExtensionPresentFlag);
    }

    [Fact]
    public void TryParse_Decodes_Flag_Fields()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            DependentSliceSegmentsEnabledFlag = true,
            OutputFlagPresentFlag = true,
            NumExtraSliceHeaderBits = 5,
            SignDataHidingEnabledFlag = true,
            CabacInitPresentFlag = true,
            ConstrainedIntraPredFlag = true,
            TransformSkipEnabledFlag = true,
            PpsSliceChromaQpOffsetsPresentFlag = true,
            WeightedPredFlag = true,
            WeightedBipredFlag = true,
            TransquantBypassEnabledFlag = true,
            EntropyCodingSyncEnabledFlag = true,
            PpsLoopFilterAcrossSlicesEnabledFlag = true,
            SliceSegmentHeaderExtensionPresentFlag = true,
        });

        Assert.True(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.True(pps!.DependentSliceSegmentsEnabledFlag);
        Assert.True(pps.OutputFlagPresentFlag);
        Assert.Equal((byte)5, pps.NumExtraSliceHeaderBits);
        Assert.True(pps.SignDataHidingEnabledFlag);
        Assert.True(pps.CabacInitPresentFlag);
        Assert.True(pps.ConstrainedIntraPredFlag);
        Assert.True(pps.TransformSkipEnabledFlag);
        Assert.True(pps.PpsSliceChromaQpOffsetsPresentFlag);
        Assert.True(pps.WeightedPredFlag);
        Assert.True(pps.WeightedBipredFlag);
        Assert.True(pps.TransquantBypassEnabledFlag);
        Assert.True(pps.EntropyCodingSyncEnabledFlag);
        Assert.True(pps.PpsLoopFilterAcrossSlicesEnabledFlag);
        Assert.True(pps.SliceSegmentHeaderExtensionPresentFlag);
    }

    [Fact]
    public void TryParse_Rejects_NonPps_NalUnitType()
    {
        byte[] nalu = PpsBuilder.Build(new PpsSpec());
        // Replace NAL type with SPS_NUT (33) instead of PPS_NUT (34).
        nalu[0] = (byte)(33 << 1);

        Assert.False(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Zero_Bit_Set()
    {
        byte[] nalu = PpsBuilder.Build(new PpsSpec());
        nalu[0] |= 0x80; // set forbidden_zero_bit

        Assert.False(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        byte[] tiny = new byte[] { 0x44 };
        Assert.False(HevcPictureParameterSet.TryParse(tiny, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        // Header but the RBSP is just zeros — first ue() field will eventually
        // exhaust the buffer.
        byte[] short_ = new byte[] { 0x44, 0x01, 0x00 };
        Assert.False(HevcPictureParameterSet.TryParse(short_, out var pps));
        Assert.Null(pps);
    }

    [Fact]
    public void TryParse_Rejects_Tile_Column_Count_Over_Limit()
    {
        var nalu = PpsBuilder.Build(new PpsSpec
        {
            TilesEnabledFlag = true,
            NumTileColumnsMinus1 = 512,
            NumTileRowsMinus1 = 0,
            UniformSpacingFlag = true,
            LoopFilterAcrossTilesEnabledFlag = true,
        });

        Assert.False(HevcPictureParameterSet.TryParse(nalu, out var pps));
        Assert.Null(pps);
    }

    private sealed class PpsSpec
    {
        public uint PicParameterSetId { get; init; }
        public uint SeqParameterSetId { get; init; }
        public bool DependentSliceSegmentsEnabledFlag { get; init; }
        public bool OutputFlagPresentFlag { get; init; }
        public byte NumExtraSliceHeaderBits { get; init; }
        public bool SignDataHidingEnabledFlag { get; init; }
        public bool CabacInitPresentFlag { get; init; }
        public uint NumRefIdxL0DefaultActiveMinus1 { get; init; }
        public uint NumRefIdxL1DefaultActiveMinus1 { get; init; }
        public int InitQpMinus26 { get; init; }
        public bool ConstrainedIntraPredFlag { get; init; }
        public bool TransformSkipEnabledFlag { get; init; }
        public bool CuQpDeltaEnabledFlag { get; init; }
        public uint DiffCuQpDeltaDepth { get; init; }
        public int PpsCbQpOffset { get; init; }
        public int PpsCrQpOffset { get; init; }
        public bool PpsSliceChromaQpOffsetsPresentFlag { get; init; }
        public bool WeightedPredFlag { get; init; }
        public bool WeightedBipredFlag { get; init; }
        public bool TransquantBypassEnabledFlag { get; init; }
        public bool TilesEnabledFlag { get; init; }
        public bool EntropyCodingSyncEnabledFlag { get; init; }
        public uint NumTileColumnsMinus1 { get; init; }
        public uint NumTileRowsMinus1 { get; init; }
        public bool UniformSpacingFlag { get; init; }
        public uint[] ColumnWidthsMinus1 { get; init; } = Array.Empty<uint>();
        public uint[] RowHeightsMinus1 { get; init; } = Array.Empty<uint>();
        public bool LoopFilterAcrossTilesEnabledFlag { get; init; }
        public bool PpsLoopFilterAcrossSlicesEnabledFlag { get; init; }
        public bool DeblockingFilterControlPresentFlag { get; init; }
        public bool DeblockingFilterOverrideEnabledFlag { get; init; }
        public bool PpsDeblockingFilterDisabledFlag { get; init; }
        public int PpsBetaOffsetDiv2 { get; init; }
        public int PpsTcOffsetDiv2 { get; init; }
        public bool PpsScalingListDataPresentFlag { get; init; }
        public bool ListsModificationPresentFlag { get; init; }
        public uint Log2ParallelMergeLevelMinus2 { get; init; }
        public bool SliceSegmentHeaderExtensionPresentFlag { get; init; }
        public bool PpsExtensionPresentFlag { get; init; }
    }

    private static class PpsBuilder
    {
        public static byte[] Build(PpsSpec spec)
        {
            var w = new BitWriter();
            w.WriteUe(spec.PicParameterSetId);
            w.WriteUe(spec.SeqParameterSetId);
            w.WriteBit(spec.DependentSliceSegmentsEnabledFlag);
            w.WriteBit(spec.OutputFlagPresentFlag);
            w.WriteBits(spec.NumExtraSliceHeaderBits, 3);
            w.WriteBit(spec.SignDataHidingEnabledFlag);
            w.WriteBit(spec.CabacInitPresentFlag);
            w.WriteUe(spec.NumRefIdxL0DefaultActiveMinus1);
            w.WriteUe(spec.NumRefIdxL1DefaultActiveMinus1);
            w.WriteSe(spec.InitQpMinus26);
            w.WriteBit(spec.ConstrainedIntraPredFlag);
            w.WriteBit(spec.TransformSkipEnabledFlag);
            w.WriteBit(spec.CuQpDeltaEnabledFlag);
            if (spec.CuQpDeltaEnabledFlag) w.WriteUe(spec.DiffCuQpDeltaDepth);
            w.WriteSe(spec.PpsCbQpOffset);
            w.WriteSe(spec.PpsCrQpOffset);
            w.WriteBit(spec.PpsSliceChromaQpOffsetsPresentFlag);
            w.WriteBit(spec.WeightedPredFlag);
            w.WriteBit(spec.WeightedBipredFlag);
            w.WriteBit(spec.TransquantBypassEnabledFlag);
            w.WriteBit(spec.TilesEnabledFlag);
            w.WriteBit(spec.EntropyCodingSyncEnabledFlag);
            if (spec.TilesEnabledFlag)
            {
                w.WriteUe(spec.NumTileColumnsMinus1);
                w.WriteUe(spec.NumTileRowsMinus1);
                w.WriteBit(spec.UniformSpacingFlag);
                if (!spec.UniformSpacingFlag)
                {
                    for (int i = 0; i < spec.NumTileColumnsMinus1; i++)
                        w.WriteUe(spec.ColumnWidthsMinus1[i]);
                    for (int i = 0; i < spec.NumTileRowsMinus1; i++)
                        w.WriteUe(spec.RowHeightsMinus1[i]);
                }
                w.WriteBit(spec.LoopFilterAcrossTilesEnabledFlag);
            }
            w.WriteBit(spec.PpsLoopFilterAcrossSlicesEnabledFlag);
            w.WriteBit(spec.DeblockingFilterControlPresentFlag);
            if (spec.DeblockingFilterControlPresentFlag)
            {
                w.WriteBit(spec.DeblockingFilterOverrideEnabledFlag);
                w.WriteBit(spec.PpsDeblockingFilterDisabledFlag);
                if (!spec.PpsDeblockingFilterDisabledFlag)
                {
                    w.WriteSe(spec.PpsBetaOffsetDiv2);
                    w.WriteSe(spec.PpsTcOffsetDiv2);
                }
            }
            w.WriteBit(spec.PpsScalingListDataPresentFlag);
            if (spec.PpsScalingListDataPresentFlag)
            {
                // Minimal scaling list data: every (sizeId, matrixId)
                // sets pred_mode_flag=0 and writes a pred_matrix_id_delta
                // of 0 (single 1-bit zero ue codeword), exercising the
                // skip path without inflating the bitstream.
                for (int sizeId = 0; sizeId < 4; sizeId++)
                {
                    int matrixCount = sizeId == 3 ? 2 : 6;
                    for (int matrixId = 0; matrixId < matrixCount; matrixId++)
                    {
                        w.WriteBit(false);
                        w.WriteUe(0);
                    }
                }
            }
            w.WriteBit(spec.ListsModificationPresentFlag);
            w.WriteUe(spec.Log2ParallelMergeLevelMinus2);
            w.WriteBit(spec.SliceSegmentHeaderExtensionPresentFlag);
            w.WriteBit(spec.PpsExtensionPresentFlag);

            byte[] rbsp = w.ToArray();
            // Prepend 2-byte NAL header: forbidden_zero=0, type=34, layer_id=0, temporal_id_plus1=1.
            var nalu = new byte[rbsp.Length + 2];
            nalu[0] = 34 << 1;
            nalu[1] = 0x01;
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
