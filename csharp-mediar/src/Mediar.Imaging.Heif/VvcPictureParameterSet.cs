using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over a VVC (H.266) Picture Parameter Set carried as a
/// length-prefixed NAL unit inside a <c>vvcC</c> parameter-set array.
/// Decodes the PPS RBSP per ITU-T H.266 7.3.2.4 covering the leading
/// picture-geometry, conformance/scaling window, reference-list,
/// quantization, chroma-tool-offset, deblocking-filter, and extension
/// blocks. The picture-partition sub-stream (tiles, slices, subpic id
/// mapping) is intentionally outside scope so the parser is rejected
/// deterministically when partitioning is signaled.
/// </summary>
public sealed record VvcPictureParameterSet
{
    /// <summary>PPS_NUT in the VVC NAL unit type registry (16).</summary>
    public const int PpsNalUnitType = 16;

    /// <summary>6-bit <c>pps_pic_parameter_set_id</c>.</summary>
    public required byte PicParameterSetId { get; init; }

    /// <summary>4-bit <c>pps_seq_parameter_set_id</c>.</summary>
    public required byte SeqParameterSetId { get; init; }

    /// <summary><c>pps_mixed_nalu_types_in_pic_flag</c>.</summary>
    public required bool MixedNaluTypesInPicFlag { get; init; }

    /// <summary><c>pps_pic_width_in_luma_samples</c>.</summary>
    public required uint PicWidthInLumaSamples { get; init; }

    /// <summary><c>pps_pic_height_in_luma_samples</c>.</summary>
    public required uint PicHeightInLumaSamples { get; init; }

    /// <summary><c>pps_conformance_window_flag</c>.</summary>
    public required bool ConformanceWindowFlag { get; init; }

    /// <summary><c>pps_conf_win_left_offset</c> when conformance window
    /// signaling is present; otherwise null.</summary>
    public required uint? ConfWinLeftOffset { get; init; }

    /// <summary><c>pps_conf_win_right_offset</c> when conformance
    /// window signaling is present; otherwise null.</summary>
    public required uint? ConfWinRightOffset { get; init; }

    /// <summary><c>pps_conf_win_top_offset</c> when conformance window
    /// signaling is present; otherwise null.</summary>
    public required uint? ConfWinTopOffset { get; init; }

    /// <summary><c>pps_conf_win_bottom_offset</c> when conformance
    /// window signaling is present; otherwise null.</summary>
    public required uint? ConfWinBottomOffset { get; init; }

    /// <summary><c>pps_scaling_window_explicit_signalling_flag</c>.</summary>
    public required bool ScalingWindowExplicitSignallingFlag { get; init; }

    /// <summary><c>pps_scaling_win_left_offset</c> when explicit scaling
    /// window signaling is present; otherwise null.</summary>
    public required int? ScalingWinLeftOffset { get; init; }

    /// <summary><c>pps_scaling_win_right_offset</c>.</summary>
    public required int? ScalingWinRightOffset { get; init; }

    /// <summary><c>pps_scaling_win_top_offset</c>.</summary>
    public required int? ScalingWinTopOffset { get; init; }

    /// <summary><c>pps_scaling_win_bottom_offset</c>.</summary>
    public required int? ScalingWinBottomOffset { get; init; }

    /// <summary><c>pps_output_flag_present_flag</c>.</summary>
    public required bool OutputFlagPresentFlag { get; init; }

    /// <summary><c>pps_no_pic_partition_flag</c>. Always true on a
    /// successful parse; this parser does not decode the picture
    /// partition sub-stream.</summary>
    public required bool NoPicPartitionFlag { get; init; }

    /// <summary><c>pps_subpic_id_mapping_present_flag</c>. Always
    /// false on a successful parse; explicit subpic id mapping
    /// requires the corresponding SPS to derive
    /// <c>num_subpics_minus1</c> when picture partitioning is
    /// disabled and is therefore rejected.</summary>
    public required bool SubpicIdMappingPresentFlag { get; init; }

    /// <summary><c>pps_cabac_init_present_flag</c>.</summary>
    public required bool CabacInitPresentFlag { get; init; }

    /// <summary><c>pps_num_ref_idx_default_active_minus1[0]</c>.</summary>
    public required uint NumRefIdxL0DefaultActiveMinus1 { get; init; }

    /// <summary><c>pps_num_ref_idx_default_active_minus1[1]</c>.</summary>
    public required uint NumRefIdxL1DefaultActiveMinus1 { get; init; }

    /// <summary><c>pps_rpl1_idx_present_flag</c>.</summary>
    public required bool Rpl1IdxPresentFlag { get; init; }

    /// <summary><c>pps_weighted_pred_flag</c>.</summary>
    public required bool WeightedPredFlag { get; init; }

