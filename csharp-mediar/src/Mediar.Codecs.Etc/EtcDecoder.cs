using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Etc;

/// <summary>
/// Decompresses ETC1 / ETC2 / EAC block-compressed surfaces into top-down
/// pixel buffers. Implements the Khronos KDF 1.4 reference specification
/// for the entire ETC family and is container-agnostic: callers pass the
/// raw block payload from any envelope (KTX, KTX2, PKM, PVR, …).
/// </summary>
/// <remarks>
/// All block formats decode 4×4 pixel tiles. The 64-bit ETC1 / ETC2-RGB
/// block packs a colour header (32 MSBs) plus a per-pixel 2-bit index
/// (32 LSBs). Within the index word, pixel ``p`` (column-major: ``x =
/// p/4``, ``y = p%4``) reads bit ``p`` (LSB) and bit ``p + 16`` (MSB).
/// </remarks>
public static class EtcDecoder
{
    /// <summary>Returns the byte size of one 4×4 block for <paramref name="format"/>.</summary>
    public static int BytesPerBlock(EtcFormat format) => format switch
    {
        EtcFormat.Etc1Rgb or EtcFormat.Etc2Rgb or EtcFormat.Etc2RgbA1
            or EtcFormat.EacR11Unorm or EtcFormat.EacR11Snorm => 8,
        EtcFormat.Etc2Rgba8 or EtcFormat.EacRg11Unorm or EtcFormat.EacRg11Snorm => 16,
        _ => 0,
    };

