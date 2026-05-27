using System.Buffers.Binary;
using Mediar.Codecs.Etc;
using Xunit;

namespace Mediar.Tests.Etc;

public class EtcDecoderTests
{
    [Fact]
    public void BytesPerBlock_Returns_Correct_Sizes()
    {
        Assert.Equal(8, EtcDecoder.BytesPerBlock(EtcFormat.Etc1Rgb));
        Assert.Equal(8, EtcDecoder.BytesPerBlock(EtcFormat.Etc2Rgb));
        Assert.Equal(8, EtcDecoder.BytesPerBlock(EtcFormat.Etc2RgbA1));
        Assert.Equal(16, EtcDecoder.BytesPerBlock(EtcFormat.Etc2Rgba8));
        Assert.Equal(8, EtcDecoder.BytesPerBlock(EtcFormat.EacR11Unorm));
        Assert.Equal(8, EtcDecoder.BytesPerBlock(EtcFormat.EacR11Snorm));
        Assert.Equal(16, EtcDecoder.BytesPerBlock(EtcFormat.EacRg11Unorm));
        Assert.Equal(16, EtcDecoder.BytesPerBlock(EtcFormat.EacRg11Snorm));
        Assert.Equal(0, EtcDecoder.BytesPerBlock(EtcFormat.None));
    }

