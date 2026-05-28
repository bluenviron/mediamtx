using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over a VVC (H.266) Video Parameter Set carried as a
/// length-prefixed NAL unit. Decodes the VPS RBSP per ITU-T H.266
/// 7.3.2.2 covering the identification fields, the single-layer
/// profile-tier-level block, the sub-layer level present flags and
/// per-sub-layer level idcs, and the sub-profile IDC list. The
/// multi-layer, DPB, OLS, general_constraint_info, general timing
/// HRD, and VPS extension sub-streams are intentionally outside
/// scope and trigger a deterministic rejection from
/// <see cref="TryParse"/>.
/// </summary>
public sealed record VvcVideoParameterSet
{
    /// <summary>VPS_NUT in the VVC NAL unit type registry (14).</summary>
    public const int VpsNalUnitType = 14;

    private const int MaxSubProfiles = 8;
    private const int MaxSubLayers = 7;

    /// <summary>4-bit <c>vps_video_parameter_set_id</c>.</summary>
    public required byte VideoParameterSetId { get; init; }

    /// <summary>6-bit <c>vps_max_layers_minus1</c>. Always zero on a
    /// successful parse; multi-layer configurations are outside
    /// scope.</summary>
    public required byte MaxLayersMinus1 { get; init; }

    /// <summary>3-bit <c>vps_max_sublayers_minus1</c>.</summary>
    public required byte MaxSubLayersMinus1 { get; init; }

    /// <summary>6-bit <c>vps_layer_id[0]</c> for the single supported
    /// layer.</summary>
    public required byte LayerId { get; init; }

