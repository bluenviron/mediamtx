using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Dds;

/// <summary>
/// Decompresses BC7 (BPTC unorm) block-compressed surfaces into top-down BGRA32
/// pixel buffers. BC7 stores each 4×4 block in exactly 16 bytes (128 bits)
/// using one of 8 modes; each mode picks a different trade-off between number
/// of subsets (1/2/3), per-channel endpoint precision, P-bits, index precision
/// and an optional alpha channel.
/// </summary>
/// <remarks>
/// Implementation follows the Khronos / Microsoft DXGI specification for
/// BC7 / BPTC_UNORM. All modes (0-7) are supported. Mode 8 (reserved /
/// padding) decodes to a fully-transparent black block.
/// </remarks>
internal static class Bc7Decoder
{
    /// <summary>Decodes an entire BC7-compressed surface into a top-down BGRA32 buffer.</summary>
    public static byte[] DecodeBc7(ReadOnlySpan<byte> src, int width, int height)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        var output = new byte[width * height * 4];
        Span<byte> block = stackalloc byte[64]; // 16 BGRA pixels per 4×4 block

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeBlock(src.Slice(srcOff, 16), block);
                // Splat block into output.
                for (int py = 0; py < 4; py++)
                {
                    int gy = by * 4 + py;
                    if (gy >= height) break;
                    for (int px = 0; px < 4; px++)
                    {
                        int gx = bx * 4 + px;
                        if (gx >= width) break;
                        int srcIdx = (py * 4 + px) * 4;
                        int dstIdx = (gy * width + gx) * 4;
                        output[dstIdx + 0] = block[srcIdx + 0];
                        output[dstIdx + 1] = block[srcIdx + 1];
                        output[dstIdx + 2] = block[srcIdx + 2];
                        output[dstIdx + 3] = block[srcIdx + 3];
                    }
                }
                srcOff += 16;
            }
        }
        return output;
    }

    /// <summary>Per-mode parameter table.</summary>
    private readonly record struct ModeInfo(
        int Subsets,
        int PartitionBits,
        int RotationBits,
        int IdxSelBits,
        int ColorBits,
        int AlphaBits,
        int EpPBits, // 1 P-bit per endpoint (shared by RGB+A)
        int SpPBits, // 1 P-bit shared by both endpoints in a subset
        int IdxBits,
        int Idx2Bits);

    private static readonly ModeInfo[] s_modes =
    [
        // Subsets, PartBits, RotBits, IdxSel, Cbits, Abits, EpP, SpP, Idx, Idx2
        new(3, 4, 0, 0, 4, 0, 1, 0, 3, 0), // Mode 0
        new(2, 6, 0, 0, 6, 0, 0, 1, 3, 0), // Mode 1
        new(3, 6, 0, 0, 5, 0, 0, 0, 2, 0), // Mode 2
        new(2, 6, 0, 0, 7, 0, 1, 0, 2, 0), // Mode 3
        new(1, 0, 2, 1, 5, 6, 0, 0, 2, 3), // Mode 4
        new(1, 0, 2, 0, 7, 8, 0, 0, 2, 2), // Mode 5
        new(1, 0, 0, 0, 7, 7, 1, 0, 4, 0), // Mode 6
        new(2, 6, 0, 0, 5, 5, 1, 0, 2, 0), // Mode 7
    ];

    private static void DecodeBlock(ReadOnlySpan<byte> src, Span<byte> dst)
    {
        // Find mode = first bit set in byte 0 (LSB).
        byte b0 = src[0];
        int mode = -1;
        for (int i = 0; i < 8; i++)
        {
            if ((b0 & (1 << i)) != 0) { mode = i; break; }
        }
        if (mode < 0)
        {
            // Mode 8 (reserved) — output transparent black.
            dst.Clear();
            return;
        }

        var info = s_modes[mode];
        var reader = new BitReader(src);
        reader.Consume(mode + 1); // mode bits

        int partition = info.PartitionBits > 0 ? reader.Read(info.PartitionBits) : 0;
        int rotation = info.RotationBits > 0 ? reader.Read(info.RotationBits) : 0;
        int idxSel = info.IdxSelBits > 0 ? reader.Read(info.IdxSelBits) : 0;

        int subsets = info.Subsets;
        int numEndpoints = subsets * 2;

        // Endpoint colour components: R[i], G[i], B[i], then optionally A[i] for each endpoint.
        Span<byte> r = stackalloc byte[6];
        Span<byte> g = stackalloc byte[6];
        Span<byte> b = stackalloc byte[6];
        Span<byte> a = stackalloc byte[6];

        for (int i = 0; i < numEndpoints; i++) r[i] = (byte)reader.Read(info.ColorBits);
        for (int i = 0; i < numEndpoints; i++) g[i] = (byte)reader.Read(info.ColorBits);
        for (int i = 0; i < numEndpoints; i++) b[i] = (byte)reader.Read(info.ColorBits);
        if (info.AlphaBits > 0)
        {
            for (int i = 0; i < numEndpoints; i++) a[i] = (byte)reader.Read(info.AlphaBits);
        }
        else
        {
            for (int i = 0; i < numEndpoints; i++) a[i] = 0xFF;
        }

        // P-bits.
        Span<byte> pBits = stackalloc byte[6];
        if (info.EpPBits > 0)
        {
            for (int i = 0; i < numEndpoints; i++) pBits[i] = (byte)reader.Read(1);
        }
        else if (info.SpPBits > 0)
        {
            for (int s = 0; s < subsets; s++)
            {
                byte p = (byte)reader.Read(1);
                pBits[2 * s] = p;
                pBits[2 * s + 1] = p;
            }
        }

        // Promote endpoint components to 8 bits.
        int cBits = info.ColorBits + (info.EpPBits + info.SpPBits);
        int aBits = info.AlphaBits > 0 ? info.AlphaBits + (info.EpPBits + info.SpPBits) : 0;
        for (int i = 0; i < numEndpoints; i++)
        {
            if (info.EpPBits + info.SpPBits > 0)
            {
                r[i] = (byte)((r[i] << 1) | pBits[i]);
                g[i] = (byte)((g[i] << 1) | pBits[i]);
                b[i] = (byte)((b[i] << 1) | pBits[i]);
                if (info.AlphaBits > 0)
                {
                    a[i] = (byte)((a[i] << 1) | pBits[i]);
                }
            }
            r[i] = Unquantize(r[i], cBits);
            g[i] = Unquantize(g[i], cBits);
            b[i] = Unquantize(b[i], cBits);
            if (info.AlphaBits > 0)
            {
                a[i] = Unquantize(a[i], aBits);
            }
        }

        // Determine partition (subset id per pixel).
        Span<byte> partitionTable = stackalloc byte[16];
        if (subsets == 1)
        {
            partitionTable.Clear();
        }
        else if (subsets == 2)
        {
            for (int i = 0; i < 16; i++) partitionTable[i] = (byte)((s_partition2[partition] >> i) & 1);
        }
        else
        {
            // 3-subset: each pixel takes 2 bits in the partition table.
            ulong p3 = s_partition3[partition];
            for (int i = 0; i < 16; i++) partitionTable[i] = (byte)((p3 >> (2 * i)) & 3);
        }

        // Compute anchor indices per subset.
        Span<int> anchors = stackalloc int[3];
        anchors[0] = 0;
        if (subsets == 2)
        {
            anchors[1] = s_anchor2[partition];
        }
        else if (subsets == 3)
        {
            anchors[1] = s_anchor3a[partition];
            anchors[2] = s_anchor3b[partition];
        }

        // Read primary indices.
        Span<byte> colorIndices = stackalloc byte[16];
        for (int i = 0; i < 16; i++)
        {
            int sub = partitionTable[i];
            bool isAnchor = i == anchors[sub];
            int bits = info.IdxBits - (isAnchor ? 1 : 0);
            colorIndices[i] = (byte)reader.Read(bits);
        }

        // Read alpha indices for modes with separate alpha indices.
        Span<byte> alphaIndices = stackalloc byte[16];
        if (info.Idx2Bits > 0)
        {
            for (int i = 0; i < 16; i++)
            {
                int bits = info.Idx2Bits - (i == 0 ? 1 : 0);
                alphaIndices[i] = (byte)reader.Read(bits);
            }
        }

        // Decode pixels.
        int colorIdxPrecision = info.IdxBits;
        int alphaIdxPrecision = info.Idx2Bits > 0 ? info.Idx2Bits : info.IdxBits;

        for (int i = 0; i < 16; i++)
        {
            int sub = partitionTable[i];
            int e0 = 2 * sub;
            int e1 = 2 * sub + 1;

            // Determine which index drives which channel (idxSel + rotation).
            byte cIdx, aIdx;
            int cIdxPrec, aIdxPrec;
            if (info.Idx2Bits > 0)
            {
                if (idxSel == 0)
                {
                    cIdx = colorIndices[i]; aIdx = alphaIndices[i];
                    cIdxPrec = colorIdxPrecision; aIdxPrec = alphaIdxPrecision;
                }
                else
                {
                    cIdx = alphaIndices[i]; aIdx = colorIndices[i];
                    cIdxPrec = alphaIdxPrecision; aIdxPrec = colorIdxPrecision;
                }
            }
            else
            {
                cIdx = colorIndices[i]; aIdx = colorIndices[i];
                cIdxPrec = colorIdxPrecision; aIdxPrec = colorIdxPrecision;
            }

            byte rOut = Interpolate(r[e0], r[e1], cIdx, cIdxPrec);
            byte gOut = Interpolate(g[e0], g[e1], cIdx, cIdxPrec);
            byte bOut = Interpolate(b[e0], b[e1], cIdx, cIdxPrec);
            byte aOut = Interpolate(a[e0], a[e1], aIdx, aIdxPrec);

            // Channel rotation (modes 4, 5): swap alpha with a colour channel.
            if (rotation == 1) (rOut, aOut) = (aOut, rOut);
            else if (rotation == 2) (gOut, aOut) = (aOut, gOut);
            else if (rotation == 3) (bOut, aOut) = (aOut, bOut);

            int o = i * 4;
            dst[o + 0] = bOut;
            dst[o + 1] = gOut;
            dst[o + 2] = rOut;
            dst[o + 3] = aOut;
        }
    }

    /// <summary>Expand an n-bit endpoint component to 8 bits with bit replication.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static byte Unquantize(byte value, int sourceBits)
    {
        if (sourceBits >= 8) return value;
        int shifted = value << (8 - sourceBits);
        return (byte)(shifted | (shifted >> sourceBits));
    }

    /// <summary>BC7-spec interpolation between two endpoint components using an n-bit index.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static byte Interpolate(byte e0, byte e1, byte index, int indexBits)
    {
        ReadOnlySpan<ushort> weights = indexBits switch
        {
            2 => s_weights2,
            3 => s_weights3,
            4 => s_weights4,
            _ => s_weights2,
        };
        int w = weights[index];
        int v = ((64 - w) * e0 + w * e1 + 32) >> 6;
        return (byte)v;
    }

    private static readonly ushort[] s_weights2 = [0, 21, 43, 64];
    private static readonly ushort[] s_weights3 = [0, 9, 18, 27, 37, 46, 55, 64];
    private static readonly ushort[] s_weights4 = [0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64];

    // 2-subset partition table: bit i indicates which subset (0 or 1) pixel i belongs to.
    private static readonly ushort[] s_partition2 =
    [
        0xCCCC, 0x8888, 0xEEEE, 0xECC8, 0xC880, 0xFEEC, 0xFEC8, 0xEC80,
        0xC800, 0xFFEC, 0xFE80, 0xE800, 0xFFE8, 0xFF00, 0xFFF0, 0xF000,
        0xF710, 0x008E, 0x7100, 0x08CE, 0x008C, 0x7310, 0x3100, 0x8CCE,
        0x088C, 0x3110, 0x6666, 0x366C, 0x17E8, 0x0FF0, 0x718E, 0x399C,
        0xAAAA, 0xF0F0, 0x5A5A, 0x33CC, 0x3C3C, 0x55AA, 0x9696, 0xA55A,
        0x73CE, 0x13C8, 0x324C, 0x3BDC, 0x6996, 0xC33C, 0x9966, 0x0660,
        0x0272, 0x04E4, 0x4E40, 0x2720, 0xC936, 0x936C, 0x39C6, 0x639C,
        0x9336, 0x9CC6, 0x817E, 0xE718, 0xCCF0, 0x0FCC, 0x7744, 0xEE22,
    ];

    // 3-subset partition table: 2 bits per pixel encoding subset 0/1/2.
    private static readonly ulong[] s_partition3 =
    [
        0xAA685050UL, 0x6A5A5040UL, 0x5A5A4200UL, 0x5450A0A8UL,
        0xA5A50000UL, 0xA0A05050UL, 0x5555A0A0UL, 0x5A5A5050UL,
        0xAA550000UL, 0xAA555500UL, 0xAAAA5500UL, 0x90909090UL,
        0x94949494UL, 0xA4A4A4A4UL, 0xA9A59450UL, 0x2A0A4250UL,
        0xA5945040UL, 0x0A425054UL, 0xA5A5A500UL, 0x55A0A0A0UL,
        0xA8A85454UL, 0x6A6A4040UL, 0xA4A45000UL, 0x1A1A0500UL,
        0x0050A4A4UL, 0xAAA59090UL, 0x14696914UL, 0x69691400UL,
        0xA08585A0UL, 0xAA821414UL, 0x50A4A450UL, 0x6A5A0200UL,
        0xA9A58000UL, 0x5090A0A8UL, 0xA8A09050UL, 0x24242424UL,
        0x00AA5500UL, 0x24924924UL, 0x24499224UL, 0x50A50A50UL,
        0x500AA550UL, 0xAAAA4444UL, 0x66660000UL, 0xA5A0A5A0UL,
        0x50A050A0UL, 0x69286928UL, 0x44AAAA44UL, 0x66666600UL,
        0xAA444444UL, 0x54A854A8UL, 0x95809580UL, 0x96969600UL,
        0xA85454A8UL, 0x80959580UL, 0xAA141414UL, 0x96960000UL,
        0xAAAA1414UL, 0xA05050A0UL, 0xA0A5A5A0UL, 0x96000000UL,
        0x40804080UL, 0xA9A8A9A8UL, 0xAAAAAA44UL, 0x2A4A5254UL,
    ];

    // 2-subset anchor table: which pixel within subset 1 is the anchor.
    private static readonly byte[] s_anchor2 =
    [
        15, 15, 15, 15, 15, 15, 15, 15,
        15, 15, 15, 15, 15, 15, 15, 15,
        15,  2,  8,  2,  2,  8,  8, 15,
         2,  8,  2,  2,  8,  8,  2,  2,
        15, 15,  6,  8,  2,  8, 15, 15,
         2,  8,  2,  2,  2, 15, 15,  6,
         6,  2,  6,  8, 15, 15,  2,  2,
        15, 15, 15, 15, 15,  2,  2, 15,
    ];

    // 3-subset anchor table (subset 1).
    private static readonly byte[] s_anchor3a =
    [
         3,  3, 15, 15,  8,  3, 15, 15,
         8,  8,  6,  6,  6,  5,  3,  3,
         3,  3,  8, 15,  3,  3,  6, 10,
         5,  8,  8,  6,  8,  5, 15, 15,
         8, 15,  3,  5,  6, 10,  8, 15,
        15,  3, 15,  5, 15, 15, 15, 15,
         3, 15,  5,  5,  5,  8,  5, 10,
         5, 10,  8, 13, 15, 12,  3,  3,
    ];

    // 3-subset anchor table (subset 2).
    private static readonly byte[] s_anchor3b =
    [
        15,  8,  8,  3, 15, 15,  3,  8,
        15, 15, 15, 15, 15, 15, 15,  8,
        15,  8, 15,  3, 15,  8, 15,  8,
         3, 15,  6, 10, 15, 15, 10,  8,
        15,  3, 15, 10, 10,  8,  9, 10,
         6, 15,  8, 15,  3,  6,  6,  8,
        15,  3, 15, 15, 15, 15, 15, 15,
        15, 15, 15, 15,  3, 15, 15,  8,
    ];

    /// <summary>Compact LSB-first bit reader over a 128-bit BC7 block.</summary>
    private ref struct BitReader
    {
        private readonly ulong _lo;
        private readonly ulong _hi;
        private int _pos;

        public BitReader(ReadOnlySpan<byte> src)
        {
            _lo = System.Buffers.Binary.BinaryPrimitives.ReadUInt64LittleEndian(src[..8]);
            _hi = System.Buffers.Binary.BinaryPrimitives.ReadUInt64LittleEndian(src.Slice(8, 8));
            _pos = 0;
        }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public void Consume(int n) { _pos += n; }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public int Read(int n)
        {
            if (n <= 0) return 0;
            int p = _pos;
            _pos += n;
            int v = 0;
            for (int i = 0; i < n; i++)
            {
                int bp = p + i;
                ulong source = bp < 64 ? _lo : _hi;
                int shift = bp < 64 ? bp : bp - 64;
                int bit = (int)((source >> shift) & 1UL);
                v |= bit << i;
            }
            return v;
        }
    }
}
