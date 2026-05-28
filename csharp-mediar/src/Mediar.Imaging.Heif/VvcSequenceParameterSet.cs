namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over a VVC (H.266) Sequence Parameter Set carried as
/// a length-prefixed NAL unit inside a <c>vvcC</c> parameter-set
/// array. Only a bounded subset of the SPS RBSP is decoded per
/// ITU-T H.266 7.3.2.3 - identification fields, profile-tier-level
/// (when <c>sps_ptl_dpb_hrd_params_present_flag</c> is set and the
/// constraint info / sub-profiles blocks are empty), picture
/// geometry with the conformance window applied, and per-component
/// bit depth. Streams that exercise the GCI fields, the sub-picture
/// layout, or any of the optional sub-profile entries cause
/// <see cref="TryParse"/> to return false.
/// </summary>
public sealed record VvcSequenceParameterSet
{
    /// <summary>SPS_NUT in the VVC NAL unit type registry (15).</summary>
    public const int SpsNalUnitType = 15;

    /// <summary>4-bit <c>sps_seq_parameter_set_id</c>.</summary>
    public required byte SequenceParameterSetId { get; init; }

    /// <summary>4-bit <c>sps_video_parameter_set_id</c>.</summary>
    public required byte VideoParameterSetId { get; init; }

    /// <summary>3-bit <c>sps_max_sublayers_minus1</c>.</summary>
    public required byte MaxSubLayersMinus1 { get; init; }

    /// <summary>2-bit <c>sps_chroma_format_idc</c>
    /// (0=4:0:0, 1=4:2:0, 2=4:2:2, 3=4:4:4).</summary>
    public required byte ChromaFormatIdc { get; init; }

    /// <summary>2-bit <c>sps_log2_ctu_size_minus5</c>.</summary>
    public required byte Log2CtuSizeMinus5 { get; init; }

    /// <summary>1-bit <c>sps_ptl_dpb_hrd_params_present_flag</c>.</summary>
    public required bool PtlDpbHrdParamsPresentFlag { get; init; }

