using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over an HEVC Picture Parameter Set carried as a
/// length-prefixed NAL unit inside an <c>hvcC</c> parameter-set
/// array. Decodes the PPS RBSP per ITU-T H.265 7.3.2.3.1 up to and
/// including <c>pps_extension_present_flag</c>; the per-extension
/// blocks (range, multilayer, 3d, scc) are intentionally not
/// decoded. Inline scaling list data is skipped via a bit-accurate
/// reader so PPSes that carry their own scaling matrices still
/// parse cleanly.
/// </summary>
public sealed record HevcPictureParameterSet
{
    /// <summary>PPS_NUT in the HEVC NAL unit type registry (34).</summary>
    public const int PpsNalUnitType = 34;

    private const int MaxTileColumns = 256;
    private const int MaxTileRows = 256;

    /// <summary><c>pps_pic_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint PicParameterSetId { get; init; }

    /// <summary><c>pps_seq_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint SeqParameterSetId { get; init; }

    /// <summary><c>dependent_slice_segments_enabled_flag</c>.</summary>
    public required bool DependentSliceSegmentsEnabledFlag { get; init; }

    /// <summary><c>output_flag_present_flag</c>.</summary>
    public required bool OutputFlagPresentFlag { get; init; }

    /// <summary>3-bit <c>num_extra_slice_header_bits</c>.</summary>
    public required byte NumExtraSliceHeaderBits { get; init; }

    /// <summary><c>sign_data_hiding_enabled_flag</c>.</summary>
    public required bool SignDataHidingEnabledFlag { get; init; }

    /// <summary><c>cabac_init_present_flag</c>.</summary>
    public required bool CabacInitPresentFlag { get; init; }

    /// <summary><c>num_ref_idx_l0_default_active_minus1</c>.</summary>
    public required uint NumRefIdxL0DefaultActiveMinus1 { get; init; }

    /// <summary><c>num_ref_idx_l1_default_active_minus1</c>.</summary>
    public required uint NumRefIdxL1DefaultActiveMinus1 { get; init; }

    /// <summary><c>init_qp_minus26</c>, signed exp-Golomb decoded.</summary>
    public required int InitQpMinus26 { get; init; }

    /// <summary><c>constrained_intra_pred_flag</c>.</summary>
    public required bool ConstrainedIntraPredFlag { get; init; }

    /// <summary><c>transform_skip_enabled_flag</c>.</summary>
    public required bool TransformSkipEnabledFlag { get; init; }

    /// <summary><c>cu_qp_delta_enabled_flag</c>.</summary>
    public required bool CuQpDeltaEnabledFlag { get; init; }

    /// <summary><c>diff_cu_qp_delta_depth</c> when
    /// <see cref="CuQpDeltaEnabledFlag"/> is true; otherwise null.</summary>
    public required uint? DiffCuQpDeltaDepth { get; init; }

    /// <summary><c>pps_cb_qp_offset</c>, signed exp-Golomb decoded.</summary>
    public required int PpsCbQpOffset { get; init; }

    /// <summary><c>pps_cr_qp_offset</c>, signed exp-Golomb decoded.</summary>
    public required int PpsCrQpOffset { get; init; }

    /// <summary><c>pps_slice_chroma_qp_offsets_present_flag</c>.</summary>
    public required bool PpsSliceChromaQpOffsetsPresentFlag { get; init; }

    /// <summary><c>weighted_pred_flag</c>.</summary>
    public required bool WeightedPredFlag { get; init; }

    /// <summary><c>weighted_bipred_flag</c>.</summary>
    public required bool WeightedBipredFlag { get; init; }

    /// <summary><c>transquant_bypass_enabled_flag</c>.</summary>
    public required bool TransquantBypassEnabledFlag { get; init; }

    /// <summary><c>tiles_enabled_flag</c>.</summary>
    public required bool TilesEnabledFlag { get; init; }

    /// <summary><c>entropy_coding_sync_enabled_flag</c>.</summary>
    public required bool EntropyCodingSyncEnabledFlag { get; init; }

    /// <summary>Total tile column count (<c>num_tile_columns_minus1 + 1</c>)
    /// when <see cref="TilesEnabledFlag"/> is true; 1 otherwise.</summary>
    public required uint NumTileColumns { get; init; }

    /// <summary>Total tile row count (<c>num_tile_rows_minus1 + 1</c>)
    /// when <see cref="TilesEnabledFlag"/> is true; 1 otherwise.</summary>
    public required uint NumTileRows { get; init; }

    /// <summary><c>uniform_spacing_flag</c> when
    /// <see cref="TilesEnabledFlag"/> is true; otherwise null.</summary>
    public required bool? UniformSpacingFlag { get; init; }

    /// <summary>Per-column <c>column_width_minus1[i]</c> values when
    /// tiles are enabled with explicit (non-uniform) spacing;
    /// otherwise empty.</summary>
    public required ImmutableArray<uint> ColumnWidthsMinus1 { get; init; }

