using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Dds;

/// <summary>
/// Decompresses BC6H (BPTC half-float) block-compressed surfaces into
/// top-down <see cref="PixelFormat.Rgb96Float"/> pixel buffers. BC6H stores
/// each 4×4 block in 16 bytes using one of 14 modes; this decoder supports
/// the two pure 1-subset modes (11 and 14) — the modes most commonly emitted
/// by typical HDR encoders. Partitioned modes (1-10, 12-13) throw a clear
/// <see cref="NotSupportedException"/> so callers can fall back gracefully.
/// </summary>
/// <remarks>
/// References: Microsoft DXGI BC6H_UF16 / BC6H_SF16 specification and the
/// Khronos KTX 2.0 BPTC float format pages. All interpolation is performed
/// in 16-bit integer space and reinterpreted as <see cref="Half"/> on output.
/// </remarks>
internal static class Bc6hDecoder
{
    /// <summary>Decompresses a BC6H surface (UF16 / SF16 selected by <paramref name="signed"/>) into top-down Rgb96Float.</summary>
    public static byte[] DecodeBc6h(ReadOnlySpan<byte> src, int width, int height, bool signed)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
        // 3 floats × 4 bytes = 12 bytes per pixel.
        var output = new byte[width * height * 12];
        Span<float> block = stackalloc float[16 * 3];

