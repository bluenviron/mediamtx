namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over an AVC (H.264) Picture Parameter Set carried as a
/// length-prefixed NAL unit inside an <c>avcC</c> parameter-set array.
/// Decodes the PPS RBSP per ITU-T H.264 7.3.2.2.
/// </summary>
/// <remarks>
/// Bounded scope: only single-slice-group PPSes
/// (<c>num_slice_groups_minus1 == 0</c>) are supported. Multi-slice-
/// group PPSes (slice-group maps via <c>slice_group_map_type</c>) are
/// rare in modern AVC bitstreams and return <c>false</c> from
/// <see cref="TryParse"/>. The FRExt post-tail
/// (<c>transform_8x8_mode_flag</c> + scaling matrix +
/// <c>second_chroma_qp_index_offset</c>) is gated by
/// <c>more_rbsp_data()</c> and surfaced as nullable when present.
/// Scaling matrix decode follows ITU-T H.264 7.3.2.2 which depends on
/// the associated SPS's <c>chroma_format_idc</c>; the optional
/// <c>chromaFormatIdc</c> parameter on <see cref="TryParse"/>
/// lets callers pass the value from a previously-decoded SPS. The
/// default (1, 4:2:0) covers the overwhelming majority of real-world
/// bitstreams.
/// </remarks>
public sealed record AvcPictureParameterSet
{
    /// <summary>PPS NAL unit type in the H.264 NAL unit type registry (8).</summary>
    public const int PpsNalUnitType = 8;

    /// <summary><c>pic_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint PicParameterSetId { get; init; }

    /// <summary><c>seq_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint SeqParameterSetId { get; init; }

    /// <summary><c>entropy_coding_mode_flag</c> — false = CAVLC, true = CABAC.</summary>
    public required bool EntropyCodingModeFlag { get; init; }

    /// <summary><c>bottom_field_pic_order_in_frame_present_flag</c>.</summary>
    public required bool BottomFieldPicOrderInFramePresentFlag { get; init; }

    /// <summary>
    /// <c>num_slice_groups_minus1</c>. Always 0 in the bounded subset
    /// supported by this parser (multi-slice-group PPSes are rejected).
    /// </summary>
    public required uint NumSliceGroupsMinus1 { get; init; }

    /// <summary><c>num_ref_idx_l0_default_active_minus1</c>.</summary>
    public required uint NumRefIdxL0DefaultActiveMinus1 { get; init; }

    /// <summary><c>num_ref_idx_l1_default_active_minus1</c>.</summary>
    public required uint NumRefIdxL1DefaultActiveMinus1 { get; init; }

    /// <summary><c>weighted_pred_flag</c>.</summary>
    public required bool WeightedPredFlag { get; init; }

    /// <summary><c>weighted_bipred_idc</c>, 2-bit unsigned (0/1/2; 3 reserved).</summary>
    public required byte WeightedBipredIdc { get; init; }

    /// <summary><c>pic_init_qp_minus26</c>, signed exp-Golomb decoded.</summary>
    public required int PicInitQpMinus26 { get; init; }

    /// <summary><c>pic_init_qs_minus26</c>, signed exp-Golomb decoded.</summary>
    public required int PicInitQsMinus26 { get; init; }

    /// <summary><c>chroma_qp_index_offset</c>, signed exp-Golomb decoded.</summary>
    public required int ChromaQpIndexOffset { get; init; }

    /// <summary><c>deblocking_filter_control_present_flag</c>.</summary>
    public required bool DeblockingFilterControlPresentFlag { get; init; }

    /// <summary><c>constrained_intra_pred_flag</c>.</summary>
    public required bool ConstrainedIntraPredFlag { get; init; }

    /// <summary><c>redundant_pic_cnt_present_flag</c>.</summary>
    public required bool RedundantPicCntPresentFlag { get; init; }

    /// <summary>
    /// <c>transform_8x8_mode_flag</c> from the FRExt post-tail. <c>null</c>
    /// when the post-tail is absent (i.e. <c>more_rbsp_data()</c> was false
    /// after <c>redundant_pic_cnt_present_flag</c>).
    /// </summary>
    public required bool? Transform8x8ModeFlag { get; init; }

    /// <summary>
    /// <c>pic_scaling_matrix_present_flag</c> from the FRExt post-tail.
    /// <c>null</c> when the post-tail is absent.
    /// </summary>
    public required bool? PicScalingMatrixPresentFlag { get; init; }

    /// <summary>
    /// <c>second_chroma_qp_index_offset</c>, signed exp-Golomb decoded
    /// from the FRExt post-tail. <c>null</c> when the post-tail is absent;
    /// in that case clients should reuse <see cref="ChromaQpIndexOffset"/>
    /// per ITU-T H.264 7.4.2.2.
    /// </summary>
    public required int? SecondChromaQpIndexOffset { get; init; }

