using System.Collections.Immutable;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HevcVideoParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_Minimal_Vps()
    {
        var nalu = VpsBuilder.Build(new VpsSpec());

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.NotNull(vps);
        Assert.Equal(0, vps!.VideoParameterSetId);
        Assert.True(vps.BaseLayerInternalFlag);
        Assert.True(vps.BaseLayerAvailableFlag);
        Assert.Equal(0, vps.MaxLayersMinus1);
        Assert.Equal(0, vps.MaxSubLayersMinus1);
        Assert.True(vps.TemporalIdNestingFlag);
        Assert.Equal(0xFFFF, vps.Reserved0xffff16Bits);
        Assert.True(vps.SubLayerOrderingInfoPresentFlag);
        Assert.Single(vps.MaxDecPicBufferingMinus1);
        Assert.Single(vps.MaxNumReorderPics);
        Assert.Single(vps.MaxLatencyIncreasePlus1);
        Assert.Equal(0, vps.MaxLayerId);
        Assert.Equal(0u, vps.NumLayerSetsMinus1);
        Assert.Empty(vps.LayerIdIncludedBitmaps);
        Assert.False(vps.TimingInfoPresentFlag);
        Assert.Null(vps.NumUnitsInTick);
        Assert.Null(vps.TimeScale);
        Assert.Null(vps.PocProportionalToTimingFlag);
        Assert.Null(vps.NumTicksPocDiffOneMinus1);
        Assert.Equal(0u, vps.NumHrdParameters);
        Assert.False(vps.ExtensionFlag);
    }

    [Fact]
    public void TryParse_Decodes_VpsId_And_Layer_Counts()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VpsId = 7,
            MaxLayersMinus1 = 3,
            MaxSubLayersMinus1 = 4,
            BaseLayerInternalFlag = false,
            BaseLayerAvailableFlag = false,
            TemporalIdNestingFlag = false,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(7, vps!.VideoParameterSetId);
        Assert.False(vps.BaseLayerInternalFlag);
        Assert.False(vps.BaseLayerAvailableFlag);
        Assert.Equal(3, vps.MaxLayersMinus1);
        Assert.Equal(4, vps.MaxSubLayersMinus1);
        Assert.False(vps.TemporalIdNestingFlag);
    }

    [Fact]
    public void TryParse_Decodes_Profile_Tier_Level()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            GeneralProfileSpace = 1,
            GeneralTierFlag = true,
            GeneralProfileIdc = 4,
            GeneralProfileCompatibilityFlags = 0xDEADBEEFu,
            GeneralConstraintIndicatorFlags = 0x0000_0123_4567_89ABu,
            GeneralLevelIdc = 153,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(1, vps!.GeneralProfileSpace);
        Assert.True(vps.GeneralTierFlag);
        Assert.Equal(4, vps.GeneralProfileIdc);
        Assert.Equal(0xDEADBEEFu, vps.GeneralProfileCompatibilityFlags);
        Assert.Equal(0x0000_0123_4567_89ABul, vps.GeneralConstraintIndicatorFlags);
        Assert.Equal(153, vps.GeneralLevelIdc);
    }

    [Fact]
    public void TryParse_Decodes_SubLayer_Ordering_Info_When_Present()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 2,
            SubLayerOrderingInfoPresentFlag = true,
            MaxDecPicBufferingMinus1 = ImmutableArray.Create(2u, 4u, 6u),
            MaxNumReorderPics = ImmutableArray.Create(0u, 1u, 2u),
            MaxLatencyIncreasePlus1 = ImmutableArray.Create(0u, 0u, 3u),
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.SubLayerOrderingInfoPresentFlag);
        Assert.Equal(new uint[] { 2, 4, 6 }, vps.MaxDecPicBufferingMinus1);
        Assert.Equal(new uint[] { 0, 1, 2 }, vps.MaxNumReorderPics);
        Assert.Equal(new uint[] { 0, 0, 3 }, vps.MaxLatencyIncreasePlus1);
    }

    [Fact]
    public void TryParse_Decodes_SubLayer_Ordering_Info_When_Absent()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 3,
            SubLayerOrderingInfoPresentFlag = false,
            MaxDecPicBufferingMinus1 = ImmutableArray.Create(5u),
            MaxNumReorderPics = ImmutableArray.Create(2u),
            MaxLatencyIncreasePlus1 = ImmutableArray.Create(7u),
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.False(vps!.SubLayerOrderingInfoPresentFlag);
        // Only the value for the highest sub-layer is coded when absent.
        Assert.Single(vps.MaxDecPicBufferingMinus1);
        Assert.Equal(5u, vps.MaxDecPicBufferingMinus1[0]);
        Assert.Single(vps.MaxNumReorderPics);
        Assert.Equal(2u, vps.MaxNumReorderPics[0]);
        Assert.Single(vps.MaxLatencyIncreasePlus1);
        Assert.Equal(7u, vps.MaxLatencyIncreasePlus1[0]);
    }

    [Fact]
    public void TryParse_Decodes_Layer_Sets()
    {
        // 3 layers (max_layer_id = 2), 2 signaled layer sets (indices 1, 2)
        // beyond the implicit base set. Layer set 1 includes layers 0,1;
        // layer set 2 includes layers 0,2.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayerId = 2,
            NumLayerSetsMinus1 = 2,
            LayerIdIncludedBitmaps = ImmutableArray.Create(0b011ul, 0b101ul),
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(2, vps!.MaxLayerId);
        Assert.Equal(2u, vps.NumLayerSetsMinus1);
        Assert.Equal(2, vps.LayerIdIncludedBitmaps.Length);
        Assert.Equal(0b011ul, vps.LayerIdIncludedBitmaps[0]);
        Assert.Equal(0b101ul, vps.LayerIdIncludedBitmaps[1]);
    }

    [Fact]
    public void TryParse_Decodes_Timing_Info_With_Poc_Proportional()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            TimingInfoPresentFlag = true,
            NumUnitsInTick = 1001,
            TimeScale = 60000,
            PocProportionalToTimingFlag = true,
            NumTicksPocDiffOneMinus1 = 0,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.TimingInfoPresentFlag);
        Assert.Equal(1001u, vps.NumUnitsInTick);
        Assert.Equal(60000u, vps.TimeScale);
        Assert.Equal(true, vps.PocProportionalToTimingFlag);
        Assert.Equal(0u, vps.NumTicksPocDiffOneMinus1);
        Assert.Equal(0u, vps.NumHrdParameters);
    }

    [Fact]
    public void TryParse_Decodes_Timing_Info_Without_Poc_Proportional()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            TimingInfoPresentFlag = true,
            NumUnitsInTick = 1,
            TimeScale = 30,
            PocProportionalToTimingFlag = false,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.TimingInfoPresentFlag);
        Assert.Equal(1u, vps.NumUnitsInTick);
        Assert.Equal(30u, vps.TimeScale);
        Assert.Equal(false, vps.PocProportionalToTimingFlag);
        Assert.Null(vps.NumTicksPocDiffOneMinus1);
    }

    [Fact]
    public void TryParse_Decodes_Extension_Flag()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            ExtensionFlag = true,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.ExtensionFlag);
    }

    [Fact]
    public void TryParse_Decodes_With_Multiple_SubLayer_Profile_Entries()
    {
        // max_sub_layers_minus1 = 2 with profile-present flags exercising
        // the sub-layer profile/level skip path.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 2,
            SubLayerProfilePresentMask = 0b11,
            SubLayerLevelPresentMask = 0b11,
            SubLayerLevelIdcs = new byte[] { 90, 120 },
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(2, vps!.MaxSubLayersMinus1);
    }

    [Fact]
    public void TryParse_Rejects_NonVps_NalType()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            NalUnitTypeOverride = 33, // SPS_NUT instead of VPS_NUT
        });

        Assert.False(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Zero_Bit_Set()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            ForbiddenZeroBit = true,
        });

        Assert.False(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(HevcVideoParameterSet.TryParse(ReadOnlySpan<byte>.Empty, out _));
        Assert.False(HevcVideoParameterSet.TryParse(new byte[] { 0x40 }, out _));
        Assert.False(HevcVideoParameterSet.TryParse(new byte[] { 0x40, 0x01 }, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Rbsp()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayerId = 4,
            NumLayerSetsMinus1 = 2,
            LayerIdIncludedBitmaps = ImmutableArray.Create(0b01010ul, 0b10101ul),
        });

        // Strip enough bytes that the layer-set bitmap reads will overrun
        // the buffer regardless of the more_rbsp_data graceful-degradation
        // path (there is no graceful path for u(1) reads within a layer
        // set bitmap loop).
        var truncated = nalu.AsSpan(0, 4).ToArray();
        Assert.False(HevcVideoParameterSet.TryParse(truncated, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Rejects_NonZero_Num_Hrd_Parameters()
    {
        // VPS-level HRD parameter blocks are intentionally outside the
        // bounded parser scope and must be rejected deterministically.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            TimingInfoPresentFlag = true,
            NumUnitsInTick = 1,
            TimeScale = 30,
            PocProportionalToTimingFlag = false,
            NumHrdParametersOverride = 1,
        });

        Assert.False(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Roundtrips_Through_Emulation_Prevention_Encoding()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VpsId = 5,
            MaxLayersMinus1 = 1,
            GeneralLevelIdc = 120,
        });

        // Walk the RBSP bytes (after 2-byte NAL header) and inject an
        // emulation-prevention 0x03 byte before any 0x00..0x03 that
        // follows two consecutive zero bytes, exercising the parser's
        // StripEmulationPreventionBytes path end-to-end.
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

        Assert.True(HevcVideoParameterSet.TryParse(encoded.ToArray(), out var vps));
        Assert.Equal(5, vps!.VideoParameterSetId);
        Assert.Equal(1, vps.MaxLayersMinus1);
        Assert.Equal(120, vps.GeneralLevelIdc);
    }

    [Fact]
    public void VpsNalUnitType_Constant_Is_32()
    {
        Assert.Equal(32, HevcVideoParameterSet.VpsNalUnitType);
    }

    [Theory]
    [InlineData((byte)0)]   // TRAIL_N
    [InlineData((byte)1)]   // TRAIL_R
    [InlineData((byte)9)]   // RASL_N
    [InlineData((byte)19)]  // IDR_W_RADL
    [InlineData((byte)21)]  // CRA
    [InlineData((byte)33)]  // SPS_NUT
    [InlineData((byte)34)]  // PPS_NUT
    [InlineData((byte)35)]  // AUD_NUT
    [InlineData((byte)39)]  // PREFIX_SEI_NUT
    [InlineData((byte)40)]  // SUFFIX_SEI_NUT
    [InlineData((byte)63)]  // RSV_NVCL47
    public void TryParse_Rejects_Various_NalUnitTypes(byte nutOverride)
    {
        var nalu = VpsBuilder.Build(new VpsSpec { NalUnitTypeOverride = nutOverride });
        Assert.False(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Two_Identical_Parses_Have_Equal_Scalar_Members()
    {
        var nalu = VpsBuilder.Build(new VpsSpec { VpsId = 3, GeneralLevelIdc = 90 });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var a));
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var b));
        Assert.NotNull(a);
        Assert.NotNull(b);
        Assert.Equal(a!.VideoParameterSetId, b!.VideoParameterSetId);
        Assert.Equal(a.GeneralLevelIdc, b.GeneralLevelIdc);
        Assert.Equal(a.MaxLayersMinus1, b.MaxLayersMinus1);
        Assert.Equal(a.MaxSubLayersMinus1, b.MaxSubLayersMinus1);
        Assert.Equal(a.GeneralProfileCompatibilityFlags, b.GeneralProfileCompatibilityFlags);
        Assert.Equal(a.GeneralConstraintIndicatorFlags, b.GeneralConstraintIndicatorFlags);
    }

    [Fact]
    public void With_Expression_Modifies_Single_Field()
    {
        var nalu = VpsBuilder.Build(new VpsSpec { VpsId = 1 });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        var changed = vps! with { VideoParameterSetId = 12 };
        Assert.Equal(12, changed.VideoParameterSetId);
        Assert.Equal(vps.GeneralLevelIdc, changed.GeneralLevelIdc);
        Assert.Equal(vps.MaxLayersMinus1, changed.MaxLayersMinus1);
        Assert.NotSame(vps, changed);
    }

    [Fact]
    public void TryParse_Decodes_When_Only_SubLayer_Profile_Present()
    {
        // Profile present without level present must still parse and skip
        // the appropriate number of profile bits per signaled sub-layer.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 1,
            SubLayerProfilePresentMask = 0b01,
            SubLayerLevelPresentMask = 0b00,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(1, vps!.MaxSubLayersMinus1);
    }

    [Fact]
    public void TryParse_Decodes_When_Only_SubLayer_Level_Present()
    {
        // Level present without profile present — exercises the
        // level-skip branch in isolation.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 1,
            SubLayerProfilePresentMask = 0b00,
            SubLayerLevelPresentMask = 0b01,
            SubLayerLevelIdcs = new byte[] { 75 },
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(1, vps!.MaxSubLayersMinus1);
    }

    [Fact]
    public void TryParse_Decodes_When_Neither_SubLayer_Profile_Nor_Level_Present()
    {
        // The reserved 2-bit padding loop must still execute for
        // max_sub_layers_minus1 > 0 even when no per-sub-layer entries
        // are signaled.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxSubLayersMinus1 = 3,
            SubLayerProfilePresentMask = 0,
            SubLayerLevelPresentMask = 0,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(3, vps!.MaxSubLayersMinus1);
    }

    [Fact]
    public void TryParse_Surfaces_Reserved16Bits_Verbatim()
    {
        // Spec mandates 0xFFFF but parser does not validate; arbitrary
        // values must round-trip into the property unchanged.
        var nalu = VpsBuilder.Build(new VpsSpec { Reserved0xffff16Bits = 0x1234 });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal((ushort)0x1234, vps!.Reserved0xffff16Bits);
    }

    [Fact]
    public void TryParse_Accepts_Max_Layer_Count_At_Boundary()
    {
        // MaxLayerId = 63 means layerCount = 64 = MaxLayerCount, which is
        // accepted (the parser rejects only when > 64).
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayerId = 63,
            NumLayerSetsMinus1 = 1,
            LayerIdIncludedBitmaps = ImmutableArray.Create(0x0000_0000_0000_0001ul),
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(63, vps!.MaxLayerId);
        Assert.Single(vps.LayerIdIncludedBitmaps);
    }

    [Fact]
    public void TryParse_Decodes_Three_Layer_Sets()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayerId = 1,
            NumLayerSetsMinus1 = 3,
            LayerIdIncludedBitmaps = ImmutableArray.Create(0b01ul, 0b10ul, 0b11ul),
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(3u, vps!.NumLayerSetsMinus1);
        Assert.Equal(3, vps.LayerIdIncludedBitmaps.Length);
        Assert.Equal(0b01ul, vps.LayerIdIncludedBitmaps[0]);
        Assert.Equal(0b10ul, vps.LayerIdIncludedBitmaps[1]);
        Assert.Equal(0b11ul, vps.LayerIdIncludedBitmaps[2]);
    }

    [Fact]
    public void TryParse_NumLayerSetsMinus1_Zero_Yields_Empty_Bitmap_Array()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayerId = 2,
            NumLayerSetsMinus1 = 0,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(0u, vps!.NumLayerSetsMinus1);
        Assert.Empty(vps.LayerIdIncludedBitmaps);
    }

    [Fact]
    public void TryParse_General_Tier_High_Decodes()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            GeneralTierFlag = true,
            GeneralProfileIdc = 2,
            GeneralLevelIdc = 186,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.GeneralTierFlag);
        Assert.Equal(2, vps.GeneralProfileIdc);
        Assert.Equal(186, vps.GeneralLevelIdc);
    }

    [Fact]
    public void TryParse_Sets_NumHrdParameters_Zero_When_Timing_Info_Absent()
    {
        var nalu = VpsBuilder.Build(new VpsSpec { TimingInfoPresentFlag = false });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.False(vps!.TimingInfoPresentFlag);
        Assert.Equal(0u, vps.NumHrdParameters);
    }

    [Fact]
    public void TryParse_All_Constraint_Indicator_Bits_Roundtrip()
    {
        // Highest representable 48-bit value to validate the split-read
        // (24-bit high + 24-bit low) is reassembled correctly.
        const ulong max48 = (1UL << 48) - 1;
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            GeneralConstraintIndicatorFlags = max48,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(max48, vps!.GeneralConstraintIndicatorFlags);
    }

    [Fact]
    public void TryParse_Zero_Profile_Compatibility_Roundtrips()
    {
        var nalu = VpsBuilder.Build(new VpsSpec { GeneralProfileCompatibilityFlags = 0u });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(0u, vps!.GeneralProfileCompatibilityFlags);
    }

    [Fact]
    public void TryParse_MaxLayers_And_MaxSubLayers_Boundary_Values()
    {
        // 6 bits => 0..63, 3 bits => 0..7.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            MaxLayersMinus1 = 63,
            MaxSubLayersMinus1 = 6, // keep room for reserved 2-bit padding loop
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(63, vps!.MaxLayersMinus1);
        Assert.Equal(6, vps.MaxSubLayersMinus1);
    }

    [Fact]
    public void TryParse_VpsId_Max_Roundtrips()
    {
        // 4-bit field => max value 15.
        var nalu = VpsBuilder.Build(new VpsSpec { VpsId = 15 });
        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(15, vps!.VideoParameterSetId);
    }

    [Fact]
    public void TryParse_PocProportionalToTiming_True_With_Nonzero_NumTicks()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            TimingInfoPresentFlag = true,
            NumUnitsInTick = 1001,
            TimeScale = 24000,
            PocProportionalToTimingFlag = true,
            NumTicksPocDiffOneMinus1 = 7,
        });

        Assert.True(HevcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(true, vps!.PocProportionalToTimingFlag);
        Assert.Equal(7u, vps.NumTicksPocDiffOneMinus1);
    }

    // -----------------------------------------------------------------
    // Test helpers
    // -----------------------------------------------------------------

    private sealed record VpsSpec
    {
        public byte VpsId { get; init; }
        public bool BaseLayerInternalFlag { get; init; } = true;
        public bool BaseLayerAvailableFlag { get; init; } = true;
        public byte MaxLayersMinus1 { get; init; }
        public byte MaxSubLayersMinus1 { get; init; }
        public bool TemporalIdNestingFlag { get; init; } = true;
        public ushort Reserved0xffff16Bits { get; init; } = 0xFFFF;

        public byte GeneralProfileSpace { get; init; }
        public bool GeneralTierFlag { get; init; }
        public byte GeneralProfileIdc { get; init; } = 1;
        public uint GeneralProfileCompatibilityFlags { get; init; } = 0x60000000;
        public ulong GeneralConstraintIndicatorFlags { get; init; }
        public byte GeneralLevelIdc { get; init; } = 90;

        public int SubLayerProfilePresentMask { get; init; }
        public int SubLayerLevelPresentMask { get; init; }
        public byte[] SubLayerLevelIdcs { get; init; } = Array.Empty<byte>();

        public bool SubLayerOrderingInfoPresentFlag { get; init; } = true;
        public ImmutableArray<uint> MaxDecPicBufferingMinus1 { get; init; } = ImmutableArray.Create(0u);
        public ImmutableArray<uint> MaxNumReorderPics { get; init; } = ImmutableArray.Create(0u);
        public ImmutableArray<uint> MaxLatencyIncreasePlus1 { get; init; } = ImmutableArray.Create(0u);

        public byte MaxLayerId { get; init; }
        public uint NumLayerSetsMinus1 { get; init; }
        public ImmutableArray<ulong> LayerIdIncludedBitmaps { get; init; } = ImmutableArray<ulong>.Empty;

        public bool TimingInfoPresentFlag { get; init; }
        public uint NumUnitsInTick { get; init; }
        public uint TimeScale { get; init; }
        public bool PocProportionalToTimingFlag { get; init; }
        public uint NumTicksPocDiffOneMinus1 { get; init; }
        public uint NumHrdParametersOverride { get; init; }
        public bool ExtensionFlag { get; init; }

        public bool ForbiddenZeroBit { get; init; }
        public byte NalUnitTypeOverride { get; init; } = 32; // VPS_NUT
    }

    private static class VpsBuilder
    {
        public static byte[] Build(VpsSpec spec)
        {
            var w = new BitWriter();
            w.WriteBits(spec.VpsId, 4);
            w.WriteBit(spec.BaseLayerInternalFlag);
            w.WriteBit(spec.BaseLayerAvailableFlag);
            w.WriteBits(spec.MaxLayersMinus1, 6);
            w.WriteBits(spec.MaxSubLayersMinus1, 3);
            w.WriteBit(spec.TemporalIdNestingFlag);
            w.WriteBits(spec.Reserved0xffff16Bits, 16);

            // profile_tier_level(profilePresentFlag=1, maxNumSubLayersMinus1)
            w.WriteBits(spec.GeneralProfileSpace, 2);
            w.WriteBit(spec.GeneralTierFlag);
            w.WriteBits(spec.GeneralProfileIdc, 5);
            w.WriteBits(spec.GeneralProfileCompatibilityFlags, 32);
            w.WriteBits((spec.GeneralConstraintIndicatorFlags >> 24) & 0xFFFFFFul, 24);
            w.WriteBits(spec.GeneralConstraintIndicatorFlags & 0xFFFFFFul, 24);
            w.WriteBits(spec.GeneralLevelIdc, 8);

            for (int i = 0; i < spec.MaxSubLayersMinus1; i++)
            {
                bool prof = (spec.SubLayerProfilePresentMask & (1 << i)) != 0;
                bool lvl = (spec.SubLayerLevelPresentMask & (1 << i)) != 0;
                w.WriteBit(prof);
                w.WriteBit(lvl);
            }
            if (spec.MaxSubLayersMinus1 > 0)
            {
                for (int i = spec.MaxSubLayersMinus1; i < 8; i++) w.WriteBits(0, 2);
            }
            int levelIdcIdx = 0;
            for (int i = 0; i < spec.MaxSubLayersMinus1; i++)
            {
                if ((spec.SubLayerProfilePresentMask & (1 << i)) != 0)
                {
                    // 2 + 1 + 5 + 32 + 48 = 88 bits, all zero for fixture purposes.
                    w.WriteBits(0, 32);
                    w.WriteBits(0, 32);
                    w.WriteBits(0, 24);
                }
                if ((spec.SubLayerLevelPresentMask & (1 << i)) != 0)
                {
                    byte v = levelIdcIdx < spec.SubLayerLevelIdcs.Length
                        ? spec.SubLayerLevelIdcs[levelIdcIdx]
                        : (byte)0;
                    w.WriteBits(v, 8);
                    levelIdcIdx++;
                }
            }

            w.WriteBit(spec.SubLayerOrderingInfoPresentFlag);
            int startIdx = spec.SubLayerOrderingInfoPresentFlag ? 0 : spec.MaxSubLayersMinus1;
            int orderingCount = spec.MaxSubLayersMinus1 - startIdx + 1;
            for (int i = 0; i < orderingCount; i++)
            {
                w.WriteUe(i < spec.MaxDecPicBufferingMinus1.Length ? spec.MaxDecPicBufferingMinus1[i] : 0u);
                w.WriteUe(i < spec.MaxNumReorderPics.Length ? spec.MaxNumReorderPics[i] : 0u);
                w.WriteUe(i < spec.MaxLatencyIncreasePlus1.Length ? spec.MaxLatencyIncreasePlus1[i] : 0u);
            }

            w.WriteBits(spec.MaxLayerId, 6);
            w.WriteUe(spec.NumLayerSetsMinus1);
            int layerCount = spec.MaxLayerId + 1;
            for (uint i = 1; i <= spec.NumLayerSetsMinus1; i++)
            {
                ulong bitmap = spec.LayerIdIncludedBitmaps[(int)i - 1];
                for (int j = 0; j < layerCount; j++)
                {
                    w.WriteBit((bitmap & (1UL << j)) != 0);
                }
            }

            w.WriteBit(spec.TimingInfoPresentFlag);
            if (spec.TimingInfoPresentFlag)
            {
                w.WriteBits(spec.NumUnitsInTick, 32);
                w.WriteBits(spec.TimeScale, 32);
                w.WriteBit(spec.PocProportionalToTimingFlag);
                if (spec.PocProportionalToTimingFlag)
                {
                    w.WriteUe(spec.NumTicksPocDiffOneMinus1);
                }
                w.WriteUe(spec.NumHrdParametersOverride);
            }

            w.WriteBit(spec.ExtensionFlag);

            // rbsp_trailing_bits(): rbsp_stop_one_bit + alignment zero bits
            w.WriteBit(true);
            w.AlignToByte();

            byte[] rbsp = w.ToArray();
            var nalu = new byte[rbsp.Length + 2];
            int forbidden = spec.ForbiddenZeroBit ? 1 : 0;
            // Byte 0: forbidden_zero (1) + nal_unit_type (6) + nuh_layer_id MSB (1)
            nalu[0] = (byte)((forbidden << 7) | ((spec.NalUnitTypeOverride & 0x3F) << 1));
            // Byte 1: nuh_layer_id low 5 bits = 0 + nuh_temporal_id_plus1 = 1
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