        int srcOff = 0;
        for (int by = 0; by < blocksY; by++)
        {
            for (int bx = 0; bx < blocksX; bx++)
            {
                DecodeBlock(src.Slice(srcOff, 16), block, signed);
                for (int py = 0; py < 4; py++)
                {
                    int gy = by * 4 + py;
                    if (gy >= height) break;
                    for (int px = 0; px < 4; px++)
                    {
                        int gx = bx * 4 + px;
                        if (gx >= width) break;
                        int srcIdx = (py * 4 + px) * 3;
                        int dstIdx = (gy * width + gx) * 12;
                        BinaryPrimitives.WriteSingleLittleEndian(output.AsSpan(dstIdx + 0, 4), block[srcIdx + 0]);
                        BinaryPrimitives.WriteSingleLittleEndian(output.AsSpan(dstIdx + 4, 4), block[srcIdx + 1]);
                        BinaryPrimitives.WriteSingleLittleEndian(output.AsSpan(dstIdx + 8, 4), block[srcIdx + 2]);
                    }
                }
                srcOff += 16;
            }
        }
        return output;
    }

    private static void DecodeBlock(ReadOnlySpan<byte> src, Span<float> outRgb, bool signed)
    {
        var reader = new BitReader(src);
        int modePrefix = reader.Read(2);
        int modeIdx;
        if (modePrefix < 2)
        {
            modeIdx = modePrefix + 1; // mode 1 or 2 (2-subset, transformed)
        }
        else
        {
            int upper3 = reader.Read(3);
            modeIdx = LookupFiveBitMode(modePrefix | (upper3 << 2));
        }

        switch (modeIdx)
        {
            case 11:
                DecodeMode11(ref reader, outRgb, signed);
                return;
            case 14:
                DecodeMode14(ref reader, outRgb, signed);
                return;
            case 0:
            case -1:
                // Reserved / illegal mode → transparent black per spec recommendation.
                outRgb.Clear();
                return;
            default:
                throw new NotSupportedException(
                    $"BC6H mode {modeIdx} (2-subset / transformed) is not implemented in this Mediar release. " +
                    "Only the 1-subset modes 11 and 14 are currently supported.");
        }
    }

    /// <summary>Mode 11: 1 subset, no transform, 10-bit endpoint components, 4-bit indices.</summary>
    private static void DecodeMode11(ref BitReader reader, Span<float> outRgb, bool signed)
    {
        int rw = reader.Read(10);
        int gw = reader.Read(10);
        int bw = reader.Read(10);
        int rx = reader.Read(10);
        int gx = reader.Read(10);
        int bx = reader.Read(10);

        // Mode 11 endpoints are 10-bit. SF16 sign-extends them; UF16 treats as unsigned.
        int e0r = UnquantizeEndpoint(SignExtendIfSigned(rw, 10, signed), 10, signed);
        int e0g = UnquantizeEndpoint(SignExtendIfSigned(gw, 10, signed), 10, signed);
        int e0b = UnquantizeEndpoint(SignExtendIfSigned(bw, 10, signed), 10, signed);
        int e1r = UnquantizeEndpoint(SignExtendIfSigned(rx, 10, signed), 10, signed);
        int e1g = UnquantizeEndpoint(SignExtendIfSigned(gx, 10, signed), 10, signed);
        int e1b = UnquantizeEndpoint(SignExtendIfSigned(bx, 10, signed), 10, signed);

        // 16 × 4-bit indices, pixel 0 uses 3 bits (anchor).
        Span<int> indices = stackalloc int[16];
        for (int i = 0; i < 16; i++)
        {
            indices[i] = reader.Read(i == 0 ? 3 : 4);
        }

        WritePixels(outRgb, e0r, e0g, e0b, e1r, e1g, e1b, indices, 4, signed);
    }

    /// <summary>Mode 14: 1 subset, transformed, 16-bit base + 4-bit deltas per channel, 4-bit indices.</summary>
    private static void DecodeMode14(ref BitReader reader, Span<float> outRgb, bool signed)
    {
        int rw = reader.Read(16);
        int gw = reader.Read(16);
        int bw = reader.Read(16);
        int drx = reader.Read(4);
        int dgx = reader.Read(4);
        int dbx = reader.Read(4);

        // Base endpoint precision is 16 bits; UF16 treats unsigned, SF16 sign-extends.
        int e0r = SignExtendIfSigned(rw, 16, signed);
        int e0g = SignExtendIfSigned(gw, 16, signed);
        int e0b = SignExtendIfSigned(bw, 16, signed);

        // Deltas are always signed (4-bit two's complement → sign-extend to int).
        int sdrx = SignExtend(drx, 4);
        int sdgx = SignExtend(dgx, 4);
        int sdbx = SignExtend(dbx, 4);

        int e1r = ApplyDelta(e0r, sdrx, 16, signed);
        int e1g = ApplyDelta(e0g, sdgx, 16, signed);
        int e1b = ApplyDelta(e0b, sdbx, 16, signed);

        // For mode 14 the endpoints are already 16-bit; unquantize is a no-op (prec >= 15 case).
        e0r = UnquantizeEndpoint(e0r, 16, signed);
        e0g = UnquantizeEndpoint(e0g, 16, signed);
        e0b = UnquantizeEndpoint(e0b, 16, signed);
        e1r = UnquantizeEndpoint(e1r, 16, signed);
        e1g = UnquantizeEndpoint(e1g, 16, signed);
        e1b = UnquantizeEndpoint(e1b, 16, signed);

        Span<int> indices = stackalloc int[16];
        for (int i = 0; i < 16; i++)
        {
            indices[i] = reader.Read(i == 0 ? 3 : 4);
        }

        WritePixels(outRgb, e0r, e0g, e0b, e1r, e1g, e1b, indices, 4, signed);
    }

    private static void WritePixels(
        Span<float> outRgb,
        int e0r, int e0g, int e0b,
        int e1r, int e1g, int e1b,
        ReadOnlySpan<int> indices, int indexBits, bool signed)
    {
        ReadOnlySpan<ushort> weights = indexBits switch
        {
            3 => s_weights3,
            4 => s_weights4,
            _ => s_weights4,
        };

        for (int i = 0; i < 16; i++)
        {
            int w = weights[indices[i]];
            int r = Interpolate(e0r, e1r, w);
            int g = Interpolate(e0g, e1g, w);
            int b = Interpolate(e0b, e1b, w);

            outRgb[i * 3 + 0] = FinalizeToFloat(r, signed);
            outRgb[i * 3 + 1] = FinalizeToFloat(g, signed);
            outRgb[i * 3 + 2] = FinalizeToFloat(b, signed);
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Interpolate(int e0, int e1, int weight)
    {
        return ((64 - weight) * e0 + weight * e1 + 32) >> 6;
    }

    /// <summary>Convert a BC6H 16-bit interpolated value to a 32-bit float.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static float FinalizeToFloat(int value, bool signed)
    {
        if (signed)
        {
            // Apply BC6H signed finalize: scale by 31/32 then reinterpret as half.
            int finalVal = value < 0
                ? -((-value * 31) >> 5)
                : (value * 31) >> 5;
            finalVal = Math.Clamp(finalVal, -0x7BFF, 0x7BFF);
            return (float)BitConverter.Int16BitsToHalf((short)finalVal);
        }
        else
        {
            // Apply BC6H unsigned finalize: scale by 31/64 (always non-negative).
            int finalVal = (value * 31) >> 6;
            if (finalVal < 0) finalVal = 0;
            if (finalVal > 0x7BFF) finalVal = 0x7BFF;
            return (float)BitConverter.UInt16BitsToHalf((ushort)finalVal);
        }
    }

    /// <summary>BC6H endpoint unquantization (extends to 16-bit half-float-encoded value).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int UnquantizeEndpoint(int val, int prec, bool signed)
    {
        if (signed)
        {
            if (prec >= 16) return val;
            int sign = 0;
            int abs = val;
            if (abs < 0) { sign = 1; abs = -abs; }
            int unq;
            if (abs == 0) unq = 0;
            else if (abs >= ((1 << (prec - 1)) - 1)) unq = 0x7FFF;
            else unq = ((abs << 15) + 0x4000) >> (prec - 1);
            return sign != 0 ? -unq : unq;
        }
        else
        {
            if (prec >= 15) return val;
            if (val == 0) return 0;
            if (val == ((1 << prec) - 1)) return 0xFFFF;
            return ((val << 16) + 0x8000) >> prec;
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int SignExtendIfSigned(int val, int bits, bool signed) =>
        signed ? SignExtend(val, bits) : val;

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int SignExtend(int val, int bits)
    {
        int signMask = 1 << (bits - 1);
        if ((val & signMask) != 0) val -= (1 << bits);
        return val;
    }

    /// <summary>Add a delta to a base endpoint, then clip to the precision range.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ApplyDelta(int baseVal, int delta, int prec, bool signed)
    {
        int result = baseVal + delta;
        if (signed)
        {
            int max = (1 << (prec - 1)) - 1;
            int min = -(1 << (prec - 1));
            return Math.Clamp(result, min, max);
        }
        else
        {
            int mask = (1 << prec) - 1;
            return result & mask;
        }
    }

    private static int LookupFiveBitMode(int value5)
    {
        // Mapping from 5-bit mode bits (LSB-first) → 1-based BC6H mode number.
        // bits 0,1 are always 1,1 for 5-bit modes (since 2-bit prefix selected 5-bit mode).
        // Microsoft DXGI spec bit patterns (MSB-first textual) → 5-bit LSB-first value:
        //  00010 → 2 → mode 3      00011 → 3 → mode 11
        //  00110 → 6 → mode 4      00111 → 7 → mode 12
        //  01010 → 10 → mode 5     01011 → 11 → mode 13
        //  01110 → 14 → mode 6     01111 → 15 → mode 14
        //  10010 → 18 → mode 7
        //  10110 → 22 → mode 8
        //  11010 → 26 → mode 9
        //  11110 → 30 → mode 10
        return value5 switch
        {
            2 => 3,
            6 => 4,
            10 => 5,
            14 => 6,
            18 => 7,
            22 => 8,
            26 => 9,
            30 => 10,
            3 => 11,
            7 => 12,
            11 => 13,
            15 => 14,
            _ => -1, // reserved / illegal
        };
    }

    private static readonly ushort[] s_weights3 = [0, 9, 18, 27, 37, 46, 55, 64];
    private static readonly ushort[] s_weights4 = [0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64];

    /// <summary>LSB-first bit reader over a 128-bit BC6H block.</summary>
    private ref struct BitReader
    {
        private readonly ulong _lo;
        private readonly ulong _hi;
        private int _pos;

        public BitReader(ReadOnlySpan<byte> src)
        {
            _lo = BinaryPrimitives.ReadUInt64LittleEndian(src[..8]);
            _hi = BinaryPrimitives.ReadUInt64LittleEndian(src.Slice(8, 8));
            _pos = 0;
        }

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
