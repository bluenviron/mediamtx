using System.Buffers.Binary;
using Mediar.Codecs.Astc;
using Xunit;

namespace Mediar.Tests.Astc;

/// <summary>
/// Tests for <see cref="AstcDecoder"/>'s void-extent (constant-colour)
/// block decode per Khronos KDF 1.4 section 19. Both the LDR (UNORM16)
/// and HDR (FP16) variants are exercised, as are the rejection paths
/// for non-void-extent blocks and malformed extent rectangles.
/// </summary>
public sealed class AstcDecoderTests
{
    [Theory]
    [InlineData(AstcFormat.Astc4x4Unorm, 4, 4)]
    [InlineData(AstcFormat.Astc5x4Unorm, 5, 4)]
    [InlineData(AstcFormat.Astc5x5Unorm, 5, 5)]
    [InlineData(AstcFormat.Astc6x5Unorm, 6, 5)]
    [InlineData(AstcFormat.Astc6x6Unorm, 6, 6)]
    [InlineData(AstcFormat.Astc8x5Unorm, 8, 5)]
    [InlineData(AstcFormat.Astc8x6Unorm, 8, 6)]
    [InlineData(AstcFormat.Astc8x8Unorm, 8, 8)]
    [InlineData(AstcFormat.Astc10x5Unorm, 10, 5)]
    [InlineData(AstcFormat.Astc10x6Unorm, 10, 6)]
    [InlineData(AstcFormat.Astc10x8Unorm, 10, 8)]
    [InlineData(AstcFormat.Astc10x10Unorm, 10, 10)]
    [InlineData(AstcFormat.Astc12x10Unorm, 12, 10)]
    [InlineData(AstcFormat.Astc12x12Unorm, 12, 12)]
    public void Block_Dimensions_Are_Correct(AstcFormat format, int x, int y)
    {
        Assert.Equal((x, y), AstcDecoder.BlockDimensions(format));
        Assert.Equal(16, AstcDecoder.BytesPerBlock(format));
    }