    /// <summary>
    /// Decodes the PPS RBSP from a complete NAL unit (1-byte header +
    /// emulation-prevention-coded payload). Returns <c>false</c> when
    /// the NAL header is invalid, when <c>num_slice_groups_minus1</c>
    /// is non-zero (outside the supported subset), or when the RBSP is
    /// truncated mid-field.
    /// </summary>
    /// <param name="nalUnit">The complete NAL unit bytes.</param>
    /// <param name="pps">The decoded PPS on success; <c>null</c> on failure.</param>
    /// <param name="chromaFormatIdc">
    /// The associated SPS's <c>chroma_format_idc</c> (default 1 = 4:2:0).
    /// Only consulted when <c>pic_scaling_matrix_present_flag</c> is set;
    /// in that case it drives the count of scaling lists to skip
    /// (6 lists for chroma_format_idc != 3, or 6 + 2 when
    /// transform_8x8_mode_flag is also set; 12 lists for
    /// chroma_format_idc == 3 with transform_8x8_mode_flag set).
    /// </param>
    public static bool TryParse(
        ReadOnlySpan<byte> nalUnit,
        out AvcPictureParameterSet? pps,
        int chromaFormatIdc = 1)
    {
        pps = null;
        if (nalUnit.Length < 2) return false;
        if ((nalUnit[0] & 0x80) != 0) return false;
        int nalUnitType = nalUnit[0] & 0x1F;
        if (nalUnitType != PpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit[1..]);
        if (rbsp.Length == 0) return false;

        var reader = new NalUnitBitReader(rbsp);

        try
        {
            uint ppsId = reader.ReadUe();
            uint spsId = reader.ReadUe();
            bool entropyCoding = reader.ReadBit();
            bool bottomFieldPicOrder = reader.ReadBit();
            uint numSliceGroupsM1 = reader.ReadUe();

            if (numSliceGroupsM1 > 0) return false;

            uint numRefL0M1 = reader.ReadUe();
            uint numRefL1M1 = reader.ReadUe();
            bool wPred = reader.ReadBit();
            byte wBipredIdc = (byte)reader.ReadBits(2);
            int picInitQpM26 = reader.ReadSe();
            int picInitQsM26 = reader.ReadSe();
            int chromaQpIdxOffset = reader.ReadSe();
            bool deblockingCtrl = reader.ReadBit();
            bool constrIntra = reader.ReadBit();
            bool redundantPicCnt = reader.ReadBit();

            bool? transform8x8 = null;
            bool? picScalingMatrix = null;
            int? secondChromaQpIdx = null;

            if (reader.HasMoreRbspData())
            {
                bool t8x8 = reader.ReadBit();
                transform8x8 = t8x8;
                bool picScaling = reader.ReadBit();
                picScalingMatrix = picScaling;

                if (picScaling)
                {
                    int extraLists = (chromaFormatIdc == 3 ? 6 : 2) * (t8x8 ? 1 : 0);
                    int numLists = 6 + extraLists;
                    for (int i = 0; i < numLists; i++)
                    {
                        bool listPresent = reader.ReadBit();
                        if (listPresent)
                        {
                            int size = i < 6 ? 16 : 64;
                            SkipScalingList(ref reader, size);
                        }
                    }
                }

                secondChromaQpIdx = reader.ReadSe();
            }

            pps = new AvcPictureParameterSet
            {
                PicParameterSetId = ppsId,
                SeqParameterSetId = spsId,
                EntropyCodingModeFlag = entropyCoding,
                BottomFieldPicOrderInFramePresentFlag = bottomFieldPicOrder,
                NumSliceGroupsMinus1 = numSliceGroupsM1,
                NumRefIdxL0DefaultActiveMinus1 = numRefL0M1,
                NumRefIdxL1DefaultActiveMinus1 = numRefL1M1,
                WeightedPredFlag = wPred,
                WeightedBipredIdc = wBipredIdc,
                PicInitQpMinus26 = picInitQpM26,
                PicInitQsMinus26 = picInitQsM26,
                ChromaQpIndexOffset = chromaQpIdxOffset,
                DeblockingFilterControlPresentFlag = deblockingCtrl,
                ConstrainedIntraPredFlag = constrIntra,
                RedundantPicCntPresentFlag = redundantPicCnt,
                Transform8x8ModeFlag = transform8x8,
                PicScalingMatrixPresentFlag = picScalingMatrix,
                SecondChromaQpIndexOffset = secondChromaQpIdx,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            pps = null;
            return false;
        }
    }

    private static void SkipScalingList(ref NalUnitBitReader reader, int size)
    {
        int lastScale = 8;
        int nextScale = 8;
        for (int j = 0; j < size; j++)
        {
            if (nextScale != 0)
            {
                int deltaScale = reader.ReadSe();
                nextScale = (lastScale + deltaScale + 256) & 0xFF;
            }
            if (nextScale != 0)
            {
                lastScale = nextScale;
            }
        }
    }
}