    /// <summary><c>pps_weighted_bipred_flag</c>.</summary>
    public required bool WeightedBipredFlag { get; init; }

    /// <summary><c>pps_ref_wraparound_enabled_flag</c>.</summary>
    public required bool RefWraparoundEnabledFlag { get; init; }

    /// <summary><c>pps_pic_width_minus_wraparound_offset</c> when
    /// reference wraparound is enabled; otherwise null.</summary>
    public required uint? PicWidthMinusWraparoundOffset { get; init; }

    /// <summary><c>pps_init_qp_minus26</c>, signed exp-Golomb decoded.</summary>
    public required int InitQpMinus26 { get; init; }

    /// <summary><c>pps_cu_qp_delta_enabled_flag</c>.</summary>
    public required bool CuQpDeltaEnabledFlag { get; init; }

    /// <summary><c>pps_chroma_tool_offsets_present_flag</c>.</summary>
    public required bool ChromaToolOffsetsPresentFlag { get; init; }

    /// <summary><c>pps_cb_qp_offset</c> when chroma tool offsets are
    /// present; otherwise null.</summary>
    public required int? CbQpOffset { get; init; }

    /// <summary><c>pps_cr_qp_offset</c> when chroma tool offsets are
    /// present; otherwise null.</summary>
    public required int? CrQpOffset { get; init; }

    /// <summary><c>pps_joint_cbcr_qp_offset_present_flag</c> when
    /// chroma tool offsets are present; otherwise null.</summary>
    public required bool? JointCbCrQpOffsetPresentFlag { get; init; }

    /// <summary><c>pps_joint_cbcr_qp_offset_value</c> when the joint
    /// Cb/Cr offset flag is also set; otherwise null.</summary>
    public required int? JointCbCrQpOffsetValue { get; init; }

    /// <summary><c>pps_slice_chroma_qp_offsets_present_flag</c> when
    /// chroma tool offsets are present; otherwise null.</summary>
    public required bool? SliceChromaQpOffsetsPresentFlag { get; init; }

    /// <summary><c>pps_cu_chroma_qp_offset_list_enabled_flag</c> when
    /// chroma tool offsets are present; otherwise null.</summary>
    public required bool? CuChromaQpOffsetListEnabledFlag { get; init; }

    /// <summary>Per-entry <c>pps_cb_qp_offset_list[i]</c> when the
    /// CU chroma QP offset list is enabled; otherwise empty.</summary>
    public required ImmutableArray<int> CbQpOffsetList { get; init; }

    /// <summary>Per-entry <c>pps_cr_qp_offset_list[i]</c>.</summary>
    public required ImmutableArray<int> CrQpOffsetList { get; init; }

    /// <summary>Per-entry <c>pps_joint_cbcr_qp_offset_list[i]</c>
    /// when both the chroma QP offset list is enabled and the joint
    /// Cb/Cr offset flag is set; otherwise empty.</summary>
    public required ImmutableArray<int> JointCbCrQpOffsetList { get; init; }

    /// <summary><c>pps_deblocking_filter_control_present_flag</c>.</summary>
    public required bool DeblockingFilterControlPresentFlag { get; init; }

    /// <summary><c>pps_deblocking_filter_override_enabled_flag</c>
    /// when deblocking filter control is present; otherwise null.</summary>
    public required bool? DeblockingFilterOverrideEnabledFlag { get; init; }

    /// <summary><c>pps_deblocking_filter_disabled_flag</c> when
    /// deblocking filter control is present; otherwise null.</summary>
    public required bool? DeblockingFilterDisabledFlag { get; init; }

    /// <summary><c>pps_luma_beta_offset_div2</c> when deblocking
    /// filter control is present and the filter is not disabled;
    /// otherwise null.</summary>
    public required int? LumaBetaOffsetDiv2 { get; init; }

    /// <summary><c>pps_luma_tc_offset_div2</c>.</summary>
    public required int? LumaTcOffsetDiv2 { get; init; }

    /// <summary><c>pps_cb_beta_offset_div2</c> when both deblocking
    /// filter control and chroma tool offsets are present and the
    /// filter is not disabled; otherwise null.</summary>
    public required int? CbBetaOffsetDiv2 { get; init; }

    /// <summary><c>pps_cb_tc_offset_div2</c>.</summary>
    public required int? CbTcOffsetDiv2 { get; init; }

    /// <summary><c>pps_cr_beta_offset_div2</c>.</summary>
    public required int? CrBetaOffsetDiv2 { get; init; }

    /// <summary><c>pps_cr_tc_offset_div2</c>.</summary>
    public required int? CrTcOffsetDiv2 { get; init; }

