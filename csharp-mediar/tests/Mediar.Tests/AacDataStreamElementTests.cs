using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacDataStreamElementTests
{
    [Fact]
    public void Enum_Values_Match_Spec_Table_4_71()
    {
        Assert.Equal(0, (int)AacSyntacticElementType.SingleChannelElement);
        Assert.Equal(1, (int)AacSyntacticElementType.ChannelPairElement);
        Assert.Equal(2, (int)AacSyntacticElementType.CouplingChannelElement);
        Assert.Equal(3, (int)AacSyntacticElementType.LfeChannelElement);
        Assert.Equal(4, (int)AacSyntacticElementType.DataStreamElement);
        Assert.Equal(5, (int)AacSyntacticElementType.ProgramConfigElement);
        Assert.Equal(6, (int)AacSyntacticElementType.FillElement);
        Assert.Equal(7, (int)AacSyntacticElementType.End);
    }

    [Fact]
    public void ToBytes_Then_TryParse_Empty_Aligned_RoundTrips()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = ReadOnlyMemory<byte>.Empty,
        };

        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded, out int consumed));
        Assert.NotNull(decoded);
        Assert.Equal(bytes.Length, consumed);

        Assert.Equal(0, decoded!.ElementInstanceTag);
        Assert.True(decoded.DataByteAlignFlag);
        Assert.Equal(0, decoded.Data.Length);
    }

    [Fact]
    public void ToBytes_Then_TryParse_Short_Payload_Aligned_RoundTrips()
    {
        byte[] payload = [0x10, 0x20, 0x30, 0x40, 0x50];
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 3,
            DataByteAlignFlag = true,
            Data = payload,
        };

        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded, out int consumed));
        Assert.NotNull(decoded);
        Assert.Equal(bytes.Length, consumed);

        Assert.Equal(3, decoded!.ElementInstanceTag);
        Assert.True(decoded.DataByteAlignFlag);
        Assert.Equal(payload, decoded.Data.ToArray());
    }

    [Fact]
    public void ToBytes_Then_TryParse_Short_Payload_Unaligned_RoundTrips()
    {
        byte[] payload = [0xAB, 0xCD, 0xEF];
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 7,
            DataByteAlignFlag = false,
            Data = payload,
        };

        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded, out int consumed));
        Assert.NotNull(decoded);
        Assert.Equal(bytes.Length, consumed);

        Assert.Equal(7, decoded!.ElementInstanceTag);
        Assert.False(decoded.DataByteAlignFlag);
        Assert.Equal(payload, decoded.Data.ToArray());
    }

    [Fact]
    public void ToBytes_Then_TryParse_BoundarySize_254_RoundTrips()
    {
        // Count = 254 fits in the 8-bit field without escape.
        byte[] payload = new byte[254];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i & 0xFF);

        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 1,
            DataByteAlignFlag = true,
            Data = payload,
        };

        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(payload, decoded!.Data.ToArray());
    }

    [Fact]
    public void ToBytes_Then_TryParse_BoundarySize_255_Triggers_EscapeCount_Zero()
    {
        // Count exactly 255 forces the escape branch with esc_count = 0.
        byte[] payload = new byte[255];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(0xA5 ^ (i & 0xFF));

        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 2,
            DataByteAlignFlag = true,
            Data = payload,
        };

        byte[] bytes = dse.ToBytes();
        // Header layout when escape triggers: 4-bit tag + 1-bit flag + 8-bit count (255)
        // + 8-bit esc_count (0) + byte_alignment + 255 bytes = 256 + 1 byte header = 257.
        Assert.Equal(255 + 3, bytes.Length);
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(payload, decoded!.Data.ToArray());
    }

    [Fact]
    public void ToBytes_Then_TryParse_MaxDataBytes_510_RoundTrips()
    {
        // 510 bytes = 255 count + 255 esc_count, the theoretical cap.
        byte[] payload = new byte[AacDataStreamElement.MaxDataBytes];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i & 0xFF);

        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0xF,
            DataByteAlignFlag = true,
            Data = payload,
        };

        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(0xF, decoded!.ElementInstanceTag);
        Assert.Equal(payload, decoded.Data.ToArray());
    }

    [Fact]
    public void ToBytes_Over_MaxDataBytes_Throws()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = new byte[AacDataStreamElement.MaxDataBytes + 1],
        };
        Assert.Throws<InvalidOperationException>(() => dse.ToBytes());
    }

    [Fact]
    public void ToBytes_Tag_Over_4_Bits_Throws()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 16,
            DataByteAlignFlag = false,
            Data = ReadOnlyMemory<byte>.Empty,
        };
        Assert.Throws<InvalidOperationException>(() => dse.ToBytes());
    }

    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AacDataStreamElement.TryParse(ReadOnlySpan<byte>.Empty, out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_Truncated_Header_Returns_False()
    {
        // Single byte cannot supply the 13-bit header.
        Assert.False(AacDataStreamElement.TryParse(new byte[] { 0xAA }, out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_Truncated_Payload_Returns_False()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = new byte[] { 1, 2, 3, 4, 5, 6, 7, 8 },
        };
        byte[] bytes = dse.ToBytes();
        // Trim the last data byte.
        Assert.False(AacDataStreamElement.TryParse(bytes.AsSpan(0, bytes.Length - 1), out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_Reports_Exact_BytesConsumed_With_Trailing_Garbage()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 5,
            DataByteAlignFlag = true,
            Data = new byte[] { 0x11, 0x22 },
        };
        byte[] inner = dse.ToBytes();

        byte[] padded = new byte[inner.Length + 4];
        inner.CopyTo(padded, 0);
        padded[inner.Length + 0] = 0xCA;
        padded[inner.Length + 1] = 0xFE;
        padded[inner.Length + 2] = 0xBA;
        padded[inner.Length + 3] = 0xBE;

        Assert.True(AacDataStreamElement.TryParse(padded, out var decoded, out int consumed));
        Assert.Equal(inner.Length, consumed);
        Assert.Equal(dse.Data.ToArray(), decoded!.Data.ToArray());
    }

    [Fact]
    public void Unaligned_DataByteAlignFlag_Skips_No_Padding()
    {
        // When the flag is clear, the 3 bits between the count field and
        // the first data byte are NOT padded - the next 8-bit read straddles
        // the byte boundary. Round-trip exercises that path.
        byte[] payload = new byte[] { 0x12, 0x34, 0x56, 0x78, 0x9A };
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = false,
            Data = payload,
        };
        byte[] bytes = dse.ToBytes();

        // Bytes consumed = ceil((4+1+8 + 8*5) / 8) = ceil(53/8) = 7.
        Assert.Equal(7, bytes.Length);
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(payload, decoded!.Data.ToArray());
    }

    [Fact]
    public void Aligned_DataByteAlignFlag_Pads_Three_Bits_For_Short_Count()
    {
        // When the flag is set with a count < 255, the cursor stands at bit 13
        // after the count field; AlignToByte pads 3 bits to reach bit 16.
        byte[] payload = new byte[] { 0xDE, 0xAD };
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = payload,
        };
        byte[] bytes = dse.ToBytes();
        Assert.Equal(2 + 2, bytes.Length); // 2 header bytes after alignment + 2 data bytes
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(payload, decoded!.Data.ToArray());
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(15)]
    public void ToBytes_Then_TryParse_AllValidTags_Aligned_RoundTrip(int tag)
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = tag,
            DataByteAlignFlag = true,
            Data = new byte[] { 0xA1, 0xB2, 0xC3 },
        };
        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded, out int consumed));
        Assert.Equal(bytes.Length, consumed);
        Assert.Equal(tag, decoded!.ElementInstanceTag);
        Assert.True(decoded.DataByteAlignFlag);
        Assert.Equal(dse.Data.ToArray(), decoded.Data.ToArray());
    }

    [Theory]
    [InlineData(0)]
    [InlineData(15)]
    public void ToBytes_Then_TryParse_BoundaryTags_Unaligned_RoundTrip(int tag)
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = tag,
            DataByteAlignFlag = false,
            Data = new byte[] { 0x11 },
        };
        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(tag, decoded!.ElementInstanceTag);
        Assert.False(decoded.DataByteAlignFlag);
        Assert.Equal(new byte[] { 0x11 }, decoded.Data.ToArray());
    }

    [Fact]
    public void ToBytes_NegativeTag_Throws()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = -1,
            DataByteAlignFlag = true,
            Data = ReadOnlyMemory<byte>.Empty,
        };
        Assert.Throws<InvalidOperationException>(() => dse.ToBytes());
    }

    [Fact]
    public void Header_FirstByte_Layout_TagInHighNibble_FlagThenCountHighBits()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0xA, // 1010
            DataByteAlignFlag = true,
            Data = new byte[] { 0x55, 0xAA, 0xCC }, // count=3 → binary 00000 011
        };
        byte[] bytes = dse.ToBytes();
        // bit layout: [tag:4=1010][flag:1=1][count_high:3=000] => 0b1010_1000 = 0xA8
        Assert.Equal(0xA8, bytes[0]);
        // bit layout continues: [count_low:5=00011][align_pad:3=000] => 0b00011_000 = 0x18
        Assert.Equal(0x18, bytes[1]);
        Assert.Equal(0x55, bytes[2]);
        Assert.Equal(0xAA, bytes[3]);
        Assert.Equal(0xCC, bytes[4]);
    }

    [Fact]
    public void ToBytes_Then_TryParse_EscapeCount_Mid_Range_383_Bytes()
    {
        byte[] payload = new byte[255 + 128];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)i;
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 9,
            DataByteAlignFlag = true,
            Data = payload,
        };
        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        Assert.Equal(payload, decoded!.Data.ToArray());
    }

    [Fact]
    public void Record_With_Expression_PreservesUnchangedFields()
    {
        var original = new AacDataStreamElement
        {
            ElementInstanceTag = 4,
            DataByteAlignFlag = true,
            Data = new byte[] { 1, 2, 3 },
        };
        var mutated = original with { ElementInstanceTag = 7 };
        Assert.Equal(7, mutated.ElementInstanceTag);
        Assert.True(mutated.DataByteAlignFlag);
        Assert.Equal(original.Data, mutated.Data);
        Assert.Equal(4, original.ElementInstanceTag);
    }

    [Fact]
    public void Record_Equality_HoldsForSamePayloadInstance()
    {
        byte[] shared = new byte[] { 0xDE, 0xAD, 0xBE, 0xEF };
        var a = new AacDataStreamElement
        {
            ElementInstanceTag = 2,
            DataByteAlignFlag = true,
            Data = shared,
        };
        var b = new AacDataStreamElement
        {
            ElementInstanceTag = 2,
            DataByteAlignFlag = true,
            Data = shared,
        };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
    }

    [Fact]
    public void Record_Inequality_OnDifferentTag()
    {
        byte[] shared = new byte[] { 1, 2 };
        var a = new AacDataStreamElement
        {
            ElementInstanceTag = 3,
            DataByteAlignFlag = true,
            Data = shared,
        };
        var b = a with { ElementInstanceTag = 4 };
        Assert.NotEqual(a, b);
    }

    [Fact]
    public void ToBytes_DoesNotMutate_DataMemory()
    {
        byte[] payload = new byte[] { 0x10, 0x20, 0x30 };
        byte[] snapshot = (byte[])payload.Clone();
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = payload,
        };
        _ = dse.ToBytes();
        Assert.Equal(snapshot, payload);
    }

    [Fact]
    public void TryParse_ParsedPayload_IsFreshArray_DistinctFromInput()
    {
        var dse = new AacDataStreamElement
        {
            ElementInstanceTag = 0,
            DataByteAlignFlag = true,
            Data = new byte[] { 0x42 },
        };
        byte[] bytes = dse.ToBytes();
        Assert.True(AacDataStreamElement.TryParse(bytes, out var decoded));
        // Mutating the input span after the fact must not affect the decoded
        // payload (TryParse copies into a fresh byte[]).
        bytes[bytes.Length - 1] ^= 0xFF;
        Assert.Equal(0x42, decoded!.Data.Span[0]);
    }

    [Fact]
    public void MaxDataBytes_Constant_Is_510()
    {
        Assert.Equal(510, AacDataStreamElement.MaxDataBytes);
    }

    [Fact]
    public void TryParse_NoBytesConsumed_OnFailure()
    {
        Assert.False(AacDataStreamElement.TryParse(new byte[] { 0xFF }, out var decoded, out int consumed));
        Assert.Null(decoded);
        Assert.Equal(0, consumed);
    }

    [Fact]
    public void TryParse_EscapeBranchTruncated_Returns_False()
    {
        // Construct a 2-byte buffer where count==255 but no esc_count is supplied.
        // bit0..3=tag, bit4=flag, bit5..12=count=255 → 4+1+8=13 bits → 2 bytes.
        byte[] truncated = new byte[2];
        // tag=0, flag=0, count = 0b11111111
        // byte0 = [0000][0][111] = 0b00000111 = 0x07
        // byte1 = [11111][xxx] = need top 5 bits of count low to be 11111
        // count packed: bit5..12 = 11111111; bits 5..7 go in byte0 lower 3 bits,
        // bits 8..12 go in byte1 upper 5 bits.
        truncated[0] = 0x07;
        truncated[1] = 0xF8;
        Assert.False(AacDataStreamElement.TryParse(truncated, out var decoded));
        Assert.Null(decoded);
    }
}
