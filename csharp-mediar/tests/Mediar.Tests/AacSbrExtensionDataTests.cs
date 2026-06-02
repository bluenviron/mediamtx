using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSbrExtensionDataTests
{
    [Fact]
    public void TryParse_WrongExtensionType_Returns_False()
    {
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.DynamicRange, new byte[2], 12, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_NegativeBodyBitLength_Returns_False()
    {
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, new byte[1], -1, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_BodyTooSmallForBitLength_Returns_False()
    {
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, new byte[1], 16, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_SbrDataCrc_RequiresAtLeast10Bits()
    {
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrDataCrc, new byte[2], 9, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_SbrData_Empty_Payload()
    {
        // Body bit length = 0 with SbrData → empty payload, no CRC.
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, ReadOnlySpan<byte>.Empty, 0, out var data));
        Assert.NotNull(data);
        Assert.False(data!.HasCrc);
        Assert.Equal((ushort)0, data.SbrCrc);
        Assert.Equal(0, data.PayloadBitLength);
        Assert.Equal(0, data.Payload.Length);
    }

    [Fact]
    public void TryParse_SbrData_Pure_Payload_Unshifted()
    {
        // No CRC → payload starts at bit 0, copy verbatim.
        byte[] body = [0xAB, 0xCD, 0xEF];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, 24, out var data));
        Assert.False(data!.HasCrc);
        Assert.Equal((ushort)0, data.SbrCrc);
        Assert.Equal(24, data.PayloadBitLength);
        Assert.Equal(body, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_SbrData_Partial_Last_Byte_Padded()
    {
        // bodyBitLength = 12 → ceil(12/8) = 2 bytes; last byte has 4 padding bits.
        byte[] body = [0xAB, 0xCD, 0xEF];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, 12, out var data));
        Assert.Equal(12, data!.PayloadBitLength);
        Assert.Equal(2, data.Payload.Length);
        // Last byte's low nibble should be zero padding because we only
        // copied 4 of its bits.
        Assert.Equal((byte)0xAB, data.Payload.Span[0]);
        Assert.Equal((byte)0xC0, data.Payload.Span[1]);
    }

    [Fact]
    public void TryParse_SbrDataCrc_Extracts_10Bit_Crc_And_Shifts_Payload()
    {
        // Body = 0b10110011 0b11000111 0b01100110 = 24 bits.
        // CRC = first 10 bits = 0b1011001111 = 0x2CF.
        // Payload = remaining 14 bits = 0b00 0111 0110 0110 → packed
        // MSB-first: 0b00011101 0b10011000 = 0x1D, 0x98.
        // Wait: remaining 14 bits start at bit 10 of source. Bits 10..23 are:
        //   bit10=0, bit11=0 (from second byte high), bit12=0, bit13=1,
        //   bit14=1, bit15=1, bit16=0, bit17=1, bit18=1, bit19=0,
        //   bit20=0, bit21=1, bit22=1, bit23=0.
        // Packed left-aligned: 0b00011101 0b10011000 = 0x1D 0x98.
        byte[] body = [0b10110011, 0b11000111, 0b01100110];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrDataCrc, body, 24, out var data));
        Assert.True(data!.HasCrc);
        Assert.Equal((ushort)0x2CF, data.SbrCrc);
        Assert.Equal(14, data.PayloadBitLength);
        Assert.Equal(2, data.Payload.Length);
        Assert.Equal((byte)0x1D, data.Payload.Span[0]);
        Assert.Equal((byte)0x98, data.Payload.Span[1]);
    }

    [Fact]
    public void TryParse_SbrDataCrc_With_Exactly_10_Bits_Yields_Empty_Payload()
    {
        // 10 bits = 0b1010101010 = 0x2AA, packed left-aligned: 0xAA 0x80.
        byte[] body = [0xAA, 0x80];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrDataCrc, body, 10, out var data));
        Assert.True(data!.HasCrc);
        Assert.Equal((ushort)0x2AA, data.SbrCrc);
        Assert.Equal(0, data.PayloadBitLength);
        Assert.Equal(0, data.Payload.Length);
    }

    [Fact]
    public void Dispatcher_Populates_Sbr_For_SbrData_FIL()
    {
        // FIL cnt = 3 → 24 bits payload, type nibble 0xD, body = 20 bits opaque.
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(3u, 4);
        w.Write(0xDAu, 8);
        w.Write(0xBCu, 8);
        w.Write(0xDEu, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.NotNull(fil.FillExtension);
        Assert.Equal(AacFillExtensionType.SbrData, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.Sbr);
        Assert.False(fil.FillExtension.Sbr!.HasCrc);
        Assert.Equal((ushort)0, fil.FillExtension.Sbr.SbrCrc);
        Assert.Equal(20, fil.FillExtension.Sbr.PayloadBitLength);
        // Body (without type nibble) was AB CD E0 (last byte's low nibble is FIL alignment).
        // First 20 bits of that body → AB CD E0 with last 4 bits zeroed:
        Assert.Equal(new byte[] { 0xAB, 0xCD, 0xE0 }, fil.FillExtension.Sbr.Payload.ToArray());
    }

    [Fact]
    public void Dispatcher_Populates_Sbr_For_SbrDataCrc_FIL_With_Crc()
    {
        // FIL cnt = 4 → 32 bits, type nibble 0xE, body = 28 bits opaque.
        // To make CRC introspectable, place body whose first 10 bits encode 0x2AA.
        // Body bits required: 0b1010101010 + 18 arbitrary bits.
        // Build body left-aligned: 0b10101010 0b10??????
        // Pick remaining 18 bits = 0b00 0000 0000 0000 1111 → final body bytes:
        //   byte0 = 0b10101010 = 0xAA
        //   byte1 = 0b10000000 = 0x80
        //   byte2 = 0b00000000 = 0x00
        //   byte3 = 0b00001111 = 0x0F  (using top 4 bits of last byte = 0000)
        // wait, 28 bits packed = 4 bytes with last byte having 4 padding bits.
        // Let me just construct 28 specific bits and let BitWriter handle it.
        var bodyWriter = new AacBitWriter();
        bodyWriter.Write(0x2AAu, 10);   // CRC bits
        bodyWriter.Write(0x3CACBu, 18); // arbitrary remaining 18 bits
        byte[] body = bodyWriter.ToArray(); // 28 bits → 4 bytes

        // Now wrap in FIL: type nibble 0xE prepended.
        var filWriter = new AacBitWriter();
        filWriter.Write(0xEu, 4);
        var bodyReader = new BitReader(body);
        for (int i = 0; i < 28; i++) filWriter.Write(bodyReader.ReadBit() ? 1u : 0u, 1);
        byte[] filBytes = filWriter.ToArray(); // 32 bits → 4 bytes

        var rdb = new AacBitWriter();
        rdb.Write((uint)AacSyntacticElementType.FillElement, 3);
        rdb.Write(4u, 4);
        for (int i = 0; i < filBytes.Length; i++) rdb.Write(filBytes[i], 8);
        rdb.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(rdb.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.SbrDataCrc, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.Sbr);
        Assert.True(fil.FillExtension.Sbr!.HasCrc);
        Assert.Equal((ushort)0x2AA, fil.FillExtension.Sbr.SbrCrc);
        Assert.Equal(18, fil.FillExtension.Sbr.PayloadBitLength);
    }

    [Fact]
    public void Dispatcher_Leaves_Sbr_Null_For_Non_Sbr_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(1u, 4);
        w.Write(0x10u, 8); // FillData (0x1)
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.FillData, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.Sbr);
        Assert.Null(fil.FillExtension.DynamicRange);
    }

    [Fact]
    public void CrcBitWidth_Is_Ten()
    {
        Assert.Equal(10, AacSbrExtensionData.CrcBitWidth);
    }

    [Theory]
    [InlineData(AacFillExtensionType.Fill)]
    [InlineData(AacFillExtensionType.FillData)]
    [InlineData(AacFillExtensionType.DataElement)]
    [InlineData(AacFillExtensionType.DynamicRange)]
    [InlineData(AacFillExtensionType.SacData)]
    public void TryParse_Rejects_All_Non_Sbr_Types(AacFillExtensionType type)
    {
        // Only SbrData and SbrDataCrc are accepted; every other enum value
        // (including reserved gaps not in the enum) must yield false.
        Assert.False(AacSbrExtensionData.TryParse(type, new byte[4], 16, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_Rejects_Reserved_Extension_Type_Numbers()
    {
        // Values like 0x3..0xA aren't named in the enum but are still
        // legal byte values - the parser must reject them all.
        Assert.False(AacSbrExtensionData.TryParse((AacFillExtensionType)0x3, new byte[4], 16, out var data));
        Assert.Null(data);
        Assert.False(AacSbrExtensionData.TryParse((AacFillExtensionType)0xA, new byte[4], 16, out data));
        Assert.Null(data);
        Assert.False(AacSbrExtensionData.TryParse((AacFillExtensionType)0xF, new byte[4], 16, out data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_SbrData_HasCrc_Is_False()
    {
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, new byte[2], 16, out var data));
        Assert.False(data!.HasCrc);
        Assert.Equal(AacFillExtensionType.SbrData, data.ExtensionType);
    }

    [Fact]
    public void TryParse_SbrDataCrc_HasCrc_Is_True()
    {
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrDataCrc, new byte[2], 10, out var data));
        Assert.True(data!.HasCrc);
        Assert.Equal(AacFillExtensionType.SbrDataCrc, data.ExtensionType);
    }

    [Fact]
    public void TryParse_SbrData_Partial_Byte_Single_Bit()
    {
        // bodyBitLength = 1 -> ceil(1/8) = 1 byte, all but MSB are padding.
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, new byte[] { 0b1000_0000 }, 1, out var data));
        Assert.Equal(1, data!.PayloadBitLength);
        Assert.Single(data.Payload.ToArray());
        Assert.Equal((byte)0b1000_0000, data.Payload.Span[0]);
    }

    [Fact]
    public void TryParse_SbrData_Zero_Length_Body_Span()
    {
        // Empty span + bodyBitLength=0 succeeds (valid empty SbrData).
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, ReadOnlySpan<byte>.Empty, 0, out var data));
        Assert.NotNull(data);
        Assert.Equal(0, data!.PayloadBitLength);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(-100)]
    [InlineData(int.MinValue)]
    public void TryParse_Rejects_Any_Negative_BitLength(int badBitLength)
    {
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, new byte[8], badBitLength, out var data));
        Assert.Null(data);
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrDataCrc, new byte[8], badBitLength, out data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_TwoCalls_Produce_Distinct_Payload_Buffers()
    {
        // Each successful TryParse should allocate a fresh payload array so
        // that mutation in one result doesn't bleed into another.
        byte[] body = [0x11, 0x22, 0x33];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, 24, out var a));
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, 24, out var b));
        Assert.NotSame(a, b);
        Assert.False(a!.Payload.Equals(b!.Payload));
    }

    [Fact]
    public void Record_Equality_Compares_All_Fields()
    {
        byte[] payload = [0x10, 0x20];
        var a = new AacSbrExtensionData
        {
            ExtensionType = AacFillExtensionType.SbrData,
            SbrCrc = 0,
            Payload = payload,
            PayloadBitLength = 16,
        };
        var b = new AacSbrExtensionData
        {
            ExtensionType = AacFillExtensionType.SbrData,
            SbrCrc = 0,
            Payload = payload,
            PayloadBitLength = 16,
        };
        var c = a with { PayloadBitLength = 8 };

        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());
        Assert.NotEqual(a, c);
    }

    [Fact]
    public void TryParse_BodyBitLength_Exactly_Matches_Body_Bytes()
    {
        // bodyBitLength = body.Length * 8 should succeed (boundary).
        byte[] body = [0xFF, 0x00, 0xAA, 0x55];
        Assert.True(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, body.Length * 8, out var data));
        Assert.Equal(32, data!.PayloadBitLength);
        Assert.Equal(body, data.Payload.ToArray());
    }

    [Fact]
    public void TryParse_BodyBitLength_One_Above_Available_Returns_False()
    {
        byte[] body = [0x00, 0x00];
        Assert.False(AacSbrExtensionData.TryParse(
            AacFillExtensionType.SbrData, body, body.Length * 8 + 1, out var data));
        Assert.Null(data);
    }
}
