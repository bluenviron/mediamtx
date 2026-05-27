using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Bcn;

/// <summary>
/// Decompresses BC1 (DXT1) through BC5 (RGTC2) block-compressed surfaces
/// into top-down pixel buffers. BC6H (HDR) and BC7 (adaptive partitioning)
/// are deliberately out of scope of this class and live in
/// <see cref="Bc6hDecoder"/> / <see cref="Bc7Decoder"/>; <see cref="BcnFormat"/>
/// is the common dispatch enum.
/// </summary>
/// <remarks>
/// Block layouts follow the published DirectX / Khronos specifications:
/// <list type="bullet">
///   <item>BC1 / DXT1: 8 bytes per 4×4 block; two RGB565 endpoints + 32 2-bit indices, optional 1-bit punch-through alpha.</item>
///   <item>BC2 / DXT3: 16 bytes per 4×4 block; 64 bits of explicit 4-bit alpha + BC1-style colour.</item>
///   <item>BC3 / DXT5: 16 bytes per 4×4 block; interpolated 3-bit alpha + BC1-style colour.</item>
///   <item>BC4 / RGTC1: 8 bytes per 4×4 block; interpolated 3-bit single-channel (red).</item>
///   <item>BC5 / RGTC2: 16 bytes per 4×4 block; two BC4-style channels (red + green).</item>
/// </list>
/// All decoders produce top-down output and are allocation-free apart from
/// the returned buffer. The codec is container-agnostic — it can be wired
/// to DDS, KTX, KTX2, PVR, or any custom envelope by passing the raw
/// block payload to <see cref="DecodeBc1"/> … <see cref="DecodeBc5"/>.
/// </remarks>
public static class BcnDecoder
{
    /// <summary>Returns BC1/2/3/4/5/6H-UF16/6H-SF16/7 if <paramref name="fourCC"/> (or <paramref name="dxgiFormat"/> for DX10) identifies one; otherwise <see cref="BcnFormat.None"/>.</summary>
    public static BcnFormat Identify(string fourCC, uint dxgiFormat)
    {
        return fourCC switch
        {
            "DXT1" => BcnFormat.Bc1,
            "DXT2" or "DXT3" => BcnFormat.Bc2,
            "DXT4" or "DXT5" => BcnFormat.Bc3,
            "ATI1" or "BC4U" or "BC4S" => BcnFormat.Bc4,
            "ATI2" or "BC5U" or "BC5S" or "A2XY" => BcnFormat.Bc5,
            "BC6H" => BcnFormat.Bc6hUf16,
            "BC7" or "BC7L" or "ZOLA" => BcnFormat.Bc7,
            "DX10" => dxgiFormat switch
            {
                70 or 71 or 72 => BcnFormat.Bc1,
                73 or 74 or 75 => BcnFormat.Bc2,
                76 or 77 or 78 => BcnFormat.Bc3,
                79 or 80 or 81 => BcnFormat.Bc4,
                82 or 83 or 84 => BcnFormat.Bc5,
                94 or 95 => BcnFormat.Bc6hUf16, // typeless or UF16
                96 => BcnFormat.Bc6hSf16,
                97 or 98 or 99 => BcnFormat.Bc7,
                _ => BcnFormat.None,
            },
            _ => BcnFormat.None,
        };
    }

