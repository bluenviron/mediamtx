using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over an HEVC Video Parameter Set carried as a
/// length-prefixed NAL unit inside an <c>hvcC</c> parameter-set
/// array. Decodes the VPS RBSP per ITU-T H.265 7.3.2.1 covering the
/// profile/tier/level block, sub-layer ordering info, layer-set
/// inclusion bitmaps, optional VPS-level timing info, and the
/// extension flag. HRD parameter blocks
/// (<c>vps_num_hrd_parameters &gt; 0</c>) and <c>vps_extension</c>
/// data are intentionally outside scope.
/// </summary>
public sealed record HevcVideoParameterSet
{
    /// <summary>VPS_NUT in the HEVC NAL unit type registry (32).</summary>
    public const int VpsNalUnitType = 32;

    private const int MaxNumLayerSets = 1024;
    private const int MaxLayerCount = 64;

    /// <summary>4-bit <c>vps_video_parameter_set_id</c>.</summary>
    public required byte VideoParameterSetId { get; init; }

    /// <summary>1-bit <c>vps_base_layer_internal_flag</c>.</summary>
    public required bool BaseLayerInternalFlag { get; init; }

    /// <summary>1-bit <c>vps_base_layer_available_flag</c>.</summary>
    public required bool BaseLayerAvailableFlag { get; init; }

    /// <summary>6-bit <c>vps_max_layers_minus1</c>.</summary>
    public required byte MaxLayersMinus1 { get; init; }

    /// <summary>3-bit <c>vps_max_sub_layers_minus1</c>.</summary>
    public required byte MaxSubLayersMinus1 { get; init; }

    /// <summary>1-bit <c>vps_temporal_id_nesting_flag</c>.</summary>
    public required bool TemporalIdNestingFlag { get; init; }

    /// <summary>16-bit <c>vps_reserved_0xffff_16bits</c>. The spec
    /// mandates 0xFFFF; the parser surfaces the value verbatim
    /// rather than validating it.</summary>
    public required ushort Reserved0xffff16Bits { get; init; }

