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
}