    /// <summary>Decompresses an ETC1 surface into top-down RGBA32 (opaque alpha).</summary>
    public static byte[] DecodeEtc1(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEtcRgb(src, width, height, etc2: false, punchAlpha: false);

    /// <summary>Decompresses an ETC2 RGB surface into top-down RGBA32 (opaque alpha).</summary>
    public static byte[] DecodeEtc2Rgb(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEtcRgb(src, width, height, etc2: true, punchAlpha: false);

    /// <summary>Decompresses an ETC2 RGB+A1 surface into top-down RGBA32 (1-bit punch-through alpha).</summary>
    public static byte[] DecodeEtc2RgbA1(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEtcRgb(src, width, height, etc2: true, punchAlpha: true);

    /// <summary>Decompresses an ETC2 RGBA8 surface into top-down RGBA32 (EAC 8-bit alpha + ETC2 RGB).</summary>
    public static byte[] DecodeEtc2Rgba8(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> rgb = stackalloc byte[16 * 4]; // 16 RGBA pixels for one block
        Span<byte> alpha = stackalloc byte[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeEacBlock(src.Slice(srcOff, 8), alpha, signed: false, scale: 255);
                DecodeRgbBlock(src.Slice(srcOff + 8, 8), rgb, etc2: true, punchAlpha: false);
                WriteRgbaBlock(output, width, height, bx, by, rgb, alpha);
                srcOff += 16;
            }
        }
        return output;
    }

    /// <summary>Decompresses an EAC R11 unorm surface into top-down 16-bit gray (UInt16 little-endian).</summary>
    public static byte[] DecodeEacR11Unorm(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEacSingle(src, width, height, signed: false);

    /// <summary>Decompresses an EAC R11 snorm surface into top-down 16-bit gray (Int16 little-endian).</summary>
    public static byte[] DecodeEacR11Snorm(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEacSingle(src, width, height, signed: true);

    /// <summary>Decompresses an EAC RG11 unorm surface into top-down 32-bit RG pairs (UInt16 R, UInt16 G, little-endian).</summary>
    public static byte[] DecodeEacRg11Unorm(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEacDual(src, width, height, signed: false);

    /// <summary>Decompresses an EAC RG11 snorm surface into top-down 32-bit RG pairs (Int16 R, Int16 G, little-endian).</summary>
    public static byte[] DecodeEacRg11Snorm(ReadOnlySpan<byte> src, int width, int height)
        => DecodeEacDual(src, width, height, signed: true);

    // ----- ETC RGB core -----

    private static readonly int[,] EtcModifier =
    {
        {  2,   8 },
        {  5,  17 },
        {  9,  29 },
        { 13,  42 },
        { 18,  60 },
        { 24,  80 },
        { 33, 106 },
        { 47, 183 },
    };

    private static readonly int[] EtcDistance = { 3, 6, 11, 16, 23, 32, 41, 64 };

    private static byte[] DecodeEtcRgb(ReadOnlySpan<byte> src, int width, int height, bool etc2, bool punchAlpha)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> rgb = stackalloc byte[16 * 4]; // 16 RGBA pixels for one block

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeRgbBlock(src.Slice(srcOff, 8), rgb, etc2, punchAlpha);
                WriteRgbBlock(output, width, height, bx, by, rgb);
                srcOff += 8;
            }
        }
        return output;
    }

    private static void DecodeRgbBlock(ReadOnlySpan<byte> block, Span<byte> dst, bool etc2, bool punchAlpha)
    {
        ulong b = BinaryPrimitives.ReadUInt64BigEndian(block);
        bool diff = ((b >> 33) & 1) != 0;
        bool flip = ((b >> 32) & 1) != 0;

        // In ETC2 RGB+A1, the bit at position 33 of the unmodified block reads
        // as the "opaque" flag, and the bit at position 32 is still flip.
        // ETC2 with diff=1 may also dispatch to T/H/Planar modes.
        bool opaque = !punchAlpha || diff;

        if (etc2 && diff)
        {
            // Try the differential-mode endpoint expansion. If R/G/B overflow,
            // switch to T / H / Planar modes per ETC2 spec.
            int r1_5 = (int)((b >> 59) & 0x1F);
            int dr = SignExtend3((int)((b >> 56) & 0x7));
            int g1_5 = (int)((b >> 51) & 0x1F);
            int dg = SignExtend3((int)((b >> 48) & 0x7));
            int b1_5 = (int)((b >> 43) & 0x1F);
            int db = SignExtend3((int)((b >> 40) & 0x7));

            int r2 = r1_5 + dr;
            int g2 = g1_5 + dg;
            int b2 = b1_5 + db;

            if (r2 < 0 || r2 > 31)
            {
                DecodeTBlock(b, dst, punchAlpha, !punchAlpha || ((b >> 33) & 1) != 0);
                return;
            }
            if (g2 < 0 || g2 > 31)
            {
                DecodeHBlock(b, dst, punchAlpha, !punchAlpha || ((b >> 33) & 1) != 0);
                return;
            }
            if (b2 < 0 || b2 > 31)
            {
                DecodePlanarBlock(b, dst);
                return;
            }
            // Fall through to standard differential decode.
            DecodeEtc1Like(b, dst, diff: true, flip, opaque, punchAlpha);
            return;
        }

        if (etc2 && punchAlpha && !diff)
        {
            // RGB+A1 with the "opaque" flag = 0: half of the pixel indices
            // become transparent (alpha=0, RGB=0). Endpoint decoding uses
            // differential math even though "diff" reads as 0.
            int r1_5 = (int)((b >> 59) & 0x1F);
            int dr = SignExtend3((int)((b >> 56) & 0x7));
            int g1_5 = (int)((b >> 51) & 0x1F);
            int dg = SignExtend3((int)((b >> 48) & 0x7));
            int b1_5 = (int)((b >> 43) & 0x1F);
            int db = SignExtend3((int)((b >> 40) & 0x7));
            int r2 = r1_5 + dr;
            int g2 = g1_5 + dg;
            int b2 = b1_5 + db;

            if (r2 < 0 || r2 > 31)
            {
                DecodeTBlock(b, dst, punchAlpha, opaque);
                return;
            }
            if (g2 < 0 || g2 > 31)
            {
                DecodeHBlock(b, dst, punchAlpha, opaque);
                return;
            }
            if (b2 < 0 || b2 > 31)
            {
                DecodePlanarBlock(b, dst);
                return;
            }
            DecodeEtc1Like(b, dst, diff: true, flip, opaque, punchAlpha);
            return;
        }

        DecodeEtc1Like(b, dst, diff, flip, opaque, punchAlpha);
    }

    private static void DecodeEtc1Like(ulong b, Span<byte> dst, bool diff, bool flip, bool opaque, bool punchAlpha)
    {
        int r1, g1, b1, r2, g2, b2;

        if (diff)
        {
            int r1_5 = (int)((b >> 59) & 0x1F);
            int dr = SignExtend3((int)((b >> 56) & 0x7));
            int g1_5 = (int)((b >> 51) & 0x1F);
            int dg = SignExtend3((int)((b >> 48) & 0x7));
            int b1_5 = (int)((b >> 43) & 0x1F);
            int db = SignExtend3((int)((b >> 40) & 0x7));
            r1 = Expand5to8(r1_5);
            g1 = Expand5to8(g1_5);
            b1 = Expand5to8(b1_5);
            r2 = Expand5to8(r1_5 + dr);
            g2 = Expand5to8(g1_5 + dg);
            b2 = Expand5to8(b1_5 + db);
        }
        else
        {
            int r1_4 = (int)((b >> 60) & 0xF);
            int r2_4 = (int)((b >> 56) & 0xF);
            int g1_4 = (int)((b >> 52) & 0xF);
            int g2_4 = (int)((b >> 48) & 0xF);
            int b1_4 = (int)((b >> 44) & 0xF);
            int b2_4 = (int)((b >> 40) & 0xF);
            r1 = Expand4to8(r1_4); g1 = Expand4to8(g1_4); b1 = Expand4to8(b1_4);
            r2 = Expand4to8(r2_4); g2 = Expand4to8(g2_4); b2 = Expand4to8(b2_4);
        }

        int t1 = (int)((b >> 37) & 0x7);
        int t2 = (int)((b >> 34) & 0x7);
        uint indices = (uint)(b & 0xFFFFFFFFu);

        for (int p = 0; p < 16; p++)
        {
            int x = p / 4;
            int y = p % 4;
            int subBlock;
            if (flip)
            {
                subBlock = (y < 2) ? 0 : 1; // horizontal flip: top/bottom halves
            }
            else
            {
                subBlock = (x < 2) ? 0 : 1; // vertical flip: left/right halves
            }

            int br = subBlock == 0 ? r1 : r2;
            int bg = subBlock == 0 ? g1 : g2;
            int bb = subBlock == 0 ? b1 : b2;
            int table = subBlock == 0 ? t1 : t2;

            int msb = (int)((indices >> (p + 16)) & 1);
            int lsb = (int)((indices >> p) & 1);
            int idx = (msb << 1) | lsb;

            if (punchAlpha && !opaque && idx == 2)
            {
                int dp = p * 4;
                dst[dp] = 0; dst[dp + 1] = 0; dst[dp + 2] = 0; dst[dp + 3] = 0;
                continue;
            }

            int modifier = ModifierFromIndex(table, idx, punchAlpha && !opaque);
            int rr = Clamp(br + modifier);
            int gg = Clamp(bg + modifier);
            int bbb = Clamp(bb + modifier);

            int o = p * 4;
            dst[o] = (byte)rr;
            dst[o + 1] = (byte)gg;
            dst[o + 2] = (byte)bbb;
            dst[o + 3] = 255;
        }
    }

    private static int ModifierFromIndex(int table, int idx, bool nonOpaqueA1)
    {
        if (nonOpaqueA1)
        {
            // In RGB+A1 transparent regions, ETC2 reuses a restricted modifier set.
            return idx switch
            {
                0 => 0,
                1 => EtcModifier[table, 1],
                2 => 0, // already handled as transparent earlier
                3 => -EtcModifier[table, 1],
                _ => 0,
            };
        }
        return idx switch
        {
            0 => EtcModifier[table, 0],
            1 => EtcModifier[table, 1],
            2 => -EtcModifier[table, 0],
            3 => -EtcModifier[table, 1],
            _ => 0,
        };
    }

    private static void DecodeTBlock(ulong b, Span<byte> dst, bool punchAlpha, bool opaque)
    {
        int R1a = (int)((b >> 59) & 0x3);
        int R1b = (int)((b >> 56) & 0x3);
        int R1 = (R1a << 2) | R1b;
        int G1 = (int)((b >> 52) & 0xF);
        int B1 = (int)((b >> 48) & 0xF);
        int R2 = (int)((b >> 44) & 0xF);
        int G2 = (int)((b >> 40) & 0xF);
        int B2 = (int)((b >> 36) & 0xF);
        int da = (int)((b >> 34) & 0x3);
        int db = (int)((b >> 32) & 0x1);
        int distIdx = (da << 1) | db;
        int distance = EtcDistance[distIdx];

        int r1 = Expand4to8(R1), g1 = Expand4to8(G1), bb1 = Expand4to8(B1);
        int r2 = Expand4to8(R2), g2 = Expand4to8(G2), bb2 = Expand4to8(B2);

        Span<int> palR = stackalloc int[4];
        Span<int> palG = stackalloc int[4];
        Span<int> palB = stackalloc int[4];
        palR[0] = r1; palG[0] = g1; palB[0] = bb1;
        palR[1] = Clamp(r2 + distance); palG[1] = Clamp(g2 + distance); palB[1] = Clamp(bb2 + distance);
        palR[2] = r2; palG[2] = g2; palB[2] = bb2;
        palR[3] = Clamp(r2 - distance); palG[3] = Clamp(g2 - distance); palB[3] = Clamp(bb2 - distance);

        EmitPaletted(b, dst, palR, palG, palB, punchAlpha, opaque);
    }

    private static void DecodeHBlock(ulong b, Span<byte> dst, bool punchAlpha, bool opaque)
    {
        int R1 = (int)((b >> 59) & 0xF);
        int G1a = (int)((b >> 56) & 0x7);
        int G1b = (int)((b >> 52) & 0x1);
        int G1 = (G1a << 1) | G1b;
        int B1a = (int)((b >> 51) & 0x1);
        int B1b = (int)((b >> 47) & 0x7);
        int B1 = (B1a << 3) | B1b;
        int R2 = (int)((b >> 43) & 0xF);
        int G2 = (int)((b >> 39) & 0xF);
        int B2 = (int)((b >> 35) & 0xF);
        int da = (int)((b >> 34) & 0x1);
        int db = (int)((b >> 32) & 0x1);

        int r1 = Expand4to8(R1), g1 = Expand4to8(G1), bb1 = Expand4to8(B1);
        int r2 = Expand4to8(R2), g2 = Expand4to8(G2), bb2 = Expand4to8(B2);

        int c1word = (r1 << 16) | (g1 << 8) | bb1;
        int c2word = (r2 << 16) | (g2 << 8) | bb2;
        int compare = c1word >= c2word ? 1 : 0;
        int distIdx = (da << 2) | (compare << 1) | db;
        int distance = EtcDistance[distIdx];

        Span<int> palR = stackalloc int[4];
        Span<int> palG = stackalloc int[4];
        Span<int> palB = stackalloc int[4];
        palR[0] = Clamp(r1 + distance); palG[0] = Clamp(g1 + distance); palB[0] = Clamp(bb1 + distance);
        palR[1] = Clamp(r1 - distance); palG[1] = Clamp(g1 - distance); palB[1] = Clamp(bb1 - distance);
        palR[2] = Clamp(r2 + distance); palG[2] = Clamp(g2 + distance); palB[2] = Clamp(bb2 + distance);
        palR[3] = Clamp(r2 - distance); palG[3] = Clamp(g2 - distance); palB[3] = Clamp(bb2 - distance);

        EmitPaletted(b, dst, palR, palG, palB, punchAlpha, opaque);
    }

    private static void DecodePlanarBlock(ulong b, Span<byte> dst)
    {
        int R0 = (int)((b >> 57) & 0x3F);
        int G0a = (int)((b >> 56) & 0x1);
        int G0b = (int)((b >> 49) & 0x3F);
        int G0 = (G0a << 6) | G0b;
        int B0a = (int)((b >> 48) & 0x1);
        int B0b = (int)((b >> 43) & 0x3);
        int B0c = (int)((b >> 39) & 0x7);
        int B0 = (B0a << 5) | (B0b << 3) | B0c;
        int RHa = (int)((b >> 34) & 0x1F);
        int RHb = (int)((b >> 32) & 0x1);
        int RH = (RHa << 1) | RHb;
        int GH = (int)((b >> 25) & 0x7F);
        int BH = (int)((b >> 19) & 0x3F);
        int RV = (int)((b >> 13) & 0x3F);
        int GV = (int)((b >> 6) & 0x7F);
        int BV = (int)(b & 0x3F);

        int r0 = Expand6to8(R0), g0 = Expand7to8(G0), bb0 = Expand6to8(B0);
        int rh = Expand6to8(RH), gh = Expand7to8(GH), bh = Expand6to8(BH);
        int rv = Expand6to8(RV), gv = Expand7to8(GV), bv = Expand6to8(BV);

        for (int p = 0; p < 16; p++)
        {
            int x = p / 4;
            int y = p % 4;
            int r = Clamp((x * (rh - r0) + y * (rv - r0) + 4 * r0 + 2) >> 2);
            int g = Clamp((x * (gh - g0) + y * (gv - g0) + 4 * g0 + 2) >> 2);
            int bbb = Clamp((x * (bh - bb0) + y * (bv - bb0) + 4 * bb0 + 2) >> 2);
            int o = p * 4;
            dst[o] = (byte)r; dst[o + 1] = (byte)g; dst[o + 2] = (byte)bbb; dst[o + 3] = 255;
        }
    }

    private static void EmitPaletted(ulong b, Span<byte> dst, Span<int> palR, Span<int> palG, Span<int> palB, bool punchAlpha, bool opaque)
    {
        uint indices = (uint)(b & 0xFFFFFFFFu);
        for (int p = 0; p < 16; p++)
        {
            int msb = (int)((indices >> (p + 16)) & 1);
            int lsb = (int)((indices >> p) & 1);
            int idx = (msb << 1) | lsb;

            if (punchAlpha && !opaque && idx == 2)
            {
                int dp = p * 4;
                dst[dp] = 0; dst[dp + 1] = 0; dst[dp + 2] = 0; dst[dp + 3] = 0;
                continue;
            }

            int o = p * 4;
            dst[o] = (byte)palR[idx];
            dst[o + 1] = (byte)palG[idx];
            dst[o + 2] = (byte)palB[idx];
            dst[o + 3] = 255;
        }
    }

    // ----- EAC (alpha + R11/RG11) core -----

    private static readonly int[,] EacModifier =
    {
        { -3, -6,  -9, -15, 2, 5, 8, 14 },
        { -3, -7, -10, -13, 2, 6, 9, 12 },
        { -2, -5,  -8, -13, 1, 4, 7, 12 },
        { -2, -4,  -6, -13, 1, 3, 5, 12 },
        { -3, -6,  -8, -12, 2, 5, 7, 11 },
        { -3, -7,  -9, -11, 2, 6, 8, 10 },
        { -4, -7,  -8, -11, 3, 6, 7, 10 },
        { -3, -5,  -8, -11, 2, 4, 7, 10 },
        { -2, -6,  -8, -10, 1, 5, 7,  9 },
        { -2, -5,  -8, -10, 1, 4, 7,  9 },
        { -2, -4,  -8, -10, 1, 3, 7,  9 },
        { -2, -5,  -7, -10, 1, 4, 6,  9 },
        { -3, -4,  -7, -10, 2, 3, 6,  9 },
        { -1, -2,  -3, -10, 0, 1, 2,  9 },
        { -4, -6,  -8,  -9, 3, 5, 7,  8 },
        { -3, -5,  -7,  -9, 2, 4, 6,  8 },
    };

    /// <summary>Decodes an EAC 8-byte alpha block into 16 alpha values scaled to 0..255.</summary>
    private static void DecodeEacBlock(ReadOnlySpan<byte> block, Span<byte> dst16, bool signed, int scale)
    {
        // 64-bit big-endian:
        // bits 63..56: base codeword (signed: signed 8-bit; unsigned: 0..255)
        // bits 55..52: multiplier (4 bits, 0..15)
        // bits 51..48: table index (4 bits, 0..15)
        // bits 47..0:  48 bits of pixel indices (3 bits per pixel × 16 pixels)
        ulong b = BinaryPrimitives.ReadUInt64BigEndian(block);
        int basev;
        if (signed)
        {
            sbyte sb = (sbyte)((b >> 56) & 0xFF);
            basev = sb;
        }
        else
        {
            basev = (int)((b >> 56) & 0xFF);
        }
        int mul = (int)((b >> 52) & 0xF);
        int tbl = (int)((b >> 48) & 0xF);
        // Per Khronos KDF 1.4, multiplier = 0 is reserved → encoders use 1
        // for the smallest scale. Decoders still treat raw 0 as "1" per
        // spec note. (For the colour-component formats below, signed/unsigned
        // values are eventually expanded to 11-bit fractional fixed-point.)
        if (mul == 0) mul = 1;

        ulong idxBits = b & 0xFFFFFFFFFFFFUL; // lower 48 bits

        for (int p = 0; p < 16; p++)
        {
            // Pixel ordering matches the ETC RGB block: column-major.
            // Bit position within the 48-bit field: pixel p's index occupies
            // bits [45 - 3p .. 47 - 3p] with the MSB at the higher position
            // — i.e. pixel 0 is the high three bits, pixel 15 the low three.
            int shift = 45 - p * 3;
            int idx = (int)((idxBits >> shift) & 0x7);
            int modifier = EacModifier[tbl, idx];
            int value = basev + modifier * mul;
            if (scale == 255)
            {
                value = Math.Clamp(value, 0, 255);
                dst16[p] = (byte)value;
            }
        }
    }

    private static byte[] DecodeEacSingle(ReadOnlySpan<byte> src, int width, int height, bool signed)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 2];
        Span<short> vals = stackalloc short[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeEac11Block(src.Slice(srcOff, 8), vals, signed);
                WriteSingleChannel16(output, width, height, bx, by, vals, signed);
                srcOff += 8;
            }
        }
        return output;
    }

    private static byte[] DecodeEacDual(ReadOnlySpan<byte> src, int width, int height, bool signed)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<short> r = stackalloc short[16];
        Span<short> g = stackalloc short[16];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeEac11Block(src.Slice(srcOff, 8), r, signed);
                DecodeEac11Block(src.Slice(srcOff + 8, 8), g, signed);
                WriteDualChannel16(output, width, height, bx, by, r, g, signed);
                srcOff += 16;
            }
        }
        return output;
    }

    private static void DecodeEac11Block(ReadOnlySpan<byte> block, Span<short> dst16, bool signed)
    {
        ulong b = BinaryPrimitives.ReadUInt64BigEndian(block);
        int basev;
        if (signed)
        {
            sbyte sb = (sbyte)((b >> 56) & 0xFF);
            basev = sb;
        }
        else
        {
            basev = (int)((b >> 56) & 0xFF);
        }
        int mul = (int)((b >> 52) & 0xF);
        int tbl = (int)((b >> 48) & 0xF);
        ulong idxBits = b & 0xFFFFFFFFFFFFUL;

        for (int p = 0; p < 16; p++)
        {
            int shift = 45 - p * 3;
            int idx = (int)((idxBits >> shift) & 0x7);
            int modifier = EacModifier[tbl, idx];

            int value;
            if (signed)
            {
                // Per spec: output = base * 8 + multiplier * modifier when mul > 0
                // else output = base * 8 + modifier. Range [-1023..1023].
                value = mul == 0 ? basev * 8 + modifier : basev * 8 + modifier * mul;
                value = Math.Clamp(value, -1023, 1023);
                // Scale signed 11-bit [-1023..1023] to Int16 range proportional
                // to its dynamic range — here we just sign-extend the 11-bit
                // value to 16-bit (multiply by 32 keeps relative order).
                dst16[p] = (short)(value * 32);
            }
            else
            {
                // Unsigned: output = base * 8 + 4 + multiplier * modifier when
                // mul > 0; else output = base * 8 + 4 + modifier. Range [0..2047].
                int effectiveMul = mul == 0 ? 1 : mul;
                value = basev * 8 + 4 + modifier * effectiveMul;
                value = Math.Clamp(value, 0, 2047);
                // Scale unsigned 11-bit [0..2047] → UInt16 [0..65535] approximately
                // (×32 + value/64 gives a good fit, but ×32 keeps exact monotonic).
                dst16[p] = (short)(value * 32);
            }
        }
    }

    // ----- Pixel-write helpers -----

    private static void WriteRgbBlock(byte[] output, int width, int height, int bx, int by, ReadOnlySpan<byte> rgba)
    {
        int baseX = bx * 4;
        int baseY = by * 4;
        for (int p = 0; p < 16; p++)
        {
            int x = baseX + p / 4;
            int y = baseY + p % 4;
            if (x >= width || y >= height) continue;
            int dst = (y * width + x) * 4;
            int src = p * 4;
            output[dst] = rgba[src];
            output[dst + 1] = rgba[src + 1];
            output[dst + 2] = rgba[src + 2];
            output[dst + 3] = rgba[src + 3];
        }
    }

    private static void WriteRgbaBlock(byte[] output, int width, int height, int bx, int by, ReadOnlySpan<byte> rgb, ReadOnlySpan<byte> alpha)
    {
        int baseX = bx * 4;
        int baseY = by * 4;
        for (int p = 0; p < 16; p++)
        {
            int x = baseX + p / 4;
            int y = baseY + p % 4;
            if (x >= width || y >= height) continue;
            int dst = (y * width + x) * 4;
            int src = p * 4;
            output[dst] = rgb[src];
            output[dst + 1] = rgb[src + 1];
            output[dst + 2] = rgb[src + 2];
            output[dst + 3] = alpha[p];
        }
    }

    private static void WriteSingleChannel16(byte[] output, int width, int height, int bx, int by, ReadOnlySpan<short> vals, bool signed)
    {
        int baseX = bx * 4;
        int baseY = by * 4;
        for (int p = 0; p < 16; p++)
        {
            int x = baseX + p / 4;
            int y = baseY + p % 4;
            if (x >= width || y >= height) continue;
            int dst = (y * width + x) * 2;
            short v = vals[p];
            output[dst] = (byte)(v & 0xFF);
            output[dst + 1] = (byte)((v >> 8) & 0xFF);
        }
    }

    private static void WriteDualChannel16(byte[] output, int width, int height, int bx, int by, ReadOnlySpan<short> r, ReadOnlySpan<short> g, bool signed)
    {
        int baseX = bx * 4;
        int baseY = by * 4;
        for (int p = 0; p < 16; p++)
        {
            int x = baseX + p / 4;
            int y = baseY + p % 4;
            if (x >= width || y >= height) continue;
            int dst = (y * width + x) * 4;
            short rv = r[p];
            short gv = g[p];
            output[dst] = (byte)(rv & 0xFF);
            output[dst + 1] = (byte)((rv >> 8) & 0xFF);
            output[dst + 2] = (byte)(gv & 0xFF);
            output[dst + 3] = (byte)((gv >> 8) & 0xFF);
        }
    }

    // ----- Bit helpers -----

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Expand4to8(int v) => (v << 4) | v;

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Expand5to8(int v) => ((v & 0x1F) << 3) | ((v & 0x1F) >> 2);

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Expand6to8(int v) => ((v & 0x3F) << 2) | ((v & 0x3F) >> 4);

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Expand7to8(int v) => ((v & 0x7F) << 1) | ((v & 0x7F) >> 6);

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int SignExtend3(int v) => (v & 0x4) != 0 ? v - 8 : v;

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Clamp(int v) => v < 0 ? 0 : (v > 255 ? 255 : v);
}