    [Fact]
    public void IsVoidExtent_Detects_Constant_Block()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0x1234, 0x5678, 0x9ABC, 0xDEF0);
        Assert.True(AstcDecoder.IsVoidExtent(block));
        Assert.False(AstcDecoder.IsVoidExtentHdr(block));
    }

    [Fact]
    public void IsVoidExtent_Rejects_Non_VoidExtent_Block()
    {
        var block = new byte[16];
        block[0] = 0x42;
        block[1] = 0x00;
        Assert.False(AstcDecoder.IsVoidExtent(block));
    }

    [Fact]
    public void IsVoidExtentHdr_Flags_F16_Block()
    {
        var block = TestAstcBuilder.HdrVoidExtent(0x3C00, 0x0000, 0x0000, 0x3C00);
        Assert.True(AstcDecoder.IsVoidExtent(block));
        Assert.True(AstcDecoder.IsVoidExtentHdr(block));
    }

    [Fact]
    public void DecodeBlock_Ldr_Fills_Every_Texel_With_TopByte()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0xAB00, 0xCD00, 0xEF00, 0xFF00);
        var rgba = new byte[4 * 4 * 4];
        Assert.True(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc4x4Unorm, rgba));
        for (int i = 0; i < 16; i++)
        {
            Assert.Equal(0xAB, rgba[i * 4 + 0]);
            Assert.Equal(0xCD, rgba[i * 4 + 1]);
            Assert.Equal(0xEF, rgba[i * 4 + 2]);
            Assert.Equal(0xFF, rgba[i * 4 + 3]);
        }
    }

    [Fact]
    public void DecodeBlock_Ldr_Variable_Footprint()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0x1000, 0x2000, 0x3000, 0xFFFF);
        var rgba = new byte[8 * 6 * 4];
        Assert.True(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc8x6Unorm, rgba));
        for (int p = 0; p < 8 * 6; p++)
        {
            Assert.Equal(0x10, rgba[p * 4 + 0]);
            Assert.Equal(0x20, rgba[p * 4 + 1]);
            Assert.Equal(0x30, rgba[p * 4 + 2]);
            Assert.Equal(0xFF, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void DecodeBlock_Hdr_Clamps_FP16_To_Byte()
    {
        var block = TestAstcBuilder.HdrVoidExtent(0x3C00, 0x3800, 0x0000, 0x4000);
        var rgba = new byte[4 * 4 * 4];
        Assert.True(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc4x4Unorm, rgba));
        Assert.Equal(255, rgba[0]);
        Assert.Equal(128, rgba[1]);
        Assert.Equal(0, rgba[2]);
        Assert.Equal(255, rgba[3]);
    }

    [Fact]
    public void DecodeBlock_Returns_False_For_Non_VoidExtent_Block()
    {
        var block = new byte[16];
        block[0] = 0x42;
        block[1] = 0x00;
        var rgba = new byte[4 * 4 * 4];
        Assert.False(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc4x4Unorm, rgba));
    }

    [Fact]
    public void DecodeBlock_Returns_False_For_Malformed_Extent_Rectangle()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0x1111, 0x2222, 0x3333, 0x4444);
        TestAstcBuilder.OverrideExtent(block, vxLowS: 100, vxHighS: 50, vxLowT: 100, vxHighT: 200);
        var rgba = new byte[4 * 4 * 4];
        Assert.False(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc4x4Unorm, rgba));
    }

    [Fact]
    public void DecodeBlock_Returns_False_When_Reserved_Bits_Not_Set()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0x1111, 0x2222, 0x3333, 0x4444);
        block[1] &= unchecked((byte)~(1 << 2));
        var rgba = new byte[4 * 4 * 4];
        Assert.False(AstcDecoder.TryDecodeBlock(block, AstcFormat.Astc4x4Unorm, rgba));
    }

    [Fact]
    public void TryDecodeVoidExtentHdr_Returns_FP16_Bits_Directly()
    {
        var block = TestAstcBuilder.HdrVoidExtent(0x3C00, 0x3800, 0x0000, 0x3C00);
        Span<ushort> rgba = stackalloc ushort[4];
        Assert.True(AstcDecoder.TryDecodeVoidExtentHdr(block, rgba));
        Assert.Equal(0x3C00, rgba[0]);
        Assert.Equal(0x3800, rgba[1]);
        Assert.Equal(0x0000, rgba[2]);
        Assert.Equal(0x3C00, rgba[3]);
    }

    [Fact]
    public void TryDecodeVoidExtentHdr_Converts_LDR_Channel_To_HalfFloat()
    {
        var block = TestAstcBuilder.LdrVoidExtent(0xFFFF, 0x0000, 0xFFFF, 0xFFFF);
        Span<ushort> rgba = stackalloc ushort[4];
        Assert.True(AstcDecoder.TryDecodeVoidExtentHdr(block, rgba));
        Assert.Equal(0x3C00, rgba[0]);
        Assert.Equal(0x0000, rgba[1]);
        Assert.Equal(0x3C00, rgba[2]);
        Assert.Equal(0x3C00, rgba[3]);
    }

    [Fact]
    public void DecodeImage_Fills_All_Block_Aligned_Output()
    {
        var b1 = TestAstcBuilder.LdrVoidExtent(0xFF00, 0x0000, 0x0000, 0xFF00);
        var b2 = TestAstcBuilder.LdrVoidExtent(0x0000, 0xFF00, 0x0000, 0xFF00);
        var b3 = TestAstcBuilder.LdrVoidExtent(0x0000, 0x0000, 0xFF00, 0xFF00);
        var b4 = TestAstcBuilder.LdrVoidExtent(0xFF00, 0xFF00, 0xFF00, 0xFF00);
        var payload = new byte[64];
        b1.CopyTo(payload.AsSpan(0));
        b2.CopyTo(payload.AsSpan(16));
        b3.CopyTo(payload.AsSpan(32));
        b4.CopyTo(payload.AsSpan(48));
        var rgba = new byte[8 * 8 * 4];

        int decoded = AstcDecoder.DecodeImage(payload, AstcFormat.Astc4x4Unorm, 8, 8, rgba, out int skipped);
        Assert.Equal(4, decoded);
        Assert.Equal(0, skipped);
        for (int y = 0; y < 4; y++)
        {
            for (int x = 0; x < 4; x++)
            {
                int o = (y * 8 + x) * 4;
                Assert.Equal(0xFF, rgba[o + 0]);
                Assert.Equal(0x00, rgba[o + 1]);
                Assert.Equal(0x00, rgba[o + 2]);
            }
        }
        for (int y = 4; y < 8; y++)
        {
            for (int x = 4; x < 8; x++)
            {
                int o = (y * 8 + x) * 4;
                Assert.Equal(0xFF, rgba[o + 0]);
                Assert.Equal(0xFF, rgba[o + 1]);
                Assert.Equal(0xFF, rgba[o + 2]);
            }
        }
    }

    [Fact]
    public void DecodeImage_Skips_Non_VoidExtent_Blocks_Leaving_Black()
    {
        var b1 = TestAstcBuilder.LdrVoidExtent(0xFF00, 0xFF00, 0xFF00, 0xFF00);
        var b2 = new byte[16];
        var payload = new byte[32];
        b1.CopyTo(payload.AsSpan(0));
        b2.CopyTo(payload.AsSpan(16));
        var rgba = new byte[8 * 4 * 4];

        int decoded = AstcDecoder.DecodeImage(payload, AstcFormat.Astc4x4Unorm, 8, 4, rgba, out int skipped);
        Assert.Equal(1, decoded);
        Assert.Equal(1, skipped);
        Assert.Equal(0xFF, rgba[0]);
        Assert.Equal(0xFF, rgba[1]);
        Assert.Equal(0xFF, rgba[2]);
        Assert.Equal(0xFF, rgba[3]);
        Assert.Equal(0x00, rgba[(0 * 8 + 4) * 4 + 0]);
        Assert.Equal(0x00, rgba[(0 * 8 + 4) * 4 + 1]);
        Assert.Equal(0x00, rgba[(0 * 8 + 4) * 4 + 2]);
        Assert.Equal(0x00, rgba[(0 * 8 + 4) * 4 + 3]);
    }

    [Fact]
    public void DecodeImage_Handles_Non_Block_Aligned_Image_Sizes()
    {
        var b1 = TestAstcBuilder.LdrVoidExtent(0xAA00, 0xBB00, 0xCC00, 0xFF00);
        var b2 = TestAstcBuilder.LdrVoidExtent(0x1100, 0x2200, 0x3300, 0x4400);
        var payload = new byte[32];
        b1.CopyTo(payload.AsSpan(0));
        b2.CopyTo(payload.AsSpan(16));
        var rgba = new byte[5 * 3 * 4];

        int decoded = AstcDecoder.DecodeImage(payload, AstcFormat.Astc4x4Unorm, 5, 3, rgba, out int skipped);
        Assert.Equal(2, decoded);
        Assert.Equal(0, skipped);
        Assert.Equal(0xAA, rgba[0]);
        Assert.Equal(0x11, rgba[(0 * 5 + 4) * 4 + 0]);
    }
}

