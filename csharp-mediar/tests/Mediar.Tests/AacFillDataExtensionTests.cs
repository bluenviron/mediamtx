using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacFillDataExtensionTests
{
    [Fact]
    public void TryParse_ZeroBits_Returns_False()
    {
        Assert.False(AacFillDataExtension.TryParse(ReadOnlySpan<byte>.Empty, 0, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_BodyTooSmall_Returns_False()
    {
        Assert.False(AacFillDataExtension.TryParse(new byte[1], 16, out var data));
        Assert.Null(data);
    }

    [Theory]
    [InlineData(5)]   // 4 + 1
    [InlineData(7)]   // 4 + 3
    [InlineData(11)] // 4 + 7
    public void TryParse_NonByteAlignedRemainder_Returns_False(int bodyBitLength)
    {
        Assert.False(AacFillDataExtension.TryParse(new byte[2], bodyBitLength, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_FourBitsOnly_Zero_Nibble_Empty_Bytes_Is_Conformant()
    {
        byte[] body = [0x00];
        Assert.True(AacFillDataExtension.TryParse(body, 4, out var data));
        Assert.Equal((byte)0x0, data!.FillNibble);
        Assert.Equal(0, data.FillBytes.Length);
        Assert.True(data.IsConformant);
    }

    [Fact]
    public void TryParse_FourBitsOnly_Nonzero_Nibble_Not_Conformant()
    {
        byte[] body = [0x50]; // top nibble 0x5
        Assert.True(AacFillDataExtension.TryParse(body, 4, out var data));
        Assert.Equal((byte)0x5, data!.FillNibble);
        Assert.Equal(0, data.FillBytes.Length);
        Assert.False(data.IsConformant);
    }

    [Fact]
    public void TryParse_Nibble_Plus_Conformant_Fill_Bytes_Roundtrips()
    {
        // 4-bit fill_nibble = 0x0 then 3 fill_byte = 0xA5.
        var w = new AacBitWriter();
        w.Write(0x0u, 4);
        w.Write(0xA5u, 8);
        w.Write(0xA5u, 8);
        w.Write(0xA5u, 8);
        byte[] body = w.ToArray();

        Assert.True(AacFillDataExtension.TryParse(body, 28, out var data));
        Assert.Equal((byte)0x0, data!.FillNibble);
        Assert.Equal(3, data.FillBytes.Length);
        Assert.Equal(new byte[] { 0xA5, 0xA5, 0xA5 }, data.FillBytes.ToArray());
        Assert.True(data.IsConformant);
    }

    [Fact]
    public void TryParse_Nonconformant_Fill_Byte_Detected()
    {
        var w = new AacBitWriter();
        w.Write(0x0u, 4);
        w.Write(0xA5u, 8);
        w.Write(0x55u, 8); // not 0xA5
        w.Write(0xA5u, 8);
        byte[] body = w.ToArray();

        Assert.True(AacFillDataExtension.TryParse(body, 28, out var data));
        Assert.Equal(3, data!.FillBytes.Length);
        Assert.False(data.IsConformant);
    }

    [Fact]
    public void Dispatcher_Populates_FillData_For_Single_Byte_Fill_Element()
    {
        // FIL cnt = 1 → 8 bits = type(4) + fill_nibble(4). type=0x1, nibble=0x0.
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(1u, 4); // cnt
        w.Write(0x10u, 8); // type=0x1, nibble=0x0
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.FillData, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.FillData);
        Assert.Equal((byte)0x0, fil.FillExtension.FillData!.FillNibble);
        Assert.Equal(0, fil.FillExtension.FillData.FillBytes.Length);
        Assert.True(fil.FillExtension.FillData.IsConformant);
    }

    [Fact]
    public void Dispatcher_Populates_FillData_For_Multi_Byte_Conformant_Fill()
    {
        // FIL cnt = 4 → 32 bits = type(4) + nibble(4) + 3 fill_bytes(0xA5).
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(4u, 4);
        w.Write(0x10u, 8); // type=0x1, nibble=0x0
        w.Write(0xA5u, 8);
        w.Write(0xA5u, 8);
        w.Write(0xA5u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.FillData, fil.FillExtension!.ExtensionType);
        Assert.NotNull(fil.FillExtension.FillData);
        Assert.True(fil.FillExtension.FillData!.IsConformant);
        Assert.Equal(3, fil.FillExtension.FillData.FillBytes.Length);
    }

    [Fact]
    public void Dispatcher_Leaves_FillData_Null_For_Non_FillData_Type()
    {
        var w = new AacBitWriter();
        w.Write((uint)AacSyntacticElementType.FillElement, 3);
        w.Write(2u, 4);
        w.Write(0xB0u, 8); // type=0xB (DynamicRange)
        w.Write(0x00u, 8);
        w.Write((uint)AacSyntacticElementType.End, 3);

        Assert.True(AacRawDataBlock.TryParse(w.ToArray(), out var block));
        var fil = block!.Entries[0];
        Assert.Equal(AacFillExtensionType.DynamicRange, fil.FillExtension!.ExtensionType);
        Assert.Null(fil.FillExtension.FillData);
    }

    [Fact]
    public void Expected_FillNibble_Constant_Is_Zero()
    {
        Assert.Equal((byte)0x0, AacFillDataExtension.ExpectedFillNibble);
    }

    [Fact]
    public void Expected_FillByte_Constant_Is_A5()
    {
        Assert.Equal((byte)0xA5, AacFillDataExtension.ExpectedFillByte);
    }

    [Fact]
    public void TryParse_NegativeBitLength_Returns_False()
    {
        Assert.False(AacFillDataExtension.TryParse(new byte[1], -1, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_BodyShorterThanRequestedBits_Returns_False()
    {
        // 12 bits requested but body is only 1 byte (8 bits)
        Assert.False(AacFillDataExtension.TryParse(new byte[1], 12, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_OneByteOfFill_RoundTrips()
    {
        var w = new AacBitWriter();
        w.Write(0x0u, 4);
        w.Write(0xA5u, 8);
        byte[] body = w.ToArray();
        Assert.True(AacFillDataExtension.TryParse(body, 12, out var data));
        Assert.Equal((byte)0x0, data!.FillNibble);
        Assert.Equal(1, data.FillBytes.Length);
        Assert.Equal(0xA5, data.FillBytes.Span[0]);
        Assert.True(data.IsConformant);
    }

    [Fact]
    public void TryParse_MismatchedNibble_NotConformant_Even_With_Conformant_Bytes()
    {
        var w = new AacBitWriter();
        w.Write(0x3u, 4); // bad nibble
        w.Write(0xA5u, 8);
        w.Write(0xA5u, 8);
        byte[] body = w.ToArray();
        Assert.True(AacFillDataExtension.TryParse(body, 20, out var data));
        Assert.Equal((byte)0x3, data!.FillNibble);
        Assert.False(data.IsConformant);
    }

    [Fact]
    public void TryParse_AllFillBytesConformant_Boundary_Case()
    {
        var w = new AacBitWriter();
        w.Write(0x0u, 4);
        // 10 fill bytes
        for (int i = 0; i < 10; i++) w.Write(0xA5u, 8);
        byte[] body = w.ToArray();
        Assert.True(AacFillDataExtension.TryParse(body, 84, out var data));
        Assert.Equal(10, data!.FillBytes.Length);
        Assert.True(data.IsConformant);
    }

    [Fact]
    public void FillBytes_Memory_Is_Independent_Per_Instance()
    {
        var w = new AacBitWriter();
        w.Write(0x0u, 4);
        w.Write(0xA5u, 8);
        AacFillDataExtension.TryParse(w.ToArray(), 12, out var d1);

        var w2 = new AacBitWriter();
        w2.Write(0x0u, 4);
        w2.Write(0xA5u, 8);
        AacFillDataExtension.TryParse(w2.ToArray(), 12, out var d2);

        Assert.NotSame(d1, d2);
        Assert.True(d1!.FillBytes.Span.SequenceEqual(d2!.FillBytes.Span));
    }
}