    /// <summary>Per-row <c>row_height_minus1[i]</c> values when tiles
    /// are enabled with explicit (non-uniform) spacing; otherwise
    /// empty.</summary>
    public required ImmutableArray<uint> RowHeightsMinus1 { get; init; }

    /// <summary><c>loop_filter_across_tiles_enabled_flag</c> when
    /// <see cref="TilesEnabledFlag"/> is true; otherwise null.</summary>
    public required bool? LoopFilterAcrossTilesEnabledFlag { get; init; }

    /// <summary><c>pps_loop_filter_across_slices_enabled_flag</c>.</summary>
    public required bool PpsLoopFilterAcrossSlicesEnabledFlag { get; init; }

    /// <summary><c>deblocking_filter_control_present_flag</c>.</summary>
    public required bool DeblockingFilterControlPresentFlag { get; init; }

    /// <summary><c>deblocking_filter_override_enabled_flag</c> when
    /// <see cref="DeblockingFilterControlPresentFlag"/> is true;
    /// otherwise null.</summary>
    public required bool? DeblockingFilterOverrideEnabledFlag { get; init; }

    /// <summary><c>pps_deblocking_filter_disabled_flag</c> when
    /// <see cref="DeblockingFilterControlPresentFlag"/> is true;
    /// otherwise null.</summary>
    public required bool? PpsDeblockingFilterDisabledFlag { get; init; }

    /// <summary><c>pps_beta_offset_div2</c> when deblocking-filter
    /// control is present and the filter is not disabled; otherwise
    /// null.</summary>
    public required int? PpsBetaOffsetDiv2 { get; init; }

    /// <summary><c>pps_tc_offset_div2</c> when deblocking-filter
    /// control is present and the filter is not disabled; otherwise
    /// null.</summary>
    public required int? PpsTcOffsetDiv2 { get; init; }

    /// <summary><c>pps_scaling_list_data_present_flag</c>.</summary>
    public required bool PpsScalingListDataPresentFlag { get; init; }

    /// <summary><c>lists_modification_present_flag</c>.</summary>
    public required bool ListsModificationPresentFlag { get; init; }

    /// <summary><c>log2_parallel_merge_level_minus2</c>.</summary>
    public required uint Log2ParallelMergeLevelMinus2 { get; init; }

    /// <summary><c>slice_segment_header_extension_present_flag</c>.</summary>
    public required bool SliceSegmentHeaderExtensionPresentFlag { get; init; }

    /// <summary><c>pps_extension_present_flag</c>. When true the PPS
    /// carries one or more profile-specific extension blocks; this
    /// parser stops decoding at that point and the per-extension
    /// data is not surfaced.</summary>
    public required bool PpsExtensionPresentFlag { get; init; }

