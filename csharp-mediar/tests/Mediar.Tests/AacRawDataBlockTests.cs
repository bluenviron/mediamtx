using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacRawDataBlockTests
{
    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AacRawDataBlock.TryParse(ReadOnlySpan<byte>.Empty, out var block));
        Assert.Null(block);
    }

    [Fact]
    public void TryParse_End_Only_Returns_Single_Terminal_Entry()
    {
        // 3-bit id=7 (End) at bit 0 → first byte = 0b1110_0000 = 0xE0
        byte[] bytes = [0xE0];
        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Single(block.Entries);
        Assert.Equal(AacSyntacticElementType.End, block.Entries[0].Type);
        Assert.Equal(0, block.Entries[0].BitOffset);
        Assert.Equal(3, block.BitsConsumed);
    }

    [Fact]
    public void TryParse_Pce_Then_End_RoundTrips_Both_Entries()
    {
        var pce = MinimalStereoPce();
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.ProgramConfigElement, 3);
        pce.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.ProgramConfigElement, block.Entries[0].Type);
        Assert.NotNull(block.Entries[0].ProgramConfig);
        Assert.Equal(pce.SamplingFrequencyIndex, block.Entries[0].ProgramConfig!.SamplingFrequencyIndex);
        Assert.Equal(AacSyntacticElementType.End, block.Entries[1].Type);
    }

    [Fact]
    public void TryParse_Dse_Then_End_RoundTrips()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 3,
            DataByteAlignFlag = true,
            Data = new byte[] { 0x10, 0x20, 0x30 },
        };
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.DataStreamElement, 3);
        dse.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.DataStreamElement, block.Entries[0].Type);
        Assert.NotNull(block.Entries[0].DataStream);
        Assert.Equal(dse.ElementInstanceTag, block.Entries[0].DataStream!.ElementInstanceTag);
        Assert.Equal(dse.Data.ToArray(), block.Entries[0].DataStream!.Data.ToArray());
    }

    [Fact]
    public void TryParse_Pce_Dse_End_Walks_All_Three()
    {
        var pce = MinimalStereoPce();
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 1,
            DataByteAlignFlag = false,
            Data = new byte[] { 0xAA, 0xBB },
        };

        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.ProgramConfigElement, 3);
        pce.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.DataStreamElement, 3);
        dse.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(3, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.ProgramConfigElement, block.Entries[0].Type);
        Assert.Equal(AacSyntacticElementType.DataStreamElement, block.Entries[1].Type);
        Assert.Equal(AacSyntacticElementType.End, block.Entries[2].Type);
    }

    [Fact]
    public void TryParse_Fill_Short_Count_RoundTrips_Bytes()
    {
        byte[] payload = [0x12, 0x34, 0x56, 0x78];
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write((uint)payload.Length, 4); // count = 4 (no escape)
        for (int i = 0; i < payload.Length; i++) writer.Write(payload[i], 8);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.FillElement, block.Entries[0].Type);
        Assert.Equal(payload, block.Entries[0].FillExtensionBytes.ToArray());
    }

    [Fact]
    public void TryParse_Fill_Escape_Count_RoundTrips_Bytes()
    {
        // count = 15 triggers esc_count read; cnt = 14 + esc_count. With esc = 20, cnt = 34.
        byte[] payload = new byte[34];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(0xC3 ^ i);

        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(15u, 4); // count escape
        writer.Write(20u, 8); // esc_count
        for (int i = 0; i < payload.Length; i++) writer.Write(payload[i], 8);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(34, block.Entries[0].FillExtensionBytes.Length);
        Assert.Equal(payload, block.Entries[0].FillExtensionBytes.ToArray());
    }

    [Fact]
    public void TryParse_Fill_Zero_Count_RoundTrips_Empty_Bytes()
    {
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(0u, 4); // count = 0
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(AacSyntacticElementType.FillElement, block.Entries[0].Type);
        Assert.Equal(0, block.Entries[0].FillExtensionBytes.Length);
    }

    [Theory]
    [InlineData(AacSyntacticElementType.SingleChannelElement)]
    [InlineData(AacSyntacticElementType.ChannelPairElement)]
    [InlineData(AacSyntacticElementType.CouplingChannelElement)]
    [InlineData(AacSyntacticElementType.LfeChannelElement)]
    public void TryParse_Audio_Element_Surfaces_Opaque_Marker_And_Stops(AacSyntacticElementType audioType)
    {
        // SCE/CPE/CCE/LFE bodies can't be parsed yet - dispatcher must
        // surface an opaque entry and stop. Place a DSE *after* the audio
        // element to verify it's NOT reached.
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = new byte[] { 0x99 },
        };

        var writer = new AacBitWriter();
        writer.Write((uint)audioType, 3);
        // Pretend body bytes - the dispatcher should not consume any of these.
        writer.Write(0xDEADBEEFu, 32);
        writer.Write((uint)AacSyntacticElementType.DataStreamElement, 3);
        dse.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.NotNull(block);
        Assert.False(block!.TerminatedByEnd);
        Assert.Single(block.Entries);
        Assert.Equal(audioType, block.Entries[0].Type);
        Assert.Null(block.Entries[0].ProgramConfig);
        Assert.Null(block.Entries[0].DataStream);
        Assert.Equal(0, block.Entries[0].FillExtensionBytes.Length);
        // Dispatcher stopped right after the 3-bit element id.
        Assert.Equal(3, block.BitsConsumed);
    }

    [Fact]
    public void TryParse_Pce_Then_Sce_Returns_Pce_Plus_Opaque()
    {
        var pce = MinimalStereoPce();
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.ProgramConfigElement, 3);
        pce.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        writer.Write(0xFFFFu, 16); // garbage body
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.False(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.ProgramConfigElement, block.Entries[0].Type);
        Assert.NotNull(block.Entries[0].ProgramConfig);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, block.Entries[1].Type);
    }

    [Fact]
    public void TryParse_Truncated_Dse_Body_Returns_False()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = new byte[] { 1, 2, 3, 4, 5 },
        };

        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.DataStreamElement, 3);
        dse.WriteTo(writer);
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.False(AacRawDataBlock.TryParse(bytes.AsSpan(0, bytes.Length - 2), out var block));
        Assert.Null(block);
    }

    [Fact]
    public void TryParse_Truncated_Fill_Payload_Returns_False()
    {
        // FIL with count=4 promising 4 bytes; supply only 2.
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(4u, 4);
        writer.Write(0xAAu, 8);
        writer.Write(0xBBu, 8);
        byte[] bytes = writer.ToArray();

        Assert.False(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.Null(block);
    }

    [Fact]
    public void BitOffset_Tracks_Cursor_Across_Elements()
    {
        // End at bit 0 = single-element rdb. End at bit 3 follows a 3-bit FIL id with empty body.
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(0u, 4); // FIL count = 0
        writer.Write((uint)AacSyntacticElementType.End, 3);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.Equal(2, block!.Entries.Count);
        Assert.Equal(0, block.Entries[0].BitOffset);
        Assert.Equal(3 + 4, block.Entries[1].BitOffset); // FIL id + 4-bit count
    }

    [Fact]
    public void TryParse_Stream_Exhausted_Cleanly_Without_End_Returns_Success_NotTerminated()
    {
        // Single FIL with count=0 then buffer ends - well-formed up to here but no END.
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(0u, 4);
        byte[] bytes = writer.ToArray();

        Assert.True(AacRawDataBlock.TryParse(bytes, out var block));
        Assert.False(block!.TerminatedByEnd);
        Assert.Single(block.Entries);
        Assert.Equal(AacSyntacticElementType.FillElement, block.Entries[0].Type);
    }

    private static AacProgramConfigurationElement MinimalStereoPce() => new()
    {
        ElementInstanceTag = 0,
        ObjectType = 1,
        SamplingFrequencyIndex = 4,
        FrontElements =
        [
            new AacPceChannelSlot { IsCpe = true, TagSelect = 0 },
        ],
        SideElements = [],
        BackElements = [],
        LfeElements = [],
        AssocDataElements = [],
        CouplingElements = [],
        CommentField = string.Empty,
    };

    // Context-driven "full" overload tests

    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    internal static AacHuffmanCodebook GetSharedSyntheticSfCodebook() => BuildSyntheticSfCodebook();

    private static AacRawDataBlockContext BuildContext()
    {
        return new AacRawDataBlockContext
        {
            SampleRate = 48_000,
            ScaleFactorCodebook = BuildSyntheticSfCodebook(),
            SpectralCodebooks = new AacHuffmanCodebook?[16],
        };
    }

    private static void WriteEmptySceBody(AacBitWriter w, int tag, int maxSfb)
    {
        w.Write((uint)tag, 4);                 // element_instance_tag
        w.Write(0u, 8);                        // global_gain
        w.Write(0u, 1);                        // ics_reserved_bit
        w.Write((uint)AacWindowSequence.OnlyLong, 2);
        w.Write(0u, 1);                        // window_shape
        w.Write((uint)maxSfb, 6);              // max_sfb
        w.Write(0u, 1);                        // predictor_data_present
        w.Write(0u, 4);                        // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)maxSfb, 5);              // sect_len
        w.Write(0u, 1);                        // pulse_data_present
        w.Write(0u, 1);                        // tns_data_present
        w.Write(0u, 1);                        // gain_control_data_present
    }

    internal static void WriteEmptySceBodyShared(AacBitWriter w, int tag, int maxSfb)
        => WriteEmptySceBody(w, tag, maxSfb);

    private static void WriteEmptyLfeBody(AacBitWriter w, int tag, int maxSfb)
    {
        // LFE is bit-for-bit identical to SCE.
        WriteEmptySceBody(w, tag, maxSfb);
    }

    [Fact]
    public void TryParse_WithContext_NullContext_Throws()
    {
        byte[] bytes = [0xE0];
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlock.TryParse(bytes, context: null!, out _));
    }

    [Fact]
    public void TryParse_WithContext_EndOnly_StillWorks()
    {
        byte[] bytes = [0xE0];
        Assert.True(AacRawDataBlock.TryParse(bytes, BuildContext(), out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Single(block.Entries);
    }

    [Fact]
    public void TryParse_WithContext_SceFollowedByEnd_ConsumesSceAndContinues()
    {
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptySceBody(w, tag: 2, maxSfb: 10);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, block.Entries[0].Type);
        Assert.NotNull(block.Entries[0].SingleChannel);
        Assert.Equal(2, block.Entries[0].SingleChannel!.ElementInstanceTag);
        Assert.NotNull(block.Entries[0].SingleChannel!.SpectralData);
        Assert.Equal(AacSyntacticElementType.End, block.Entries[1].Type);
    }

    [Fact]
    public void TryParse_WithContext_LfeFollowedByEnd_ConsumesLfeAndContinues()
    {
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.LfeChannelElement, 3);
        WriteEmptyLfeBody(w, tag: 1, maxSfb: 6);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.NotNull(block.Entries[0].LowFrequency);
        Assert.Equal(1, block.Entries[0].LowFrequency!.ElementInstanceTag);
    }

    [Fact]
    public void TryParse_WithoutContext_StopsAtAudioElement_UnchangedBehavior()
    {
        // Backwards-compat: the existing single-arg overload still stops
        // at the first audio element with no payload populated.
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptySceBody(w, tag: 2, maxSfb: 10);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        Assert.NotNull(block);
        Assert.False(block!.TerminatedByEnd);
        Assert.Single(block.Entries);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, block.Entries[0].Type);
        Assert.Null(block.Entries[0].SingleChannel);
    }

    [Fact]
    public void TryParse_WithContext_MultipleSceElements_AllConsumed()
    {
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptySceBody(w, tag: 0, maxSfb: 8);
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptySceBody(w, tag: 1, maxSfb: 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(3, block.Entries.Count);
        Assert.Equal(0, block.Entries[0].SingleChannel!.ElementInstanceTag);
        Assert.Equal(1, block.Entries[1].SingleChannel!.ElementInstanceTag);
    }

    [Fact]
    public void TryParse_WithContext_MalformedSce_Fails()
    {
        var ctx = BuildContext();
        // Just the SCE id with no body - should fail rather than terminate gracefully.
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        // No SCE body bytes follow.
        Assert.False(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.Null(block);
    }

    // FromAudioSpecificConfig factory tests

    private static AudioSpecificConfig BuildAsc(
        int aot = 2,
        int sfIndex = 3,            // 48000 Hz
        int sampleRate = 48_000,
        int channelConfig = 2,
        bool sbrPresent = false,
        int extSampleRate = 0,
        int extSfIndex = -1)
    {
        return new AudioSpecificConfig
        {
            AudioObjectType = aot,
            SamplingFrequencyIndex = sfIndex,
            SamplingFrequency = sampleRate,
            ChannelConfiguration = channelConfig,
            ChannelCount = 2,
            SbrPresent = sbrPresent,
            ExtensionSamplingFrequency = extSampleRate,
            ExtensionSamplingFrequencyIndex = extSfIndex,
        };
    }

    [Fact]
    public void FromAudioSpecificConfig_NullConfig_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockContext.FromAudioSpecificConfig(
                config: null!,
                scaleFactorCodebook: BuildSyntheticSfCodebook(),
                spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void FromAudioSpecificConfig_NullSfCodebook_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockContext.FromAudioSpecificConfig(
                config: BuildAsc(),
                scaleFactorCodebook: null!,
                spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void FromAudioSpecificConfig_NullSpectralBooks_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacRawDataBlockContext.FromAudioSpecificConfig(
                config: BuildAsc(),
                scaleFactorCodebook: BuildSyntheticSfCodebook(),
                spectralCodebooks: null!));
    }

    [Fact]
    public void FromAudioSpecificConfig_AacLc48k_UsesRateDirectly()
    {
        var asc = BuildAsc(aot: 2, sfIndex: 3, sampleRate: 48_000);
        var ctx = AacRawDataBlockContext.FromAudioSpecificConfig(
            asc,
            BuildSyntheticSfCodebook(),
            new AacHuffmanCodebook?[16]);
        Assert.Equal(48_000, ctx.SampleRate);
    }

    [Fact]
    public void FromAudioSpecificConfig_AacLc44k1_UsesRateDirectly()
    {
        var asc = BuildAsc(aot: 2, sfIndex: 4, sampleRate: 44_100);
        var ctx = AacRawDataBlockContext.FromAudioSpecificConfig(
            asc,
            BuildSyntheticSfCodebook(),
            new AacHuffmanCodebook?[16]);
        Assert.Equal(44_100, ctx.SampleRate);
    }

    [Fact]
    public void FromAudioSpecificConfig_HeAacSbr48k_DerivesCoreRate24k()
    {
        // HE-AAC: SBR layer reports 48 kHz, AAC-LC core operates at 24 kHz.
        var asc = BuildAsc(
            aot: 2,
            sfIndex: 3,
            sampleRate: 48_000,
            sbrPresent: true,
            extSfIndex: 3,
            extSampleRate: 48_000);
        var ctx = AacRawDataBlockContext.FromAudioSpecificConfig(
            asc,
            BuildSyntheticSfCodebook(),
            new AacHuffmanCodebook?[16]);
        Assert.Equal(24_000, ctx.SampleRate);
    }

    [Fact]
    public void FromAudioSpecificConfig_HeAacSbr44k1_DerivesCoreRate22k05()
    {
        // HE-AAC: SBR @ 44.1 kHz, AAC-LC core @ 22.05 kHz.
        var asc = BuildAsc(
            aot: 2,
            sfIndex: 4,
            sampleRate: 44_100,
            sbrPresent: true,
            extSfIndex: 4,
            extSampleRate: 44_100);
        var ctx = AacRawDataBlockContext.FromAudioSpecificConfig(
            asc,
            BuildSyntheticSfCodebook(),
            new AacHuffmanCodebook?[16]);
        Assert.Equal(22_050, ctx.SampleRate);
    }

    [Fact]
    public void FromAudioSpecificConfig_ZeroSampleRate_Throws()
    {
        var asc = BuildAsc(sampleRate: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacRawDataBlockContext.FromAudioSpecificConfig(
                asc,
                BuildSyntheticSfCodebook(),
                new AacHuffmanCodebook?[16]));
        Assert.Contains("no valid sample rate", ex.Message);
    }

    [Fact]
    public void FromAudioSpecificConfig_NonStandardCoreRate_Throws()
    {
        // 192 kHz isn't in the AAC SWB offset tables.
        var asc = BuildAsc(sfIndex: 15, sampleRate: 192_000);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacRawDataBlockContext.FromAudioSpecificConfig(
                asc,
                BuildSyntheticSfCodebook(),
                new AacHuffmanCodebook?[16]));
        Assert.Contains("not a standard AAC rate", ex.Message);
    }

    [Fact]
    public void FromAudioSpecificConfig_DerivedContext_DrivesEndOnlyParseSuccessfully()
    {
        // Smoke test: the factory output is structurally a valid context.
        var asc = BuildAsc();
        var ctx = AacRawDataBlockContext.FromAudioSpecificConfig(
            asc,
            BuildSyntheticSfCodebook(),
            new AacHuffmanCodebook?[16]);
        byte[] bytes = [0xE0];
        Assert.True(AacRawDataBlock.TryParse(bytes, ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
    }

    // ----- EightShort window raw_data_block coverage -----

    private static void WriteEmptyShortSceBody(AacBitWriter w, int tag, int maxSfb, byte grouping)
    {
        w.Write((uint)tag, 4);                              // element_instance_tag
        w.Write(0u, 8);                                     // global_gain
        w.Write(0u, 1);                                     // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2);     // window_sequence
        w.Write(0u, 1);                                     // window_shape (Sine)
        w.Write((uint)maxSfb, 4);                           // max_sfb (4 bits for short)
        w.Write(grouping, 7);                               // scale_factor_grouping
        // One ZERO_HCB section per group.
        int groupCount = 1;
        for (int i = 1; i < 8; i++) if (((grouping >> (7 - i)) & 1) == 0) groupCount++;
        for (int g = 0; g < groupCount; g++)
        {
            w.Write(0u, 4);                                 // sect_cb = 0 (ZERO_HCB)
            w.Write((uint)maxSfb, 3);                       // sect_len_incr (short: 3 bits, < 7)
        }
        w.Write(0u, 1);                                     // pulse_data_present (forbidden when 1 for short)
        w.Write(0u, 1);                                     // tns_data_present
        w.Write(0u, 1);                                     // gain_control_data_present
    }

    [Fact]
    public void TryParse_WithContext_EightShortSceFollowedByEnd_ConsumesSceWithShortIcs()
    {
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptyShortSceBody(w, tag: 4, maxSfb: 4, grouping: 0x7F);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, block.Entries[0].Type);
        Assert.NotNull(block.Entries[0].SingleChannel);
        Assert.Equal(4, block.Entries[0].SingleChannel!.ElementInstanceTag);
        Assert.Equal(
            AacWindowSequence.EightShort,
            block.Entries[0].SingleChannel!.Stream.IcsInfo.WindowSequence);
        Assert.NotNull(block.Entries[0].SingleChannel!.SpectralData);
    }

    [Fact]
    public void TryParse_WithContext_EightShortLfeFollowedByEnd_ConsumesLfeWithShortIcs()
    {
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.LfeChannelElement, 3);
        WriteEmptyShortSceBody(w, tag: 1, maxSfb: 4, grouping: 0x7F); // LFE shares the SCE body shape.
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(2, block.Entries.Count);
        Assert.NotNull(block.Entries[0].LowFrequency);
        Assert.Equal(1, block.Entries[0].LowFrequency!.ElementInstanceTag);
        Assert.Equal(
            AacWindowSequence.EightShort,
            block.Entries[0].LowFrequency!.Stream.IcsInfo.WindowSequence);
    }

    [Fact]
    public void TryParse_WithContext_MixedLongAndShortSces_BothConsumed()
    {
        // Verify a raw_data_block can mix per-element window sequences across
        // back-to-back SCEs (one OnlyLong, one EightShort).
        var ctx = BuildContext();
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptySceBody(w, tag: 0, maxSfb: 6);
        w.Write((uint)AacSyntacticElementType.SingleChannelElement, 3);
        WriteEmptyShortSceBody(w, tag: 1, maxSfb: 4, grouping: 0x7F);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), ctx, out var block));
        Assert.NotNull(block);
        Assert.True(block!.TerminatedByEnd);
        Assert.Equal(3, block.Entries.Count);
        Assert.Equal(
            AacWindowSequence.OnlyLong,
            block.Entries[0].SingleChannel!.Stream.IcsInfo.WindowSequence);
        Assert.Equal(
            AacWindowSequence.EightShort,
            block.Entries[1].SingleChannel!.Stream.IcsInfo.WindowSequence);
    }
}