    /// <summary>2-bit <c>general_profile_space</c>.</summary>
    public required byte GeneralProfileSpace { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c> (false = Main, true = High).</summary>
    public required bool GeneralTierFlag { get; init; }

    /// <summary>5-bit <c>general_profile_idc</c>.</summary>
    public required byte GeneralProfileIdc { get; init; }

    /// <summary>32-bit <c>general_profile_compatibility_flag</c> bitmap.</summary>
    public required uint GeneralProfileCompatibilityFlags { get; init; }

    /// <summary>48-bit constraint indicator flags right-justified in
    /// the low 48 bits.</summary>
    public required ulong GeneralConstraintIndicatorFlags { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>.</summary>
    public required byte GeneralLevelIdc { get; init; }

    /// <summary><c>vps_sub_layer_ordering_info_present_flag</c>. When
    /// false the ordering arrays carry a single entry inferred to
    /// apply to every sub-layer.</summary>
    public required bool SubLayerOrderingInfoPresentFlag { get; init; }

    /// <summary>Per-sub-layer <c>vps_max_dec_pic_buffering_minus1[i]</c>.
    /// Length is <c>MaxSubLayersMinus1 + 1</c> when
    /// <see cref="SubLayerOrderingInfoPresentFlag"/> is true,
    /// otherwise length 1 carrying the value for the highest
    /// sub-layer.</summary>
    public required ImmutableArray<uint> MaxDecPicBufferingMinus1 { get; init; }

    /// <summary>Per-sub-layer <c>vps_max_num_reorder_pics[i]</c>.</summary>
    public required ImmutableArray<uint> MaxNumReorderPics { get; init; }

    /// <summary>Per-sub-layer <c>vps_max_latency_increase_plus1[i]</c>.</summary>
    public required ImmutableArray<uint> MaxLatencyIncreasePlus1 { get; init; }

    /// <summary>6-bit <c>vps_max_layer_id</c>.</summary>
    public required byte MaxLayerId { get; init; }

    /// <summary><c>vps_num_layer_sets_minus1</c>, exp-Golomb decoded.</summary>
    public required uint NumLayerSetsMinus1 { get; init; }

    /// <summary>One bitmap per signaled layer set (indices
    /// 1..<c>NumLayerSetsMinus1</c>; layer set 0 is the base set and
    /// is not coded). Bit <c>j</c> set indicates
    /// <c>layer_id_included_flag[i][j] == 1</c> for
    /// <c>j = 0..MaxLayerId</c>. Length equals
    /// <see cref="NumLayerSetsMinus1"/>.</summary>
    public required ImmutableArray<ulong> LayerIdIncludedBitmaps { get; init; }

    /// <summary><c>vps_timing_info_present_flag</c>.</summary>
    public required bool TimingInfoPresentFlag { get; init; }

    /// <summary><c>vps_num_units_in_tick</c> when timing info is
    /// present; otherwise null.</summary>
    public required uint? NumUnitsInTick { get; init; }

    /// <summary><c>vps_time_scale</c> when timing info is present;
    /// otherwise null.</summary>
    public required uint? TimeScale { get; init; }

    /// <summary><c>vps_poc_proportional_to_timing_flag</c> when timing
    /// info is present; otherwise null.</summary>
    public required bool? PocProportionalToTimingFlag { get; init; }

    /// <summary><c>vps_num_ticks_poc_diff_one_minus1</c> when both
    /// timing info and POC-proportional-to-timing are present;
    /// otherwise null.</summary>
    public required uint? NumTicksPocDiffOneMinus1 { get; init; }

    /// <summary><c>vps_num_hrd_parameters</c>. Always zero when
    /// parsing succeeds; non-zero values are rejected because the
    /// per-block <c>hrd_parameters()</c> syntax is intentionally
    /// outside scope.</summary>
    public required uint NumHrdParameters { get; init; }

    /// <summary><c>vps_extension_flag</c>. When true the VPS carries
    /// additional extension data which this parser discards.</summary>
    public required bool ExtensionFlag { get; init; }

    /// <summary>
    /// Parses an HEVC VPS NAL unit. Expects a 2-byte NAL unit header
    /// (<c>nal_unit_type</c> must be 32, VPS_NUT) followed by the VPS
    /// RBSP, optionally containing emulation prevention bytes
    /// (<c>0x00 0x00 0x03</c>) which are stripped before bit
    /// decoding. Returns false on any structural violation, on
    /// VPSes with one or more HRD parameter blocks, or on layer set
    /// configurations beyond a 64-layer cap.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out HevcVideoParameterSet? vps)
    {
        vps = null;
        if (nalUnit.Length < 3) return false;

        if ((nalUnit[0] & 0x80) != 0) return false;
        int nalUnitType = (nalUnit[0] >> 1) & 0x3F;
        if (nalUnitType != VpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit.Slice(2));
        var reader = new NalUnitBitReader(rbsp);

        try
        {
            byte vpsId = (byte)reader.ReadBits(4);
            bool baseLayerInternal = reader.ReadBit();
            bool baseLayerAvailable = reader.ReadBit();
            byte maxLayersMinus1 = (byte)reader.ReadBits(6);
            byte maxSubLayersMinus1 = (byte)reader.ReadBits(3);
            bool temporalIdNesting = reader.ReadBit();
            ushort reserved = (ushort)reader.ReadBits(16);

            // profile_tier_level(profilePresentFlag=1, maxNumSubLayersMinus1)
            byte profileSpace = (byte)reader.ReadBits(2);
            bool tierFlag = reader.ReadBit();
            byte profileIdc = (byte)reader.ReadBits(5);
            uint profileCompat = reader.ReadBits(32);
            ulong constraint = ((ulong)reader.ReadBits(24) << 24) | reader.ReadBits(24);
            byte levelIdc = (byte)reader.ReadBits(8);

            int subLayerProfilePresentMask = 0;
            int subLayerLevelPresentMask = 0;
            for (int i = 0; i < maxSubLayersMinus1; i++)
            {
                if (reader.ReadBit()) subLayerProfilePresentMask |= 1 << i;
                if (reader.ReadBit()) subLayerLevelPresentMask |= 1 << i;
            }
            if (maxSubLayersMinus1 > 0)
            {
                for (int i = maxSubLayersMinus1; i < 8; i++) reader.SkipBits(2);
            }
            for (int i = 0; i < maxSubLayersMinus1; i++)
            {
                if ((subLayerProfilePresentMask & (1 << i)) != 0)
                {
                    // 2 + 1 + 5 + 32 + 48 bits per sub-layer profile entry.
                    reader.SkipBits(2 + 1 + 5 + 32 + 48);
                }
                if ((subLayerLevelPresentMask & (1 << i)) != 0)
                {
                    reader.SkipBits(8);
                }
            }

            bool subLayerOrderingPresent = reader.ReadBit();
            int startIdx = subLayerOrderingPresent ? 0 : maxSubLayersMinus1;
            int orderingCount = maxSubLayersMinus1 - startIdx + 1;
            var dpbBuilder = ImmutableArray.CreateBuilder<uint>(orderingCount);
            var reorderBuilder = ImmutableArray.CreateBuilder<uint>(orderingCount);
            var latencyBuilder = ImmutableArray.CreateBuilder<uint>(orderingCount);
            for (int i = 0; i < orderingCount; i++)
            {
                dpbBuilder.Add(reader.ReadUe());
                reorderBuilder.Add(reader.ReadUe());
                latencyBuilder.Add(reader.ReadUe());
            }

            byte maxLayerId = (byte)reader.ReadBits(6);
            uint numLayerSetsMinus1 = reader.ReadUe();
            if (numLayerSetsMinus1 >= MaxNumLayerSets) return false;
            int layerCount = maxLayerId + 1;
            if (layerCount > MaxLayerCount) return false;

            var bitmapBuilder = ImmutableArray.CreateBuilder<ulong>((int)numLayerSetsMinus1);
            for (uint i = 1; i <= numLayerSetsMinus1; i++)
            {
                ulong bitmap = 0;
                for (int j = 0; j < layerCount; j++)
                {
                    if (reader.ReadBit()) bitmap |= 1UL << j;
                }
                bitmapBuilder.Add(bitmap);
            }

            bool timingInfo = reader.ReadBit();
            uint? unitsInTick = null;
            uint? timeScale = null;
            bool? pocProp = null;
            uint? numTicksPocDiff = null;
            uint numHrd = 0;
            if (timingInfo)
            {
                unitsInTick = reader.ReadBits(32);
                timeScale = reader.ReadBits(32);
                bool pocPropFlag = reader.ReadBit();
                pocProp = pocPropFlag;
                if (pocPropFlag) numTicksPocDiff = reader.ReadUe();
                numHrd = reader.ReadUe();
                if (numHrd > 0) return false;
            }

            bool extensionFlag = reader.ReadBit();

            vps = new HevcVideoParameterSet
            {
                VideoParameterSetId = vpsId,
                BaseLayerInternalFlag = baseLayerInternal,
                BaseLayerAvailableFlag = baseLayerAvailable,
                MaxLayersMinus1 = maxLayersMinus1,
                MaxSubLayersMinus1 = maxSubLayersMinus1,
                TemporalIdNestingFlag = temporalIdNesting,
                Reserved0xffff16Bits = reserved,
                GeneralProfileSpace = profileSpace,
                GeneralTierFlag = tierFlag,
                GeneralProfileIdc = profileIdc,
                GeneralProfileCompatibilityFlags = profileCompat,
                GeneralConstraintIndicatorFlags = constraint,
                GeneralLevelIdc = levelIdc,
                SubLayerOrderingInfoPresentFlag = subLayerOrderingPresent,
                MaxDecPicBufferingMinus1 = dpbBuilder.ToImmutable(),
                MaxNumReorderPics = reorderBuilder.ToImmutable(),
                MaxLatencyIncreasePlus1 = latencyBuilder.ToImmutable(),
                MaxLayerId = maxLayerId,
                NumLayerSetsMinus1 = numLayerSetsMinus1,
                LayerIdIncludedBitmaps = bitmapBuilder.ToImmutable(),
                TimingInfoPresentFlag = timingInfo,
                NumUnitsInTick = unitsInTick,
                TimeScale = timeScale,
                PocProportionalToTimingFlag = pocProp,
                NumTicksPocDiffOneMinus1 = numTicksPocDiff,
                NumHrdParameters = numHrd,
                ExtensionFlag = extensionFlag,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            vps = null;
            return false;
        }
    }
}