    /// <summary>Decompresses a BC1 surface into top-down BGRA32.</summary>
    public static byte[] DecodeBc1(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> colors = stackalloc byte[16]; // 4 BGRA colours

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                ushort c0 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff, 2));
                ushort c1 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff + 2, 2));
                uint indices = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(srcOff + 4, 4));
                BuildBc1Palette(c0, c1, colors, allowAlpha: c0 <= c1);
                WriteBlock4Bgra(output, width, height, bx, by, indices, colors);
                srcOff += 8;
            }
        }
        return output;
    }

    /// <summary>Decompresses a BC2 (DXT3) surface into top-down BGRA32.</summary>
    public static byte[] DecodeBc2(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> colors = stackalloc byte[16];
        Span<byte> alphas = stackalloc byte[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                ulong aBits = BinaryPrimitives.ReadUInt64LittleEndian(src.Slice(srcOff, 8));
                for (int i = 0; i < 16; i++)
                {
                    int nib = (int)((aBits >> (4 * i)) & 0xF);
                    alphas[i] = (byte)((nib << 4) | nib);
                }

                ushort c0 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff + 8, 2));
                ushort c1 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff + 10, 2));
                uint indices = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(srcOff + 12, 4));
                BuildBc1Palette(c0, c1, colors, allowAlpha: false);
                WriteBlock4BgraExplicitAlpha(output, width, height, bx, by, indices, colors, alphas);
                srcOff += 16;
            }
        }
        return output;
    }

    /// <summary>Decompresses a BC3 (DXT5) surface into top-down BGRA32.</summary>
    public static byte[] DecodeBc3(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> colors = stackalloc byte[16];
        Span<byte> alphas = stackalloc byte[16];
        Span<byte> alphaPalette = stackalloc byte[8];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                BuildBc4Block(src.Slice(srcOff, 8), alphaPalette, alphas);

                ushort c0 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff + 8, 2));
                ushort c1 = BinaryPrimitives.ReadUInt16LittleEndian(src.Slice(srcOff + 10, 2));
                uint indices = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(srcOff + 12, 4));
                BuildBc1Palette(c0, c1, colors, allowAlpha: false);
                WriteBlock4BgraExplicitAlpha(output, width, height, bx, by, indices, colors, alphas);
                srcOff += 16;
            }
        }
        return output;
    }

    /// <summary>Decompresses a BC4 (single-channel red) surface into top-down Gray8.</summary>
    public static byte[] DecodeBc4(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height];
        Span<byte> palette = stackalloc byte[8];
        Span<byte> values = stackalloc byte[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                BuildBc4Block(src.Slice(srcOff, 8), palette, values);
                for (int py = 0; py < 4; py++)
                {
                    int gy = by * 4 + py;
                    if (gy >= height) break;
                    for (int px = 0; px < 4; px++)
                    {
                        int gx = bx * 4 + px;
                        if (gx >= width) break;
                        output[gy * width + gx] = values[py * 4 + px];
                    }
                }
                srcOff += 8;
            }
        }
        return output;
    }

    /// <summary>Decompresses a BC5 (two-channel red+green) surface into top-down RGB24 (R, G, 0).</summary>
    public static byte[] DecodeBc5(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 3];
        Span<byte> paletteR = stackalloc byte[8];
        Span<byte> paletteG = stackalloc byte[8];
        Span<byte> reds = stackalloc byte[16];
        Span<byte> greens = stackalloc byte[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                BuildBc4Block(src.Slice(srcOff, 8), paletteR, reds);
                BuildBc4Block(src.Slice(srcOff + 8, 8), paletteG, greens);
                for (int py = 0; py < 4; py++)
                {
                    int gy = by * 4 + py;
                    if (gy >= height) break;
                    for (int px = 0; px < 4; px++)
                    {
                        int gx = bx * 4 + px;
                        if (gx >= width) break;
                        int o = (gy * width + gx) * 3;
                        output[o + 0] = reds[py * 4 + px];
                        output[o + 1] = greens[py * 4 + px];
                        output[o + 2] = 0;
                    }
                }
                srcOff += 16;
            }
        }
        return output;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void BuildBc1Palette(ushort c0, ushort c1, Span<byte> dst, bool allowAlpha)
    {
        Rgb565To888(c0, out byte r0, out byte g0, out byte b0);
        Rgb565To888(c1, out byte r1, out byte g1, out byte b1);

        dst[0] = b0; dst[1] = g0; dst[2] = r0; dst[3] = 0xFF;
        dst[4] = b1; dst[5] = g1; dst[6] = r1; dst[7] = 0xFF;

        if (allowAlpha)
        {
            // 1-bit alpha mode: c2 = (c0 + c1) / 2, c3 = transparent black.
            dst[8] = (byte)((b0 + b1) / 2);
            dst[9] = (byte)((g0 + g1) / 2);
            dst[10] = (byte)((r0 + r1) / 2);
            dst[11] = 0xFF;
            dst[12] = 0; dst[13] = 0; dst[14] = 0; dst[15] = 0;
        }
        else
        {
            // Opaque mode: c2 = (2c0 + c1)/3, c3 = (c0 + 2c1)/3.
            dst[8] = (byte)((2 * b0 + b1) / 3);
            dst[9] = (byte)((2 * g0 + g1) / 3);
            dst[10] = (byte)((2 * r0 + r1) / 3);
            dst[11] = 0xFF;
            dst[12] = (byte)((b0 + 2 * b1) / 3);
            dst[13] = (byte)((g0 + 2 * g1) / 3);
            dst[14] = (byte)((r0 + 2 * r1) / 3);
            dst[15] = 0xFF;
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void Rgb565To888(ushort c, out byte r, out byte g, out byte b)
    {
        int r5 = (c >> 11) & 0x1F;
        int g6 = (c >> 5) & 0x3F;
        int b5 = c & 0x1F;
        r = (byte)((r5 << 3) | (r5 >> 2));
        g = (byte)((g6 << 2) | (g6 >> 4));
        b = (byte)((b5 << 3) | (b5 >> 2));
    }

    private static void BuildBc4Block(ReadOnlySpan<byte> block, Span<byte> palette, Span<byte> values)
    {
        byte a0 = block[0];
        byte a1 = block[1];
        palette[0] = a0;
        palette[1] = a1;
        if (a0 > a1)
        {
            for (int i = 1; i <= 6; i++)
            {
                palette[i + 1] = (byte)(((7 - i) * a0 + i * a1) / 7);
            }
        }
        else
        {
            for (int i = 1; i <= 4; i++)
            {
                palette[i + 1] = (byte)(((5 - i) * a0 + i * a1) / 5);
            }
            palette[6] = 0;
            palette[7] = 0xFF;
        }

        // 16 × 3-bit indices packed into 48 bits = 6 bytes (bytes 2..7).
        ulong bits = 0;
        for (int i = 0; i < 6; i++)
        {
            bits |= (ulong)block[2 + i] << (i * 8);
        }
        for (int i = 0; i < 16; i++)
        {
            int idx = (int)((bits >> (i * 3)) & 0x7);
            values[i] = palette[idx];
        }
    }

    private static void WriteBlock4Bgra(
        byte[] output, int width, int height, int bx, int by, uint indices, ReadOnlySpan<byte> colors)
    {
        for (int py = 0; py < 4; py++)
        {
            int gy = by * 4 + py;
            if (gy >= height) break;
            for (int px = 0; px < 4; px++)
            {
                int gx = bx * 4 + px;
                if (gx >= width) break;
                int idx = (int)((indices >> (2 * (py * 4 + px))) & 0x3);
                int o = (gy * width + gx) * 4;
                int p = idx * 4;
                output[o + 0] = colors[p + 0];
                output[o + 1] = colors[p + 1];
                output[o + 2] = colors[p + 2];
                output[o + 3] = colors[p + 3];
            }
        }
    }

    private static void WriteBlock4BgraExplicitAlpha(
        byte[] output, int width, int height, int bx, int by,
        uint indices, ReadOnlySpan<byte> colors, ReadOnlySpan<byte> alphas)
    {
        for (int py = 0; py < 4; py++)
        {
            int gy = by * 4 + py;
            if (gy >= height) break;
            for (int px = 0; px < 4; px++)
            {
                int gx = bx * 4 + px;
                if (gx >= width) break;
                int idx = (int)((indices >> (2 * (py * 4 + px))) & 0x3);
                int o = (gy * width + gx) * 4;
                int p = idx * 4;
                output[o + 0] = colors[p + 0];
                output[o + 1] = colors[p + 1];
                output[o + 2] = colors[p + 2];
                output[o + 3] = alphas[py * 4 + px];
            }
        }
    }
}