/// <summary>
/// Test-only synthesiser for ASTC void-extent blocks per Khronos KDF 1.4 section 19.
/// Block layout (128 bits, LSB-first within bytes):
///   - Bits [8:0]   = 0x1FC (void-extent magic)
///   - Bit 9        = 0 (LDR / U16) | 1 (HDR / F16)
///   - Bits [11:10] = 0b11 (reserved, must be set for 2D void-extent)
///   - Bits [24:12] = vx_low_s  (13 bits, 0x1FFF for "no extent")
///   - Bits [37:25] = vx_high_s (13 bits)
///   - Bits [50:38] = vx_low_t  (13 bits)
///   - Bits [63:51] = vx_high_t (13 bits)
///   - Bytes 8..15  = 4 x UInt16 LE channel values (R, G, B, A)
/// </summary>
internal static class TestAstcBuilder
{
    public static byte[] LdrVoidExtent(ushort r, ushort g, ushort b, ushort a)
        => BuildVoidExtent(hdr: false, r, g, b, a);

    public static byte[] HdrVoidExtent(ushort r, ushort g, ushort b, ushort a)
        => BuildVoidExtent(hdr: true, r, g, b, a);

    public static void OverrideExtent(Span<byte> block, int vxLowS, int vxHighS, int vxLowT, int vxHighT)
    {
        WriteBits(block, 12, 13, vxLowS);
        WriteBits(block, 25, 13, vxHighS);
        WriteBits(block, 38, 13, vxLowT);
        WriteBits(block, 51, 13, vxHighT);
    }

    private static byte[] BuildVoidExtent(bool hdr, ushort r, ushort g, ushort b, ushort a)
    {
        var block = new byte[16];
        int blockMode = 0x1FC | (hdr ? (1 << 9) : 0);
        blockMode |= (3 << 10);
        block[0] = (byte)(blockMode & 0xFF);
        block[1] = (byte)((blockMode >> 8) & 0xFF);
        WriteBits(block, 12, 13, 0x1FFF);
        WriteBits(block, 25, 13, 0x1FFF);
        WriteBits(block, 38, 13, 0x1FFF);
        WriteBits(block, 51, 13, 0x1FFF);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(8), r);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(10), g);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(12), b);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(14), a);
        return block;
    }

    private static void WriteBits(Span<byte> data, int bitOffset, int bitCount, int value)
    {
        for (int i = 0; i < bitCount; i++)
        {
            int byteIdx = (bitOffset + i) >> 3;
            int bitInByte = (bitOffset + i) & 7;
            int bit = (value >> i) & 1;
            if (bit != 0)
            {
                data[byteIdx] |= (byte)(1 << bitInByte);
            }
            else
            {
                data[byteIdx] &= (byte)~(1 << bitInByte);
            }
        }
    }
}
