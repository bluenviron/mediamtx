using System.Collections.Immutable;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class VvcVideoParameterSetTests
{
    [Fact]
    public void TryParse_Decodes_Minimal_Vps()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.NotNull(vps);
        Assert.Equal(0, vps!.VideoParameterSetId);
        Assert.Equal(0, vps.MaxLayersMinus1);
        Assert.Equal(0, vps.MaxSubLayersMinus1);
        Assert.Equal(0, vps.LayerId);
        Assert.False(vps.GciPresentFlag);
        Assert.False(vps.GeneralTimingHrdParametersPresentFlag);
        Assert.False(vps.ExtensionFlag);
        Assert.Empty(vps.SubLayerLevelPresentFlags);
        Assert.Empty(vps.SubLayerLevelIdcs);
        Assert.Equal(0, vps.NumSubProfiles);
        Assert.Empty(vps.SubProfileIdcs);
    }

    [Fact]
    public void TryParse_Decodes_VpsId_And_LayerId()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 5,
            MaxSubLayersMinus1 = 0,
            LayerId = 13,
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(5, vps!.VideoParameterSetId);
        Assert.Equal(13, vps.LayerId);
    }

    [Fact]
    public void TryParse_Decodes_Profile_Tier_Level_Fields()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            GeneralProfileIdc = 65,
            GeneralTierFlag = true,
            GeneralLevelIdc = 153,
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(65, vps!.GeneralProfileIdc);
        Assert.True(vps.GeneralTierFlag);
        Assert.Equal(153, vps.GeneralLevelIdc);
    }

    [Fact]
    public void TryParse_Decodes_FrameOnly_And_Multilayer_Flags()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            FrameOnlyConstraintFlag = true,
            MultilayerEnabledFlag = true,
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.True(vps!.FrameOnlyConstraintFlag);
        Assert.True(vps.MultilayerEnabledFlag);
    }

    [Fact]
    public void TryParse_Decodes_Single_SubLayer_With_Level_Present()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 1,
            LayerId = 0,
            GeneralLevelIdc = 120,
            SubLayerLevelPresentFlags = ImmutableArray.Create(true),
            SubLayerLevelIdcs = ImmutableArray.Create<byte?>(102),
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(1, vps!.MaxSubLayersMinus1);
        Assert.Single(vps.SubLayerLevelPresentFlags);
        Assert.True(vps.SubLayerLevelPresentFlags[0]);
        Assert.Single(vps.SubLayerLevelIdcs);
        Assert.Equal((byte?)102, vps.SubLayerLevelIdcs[0]);
    }

    [Fact]
    public void TryParse_Decodes_Multi_SubLayer_Mixed_Presence()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 3,
            LayerId = 0,
            SubLayerLevelPresentFlags = ImmutableArray.Create(true, false, true),
            SubLayerLevelIdcs = ImmutableArray.Create<byte?>(80, null, 90),
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(3, vps!.MaxSubLayersMinus1);
        Assert.Equal(3, vps.SubLayerLevelPresentFlags.Length);
        Assert.True(vps.SubLayerLevelPresentFlags[0]);
        Assert.False(vps.SubLayerLevelPresentFlags[1]);
        Assert.True(vps.SubLayerLevelPresentFlags[2]);
        Assert.Equal(3, vps.SubLayerLevelIdcs.Length);
        Assert.Equal((byte?)80, vps.SubLayerLevelIdcs[0]);
        Assert.Null(vps.SubLayerLevelIdcs[1]);
        Assert.Equal((byte?)90, vps.SubLayerLevelIdcs[2]);
    }

    [Fact]
    public void TryParse_Decodes_SubProfile_Idcs()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            SubProfileIdcs = ImmutableArray.Create(0xCAFEBABEu, 0x12345678u),
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(2, vps!.NumSubProfiles);
        Assert.Equal(2, vps.SubProfileIdcs.Length);
        Assert.Equal(0xCAFEBABEu, vps.SubProfileIdcs[0]);
        Assert.Equal(0x12345678u, vps.SubProfileIdcs[1]);
    }

    [Fact]
    public void TryParse_Decodes_Max_SubProfiles()
    {
        var subProfiles = ImmutableArray.Create<uint>(1, 2, 3, 4, 5, 6, 7, 8);
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            SubProfileIdcs = subProfiles,
        });

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(8, vps!.NumSubProfiles);
        Assert.Equal(8, vps.SubProfileIdcs.Length);
        for (int i = 0; i < 8; i++)
        {
            Assert.Equal((uint)(i + 1), vps.SubProfileIdcs[i]);
        }
    }

    [Fact]
    public void TryParse_Rejects_NonVps_NalType()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            NalUnitTypeOverride = 15, // SPS_NUT
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Null(vps);
    }

    [Fact]
    public void TryParse_Rejects_Forbidden_Or_Reserved_Bit_Set()
    {
        var withForbidden = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            ForbiddenZeroBit = true,
        });
        Assert.False(VvcVideoParameterSet.TryParse(withForbidden, out _));

        var withReserved = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            ReservedZeroBit = true,
        });
        Assert.False(VvcVideoParameterSet.TryParse(withReserved, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        Assert.False(VvcVideoParameterSet.TryParse(ReadOnlySpan<byte>.Empty, out _));
        Assert.False(VvcVideoParameterSet.TryParse(new byte[] { 0x00 }, out _));
        // 0x71 = (14 << 3) | 1: VPS_NUT with temporal_id_plus1 = 1, no payload.
        Assert.False(VvcVideoParameterSet.TryParse(new byte[] { 0x00, 0x71 }, out _));
    }

    [Fact]
    public void TryParse_Rejects_MultiLayer_Vps()
    {
        // vps_max_layers_minus1 > 0 is outside the bounded scope.
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxLayersMinus1Override = 1,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Rejects_Gci_Present()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            GciPresentFlag = true,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Rejects_General_Timing_Hrd_Present()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            GeneralTimingHrdParametersPresentFlag = true,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Rejects_Extension_Flag()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            ExtensionFlag = true,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Rejects_SubProfiles_Over_Cap()
    {
        // Cap is 8; cross the cap with 9 entries.
        var nineSubProfiles = ImmutableArray.CreateRange<uint>(new uint[] { 1, 2, 3, 4, 5, 6, 7, 8, 9 });
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            SubProfileIdcs = nineSubProfiles,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Rejects_NonZero_Ptl_Alignment_Bit()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            PtlAlignmentOnes = true,
        });

        Assert.False(VvcVideoParameterSet.TryParse(nalu, out _));
    }

    [Fact]
    public void TryParse_Roundtrips_Through_Emulation_Prevention_Encoding()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            // Use four sub-profile IDCs deliberately crafted to inject
            // 0x00 0x00 patterns that the encoder must escape with the
            // 0x00 0x00 0x03 emulation prevention sequence on the wire.
            SubProfileIdcs = ImmutableArray.Create(0x00000000u, 0x00000001u, 0x00000200u, 0x00000003u),
        });

        // Verify at least one emulation prevention byte appeared.
        bool hasEmulationByte = false;
        for (int i = 0; i + 2 < nalu.Length; i++)
        {
            if (nalu[i] == 0 && nalu[i + 1] == 0 && nalu[i + 2] == 0x03)
            {
                hasEmulationByte = true;
                break;
            }
        }
        Assert.True(hasEmulationByte);

        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(4, vps!.NumSubProfiles);
        Assert.Equal(0x00000000u, vps.SubProfileIdcs[0]);
        Assert.Equal(0x00000001u, vps.SubProfileIdcs[1]);
        Assert.Equal(0x00000200u, vps.SubProfileIdcs[2]);
        Assert.Equal(0x00000003u, vps.SubProfileIdcs[3]);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(15)]
    public void TryParse_VpsId_Roundtrips(byte vpsId)
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = vpsId,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(vpsId, vps!.VideoParameterSetId);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(31)]
    [InlineData(63)]
    public void TryParse_LayerId_Roundtrips(byte layerId)
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = layerId,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(layerId, vps!.LayerId);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(120)]
    [InlineData(255)]
    public void TryParse_GeneralLevelIdc_Roundtrips(byte level)
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            GeneralLevelIdc = level,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(level, vps!.GeneralLevelIdc);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(33)]
    [InlineData(127)]
    public void TryParse_GeneralProfileIdc_Roundtrips(byte profileIdc)
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            GeneralProfileIdc = profileIdc,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(profileIdc, vps!.GeneralProfileIdc);
    }

    [Fact]
    public void TryParse_All_SubLayer_Levels_Present()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 4,
            LayerId = 0,
            SubLayerLevelPresentFlags = ImmutableArray.Create(true, true, true, true),
            SubLayerLevelIdcs = ImmutableArray.Create<byte?>(60, 70, 80, 90),
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(4, vps!.SubLayerLevelPresentFlags.Length);
        Assert.All(vps.SubLayerLevelPresentFlags, f => Assert.True(f));
    }

    [Fact]
    public void TryParse_All_SubLayer_Levels_Absent()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 4,
            LayerId = 0,
            SubLayerLevelPresentFlags = ImmutableArray.Create(false, false, false, false),
            SubLayerLevelIdcs = ImmutableArray.Create<byte?>(null, null, null, null),
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(4, vps!.SubLayerLevelPresentFlags.Length);
        Assert.All(vps.SubLayerLevelPresentFlags, f => Assert.False(f));
        Assert.All(vps.SubLayerLevelIdcs, idc => Assert.Null(idc));
    }

    [Fact]
    public void Record_Equality_From_Identical_Bytes_Matches_VisibleFields()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 3,
            MaxSubLayersMinus1 = 0,
            LayerId = 7,
            GeneralProfileIdc = 65,
            GeneralLevelIdc = 99,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var a));
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var b));
        Assert.Equal(a!.VideoParameterSetId, b!.VideoParameterSetId);
        Assert.Equal(a.LayerId, b.LayerId);
        Assert.Equal(a.GeneralProfileIdc, b.GeneralProfileIdc);
        Assert.Equal(a.GeneralLevelIdc, b.GeneralLevelIdc);
        Assert.Equal(a.NumSubProfiles, b.NumSubProfiles);
    }

    [Fact]
    public void TryParse_FrameOnly_False_And_Multilayer_False_Defaults()
    {
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.False(vps!.FrameOnlyConstraintFlag);
        Assert.False(vps.MultilayerEnabledFlag);
        Assert.False(vps.GeneralTierFlag);
    }

    [Fact]
    public void TryParse_Exactly_8_SubProfiles_Accepted()
    {
        var subProfiles = ImmutableArray.Create<uint>(0x1u, 0x2u, 0x3u, 0x4u, 0x5u, 0x6u, 0x7u, 0x8u);
        var nalu = VpsBuilder.Build(new VpsSpec
        {
            VideoParameterSetId = 0,
            MaxSubLayersMinus1 = 0,
            LayerId = 0,
            SubProfileIdcs = subProfiles,
        });
        Assert.True(VvcVideoParameterSet.TryParse(nalu, out var vps));
        Assert.Equal(8, vps!.NumSubProfiles);
    }

    // --- Test bitstream construction helpers ---

    private sealed record VpsSpec
    {
        public byte VideoParameterSetId { get; init; }
        public byte MaxSubLayersMinus1 { get; init; }
        public byte LayerId { get; init; }

        public byte GeneralProfileIdc { get; init; }
        public bool GeneralTierFlag { get; init; }
        public byte GeneralLevelIdc { get; init; }
        public bool FrameOnlyConstraintFlag { get; init; }
        public bool MultilayerEnabledFlag { get; init; }

        public bool GciPresentFlag { get; init; }
        public ImmutableArray<bool> SubLayerLevelPresentFlags { get; init; } = ImmutableArray<bool>.Empty;
        public ImmutableArray<byte?> SubLayerLevelIdcs { get; init; } = ImmutableArray<byte?>.Empty;
        public ImmutableArray<uint> SubProfileIdcs { get; init; } = ImmutableArray<uint>.Empty;

        public bool GeneralTimingHrdParametersPresentFlag { get; init; }
        public bool ExtensionFlag { get; init; }

        public bool ForbiddenZeroBit { get; init; }
        public bool ReservedZeroBit { get; init; }
        public byte NalUnitTypeOverride { get; init; } = 14; // VPS_NUT
        public byte MaxLayersMinus1Override { get; init; }
        public bool PtlAlignmentOnes { get; init; }
    }

    private static class VpsBuilder
    {
        public static byte[] Build(VpsSpec spec)
        {
            var w = new BitWriter();
            w.WriteBits(spec.VideoParameterSetId, 4);
            w.WriteBits(spec.MaxLayersMinus1Override, 6);
            w.WriteBits(spec.MaxSubLayersMinus1, 3);
            w.WriteBits(spec.LayerId, 6);

            // PTL alignment: pad to byte boundary with zeros (or ones to
            // exercise the rejection path).
            while ((w.BitCount & 7) != 0)
            {
                w.WriteBit(spec.PtlAlignmentOnes);
            }

            // profile_tier_level(1, MaxSubLayersMinus1)
            w.WriteBits(spec.GeneralProfileIdc, 7);
            w.WriteBit(spec.GeneralTierFlag);
            w.WriteBits(spec.GeneralLevelIdc, 8);
            w.WriteBit(spec.FrameOnlyConstraintFlag);
            w.WriteBit(spec.MultilayerEnabledFlag);

            // general_constraint_info(): gci_present_flag + alignment.
            w.WriteBit(spec.GciPresentFlag);
            while ((w.BitCount & 7) != 0)
            {
                w.WriteBit(false);
            }

            // ptl_sublayer_level_present_flag[i] for
            // i = MaxSubLayersMinus1 - 1 .. 0
            var presentHighToLow = new bool[spec.MaxSubLayersMinus1];
            for (int idx = 0; idx < spec.MaxSubLayersMinus1; idx++)
            {
                int subLayer = spec.MaxSubLayersMinus1 - 1 - idx;
                bool present = subLayer < spec.SubLayerLevelPresentFlags.Length
                    && spec.SubLayerLevelPresentFlags[subLayer];
                presentHighToLow[idx] = present;
                w.WriteBit(present);
            }

            // ptl_reserved_zero_bit alignment.
            while ((w.BitCount & 7) != 0)
            {
                w.WriteBit(false);
            }

            // sublayer_level_idc[i] for i = MaxSubLayersMinus1 - 1 .. 0
            for (int idx = 0; idx < spec.MaxSubLayersMinus1; idx++)
            {
                int subLayer = spec.MaxSubLayersMinus1 - 1 - idx;
                if (presentHighToLow[idx])
                {
                    byte level = subLayer < spec.SubLayerLevelIdcs.Length && spec.SubLayerLevelIdcs[subLayer].HasValue
                        ? spec.SubLayerLevelIdcs[subLayer]!.Value
                        : (byte)0;
                    w.WriteBits(level, 8);
                }
            }

            // ptl_num_sub_profiles + general_sub_profile_idc[i]
            w.WriteBits((uint)spec.SubProfileIdcs.Length, 8);
            foreach (uint sp in spec.SubProfileIdcs)
            {
                w.WriteBits(sp, 32);
            }

            w.WriteBit(spec.GeneralTimingHrdParametersPresentFlag);
            w.WriteBit(spec.ExtensionFlag);

            // rbsp_trailing_bits: stop bit + alignment zeros.
            w.WriteBit(true);
            while ((w.BitCount & 7) != 0)
            {
                w.WriteBit(false);
            }

            byte[] rbsp = w.ToArray();
            byte[] escaped = AddEmulationPreventionBytes(rbsp);

            byte b0 = 0;
            if (spec.ForbiddenZeroBit) b0 |= 0x80;
            if (spec.ReservedZeroBit) b0 |= 0x40;
            b0 |= (byte)(spec.LayerId & 0x3F);

            byte b1 = (byte)(((spec.NalUnitTypeOverride & 0x1F) << 3) | 1);

            var result = new byte[escaped.Length + 2];
            result[0] = b0;
            result[1] = b1;
            escaped.CopyTo(result, 2);
            return result;
        }

        private static byte[] AddEmulationPreventionBytes(byte[] rbsp)
        {
            var output = new List<byte>(rbsp.Length + 4);
            int zeroRun = 0;
            foreach (byte b in rbsp)
            {
                if (zeroRun >= 2 && b <= 0x03)
                {
                    output.Add(0x03);
                    zeroRun = 0;
                }
                output.Add(b);
                if (b == 0) zeroRun++;
                else zeroRun = 0;
            }
            return output.ToArray();
        }
    }

    private sealed class BitWriter
    {
        private readonly List<byte> _bytes = new();
        private byte _current;
        private int _bitsInCurrent;

        public int BitCount => _bytes.Count * 8 + _bitsInCurrent;

        public void WriteBit(bool value)
        {
            _current = (byte)((_current << 1) | (value ? 1 : 0));
            _bitsInCurrent++;
            if (_bitsInCurrent == 8)
            {
                _bytes.Add(_current);
                _current = 0;
                _bitsInCurrent = 0;
            }
        }

        public void WriteBits(uint value, int count)
        {
            for (int i = count - 1; i >= 0; i--)
            {
                WriteBit(((value >> i) & 1) == 1);
            }
        }

        public byte[] ToArray()
        {
            if (_bitsInCurrent != 0)
            {
                _bytes.Add((byte)(_current << (8 - _bitsInCurrent)));
            }
            return _bytes.ToArray();
        }
    }
}
