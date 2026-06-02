using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSacExtensionDataTests
{
    [Fact]
    public void TryParse_NegativeBitLength_Returns_False()
    {
        Assert.False(AacSacExtensionData.TryParse(ReadOnlySpan<byte>.Empty, -1, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_BufferTooSmall_Returns_False()
    {
        Assert.False(AacSacExtensionData.TryParse(new byte[1], 16, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_ZeroBits_Returns_Empty_Payload()
    {
        Assert.True(AacSacExtensionData.TryParse(ReadOnlySpan<byte>.Empty, 0, out var data));
        Assert.NotNull(data);
        Assert.Equal(0, data!.PayloadBitLength);
        Assert.Equal(0, data.Payload.Length);
    }

    [Fact]
    public void TryParse_FullByte_Roundtrip()
    {
        byte[] body = [0xAB, 0xCD];
        Assert.True(AacSacExtensionData.TryParse(body, 16, out var data));
        Assert.Equal(16, data!.PayloadBitLength);
        Assert.Equal(body, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_PartialLastByte_LeftAligned()
    {
        // 12 bits of payload = 0xAB, 0xC0 (top nibble of byte 1).
        byte[] body = [0xAB, 0xCD];
        Assert.True(AacSacExtensionData.TryParse(body, 12, out var data));
        Assert.Equal(12, data!.PayloadBitLength);
        Assert.Equal(new byte[] { 0xAB, 0xC0 }, data.Payload.ToArray());
    }

    [Fact]
    public void Dispatcher_Populates_Sac_For_Sac_FIL()
    {
        // FIL cnt = 2: 16 bits = type(4) + body(12).
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0xCu, 4); // SAC type
        w.Write(0xABCu, 12); // 12-bit SAC body
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.SacData, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.Sac);
        Assert.Equal(12, fil.FillExtension.Sac!.PayloadBitLength);
        // 12 bits 0xABC left-aligned = byte[0]=0xAB, byte[1]=0xC0.
        Assert.Equal(new byte[] { 0xAB, 0xC0 }, fil.FillExtension.Sac.Payload.ToArray());
    }

    [Fact]
    public void Dispatcher_Leaves_Sac_Null_For_Non_Sac_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0xD0u, 8); // SBR (0xD)
        w.Write(0x00u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.SbrData, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.Sac);
    }

    [Fact]
    public void TryParse_OneBit_Set_Left_Aligned()
    {
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0x80 }, 1, out var data));
        Assert.Equal(1, data!.PayloadBitLength);
        Assert.Equal(new byte[] { 0x80 }, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_OneBit_Clear()
    {
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0x7F }, 1, out var data));
        Assert.Equal(new byte[] { 0x00 }, data!.Payload.ToArray());
    }

    [Fact]
    public void TryParse_Buffer_Exactly_Sized_Succeeds()
    {
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0xFF }, 8, out var data));
        Assert.Equal(8, data!.PayloadBitLength);
        Assert.Equal(new byte[] { 0xFF }, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_Empty_Buffer_With_NonZero_BitLength_Returns_False()
    {
        Assert.False(AacSacExtensionData.TryParse(ReadOnlySpan<byte>.Empty, 1, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_Three_Byte_Payload_Roundtrip()
    {
        byte[] body = [0x12, 0x34, 0x56];
        Assert.True(AacSacExtensionData.TryParse(body, 24, out var data));
        Assert.Equal(24, data!.PayloadBitLength);
        Assert.Equal(body, data.Payload.ToArray());
    }

    [Theory]
    [InlineData(20, new byte[] { 0xAB, 0xCD, 0xE0 })] // top 4 bits of byte 2 = 0xE
    [InlineData(17, new byte[] { 0xAB, 0xCD, 0x80 })] // 17th bit is MSB of byte 2 (1) => 0x80
    [InlineData(15, new byte[] { 0xAB, 0xCC })]       // drop LSB of byte 1
    public void TryParse_Partial_LastByte_LeftAligned(int bits, byte[] expected)
    {
        byte[] body = [0xAB, 0xCD, 0xEF];
        Assert.True(AacSacExtensionData.TryParse(body, bits, out var data));
        Assert.Equal(bits, data!.PayloadBitLength);
        Assert.Equal(expected, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_Bit_Length_Greater_Than_Buffer_Returns_False()
    {
        Assert.False(AacSacExtensionData.TryParse(new byte[] { 0x01, 0x02 }, 17, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_Boundary_Byte_Count_Rounds_Up()
    {
        // 9 bits => 2 bytes of output. Bit 9 is MSB of byte 2 of input (0xFF -> 0x80).
        byte[] body = [0xAA, 0xFF];
        Assert.True(AacSacExtensionData.TryParse(body, 9, out var data));
        Assert.Equal(2, data!.Payload.Length);
        Assert.Equal(0xAA, data.Payload.Span[0]);
        Assert.Equal(0x80, data.Payload.Span[1]);
    }

    [Fact]
    public void Record_Equality_Compares_By_Reference_For_Memory()
    {
        // ReadOnlyMemory<byte> uses reference equality; same byte[] backing => equal.
        byte[] backing = [0x55];
        var a = new AacSacExtensionData { Payload = backing, PayloadBitLength = 8 };
        var b = new AacSacExtensionData { Payload = backing, PayloadBitLength = 8 };
        Assert.Equal(a, b);
    }

    [Fact]
    public void Record_Inequality_When_Bit_Length_Differs()
    {
        byte[] backing = [0x55];
        var a = new AacSacExtensionData { Payload = backing, PayloadBitLength = 8 };
        var b = new AacSacExtensionData { Payload = backing, PayloadBitLength = 7 };
        Assert.NotEqual(a, b);
    }

    [Fact]
    public void Dispatcher_Populates_Sac_With_Empty_Body()
    {
        // cnt=1: 8 bits total = type(4) + body(4). 4-bit SAC body of zero.
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(1u, 4);
        w.Write(0xCu, 4);     // SAC type nibble
        w.Write(0x0u, 4);     // 4-bit body
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.SacData, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.Sac);
        Assert.Equal(4, fil.FillExtension.Sac!.PayloadBitLength);
        Assert.Equal(new byte[] { 0x00 }, fil.FillExtension.Sac.Payload.ToArray());
    }

    [Fact]
    public void Dispatcher_Sac_Across_ExtCount_Boundary()
    {
        // cnt=15 then esc_count=10 => total 24 bytes minus 1 byte cnt fields:
        // First-byte cnt=15 means esc_count present. Cleanest: use a small FIL
        // with cnt=4 => 32 bits = type(4) + body(28). SAC body = 0x1234567 (28 bits).
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(4u, 4);
        w.Write(0xCu, 4);
        w.Write(0x1234567u, 28);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        var sac = fil.FillExtension!.Sac!;
        Assert.Equal(28, sac.PayloadBitLength);
        // 28-bit 0x1234567 left-aligned in 4 bytes => 0x12,0x34,0x56,0x70
        Assert.Equal(new byte[] { 0x12, 0x34, 0x56, 0x70 }, sac.Payload.ToArray());
    }

    [Fact]
    public void Dispatcher_Leaves_Sac_Null_For_DRC_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0xB0u, 8);  // DRC (0xB)
        w.Write(0x00u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.DynamicRange, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.Sac);
    }

    [Fact]
    public void Dispatcher_Leaves_Sac_Null_For_FillData_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0x10u, 8);  // FillData (0x1)
        w.Write(0x00u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.FillData, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.Sac);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(16)]
    [InlineData(17)]
    [InlineData(24)]
    public void Payload_ByteCount_RoundsUp_From_BitLength(int bits)
    {
        // Buffer always large enough.
        byte[] buffer = new byte[8];
        Assert.True(AacSacExtensionData.TryParse(buffer, bits, out var data));
        int expectedBytes = (bits + 7) >> 3;
        Assert.Equal(expectedBytes, data!.Payload.Length);
        Assert.Equal(bits, data.PayloadBitLength);
    }

    [Fact]
    public void TryParse_Pattern_Of_All_0xFF_Pads_With_Zero()
    {
        // 10 bits of 0xFF span two bytes; final 6 bits of byte 1 must be 0.
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0xFF, 0xFF }, 10, out var data));
        Assert.Equal(0xFF, data!.Payload.Span[0]);
        Assert.Equal(0xC0, data.Payload.Span[1]);
    }

    [Fact]
    public void TryParse_Honors_Source_Span_Length_Exactly()
    {
        // Bit length matches buffer length * 8 - 1 (succeeds);
        // bit length exceeds buffer length * 8 (fails).
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0xFF, 0xFF }, 15, out var ok));
        Assert.NotNull(ok);
        Assert.False(AacSacExtensionData.TryParse(new byte[] { 0xFF, 0xFF }, 17, out var bad));
        Assert.Null(bad);
    }

    [Fact]
    public void Record_With_Expression_Preserves_Payload()
    {
        byte[] backing = [0x42];
        var original = new AacSacExtensionData { Payload = backing, PayloadBitLength = 8 };
        var mutated = original with { PayloadBitLength = 6 };
        Assert.Equal(6, mutated.PayloadBitLength);
        Assert.Equal(backing, mutated.Payload.ToArray());
        Assert.NotEqual(original, mutated);
    }

    [Fact]
    public void TryParse_Single_Byte_All_Ones()
    {
        Assert.True(AacSacExtensionData.TryParse(new byte[] { 0xFF }, 8, out var data));
        Assert.Equal(new byte[] { 0xFF }, data!.Payload.ToArray());
        Assert.Equal(8, data.PayloadBitLength);
    }

    [Fact]
    public void TryParse_Oversized_Buffer_Is_Allowed()
    {
        // Buffer is much bigger than declared bit length: the parser only
        // reads the leading bits and leaves the tail untouched.
        byte[] buffer = new byte[16];
        Array.Fill(buffer, (byte)0xAA);
        Assert.True(AacSacExtensionData.TryParse(buffer, 4, out var data));
        Assert.Single(data!.Payload.ToArray());
        Assert.Equal(0xA0, data.Payload.Span[0]);
    }

    [Fact]
    public void TryParse_NegativeBitLength_DoesNotConsumeBuffer()
    {
        Assert.False(AacSacExtensionData.TryParse(new byte[] { 0xFF, 0xFF }, -100, out var data));
        Assert.Null(data);
    }
}