    /// <summary>7-bit <c>general_profile_idc</c>. Null when
    /// <see cref="PtlDpbHrdParamsPresentFlag"/> is false.</summary>
    public byte? GeneralProfileIdc { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c>. Null when
    /// <see cref="PtlDpbHrdParamsPresentFlag"/> is false.</summary>
    public bool? GeneralTierFlag { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>. Null when
    /// <see cref="PtlDpbHrdParamsPresentFlag"/> is false.</summary>
    public byte? GeneralLevelIdc { get; init; }

    /// <summary>1-bit <c>ptl_frame_only_constraint_flag</c>.</summary>
    public bool? PtlFrameOnlyConstraintFlag { get; init; }

    /// <summary>1-bit <c>ptl_multilayer_enabled_flag</c>.</summary>
    public bool? PtlMultilayerEnabledFlag { get; init; }

    /// <summary>1-bit <c>sps_gdr_enabled_flag</c>.</summary>
    public required bool GdrEnabledFlag { get; init; }

    /// <summary>1-bit <c>sps_ref_pic_resampling_enabled_flag</c>.</summary>
    public required bool RefPicResamplingEnabledFlag { get; init; }

    /// <summary>1-bit <c>sps_res_change_in_clvs_allowed_flag</c>.</summary>
    public required bool ResChangeInClvsAllowedFlag { get; init; }

    /// <summary><c>sps_pic_width_max_in_luma_samples</c>.</summary>
    public required uint PictureWidthMaxInLumaSamples { get; init; }

    /// <summary><c>sps_pic_height_max_in_luma_samples</c>.</summary>
    public required uint PictureHeightMaxInLumaSamples { get; init; }

    /// <summary>1-bit <c>sps_conformance_window_flag</c>.</summary>
    public required bool ConformanceWindowFlag { get; init; }

    /// <summary><c>sps_conf_win_left_offset</c>.</summary>
    public required uint ConformanceWindowLeftOffset { get; init; }

    /// <summary><c>sps_conf_win_right_offset</c>.</summary>
    public required uint ConformanceWindowRightOffset { get; init; }

    /// <summary><c>sps_conf_win_top_offset</c>.</summary>
    public required uint ConformanceWindowTopOffset { get; init; }

    /// <summary><c>sps_conf_win_bottom_offset</c>.</summary>
    public required uint ConformanceWindowBottomOffset { get; init; }

    /// <summary>1-bit <c>sps_subpic_info_present_flag</c>. Always
    /// false when the parser succeeds because the sub-picture
    /// payload is not decoded.</summary>
    public required bool SubpicInfoPresentFlag { get; init; }

    /// <summary><c>sps_bitdepth_minus8</c>.</summary>
    public required uint BitDepthMinus8 { get; init; }

    /// <summary>Effective per-component bit depth.</summary>
    public uint BitDepth => BitDepthMinus8 + 8;

    /// <summary>Effective CTU size in luma samples.</summary>
    public uint CtuSize => 1u << (Log2CtuSizeMinus5 + 5);

    /// <summary>Human-readable chroma format shorthand.</summary>
    public string ChromaFormat => ChromaFormatIdc switch
    {
        0 => "4:0:0",
        1 => "4:2:0",
        2 => "4:2:2",
        3 => "4:4:4",
        _ => $"reserved({ChromaFormatIdc})",
    };

    /// <summary>Decoded picture width after the conformance window.</summary>
    public uint DecodedWidth
    {
        get
        {
            if (!ConformanceWindowFlag) return PictureWidthMaxInLumaSamples;
            uint sub = SubWidthC;
            uint crop = sub * (ConformanceWindowLeftOffset + ConformanceWindowRightOffset);
            return PictureWidthMaxInLumaSamples > crop
                ? PictureWidthMaxInLumaSamples - crop
                : PictureWidthMaxInLumaSamples;
        }
    }

    /// <summary>Decoded picture height after the conformance window.</summary>
    public uint DecodedHeight
    {
        get
        {
            if (!ConformanceWindowFlag) return PictureHeightMaxInLumaSamples;
            uint sub = SubHeightC;
            uint crop = sub * (ConformanceWindowTopOffset + ConformanceWindowBottomOffset);
            return PictureHeightMaxInLumaSamples > crop
                ? PictureHeightMaxInLumaSamples - crop
                : PictureHeightMaxInLumaSamples;
        }
    }

    private uint SubWidthC => ChromaFormatIdc switch { 1 or 2 => 2u, _ => 1u };
    private uint SubHeightC => ChromaFormatIdc == 1 ? 2u : 1u;

    /// <summary>
    /// Attempts to decode the SPS NAL unit pointed at by
    /// <paramref name="nalUnitBytes"/>. The byte range must include
    /// the 2-byte VVC NAL header (forbidden_zero / nuh_reserved /
    /// nuh_layer_id / nal_unit_type / nuh_temporal_id_plus1) plus
    /// the RBSP payload.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnitBytes, out VvcSequenceParameterSet? sps)
    {
        sps = null;
        if (nalUnitBytes.Length < 3) return false;

        byte b0 = nalUnitBytes[0];
        byte b1 = nalUnitBytes[1];
        bool forbiddenZero = (b0 & 0x80) != 0;
        int nalUnitType = (b1 >> 3) & 0x1F;
        if (forbiddenZero || nalUnitType != SpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnitBytes[2..]);

        try
        {
            var br = new NalUnitBitReader(rbsp);
            byte spsId = (byte)br.ReadBits(4);
            byte vpsId = (byte)br.ReadBits(4);
            byte maxSubLayersMinus1 = (byte)br.ReadBits(3);
            byte chromaFormatIdc = (byte)br.ReadBits(2);
            byte log2CtuSizeMinus5 = (byte)br.ReadBits(2);
            bool ptlDpbHrdPresent = br.ReadBit();

            byte? profileIdc = null;
            bool? tierFlag = null;
            byte? levelIdc = null;
            bool? frameOnlyConstraint = null;
            bool? multilayerEnabled = null;

            if (ptlDpbHrdPresent)
            {
                profileIdc = (byte)br.ReadBits(7);
                tierFlag = br.ReadBit();
                levelIdc = (byte)br.ReadBits(8);
                frameOnlyConstraint = br.ReadBit();
                multilayerEnabled = br.ReadBit();

                bool gciPresent = br.ReadBit();
                if (gciPresent) return false;
                br.AlignToByte();

                for (int i = maxSubLayersMinus1 - 1; i >= 0; i--)
                {
                    br.ReadBit();
                }
                br.AlignToByte();

                byte numSubProfiles = (byte)br.ReadBits(8);
                if (numSubProfiles != 0) return false;
            }

            bool gdrEnabled = br.ReadBit();
            bool refPicResamplingEnabled = br.ReadBit();
            bool resChangeInClvsAllowed = false;
            if (refPicResamplingEnabled)
            {
                resChangeInClvsAllowed = br.ReadBit();
            }

            uint picWidthMax = br.ReadUe();
            uint picHeightMax = br.ReadUe();
            bool conformanceWindowFlag = br.ReadBit();
            uint leftOffset = 0, rightOffset = 0, topOffset = 0, bottomOffset = 0;
            if (conformanceWindowFlag)
            {
                leftOffset = br.ReadUe();
                rightOffset = br.ReadUe();
                topOffset = br.ReadUe();
                bottomOffset = br.ReadUe();
            }

            bool subpicInfoPresent = br.ReadBit();
            if (subpicInfoPresent) return false;

            uint bitDepthMinus8 = br.ReadUe();

            sps = new VvcSequenceParameterSet
            {
                SequenceParameterSetId = spsId,
                VideoParameterSetId = vpsId,
                MaxSubLayersMinus1 = maxSubLayersMinus1,
                ChromaFormatIdc = chromaFormatIdc,
                Log2CtuSizeMinus5 = log2CtuSizeMinus5,
                PtlDpbHrdParamsPresentFlag = ptlDpbHrdPresent,
                GeneralProfileIdc = profileIdc,
                GeneralTierFlag = tierFlag,
                GeneralLevelIdc = levelIdc,
                PtlFrameOnlyConstraintFlag = frameOnlyConstraint,
                PtlMultilayerEnabledFlag = multilayerEnabled,
                GdrEnabledFlag = gdrEnabled,
                RefPicResamplingEnabledFlag = refPicResamplingEnabled,
                ResChangeInClvsAllowedFlag = resChangeInClvsAllowed,
                PictureWidthMaxInLumaSamples = picWidthMax,
                PictureHeightMaxInLumaSamples = picHeightMax,
                ConformanceWindowFlag = conformanceWindowFlag,
                ConformanceWindowLeftOffset = leftOffset,
                ConformanceWindowRightOffset = rightOffset,
                ConformanceWindowTopOffset = topOffset,
                ConformanceWindowBottomOffset = bottomOffset,
                SubpicInfoPresentFlag = false,
                BitDepthMinus8 = bitDepthMinus8,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            return false;
        }
    }
}