    /// <summary><c>pps_picture_header_extension_present_flag</c>.</summary>
    public required bool PictureHeaderExtensionPresentFlag { get; init; }

    /// <summary><c>pps_slice_header_extension_present_flag</c>.</summary>
    public required bool SliceHeaderExtensionPresentFlag { get; init; }

    /// <summary><c>pps_extension_flag</c>. When true the PPS carries
    /// further extension data which this parser discards.</summary>
    public required bool ExtensionFlag { get; init; }

    /// <summary>
    /// Parses a VVC PPS NAL unit. Expects a 2-byte NAL unit header
    /// (forbidden_zero_bit = 0, nuh_reserved_zero_bit = 0,
    /// <c>nal_unit_type</c> = 16 PPS_NUT) followed by the PPS RBSP,
    /// optionally containing emulation prevention bytes
    /// (<c>0x00 0x00 0x03</c>) which are stripped before bit
    /// decoding. Returns false on any structural violation, on
    /// PPSes that signal picture partitioning
    /// (<c>pps_no_pic_partition_flag = 0</c>), and on PPSes that
    /// carry explicit subpic id mapping
    /// (<c>pps_subpic_id_mapping_present_flag = 1</c>).
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out VvcPictureParameterSet? pps)
    {
        pps = null;
        if (nalUnit.Length < 3) return false;

        // forbidden_zero_bit and nuh_reserved_zero_bit must both be 0.
        if ((nalUnit[0] & 0xC0) != 0) return false;
        int nalUnitType = (nalUnit[1] >> 3) & 0x1F;
        if (nalUnitType != PpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit.Slice(2));
        var reader = new NalUnitBitReader(rbsp);

        try
        {
            byte ppsId = (byte)reader.ReadBits(6);
            byte spsId = (byte)reader.ReadBits(4);
            bool mixedNaluTypes = reader.ReadBit();
            uint picWidth = reader.ReadUe();
            uint picHeight = reader.ReadUe();
            if (picWidth == 0 || picHeight == 0) return false;

            bool confWindowFlag = reader.ReadBit();
            uint? confLeft = null, confRight = null, confTop = null, confBottom = null;
            if (confWindowFlag)
            {
                confLeft = reader.ReadUe();
                confRight = reader.ReadUe();
                confTop = reader.ReadUe();
                confBottom = reader.ReadUe();
            }

            bool scalingWindowFlag = reader.ReadBit();
            int? scaleLeft = null, scaleRight = null, scaleTop = null, scaleBottom = null;
            if (scalingWindowFlag)
            {
                scaleLeft = reader.ReadSe();
                scaleRight = reader.ReadSe();
                scaleTop = reader.ReadSe();
                scaleBottom = reader.ReadSe();
            }

            bool outputFlagPresent = reader.ReadBit();
            bool noPicPartition = reader.ReadBit();
            if (!noPicPartition) return false;

            bool subpicIdMapping = reader.ReadBit();
            if (subpicIdMapping) return false;

            bool cabacInit = reader.ReadBit();
            uint numL0 = reader.ReadUe();
            uint numL1 = reader.ReadUe();
            bool rpl1IdxPresent = reader.ReadBit();
            bool weightedPred = reader.ReadBit();
            bool weightedBipred = reader.ReadBit();
            bool refWraparound = reader.ReadBit();
            uint? wraparoundOffset = refWraparound ? reader.ReadUe() : null;
            int initQp = reader.ReadSe();
            bool cuQpDeltaEnabled = reader.ReadBit();
            bool chromaToolOffsets = reader.ReadBit();

            int? cbQp = null, crQp = null;
            bool? jointFlag = null;
            int? jointVal = null;
            bool? sliceChromaQpOffsets = null;
            bool? cuChromaQpOffsetListEnabled = null;
            ImmutableArray<int> cbList = ImmutableArray<int>.Empty;
            ImmutableArray<int> crList = ImmutableArray<int>.Empty;
            ImmutableArray<int> jointList = ImmutableArray<int>.Empty;
            if (chromaToolOffsets)
            {
                cbQp = reader.ReadSe();
                crQp = reader.ReadSe();
                bool jflag = reader.ReadBit();
                jointFlag = jflag;
                if (jflag) jointVal = reader.ReadSe();
                sliceChromaQpOffsets = reader.ReadBit();
                bool listEnabled = reader.ReadBit();
                cuChromaQpOffsetListEnabled = listEnabled;
                if (listEnabled)
                {
                    uint listLenMinus1 = reader.ReadUe();
                    // pps_chroma_qp_offset_list_len_minus1 is bounded to 5
                    // in the spec; clamp generously to avoid runaway loops.
                    if (listLenMinus1 > 31) return false;
                    int count = (int)listLenMinus1 + 1;
                    var cb = ImmutableArray.CreateBuilder<int>(count);
                    var cr = ImmutableArray.CreateBuilder<int>(count);
                    var joint = ImmutableArray.CreateBuilder<int>(jflag ? count : 0);
                    for (int i = 0; i < count; i++)
                    {
                        cb.Add(reader.ReadSe());
                        cr.Add(reader.ReadSe());
                        if (jflag) joint.Add(reader.ReadSe());
                    }
                    cbList = cb.ToImmutable();
                    crList = cr.ToImmutable();
                    jointList = jflag ? joint.ToImmutable() : ImmutableArray<int>.Empty;
                }
            }

            bool deblockingCtrl = reader.ReadBit();
            bool? deblockOverride = null;
            bool? deblockDisabled = null;
            int? lumaBeta = null, lumaTc = null;
            int? cbBeta = null, cbTc = null, crBeta = null, crTc = null;
            if (deblockingCtrl)
            {
                deblockOverride = reader.ReadBit();
                bool disabled = reader.ReadBit();
                deblockDisabled = disabled;
                // pps_dbf_info_in_ph_flag is gated by !pps_no_pic_partition_flag,
                // which we already required to be true; skip it.
                if (!disabled)
                {
                    lumaBeta = reader.ReadSe();
                    lumaTc = reader.ReadSe();
                    if (chromaToolOffsets)
                    {
                        cbBeta = reader.ReadSe();
                        cbTc = reader.ReadSe();
                        crBeta = reader.ReadSe();
                        crTc = reader.ReadSe();
                    }
                }
            }

            bool picHdrExt = reader.ReadBit();
            bool sliceHdrExt = reader.ReadBit();
            bool extFlag = reader.ReadBit();

            pps = new VvcPictureParameterSet
            {
                PicParameterSetId = ppsId,
                SeqParameterSetId = spsId,
                MixedNaluTypesInPicFlag = mixedNaluTypes,
                PicWidthInLumaSamples = picWidth,
                PicHeightInLumaSamples = picHeight,
                ConformanceWindowFlag = confWindowFlag,
                ConfWinLeftOffset = confLeft,
                ConfWinRightOffset = confRight,
                ConfWinTopOffset = confTop,
                ConfWinBottomOffset = confBottom,
                ScalingWindowExplicitSignallingFlag = scalingWindowFlag,
                ScalingWinLeftOffset = scaleLeft,
                ScalingWinRightOffset = scaleRight,
                ScalingWinTopOffset = scaleTop,
                ScalingWinBottomOffset = scaleBottom,
                OutputFlagPresentFlag = outputFlagPresent,
                NoPicPartitionFlag = noPicPartition,
                SubpicIdMappingPresentFlag = subpicIdMapping,
                CabacInitPresentFlag = cabacInit,
                NumRefIdxL0DefaultActiveMinus1 = numL0,
                NumRefIdxL1DefaultActiveMinus1 = numL1,
                Rpl1IdxPresentFlag = rpl1IdxPresent,
                WeightedPredFlag = weightedPred,
                WeightedBipredFlag = weightedBipred,
                RefWraparoundEnabledFlag = refWraparound,
                PicWidthMinusWraparoundOffset = wraparoundOffset,
                InitQpMinus26 = initQp,
                CuQpDeltaEnabledFlag = cuQpDeltaEnabled,
                ChromaToolOffsetsPresentFlag = chromaToolOffsets,
                CbQpOffset = cbQp,
                CrQpOffset = crQp,
                JointCbCrQpOffsetPresentFlag = jointFlag,
                JointCbCrQpOffsetValue = jointVal,
                SliceChromaQpOffsetsPresentFlag = sliceChromaQpOffsets,
                CuChromaQpOffsetListEnabledFlag = cuChromaQpOffsetListEnabled,
                CbQpOffsetList = cbList,
                CrQpOffsetList = crList,
                JointCbCrQpOffsetList = jointList,
                DeblockingFilterControlPresentFlag = deblockingCtrl,
                DeblockingFilterOverrideEnabledFlag = deblockOverride,
                DeblockingFilterDisabledFlag = deblockDisabled,
                LumaBetaOffsetDiv2 = lumaBeta,
                LumaTcOffsetDiv2 = lumaTc,
                CbBetaOffsetDiv2 = cbBeta,
                CbTcOffsetDiv2 = cbTc,
                CrBetaOffsetDiv2 = crBeta,
                CrTcOffsetDiv2 = crTc,
                PictureHeaderExtensionPresentFlag = picHdrExt,
                SliceHeaderExtensionPresentFlag = sliceHdrExt,
                ExtensionFlag = extFlag,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            pps = null;
            return false;
        }
    }
}
