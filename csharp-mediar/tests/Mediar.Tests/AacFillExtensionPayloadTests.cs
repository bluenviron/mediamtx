using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacFillExtensionPayloadTests
{
    [Fact]
    public void Enum_Has_Spec_Defined_Values()
    {
        // ISO/IEC 14496-3 Table 4.51.
        Assert.Equal(0x0, (byte)AacFillExtensionType.Fill);
        Assert.Equal(0x1, (byte)AacFillExtensionType.FillData);
        Assert.Equal(0x2, (byte)AacFillExtensionType.DataElement);
        Assert.Equal(0xB, (byte)AacFillExtensionType.DynamicRange);
        Assert.Equal(0xC, (byte)AacFillExtensionType.SacData);
        Assert.Equal(0xD, (byte)AacFillExtensionType.SbrData);
        Assert.Equal(0xE, (byte)AacFillExtensionType.SbrDataCrc);
    }

    [Theory]
    [InlineData((byte)0x0, true)]
    [InlineData((byte)0x1, true)]
    [InlineData((byte)0x2, true)]
    [InlineData((byte)0x3, false)]
    [InlineData((byte)0xA, false)]
    [InlineData((byte)0xB, true)]
    [InlineData((byte)0xC, true)]
    [InlineData((byte)0xD, true)]
    [InlineData((byte)0xE, true)]
    [InlineData((byte)0xF, false)]
    public void IsKnown_Matches_Table_4_51(byte rawType, bool expected)
    {
        Assert.Equal(expected, AacFillExtensionPayload.IsKnown(rawType));
    }

    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AacFillExtensionPayload.TryParse(ReadOnlySpan<byte>.Empty, out var payload));
        Assert.Null(payload);
    }

    [Fact]
    public void TryParse_Single_Byte_Returns_Type_And_4_Body_Bits()
    {
        // 0xD3 = 0b1101_0011 → type = 0xD (SBR), body bits = 0b0011 padded to 0b0011_0000 = 0x30.
        byte[] data = [0xD3];
        Assert.True(AacFillExtensionPayload.TryParse(data, out var payload));
        Assert.NotNull(payload);
        Assert.Equal((byte)0xD, payload!.RawType);
        Assert.Equal(AacFillExtensionType.SbrData, payload.ExtensionType);
        Assert.True(payload.IsKnownExtensionType);
        Assert.Equal(4, payload.BodyBitLength);
        Assert.Equal(new byte[] { 0x30 }, payload.Body.ToArray());
    }

    [Fact]
    public void TryParse_Two_Bytes_Returns_Type_And_12_Body_Bits()
    {
        // 0xB1 0x23 → type = 0xB (DynamicRange), body bits = 0b0001 0010 0011 = 0x123 in 12 bits.
        // Packed MSB-first into 2 bytes: 0b0001_0010, 0b0011_0000 = 0x12, 0x30.
        byte[] data = [0xB1, 0x23];
        Assert.True(AacFillExtensionPayload.TryParse(data, out var payload));
        Assert.NotNull(payload);
        Assert.Equal((byte)0xB, payload!.RawType);
        Assert.Equal(AacFillExtensionType.DynamicRange, payload.ExtensionType);
        Assert.True(payload.IsKnownExtensionType);
        Assert.Equal(12, payload.BodyBitLength);
        Assert.Equal(new byte[] { 0x12, 0x30 }, payload.Body.ToArray());
    }

    [Fact]
    public void TryParse_Reserved_Type_Surfaces_Raw()
    {
        // 0x40 → type = 0x4 (reserved), body = 0.
        byte[] data = [0x40];
        Assert.True(AacFillExtensionPayload.TryParse(data, out var payload));
        Assert.Equal((byte)0x4, payload!.RawType);
        Assert.False(payload.IsKnownExtensionType);
        Assert.Equal((AacFillExtensionType)0x4, payload.ExtensionType);
        Assert.Equal(4, payload.BodyBitLength);
        Assert.Equal(new byte[] { 0x00 }, payload.Body.ToArray());
    }

    [Fact]
    public void TryParse_Five_Bytes_Body_Bit_Length_Is_36()
    {
        // 5 bytes = 40 bits. Body bit length = 36. ceil(36/8) = 5 body bytes.
        byte[] data = [0x1A, 0xBC, 0xDE, 0xF0, 0x12];
        Assert.True(AacFillExtensionPayload.TryParse(data, out var payload));
        Assert.Equal((byte)0x1, payload!.RawType);
        Assert.Equal(AacFillExtensionType.FillData, payload.ExtensionType);
        Assert.Equal(36, payload.BodyBitLength);
        Assert.Equal(5, payload.Body.Length);
        // Shift left 4 bits over the source bytes (after the leading nibble).
        // bytes after nibble removal (MSB-first stream): A BC DE F0 12 → 0xABCDEF012
        // packed into 5 bytes left-aligned: AB CD EF 01 20.
        Assert.Equal(new byte[] { 0xAB, 0xCD, 0xEF, 0x01, 0x20 }, payload.Body.ToArray());
    }

    [Fact]
    public void Dispatcher_Populates_FillExtension_For_Short_Count()
    {
        // FIL with count = 3, payload = 0xDA 0xBC 0xDE → extension_type = 0xD (SBR),
        // body bits = 20 (3*8 - 4), body packed = AB CD E0.
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(3u, 4); // count
        writer.Write(0xDAu, 8);
        writer.Write(0xBCu, 8);
        writer.Write(0xDEu, 8);
        writer.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(writer.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacSyntacticElementType.FillElement, fil.Type);
        Assert.NotNull(fil.FillExtension);
        Assert.Equal((byte)0xD, fil.FillExtension!.RawType);
        Assert.Equal(AacFillExtensionType.SbrData, fil.FillExtension.ExtensionType);
        Assert.Equal(20, fil.FillExtension.BodyBitLength);
        Assert.Equal(new byte[] { 0xAB, 0xCD, 0xE0 }, fil.FillExtension.Body.ToArray());
    }

    [Fact]
    public void Dispatcher_Populates_FillExtension_For_Escape_Count()
    {
        // count = 15, esc_count = 0 → cnt = 14. Use extension_type 0xB.
        byte[] payload = new byte[14];
        payload[0] = 0xB0;
        for (int i = 1; i < payload.Length; i++) payload[i] = (byte)(0x80 ^ i);

        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(15u, 4);
        writer.Write(0u, 8);
        for (int i = 0; i < payload.Length; i++) writer.Write(payload[i], 8);
        writer.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(writer.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(14, fil.FillExtensionBytes.Length);
        Assert.NotNull(fil.FillExtension);
        Assert.Equal((byte)0xB, fil.FillExtension!.RawType);
        Assert.Equal(AacFillExtensionType.DynamicRange, fil.FillExtension.ExtensionType);
        Assert.Equal(14 * 8 - 4, fil.FillExtension.BodyBitLength);
    }

    [Fact]
    public void Dispatcher_Leaves_FillExtension_Null_For_Zero_Count()
    {
        var writer = new AacBitWriter();
        writer.Write((uint)AacSyntacticElementType.FillElement, 3);
        writer.Write(0u, 4); // count = 0 → no extension_type field
        writer.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(writer.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacSyntacticElementType.FillElement, fil.Type);
        Assert.Equal(0, fil.FillExtensionBytes.Length);
        Assert.Null(fil.FillExtension);
    }
}
