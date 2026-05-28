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
}
