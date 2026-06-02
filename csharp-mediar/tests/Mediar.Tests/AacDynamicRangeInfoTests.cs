using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacDynamicRangeInfoTests
{
    [Fact]
    public void TryParse_NegativeBodyBitLength_Returns_False()
    {
        Assert.False(AacDynamicRangeInfo.TryParse(new byte[1], -1, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_TooSmallForBodyBitLength_Returns_False()
    {
        // Claim 16 bits of body but only supply 1 byte.
        Assert.False(AacDynamicRangeInfo.TryParse(new byte[1], 16, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AacDynamicRangeInfo.TryParse(ReadOnlySpan<byte>.Empty, 0, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_Minimal_All_Flags_Off_Default_One_Band()
    {
        // pce_tag_present=0, excluded=0, drc_bands=0, prog_ref=0 → 4 bits.
        // Then drc_num_bands defaults to 1 → 1 sign bit + 7-bit ctl = 8 bits.
        // Total = 12 bits.
        var w = new AacBitWriter();
        w.Write(0u, 1); // pce_tag_present
        w.Write(0u, 1); // excluded_chns_present
        w.Write(0u, 1); // drc_bands_present
        w.Write(0u, 1); // prog_ref_level_present
        w.Write(1u, 1); // dyn_rng_sgn[0] = 1
        w.Write(0x55u, 7); // dyn_rng_ctl[0] = 0x55
        int bodyBits = 12;
        var bytes = w.ToArray();

        Assert.True(AacDynamicRangeInfo.TryParse(bytes, bodyBits, out var info));
        Assert.NotNull(info);
        Assert.False(info!.PceTagPresent);
        Assert.False(info.ExcludedChannelsPresent);
        Assert.Null(info.ExcludedChannels);
        Assert.False(info.DrcBandsPresent);
        Assert.Equal(1, info.DrcNumBands);
        Assert.Equal(0, info.DrcBandTop.Length);
        Assert.False(info.ProgRefLevelPresent);
        Assert.Single(info.Bands);
        Assert.True(info.Bands[0].Sign);
        Assert.Equal((byte)0x55, info.Bands[0].Ctl);
        Assert.Equal(12, info.BitsConsumed);
    }

    [Fact]
    public void TryParse_WithPceTag_Parses_Both_Fields()
    {
        var w = new AacBitWriter();
        w.Write(1u, 1); // pce_tag_present
        w.Write(0xAu, 4); // pce_instance_tag
        w.Write(0x5u, 4); // drc_tag_reserved_bits
        w.Write(0u, 1); // excluded_chns_present
        w.Write(0u, 1); // drc_bands_present
        w.Write(0u, 1); // prog_ref_level_present
        w.Write(0u, 1); // dyn_rng_sgn[0]
        w.Write(0x21u, 7); // dyn_rng_ctl[0]
        int bodyBits = 1 + 8 + 1 + 1 + 1 + 8;
        var bytes = w.ToArray();

        Assert.True(AacDynamicRangeInfo.TryParse(bytes, bodyBits, out var info));
        Assert.True(info!.PceTagPresent);
        Assert.Equal((byte)0xA, info.PceInstanceTag);
        Assert.Equal((byte)0x5, info.DrcTagReservedBits);
        Assert.False(info.Bands[0].Sign);
        Assert.Equal((byte)0x21, info.Bands[0].Ctl);
    }

    [Fact]
    public void TryParse_WithExcludedChannels_SingleChunk()
    {
        // exclude_mask = 0b1010101, additional_excluded_chns = 0 → one chunk byte
        // = 0b10101010 = 0xAA.
        var w = new AacBitWriter();
        w.Write(0u, 1); // pce_tag_present
        w.Write(1u, 1); // excluded_chns_present
        w.Write(0x55u, 7); // exclude_mask
        w.Write(0u, 1); // additional_excluded_chns
        w.Write(0u, 1); // drc_bands_present
        w.Write(0u, 1); // prog_ref_level_present
        w.Write(0u, 1); // dyn_rng_sgn[0]
        w.Write(0x33u, 7); // dyn_rng_ctl[0]
        int bodyBits = 1 + 1 + 8 + 1 + 1 + 8;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.True(info!.ExcludedChannelsPresent);
        Assert.NotNull(info.ExcludedChannels);
        Assert.Equal(1, info.ExcludedChannels!.ChunkCount);
        Assert.Equal(8, info.ExcludedChannels.BitsConsumed);
        Assert.Equal(new byte[] { 0xAA }, info.ExcludedChannels.ChunkBytes.ToArray());
    }

    [Fact]
    public void TryParse_WithExcludedChannels_MultipleChunks()
    {
        // 3 chunks: first two have additional_excluded_chns=1, last has 0.
        // exclude_masks: 0x7F, 0x00, 0x2A.
        // chunk0 = (0x7F<<1) | 1 = 0xFF
        // chunk1 = (0x00<<1) | 1 = 0x01
        // chunk2 = (0x2A<<1) | 0 = 0x54
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(0x7Fu, 7); w.Write(1u, 1);
        w.Write(0x00u, 7); w.Write(1u, 1);
        w.Write(0x2Au, 7); w.Write(0u, 1);
        w.Write(0u, 1); // drc_bands_present
        w.Write(0u, 1); // prog_ref_level_present
        w.Write(1u, 1); w.Write(0x44u, 7);
        int bodyBits = 1 + 1 + 24 + 1 + 1 + 8;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Equal(3, info!.ExcludedChannels!.ChunkCount);
        Assert.Equal(new byte[] { 0xFF, 0x01, 0x54 }, info.ExcludedChannels.ChunkBytes.ToArray());
    }

    [Fact]
    public void TryParse_WithDrcBands_FullBandSet()
    {
        // drc_band_incr = 3 → drc_num_bands = 4. drc_interpolation_scheme = 5.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(1u, 1); // drc_bands_present
        w.Write(3u, 4); // drc_band_incr
        w.Write(5u, 4); // drc_interpolation_scheme
        w.Write(0x10u, 8); // drc_band_top[0]
        w.Write(0x20u, 8); // drc_band_top[1]
        w.Write(0x30u, 8); // drc_band_top[2]
        w.Write(0x40u, 8); // drc_band_top[3]
        w.Write(0u, 1); // prog_ref_level_present
        // 4 (sign, ctl) pairs:
        w.Write(1u, 1); w.Write(0x01u, 7);
        w.Write(0u, 1); w.Write(0x02u, 7);
        w.Write(1u, 1); w.Write(0x03u, 7);
        w.Write(0u, 1); w.Write(0x04u, 7);
        int bodyBits = 1 + 1 + 1 + 8 + 32 + 1 + 32;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.True(info!.DrcBandsPresent);
        Assert.Equal((byte)3, info.DrcBandIncr);
        Assert.Equal((byte)5, info.DrcInterpolationScheme);
        Assert.Equal(4, info.DrcNumBands);
        Assert.Equal(new byte[] { 0x10, 0x20, 0x30, 0x40 }, info.DrcBandTop.ToArray());
        Assert.Equal(4, info.Bands.Count);
        Assert.True(info.Bands[0].Sign); Assert.Equal((byte)0x01, info.Bands[0].Ctl);
        Assert.False(info.Bands[1].Sign); Assert.Equal((byte)0x02, info.Bands[1].Ctl);
        Assert.True(info.Bands[2].Sign); Assert.Equal((byte)0x03, info.Bands[2].Ctl);
        Assert.False(info.Bands[3].Sign); Assert.Equal((byte)0x04, info.Bands[3].Ctl);
    }

    [Fact]
    public void TryParse_WithProgRefLevel_Parses_Both_Fields()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(1u, 1); // prog_ref_level_present
        w.Write(0x73u, 7); // prog_ref_level
        w.Write(1u, 1); // reserved bit
        w.Write(0u, 1); w.Write(0x00u, 7);
        int bodyBits = 1 + 1 + 1 + 1 + 8 + 8;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.True(info!.ProgRefLevelPresent);
        Assert.Equal((byte)0x73, info.ProgRefLevel);
        Assert.Equal((byte)1, info.ProgRefLevelReservedBit);
    }

    [Fact]
    public void TryParse_AllFlags_FullRoundTrip()
    {
        // PCE tag + 2 excluded chunks + 16 bands + prog ref level.
        var w = new AacBitWriter();
        w.Write(1u, 1); w.Write(0x3u, 4); w.Write(0xCu, 4); // PCE tag block
        w.Write(1u, 1); // excluded present
        w.Write(0x44u, 7); w.Write(1u, 1);
        w.Write(0x22u, 7); w.Write(0u, 1);
        w.Write(1u, 1); // drc bands present
        w.Write(15u, 4); // drc_band_incr → drc_num_bands = 16
        w.Write(7u, 4); // drc_interpolation_scheme
        for (int i = 0; i < 16; i++) w.Write((uint)(0x10 + i), 8);
        w.Write(1u, 1); // prog_ref_level_present
        w.Write(0x55u, 7); w.Write(0u, 1);
        for (int i = 0; i < 16; i++)
        {
            w.Write((uint)(i & 1), 1);
            w.Write((uint)(i * 7 & 0x7F), 7);
        }
        int bodyBits =
            1 + 8 +              // pce tag block
            1 + 16 +             // excluded (2 chunks)
            1 + 8 + 16 * 8 +     // drc bands header + drc_band_top[16]
            1 + 8 +              // prog ref level
            16 * 8;              // 16 (sgn, ctl) pairs

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.True(info!.PceTagPresent);
        Assert.Equal((byte)0x3, info.PceInstanceTag);
        Assert.Equal((byte)0xC, info.DrcTagReservedBits);
        Assert.Equal(2, info.ExcludedChannels!.ChunkCount);
        Assert.Equal(16, info.DrcNumBands);
        Assert.Equal(16, info.DrcBandTop.Length);
        Assert.Equal((byte)0x10, info.DrcBandTop.Span[0]);
        Assert.Equal((byte)0x1F, info.DrcBandTop.Span[15]);
        Assert.True(info.ProgRefLevelPresent);
        Assert.Equal((byte)0x55, info.ProgRefLevel);
        Assert.Equal(16, info.Bands.Count);
        Assert.Equal(bodyBits, info.BitsConsumed);
    }

    [Fact]
    public void TryParse_BudgetExceeded_Returns_False()
    {
        // Header demands a 4-band drc set + final 4 pair bytes, but we
        // claim a smaller budget than the structure needs.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(3u, 4);
        w.Write(0u, 4);
        for (int i = 0; i < 4; i++) w.Write(0u, 8);
        w.Write(0u, 1);
        for (int i = 0; i < 4; i++) { w.Write(0u, 1); w.Write(0u, 7); }
        int realBits = 1 + 1 + 1 + 8 + 32 + 1 + 32;

        Assert.False(AacDynamicRangeInfo.TryParse(w.ToArray(), realBits - 1, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void Dispatcher_Populates_DynamicRange_For_DRC_Fill()
    {
        // Build a FIL element with extension_type = 0xB and a minimal DRC body
        // (4 flag bits + 1 default band pair = 12 bits). To fit cleanly into FIL
        // (cnt * 8 bits, leading 4 = type), we need 12 body bits → cnt * 8 - 4 = 12
        // → cnt = 2.
        var bodyWriter = new AacBitWriter();
        // dynamic_range_info(): all flags off then 1 default band (sign=1, ctl=0x42)
        bodyWriter.Write(0u, 4);
        bodyWriter.Write(1u, 1);
        bodyWriter.Write(0x42u, 7);
        byte[] bodyBytes = bodyWriter.ToArray(); // 12 bits → padded to 2 bytes
        Assert.Equal(2, bodyBytes.Length);

        // FIL bytes = [type | body[0]>>4, (body[0]<<4) | body[1]>>4]
        // But our payload encoder is simpler: write type nibble first, then 12 body bits.
        var filWriter = new AacBitWriter();
        filWriter.Write(0xBu, 4); // extension_type
        // Copy 12 body bits.
        var bodyReader = new BitReader(bodyBytes);
        for (int i = 0; i < 12; i++) filWriter.Write(bodyReader.ReadBit() ? 1u : 0u, 1);
        byte[] filBytes = filWriter.ToArray();
        Assert.Equal(2, filBytes.Length);

        // Now wrap in a raw_data_block: FIL id (3) + count nibble + 2 bytes + END (3).
        var rdb = new AacBitWriter();
        rdb.Write((uint)AacSyntacticElementType.FillElement, 3);
        rdb.Write(2u, 4); // cnt = 2
        for (int i = 0; i < filBytes.Length; i++) rdb.Write(filBytes[i], 8);
        rdb.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(rdb.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.NotNull(fil.FillExtension);
        Assert.Equal(AacFillExtensionType.DynamicRange, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.DynamicRange);
        Assert.Equal(1, fil.FillExtension.DynamicRange!.DrcNumBands);
        Assert.True(fil.FillExtension.DynamicRange.Bands[0].Sign);
        Assert.Equal((byte)0x42, fil.FillExtension.DynamicRange.Bands[0].Ctl);
    }

    [Fact]
    public void Dispatcher_Leaves_DynamicRange_Null_For_Non_DRC_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0xD0u, 8); // type 0xD (SBR)
        w.Write(0x00u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.SbrData, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.DynamicRange);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(-100)]
    [InlineData(int.MinValue)]
    public void TryParse_NegativeBudget_AnyLength_Returns_False(int budget)
    {
        Assert.False(AacDynamicRangeInfo.TryParse(new byte[8], budget, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_BodyBitLength_Zero_Returns_False()
    {
        // Header insists on at least one bit for pce_tag_present.
        var bytes = new byte[1];
        Assert.False(AacDynamicRangeInfo.TryParse(bytes, 0, out var info));
        Assert.Null(info);
    }

    [Theory]
    [InlineData(0, 1)]
    [InlineData(1, 2)]
    [InlineData(3, 4)]
    [InlineData(7, 8)]
    [InlineData(15, 16)]
    public void TryParse_DrcBandIncr_Maps_To_DrcNumBands(int incr, int expectedBands)
    {
        var w = new AacBitWriter();
        w.Write(0u, 1); // pce_tag_present
        w.Write(0u, 1); // excluded_chns_present
        w.Write(1u, 1); // drc_bands_present
        w.Write((uint)incr, 4); // drc_band_incr
        w.Write(0u, 4); // drc_interpolation_scheme
        for (int i = 0; i < expectedBands; i++) w.Write(0u, 8);
        w.Write(0u, 1); // prog_ref_level_present
        for (int i = 0; i < expectedBands; i++)
        {
            w.Write(0u, 1); w.Write(0u, 7);
        }
        int bodyBits = 1 + 1 + 1 + 8 + (expectedBands * 8) + 1 + (expectedBands * 8);

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Equal(expectedBands, info!.DrcNumBands);
        Assert.Equal(expectedBands, info.DrcBandTop.Length);
        Assert.Equal(expectedBands, info.Bands.Count);
    }

    [Fact]
    public void TryParse_Defaults_Are_Zero_When_Flags_Off()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 7);
        int bodyBits = 12;
        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Equal(0, info!.PceInstanceTag);
        Assert.Equal(0, info.DrcTagReservedBits);
        Assert.Equal(0, info.DrcBandIncr);
        Assert.Equal(0, info.DrcInterpolationScheme);
        Assert.Equal(0, info.DrcBandTop.Length);
        Assert.Equal(0, info.ProgRefLevel);
        Assert.Equal(0, info.ProgRefLevelReservedBit);
        Assert.False(info.Bands[0].Sign);
    }

    [Fact]
    public void TryParse_ProgRefLevel_Boundaries_Round_Trip()
    {
        // Combined: both extremes for prog_ref_level (0 and 0x7F) verified
        // in one body via two consecutive parses of the same bytes.
        foreach (byte prl in new byte[] { 0x00, 0x7F })
        {
            var w = new AacBitWriter();
            w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
            w.Write(1u, 1); // prog_ref_level_present
            w.Write(prl, 7);
            w.Write(0u, 1); // reserved bit
            w.Write(0u, 1); w.Write(0u, 7);
            int bodyBits = 1 + 1 + 1 + 1 + 8 + 8;

            Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
            Assert.Equal(prl, info!.ProgRefLevel);
        }
    }

    [Fact]
    public void TryParse_DrcBands_BandTop_Truncation_Returns_False()
    {
        // Claim drc_band_incr=3 (4 bands) but only supply space for 2 band_top bytes.
        var w = new AacBitWriter();
        w.Write(0u, 1); w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(3u, 4); w.Write(0u, 4);
        w.Write(0u, 8); w.Write(0u, 8);
        int bodyBits = 1 + 1 + 1 + 8 + 16;
        Assert.False(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_ExcludedChannels_Truncated_MidChunk_Returns_False()
    {
        // Claim excluded_chns_present but only 4 bits remain in the budget.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(1u, 1); // excluded_chns_present
        w.Write(0u, 4); // only 4 bits of "mask" provided
        int bodyBits = 1 + 1 + 4;
        Assert.False(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Null(info);
    }

    [Fact]
    public void TryParse_ExcludedChannels_BitsConsumed_Matches_ChunkCount()
    {
        // 2 chunks → 16 bits in ExcludedChannels.BitsConsumed.
        var w = new AacBitWriter();
        w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(0x11u, 7); w.Write(1u, 1);
        w.Write(0x22u, 7); w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 7);
        int bodyBits = 1 + 1 + 16 + 1 + 1 + 8;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        Assert.Equal(2, info!.ExcludedChannels!.ChunkCount);
        Assert.Equal(16, info.ExcludedChannels.BitsConsumed);
    }

    [Fact]
    public void DrcBand_RecordStruct_Equality_And_Hash_Match()
    {
        var a = new AacDrcBand { Sign = true, Ctl = 0x21 };
        var b = new AacDrcBand { Sign = true, Ctl = 0x21 };
        var c = new AacDrcBand { Sign = false, Ctl = 0x21 };
        var d = new AacDrcBand { Sign = true, Ctl = 0x22 };

        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
        Assert.NotEqual(a, c);
        Assert.NotEqual(a, d);
    }

    [Fact]
    public void DynamicRangeInfo_With_Expression_Returns_Modified_Copy()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(1u, 1); w.Write(0x55u, 7);
        int bodyBits = 12;

        Assert.True(AacDynamicRangeInfo.TryParse(w.ToArray(), bodyBits, out var info));
        var modified = info! with { ProgRefLevel = 0x40, ProgRefLevelPresent = true };
        Assert.Equal((byte)0x40, modified.ProgRefLevel);
        Assert.True(modified.ProgRefLevelPresent);
        Assert.False(info.ProgRefLevelPresent);
        Assert.NotSame(info, modified);
    }

    [Fact]
    public void DynamicRangeInfo_Record_Equality_From_Identical_Bytes()
    {
        var w = new AacBitWriter();
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(1u, 1); w.Write(0x33u, 7);
        var bytes = w.ToArray();

        Assert.True(AacDynamicRangeInfo.TryParse(bytes, 12, out var a));
        Assert.True(AacDynamicRangeInfo.TryParse(bytes, 12, out var b));
        // Per-band content equal (DrcBand is a value record struct); top-
        // level record uses reference identity for ReadOnlyMemory fields,
        // so just compare the visible fields.
        Assert.Equal(a!.PceTagPresent, b!.PceTagPresent);
        Assert.Equal(a.DrcNumBands, b.DrcNumBands);
        Assert.Equal(a.Bands[0], b.Bands[0]);
        Assert.Equal(a.BitsConsumed, b.BitsConsumed);
    }
}