    /// <summary>
    /// Parses an HEVC PPS NAL unit. Expects a 2-byte NAL unit header
    /// (<c>nal_unit_type</c> must be 34, PPS_NUT) followed by the PPS
    /// RBSP, optionally containing emulation prevention bytes
    /// (<c>0x00 0x00 0x03</c>) which are stripped before bit
    /// decoding. Returns false on any structural violation.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out HevcPictureParameterSet? pps)
    {
        pps = null;
        if (nalUnit.Length < 3) return false;

        if ((nalUnit[0] & 0x80) != 0) return false;
        int nalUnitType = (nalUnit[0] >> 1) & 0x3F;
        if (nalUnitType != PpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit.Slice(2));
        var reader = new NalUnitBitReader(rbsp);

        try
        {
            uint ppsId = reader.ReadUe();
            uint spsId = reader.ReadUe();
            bool depSliceSeg = reader.ReadBit();
            bool outputFlag = reader.ReadBit();
            byte numExtra = (byte)reader.ReadBits(3);
            bool signHiding = reader.ReadBit();
            bool cabacInit = reader.ReadBit();
            uint numL0 = reader.ReadUe();
            uint numL1 = reader.ReadUe();
            int initQp = reader.ReadSe();
            bool constrIntra = reader.ReadBit();
            bool tsEnabled = reader.ReadBit();
            bool cuQpDelta = reader.ReadBit();
            uint? diffCuQpDepth = cuQpDelta ? reader.ReadUe() : null;
            int cbQp = reader.ReadSe();
            int crQp = reader.ReadSe();
            bool sliceChromaQp = reader.ReadBit();
            bool wPred = reader.ReadBit();
            bool wBipred = reader.ReadBit();
            bool tqBypass = reader.ReadBit();
            bool tilesEnabled = reader.ReadBit();
            bool entropySync = reader.ReadBit();

            uint numColumns = 1;
            uint numRows = 1;
            bool? uniformSpacing = null;
            ImmutableArray<uint> columnWidthsMinus1 = ImmutableArray<uint>.Empty;
            ImmutableArray<uint> rowHeightsMinus1 = ImmutableArray<uint>.Empty;
            bool? loopFilterAcrossTiles = null;
            if (tilesEnabled)
            {
                uint numColsM1 = reader.ReadUe();
                uint numRowsM1 = reader.ReadUe();
                if (numColsM1 >= MaxTileColumns || numRowsM1 >= MaxTileRows) return false;
                numColumns = numColsM1 + 1;
                numRows = numRowsM1 + 1;
                bool uniform = reader.ReadBit();
                uniformSpacing = uniform;
                if (!uniform)
                {
                    var cwBuilder = ImmutableArray.CreateBuilder<uint>((int)numColsM1);
                    for (int i = 0; i < numColsM1; i++) cwBuilder.Add(reader.ReadUe());
                    columnWidthsMinus1 = cwBuilder.ToImmutable();
                    var rhBuilder = ImmutableArray.CreateBuilder<uint>((int)numRowsM1);
                    for (int i = 0; i < numRowsM1; i++) rhBuilder.Add(reader.ReadUe());
                    rowHeightsMinus1 = rhBuilder.ToImmutable();
                }
                loopFilterAcrossTiles = reader.ReadBit();
            }

            bool loopFilterAcrossSlices = reader.ReadBit();
            bool deblockingCtrl = reader.ReadBit();
            bool? deblockOverride = null;
            bool? deblockDisabled = null;
            int? betaOffset = null;
            int? tcOffset = null;
            if (deblockingCtrl)
            {
                deblockOverride = reader.ReadBit();
                bool disabled = reader.ReadBit();
                deblockDisabled = disabled;
                if (!disabled)
                {
                    betaOffset = reader.ReadSe();
                    tcOffset = reader.ReadSe();
                }
            }

            bool scalingListDataPresent = reader.ReadBit();
            if (scalingListDataPresent)
            {
                SkipScalingListData(ref reader);
            }

            bool listsModification = reader.ReadBit();
            uint log2ParallelMerge = reader.ReadUe();
            bool sliceSegExtPresent = reader.ReadBit();
            bool ppsExtPresent = reader.ReadBit();

            pps = new HevcPictureParameterSet
            {
                PicParameterSetId = ppsId,
                SeqParameterSetId = spsId,
                DependentSliceSegmentsEnabledFlag = depSliceSeg,
                OutputFlagPresentFlag = outputFlag,
                NumExtraSliceHeaderBits = numExtra,
                SignDataHidingEnabledFlag = signHiding,
                CabacInitPresentFlag = cabacInit,
                NumRefIdxL0DefaultActiveMinus1 = numL0,
                NumRefIdxL1DefaultActiveMinus1 = numL1,
                InitQpMinus26 = initQp,
                ConstrainedIntraPredFlag = constrIntra,
                TransformSkipEnabledFlag = tsEnabled,
                CuQpDeltaEnabledFlag = cuQpDelta,
                DiffCuQpDeltaDepth = diffCuQpDepth,
                PpsCbQpOffset = cbQp,
                PpsCrQpOffset = crQp,
                PpsSliceChromaQpOffsetsPresentFlag = sliceChromaQp,
                WeightedPredFlag = wPred,
                WeightedBipredFlag = wBipred,
                TransquantBypassEnabledFlag = tqBypass,
                TilesEnabledFlag = tilesEnabled,
                EntropyCodingSyncEnabledFlag = entropySync,
                NumTileColumns = numColumns,
                NumTileRows = numRows,
                UniformSpacingFlag = uniformSpacing,
                ColumnWidthsMinus1 = columnWidthsMinus1,
                RowHeightsMinus1 = rowHeightsMinus1,
                LoopFilterAcrossTilesEnabledFlag = loopFilterAcrossTiles,
                PpsLoopFilterAcrossSlicesEnabledFlag = loopFilterAcrossSlices,
                DeblockingFilterControlPresentFlag = deblockingCtrl,
                DeblockingFilterOverrideEnabledFlag = deblockOverride,
                PpsDeblockingFilterDisabledFlag = deblockDisabled,
                PpsBetaOffsetDiv2 = betaOffset,
                PpsTcOffsetDiv2 = tcOffset,
                PpsScalingListDataPresentFlag = scalingListDataPresent,
                ListsModificationPresentFlag = listsModification,
                Log2ParallelMergeLevelMinus2 = log2ParallelMerge,
                SliceSegmentHeaderExtensionPresentFlag = sliceSegExtPresent,
                PpsExtensionPresentFlag = ppsExtPresent,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            pps = null;
            return false;
        }
    }

    private static void SkipScalingListData(ref NalUnitBitReader reader)
    {
        for (int sizeId = 0; sizeId < 4; sizeId++)
        {
            int matrixCount = sizeId == 3 ? 2 : 6;
            for (int matrixId = 0; matrixId < matrixCount; matrixId++)
            {
                bool predMode = reader.ReadBit();
                if (!predMode)
                {
                    _ = reader.ReadUe();
                }
                else
                {
                    if (sizeId > 1) _ = reader.ReadSe();
                    int coefNum = Math.Min(64, 1 << (4 + (sizeId << 1)));
                    for (int i = 0; i < coefNum; i++) _ = reader.ReadSe();
                }
            }
        }
    }
}