    /// <summary>7-bit <c>general_profile_idc</c>.</summary>
    public required byte GeneralProfileIdc { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c> (false = Main, true =
    /// High).</summary>
    public required bool GeneralTierFlag { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>.</summary>
    public required byte GeneralLevelIdc { get; init; }

    /// <summary>1-bit <c>ptl_frame_only_constraint_flag</c>.</summary>
    public required bool FrameOnlyConstraintFlag { get; init; }

    /// <summary>1-bit <c>ptl_multilayer_enabled_flag</c>.</summary>
    public required bool MultilayerEnabledFlag { get; init; }

    /// <summary><c>gci_present_flag</c>. Always false on a successful
    /// parse; <c>general_constraint_info()</c> body is intentionally
    /// outside scope.</summary>
    public required bool GciPresentFlag { get; init; }

    /// <summary><c>ptl_sublayer_level_present_flag[i]</c> for
    /// <c>i = MaxSubLayersMinus1 - 1 .. 0</c>, indexed as
    /// <c>[0]</c> = sub-layer 0, <c>[MaxSubLayersMinus1 - 1]</c> =
    /// sub-layer <c>MaxSubLayersMinus1 - 1</c>. Empty when
    /// <c>MaxSubLayersMinus1 == 0</c>.</summary>
    public required ImmutableArray<bool> SubLayerLevelPresentFlags { get; init; }

    /// <summary>Per-sub-layer <c>sublayer_level_idc[i]</c> for
    /// <c>i = 0 .. MaxSubLayersMinus1 - 1</c>; entries are null when
    /// the corresponding present-flag is false. Empty when
    /// <c>MaxSubLayersMinus1 == 0</c>.</summary>
    public required ImmutableArray<byte?> SubLayerLevelIdcs { get; init; }

    /// <summary>8-bit <c>ptl_num_sub_profiles</c>.</summary>
    public required byte NumSubProfiles { get; init; }

    /// <summary>32-bit <c>general_sub_profile_idc[i]</c> entries; length
    /// equals <see cref="NumSubProfiles"/>.</summary>
    public required ImmutableArray<uint> SubProfileIdcs { get; init; }

    /// <summary><c>vps_general_timing_hrd_parameters_present_flag</c>.
    /// Always false on a successful parse;
    /// <c>general_timing_hrd_parameters()</c> is intentionally outside
    /// scope.</summary>
    public required bool GeneralTimingHrdParametersPresentFlag { get; init; }

    /// <summary><c>vps_extension_flag</c>. Always false on a successful
    /// parse; the VPS extension sub-stream is intentionally outside
    /// scope.</summary>
    public required bool ExtensionFlag { get; init; }

    /// <summary>
    /// Parses a VVC VPS NAL unit. Expects a 2-byte NAL unit header
    /// (forbidden_zero_bit = 0, nuh_reserved_zero_bit = 0,
    /// <c>nal_unit_type</c> = 14 VPS_NUT) followed by the VPS RBSP,
    /// optionally containing emulation prevention bytes
    /// (<c>0x00 0x00 0x03</c>) which are stripped before bit decoding.
    /// Returns false on any structural violation, on VPSes that signal
    /// multiple layers (<c>vps_max_layers_minus1 &gt; 0</c>), on VPSes
    /// that carry general constraint info (<c>gci_present_flag = 1</c>),
    /// on VPSes with a present general timing HRD block, on VPSes that
    /// raise the extension flag, on VPSes that exceed an
    /// 8-entry cap on sub-profile IDC count, and on PTL alignment
    /// bits that are not all zero.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out VvcVideoParameterSet? vps)
    {
        vps = null;
        if (nalUnit.Length < 3) return false;

        if ((nalUnit[0] & 0xC0) != 0) return false;
        int nalUnitType = (nalUnit[1] >> 3) & 0x1F;
        if (nalUnitType != VpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit.Slice(2));
        var reader = new NalUnitBitReader(rbsp);

        try
        {
            byte vpsId = (byte)reader.ReadBits(4);
            byte maxLayersMinus1 = (byte)reader.ReadBits(6);
            if (maxLayersMinus1 != 0) return false;
            byte maxSubLayersMinus1 = (byte)reader.ReadBits(3);
            if (maxSubLayersMinus1 > MaxSubLayers - 1) return false;

            byte layerId = (byte)reader.ReadBits(6);

            // vps_ptl_alignment_zero_bit: must align to byte boundary
            // with zero bits before the profile_tier_level loop.
            while (!reader.IsByteAligned())
            {
                if (reader.ReadBit()) return false;
            }

            byte profileIdc = (byte)reader.ReadBits(7);
            bool tierFlag = reader.ReadBit();
            byte levelIdc = (byte)reader.ReadBits(8);
            bool frameOnlyConstraint = reader.ReadBit();
            bool multilayerEnabled = reader.ReadBit();

            bool gciPresent = reader.ReadBit();
            if (gciPresent) return false;
            while (!reader.IsByteAligned())
            {
                if (reader.ReadBit()) return false;
            }

            var presentFlagsBuilder = ImmutableArray.CreateBuilder<bool>(maxSubLayersMinus1);
            for (int i = maxSubLayersMinus1 - 1; i >= 0; i--)
            {
                presentFlagsBuilder.Add(reader.ReadBit());
            }
            ImmutableArray<bool> presentFlagsHighToLow = presentFlagsBuilder.MoveToImmutable();

            while (!reader.IsByteAligned())
            {
                if (reader.ReadBit()) return false;
            }

            byte?[] levelIdcsBuf = new byte?[maxSubLayersMinus1];
            for (int idx = 0; idx < maxSubLayersMinus1; idx++)
            {
                int subLayer = maxSubLayersMinus1 - 1 - idx;
                if (presentFlagsHighToLow[idx])
                {
                    levelIdcsBuf[subLayer] = (byte)reader.ReadBits(8);
                }
            }

            var presentFlagsLowToHigh = ImmutableArray.CreateBuilder<bool>(maxSubLayersMinus1);
            for (int subLayer = 0; subLayer < maxSubLayersMinus1; subLayer++)
            {
                int idx = maxSubLayersMinus1 - 1 - subLayer;
                presentFlagsLowToHigh.Add(presentFlagsHighToLow[idx]);
            }

            byte numSubProfiles = (byte)reader.ReadBits(8);
            if (numSubProfiles > MaxSubProfiles) return false;
            var subProfilesBuilder = ImmutableArray.CreateBuilder<uint>(numSubProfiles);
            for (int i = 0; i < numSubProfiles; i++)
            {
                subProfilesBuilder.Add(reader.ReadBits(32));
            }

            bool generalTimingHrdPresent = reader.ReadBit();
            if (generalTimingHrdPresent) return false;

            bool extensionFlag = reader.ReadBit();
            if (extensionFlag) return false;

            vps = new VvcVideoParameterSet
            {
                VideoParameterSetId = vpsId,
                MaxLayersMinus1 = maxLayersMinus1,
                MaxSubLayersMinus1 = maxSubLayersMinus1,
                LayerId = layerId,
                GeneralProfileIdc = profileIdc,
                GeneralTierFlag = tierFlag,
                GeneralLevelIdc = levelIdc,
                FrameOnlyConstraintFlag = frameOnlyConstraint,
                MultilayerEnabledFlag = multilayerEnabled,
                GciPresentFlag = false,
                SubLayerLevelPresentFlags = presentFlagsLowToHigh.MoveToImmutable(),
                SubLayerLevelIdcs = ImmutableArray.Create(levelIdcsBuf),
                NumSubProfiles = numSubProfiles,
                SubProfileIdcs = subProfilesBuilder.MoveToImmutable(),
                GeneralTimingHrdParametersPresentFlag = false,
                ExtensionFlag = false,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            return false;
        }
    }
}