    [Fact]
    public void Etc1_AllZero_Block_Produces_Opaque_Near_Black()
    {
        // diff=0 individual mode, all endpoints = 0, table 0, all indices = 0
        // -> modifier = EtcModifier[0,0] = +2, output = (2,2,2,255).
        var block = new byte[8];
        var rgba = EtcDecoder.DecodeEtc1(block, 4, 4);
        Assert.Equal(64, rgba.Length);
        for (int p = 0; p < 16; p++)
        {
            Assert.Equal(2, rgba[p * 4 + 0]);
            Assert.Equal(2, rgba[p * 4 + 1]);
            Assert.Equal(2, rgba[p * 4 + 2]);
            Assert.Equal(255, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void Etc1_All_Indices_Three_Produces_Black_Opaque()
    {
        // Set the lower 32 bits (indices field) all = 1 -> every pixel idx = 3
        // Modifier from idx=3, table=0 = -EtcModifier[0,1] = -8
        // Clamp(0 + (-8)) = 0; output = (0,0,0,255).
        ulong b = 0xFFFFFFFFUL;
        var block = new byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(block, b);
        var rgba = EtcDecoder.DecodeEtc1(block, 4, 4);
        for (int p = 0; p < 16; p++)
        {
            Assert.Equal(0, rgba[p * 4 + 0]);
            Assert.Equal(0, rgba[p * 4 + 1]);
            Assert.Equal(0, rgba[p * 4 + 2]);
            Assert.Equal(255, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void Etc2_Rgb_AllZero_Block_Decodes_Like_Etc1()
    {
        // diff=0 path falls through to the ETC1-like decoder.
        var block = new byte[8];
        var rgba = EtcDecoder.DecodeEtc2Rgb(block, 4, 4);
        Assert.Equal(64, rgba.Length);
        for (int p = 0; p < 16; p++)
        {
            Assert.Equal(2, rgba[p * 4 + 0]);
            Assert.Equal(255, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void Etc2_RgbA1_With_Opaque_Bit_Set_Decodes_Opaque()
    {
        // Bit 33 = 1 (opaque flag set). Differential endpoints = 0, no overflow,
        // falls through to ETC1-like differential decode with idx=0 -> modifier +2.
        ulong b = 1UL << 33;
        var block = new byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(block, b);
        var rgba = EtcDecoder.DecodeEtc2RgbA1(block, 4, 4);
        for (int p = 0; p < 16; p++)
        {
            Assert.Equal(255, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void Etc2_RgbA1_With_Opaque_Bit_Clear_And_Index_2_Pixel_Is_Transparent()
    {
        // Bit 33 = 0 (opaque flag clear). Set bit 16 = 1, leave bit 0 = 0 so
        // pixel 0 has msb=1, lsb=0 -> idx=2 -> transparent (RGBA=0).
        ulong b = 1UL << 16;
        var block = new byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(block, b);
        var rgba = EtcDecoder.DecodeEtc2RgbA1(block, 4, 4);
        // Pixel 0 in column-major maps to output index 0 (x=0,y=0).
        Assert.Equal(0, rgba[0]);
        Assert.Equal(0, rgba[1]);
        Assert.Equal(0, rgba[2]);
        Assert.Equal(0, rgba[3]);
    }

    [Fact]
    public void Etc2_Rgba8_Produces_RGBA32_With_Alpha_From_EAC_Block()
    {
        // 16-byte block: 8 bytes EAC alpha (all zeros -> base=0, mult=0->1, table=0,
        // all indices=0 -> modifier=-3 -> value = -3 clamped to 0) + 8 bytes
        // ETC2 RGB (all zeros -> RGB=2 like Etc1 all-zero).
        var block = new byte[16];
        var rgba = EtcDecoder.DecodeEtc2Rgba8(block, 4, 4);
        Assert.Equal(64, rgba.Length);
        for (int p = 0; p < 16; p++)
        {
            Assert.Equal(2, rgba[p * 4 + 0]);
            Assert.Equal(2, rgba[p * 4 + 1]);
            Assert.Equal(2, rgba[p * 4 + 2]);
            Assert.Equal(0, rgba[p * 4 + 3]);
        }
    }

    [Fact]
    public void EacR11Unorm_Decodes_To_16_Bit_Gray()
    {
        // base=128 (0x80), mult=0 (=>1), table=0, all 16 indices=4 ("100")
        // -> modifier = EacModifier[0,4] = 2
        // -> value = 128*8 + 4 + 2*1 = 1030
        // -> stored = (short)(1030*32) = 32960 (bit pattern 0x80C0 LE).
        ulong b = (ulong)0x80 << 56;
        for (int p = 0; p < 16; p++)
        {
            int shift = 45 - p * 3;
            b |= 1UL << (shift + 2); // set MSB of each 3-bit index
        }
        var block = new byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(block, b);
        var gray = EtcDecoder.DecodeEacR11Unorm(block, 4, 4);
        Assert.Equal(32, gray.Length);
        for (int p = 0; p < 16; p++)
        {
            ushort v = BinaryPrimitives.ReadUInt16LittleEndian(gray.AsSpan(p * 2, 2));
            Assert.Equal(32960, (int)v);
        }
    }

    [Fact]
    public void EacR11Snorm_Decodes_To_16_Bit_Signed_Gray()
    {
        // signed base=0, mult=0 -> output = base*8 + modifier
        // table=0, all indices=4 -> modifier=2 -> value=2 -> stored=(short)(2*32)=64.
        ulong b = 0UL;
        for (int p = 0; p < 16; p++)
        {
            int shift = 45 - p * 3;
            b |= 1UL << (shift + 2);
        }
        var block = new byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(block, b);
        var gray = EtcDecoder.DecodeEacR11Snorm(block, 4, 4);
        Assert.Equal(32, gray.Length);
        for (int p = 0; p < 16; p++)
        {
            short v = BinaryPrimitives.ReadInt16LittleEndian(gray.AsSpan(p * 2, 2));
            Assert.Equal((short)64, v);
        }
    }

    [Fact]
    public void EacRg11Unorm_Produces_R16_G16_Pairs_Per_Pixel()
    {
        // Two consecutive all-zero EAC R11 blocks -> per pixel value = 0+4+(-3)*1 = 1
        // stored = (short)(1*32) = 32 (LE bytes [0x20, 0x00]).
        var block = new byte[16];
        var rg = EtcDecoder.DecodeEacRg11Unorm(block, 4, 4);
        Assert.Equal(64, rg.Length);
        for (int p = 0; p < 16; p++)
        {
            ushort r = BinaryPrimitives.ReadUInt16LittleEndian(rg.AsSpan(p * 4 + 0, 2));
            ushort g = BinaryPrimitives.ReadUInt16LittleEndian(rg.AsSpan(p * 4 + 2, 2));
            Assert.Equal((ushort)32, r);
            Assert.Equal((ushort)32, g);
        }
    }

    [Fact]
    public void EacRg11Snorm_Decodes_Two_Channels()
    {
        // All-zero block -> snorm path: value = 0*8 + (-3) = -3 -> stored = (-3)*32 = -96.
        var block = new byte[16];
        var rg = EtcDecoder.DecodeEacRg11Snorm(block, 4, 4);
        Assert.Equal(64, rg.Length);
        for (int p = 0; p < 16; p++)
        {
            short r = BinaryPrimitives.ReadInt16LittleEndian(rg.AsSpan(p * 4 + 0, 2));
            short g = BinaryPrimitives.ReadInt16LittleEndian(rg.AsSpan(p * 4 + 2, 2));
            Assert.Equal((short)-96, r);
            Assert.Equal((short)-96, g);
        }
    }

    [Fact]
    public void DecodeEtc1_NonMultiple_Of_4_Dimensions_Clips_Excess_Pixels()
    {
        // 3x3 output from a single 4x4 block. All-zero block -> all (2,2,2,255).
        var block = new byte[8];
        var rgba = EtcDecoder.DecodeEtc1(block, 3, 3);
        Assert.Equal(36, rgba.Length);
        for (int i = 0; i < 9; i++)
        {
            Assert.Equal(2, rgba[i * 4 + 0]);
            Assert.Equal(255, rgba[i * 4 + 3]);
        }
    }
}
