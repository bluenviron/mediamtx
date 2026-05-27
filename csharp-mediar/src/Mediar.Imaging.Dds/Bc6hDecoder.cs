using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Dds;

/// <summary>
/// Full BC6H (BPTC half-float) decompressor producing top-down
/// <see cref="PixelFormat.Rgb96Float"/> pixel buffers. Implements all
/// 14 BC6H modes (single- and two-subset, transformed and untransformed)
/// from the Khronos Data Format Specification 1.4 Section 20.2.
/// </summary>
/// <remarks>
/// Mode numbers used internally follow the Khronos spec (Table 134):
/// modes 0, 1 are encoded in 2 bits; modes 2, 3, 6, 7, 10, 11, 14, 15,
/// 18, 22, 26, 30 in 5 bits. Modes 3, 7, 11, 15 use 1 subset; the rest
/// use 2 subsets. Reserved mode numbers (19, 23, 27, 31) decode to the
/// spec-mandated transparent black (0, 0, 0, 1).
///
/// Both signed (DXGI BC6H_SF16) and unsigned (DXGI BC6H_UF16) variants
/// are supported via a single <c>signed</c> flag. All interpolation is
/// performed in 16-bit integer space and reinterpreted as
/// <see cref="Half"/> on output, matching the spec's bit-exact decode.
/// </remarks>
internal static class Bc6hDecoder
{
    /// <summary>Decompresses a BC6H surface (UF16 / SF16 selected by <paramref name="signed"/>) into top-down Rgb96Float.</summary>
    public static byte[] DecodeBc6h(ReadOnlySpan<byte> src, int width, int height, bool signed)
    {
        int blocksX = (width + 3) / 4;
        int blocksY = (height + 3) / 4;
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
        ulong lo = BinaryPrimitives.ReadUInt64LittleEndian(src[..8]);
        ulong hi = BinaryPrimitives.ReadUInt64LittleEndian(src.Slice(8, 8));
        var bits = new BlockBits(lo, hi);

        int low2 = bits.Bits(0, 2);
        int mode = low2 < 2 ? low2 : (low2 | (bits.Bits(2, 3) << 2));

        // Reserved modes per spec § 20.2: 19, 23, 27, 31 decode to (0, 0, 0, 1).
        if (mode is 19 or 23 or 27 or 31)
        {
            outRgb.Clear();
            return;
        }

        var info = s_modeInfo[mode];
        Bc6hEndpoints ep = default;
        int partition = 0;

        switch (mode)
        {
            case 0:  ExtractMode0(bits, ref ep, out partition); break;
            case 1:  ExtractMode1(bits, ref ep, out partition); break;
            case 2:  ExtractMode2(bits, ref ep, out partition); break;
            case 3:  ExtractMode3(bits, ref ep); break;
            case 6:  ExtractMode6(bits, ref ep, out partition); break;
            case 7:  ExtractMode7(bits, ref ep); break;
            case 10: ExtractMode10(bits, ref ep, out partition); break;
            case 11: ExtractMode11(bits, ref ep); break;
            case 14: ExtractMode14(bits, ref ep, out partition); break;
            case 15: ExtractMode15(bits, ref ep); break;
            case 18: ExtractMode18(bits, ref ep, out partition); break;
            case 22: ExtractMode22(bits, ref ep, out partition); break;
            case 26: ExtractMode26(bits, ref ep, out partition); break;
            case 30: ExtractMode30(bits, ref ep, out partition); break;
        }

        // Convert raw endpoint bit-patterns into unquantized 16-bit interpolation values.
        TransformAndUnquantize(ref ep, info, signed);

        // Decode 16 index values and emit final per-pixel RGB.
        DecodePixels(bits, ep, info, partition, signed, outRgb);
    }

    // ─────────────────────────────────────────────────────────────────────
    // Per-mode endpoint extractors. Each follows the bit-layout in
    // Khronos KDF spec 1.4 Tables 135 / 136 / 137 ("Block descriptions
    // for BC6H block modes" and the lower/upper bit interpretation tables).
    // Bit positions are absolute (0..127) within the 128-bit block.
    // ─────────────────────────────────────────────────────────────────────

    private static void ExtractMode0(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 10);
        ep.G0 = b.Bits(15, 10);
        ep.B0 = b.Bits(25, 10);
        ep.R1 = b.Bits(35, 5);
        ep.G1 = b.Bits(45, 5);
        ep.B1 = b.Bits(55, 5);
        ep.R2 = b.Bits(65, 5);
        ep.R3 = b.Bits(71, 5);
        ep.G2 = b.Bits(41, 4) | (b.Bit(2) << 4);
        ep.G3 = b.Bits(51, 4) | (b.Bit(40) << 4);
        ep.B2 = b.Bits(61, 4) | (b.Bit(3) << 4);
        ep.B3 = b.Bit(50) | (b.Bit(60) << 1) | (b.Bit(70) << 2)
              | (b.Bit(76) << 3) | (b.Bit(4) << 4);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode1(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 7);
        ep.G0 = b.Bits(15, 7);
        ep.B0 = b.Bits(25, 7);
        ep.R1 = b.Bits(35, 6);
        ep.G1 = b.Bits(45, 6);
        ep.B1 = b.Bits(55, 6);
        ep.R2 = b.Bits(65, 6);
        ep.R3 = b.Bits(71, 6);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4) | (b.Bit(2) << 5);
        ep.G3 = b.Bits(51, 4) | (b.Bit(3) << 4)  | (b.Bit(4) << 5);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4) | (b.Bit(22) << 5);
        ep.B3 = b.Bit(12) | (b.Bit(13) << 1) | (b.Bit(23) << 2)
              | (b.Bit(32) << 3) | (b.Bit(34) << 4) | (b.Bit(33) << 5);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode2(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 10)  | (b.Bit(40) << 10);
        ep.G0 = b.Bits(15, 10) | (b.Bit(49) << 10);
        ep.B0 = b.Bits(25, 10) | (b.Bit(59) << 10);
        ep.R1 = b.Bits(35, 5);
        ep.G1 = b.Bits(45, 4);
        ep.B1 = b.Bits(55, 4);
        ep.R2 = b.Bits(65, 5);
        ep.R3 = b.Bits(71, 5);
        ep.G2 = b.Bits(41, 4);
        ep.G3 = b.Bits(51, 4);
        ep.B2 = b.Bits(61, 4);
        ep.B3 = b.Bit(50) | (b.Bit(60) << 1) | (b.Bit(70) << 2) | (b.Bit(76) << 3);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode3(BlockBits b, ref Bc6hEndpoints ep)
    {
        ep.R0 = b.Bits(5, 10);
        ep.G0 = b.Bits(15, 10);
        ep.B0 = b.Bits(25, 10);
        ep.R1 = b.Bits(35, 10);
        ep.G1 = b.Bits(45, 10);
        ep.B1 = b.Bits(55, 10);
    }

    private static void ExtractMode6(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 10)  | (b.Bit(39) << 10);
        ep.G0 = b.Bits(15, 10) | (b.Bit(50) << 10);
        ep.B0 = b.Bits(25, 10) | (b.Bit(59) << 10);
        ep.R1 = b.Bits(35, 4);
        ep.G1 = b.Bits(45, 5);
        ep.B1 = b.Bits(55, 4);
        ep.R2 = b.Bits(65, 4);
        ep.R3 = b.Bits(71, 4);
        ep.G2 = b.Bits(41, 4) | (b.Bit(75) << 4);
        ep.G3 = b.Bits(51, 4) | (b.Bit(40) << 4);
        ep.B2 = b.Bits(61, 4);
        ep.B3 = b.Bit(69) | (b.Bit(60) << 1) | (b.Bit(70) << 2) | (b.Bit(76) << 3);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode7(BlockBits b, ref Bc6hEndpoints ep)
    {
        ep.R0 = b.Bits(5, 10)  | (b.Bit(44) << 10);
        ep.G0 = b.Bits(15, 10) | (b.Bit(54) << 10);
        ep.B0 = b.Bits(25, 10) | (b.Bit(64) << 10);
        ep.R1 = b.Bits(35, 9);
        ep.G1 = b.Bits(45, 9);
        ep.B1 = b.Bits(55, 9);
    }

    private static void ExtractMode10(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 10)  | (b.Bit(39) << 10);
        ep.G0 = b.Bits(15, 10) | (b.Bit(49) << 10);
        ep.B0 = b.Bits(25, 10) | (b.Bit(60) << 10);
        ep.R1 = b.Bits(35, 4);
        ep.G1 = b.Bits(45, 4);
        ep.B1 = b.Bits(55, 5);
        ep.R2 = b.Bits(65, 4);
        ep.R3 = b.Bits(71, 4);
        ep.G2 = b.Bits(41, 4);
        ep.G3 = b.Bits(51, 4);
        ep.B2 = b.Bits(61, 4) | (b.Bit(40) << 4);
        ep.B3 = b.Bit(50) | (b.Bit(69) << 1) | (b.Bit(70) << 2)
              | (b.Bit(76) << 3) | (b.Bit(75) << 4);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode11(BlockBits b, ref Bc6hEndpoints ep)
    {
        // R0[10..11] / G0[10..11] / B0[10..11] are stored in reversed bit
        // order — first bit read goes to the higher (MSB) target bit.
        ep.R0 = b.Bits(5, 10)  | (b.Bit(44) << 10) | (b.Bit(43) << 11);
        ep.G0 = b.Bits(15, 10) | (b.Bit(54) << 10) | (b.Bit(53) << 11);
        ep.B0 = b.Bits(25, 10) | (b.Bit(64) << 10) | (b.Bit(63) << 11);
        ep.R1 = b.Bits(35, 8);
        ep.G1 = b.Bits(45, 8);
        ep.B1 = b.Bits(55, 8);
    }

    private static void ExtractMode14(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 9);
        ep.G0 = b.Bits(15, 9);
        ep.B0 = b.Bits(25, 9);
        ep.R1 = b.Bits(35, 5);
        ep.G1 = b.Bits(45, 5);
        ep.B1 = b.Bits(55, 5);
        ep.R2 = b.Bits(65, 5);
        ep.R3 = b.Bits(71, 5);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4);
        ep.G3 = b.Bits(51, 4) | (b.Bit(40) << 4);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4);
        ep.B3 = b.Bit(50) | (b.Bit(60) << 1) | (b.Bit(70) << 2)
              | (b.Bit(76) << 3) | (b.Bit(34) << 4);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode15(BlockBits b, ref Bc6hEndpoints ep)
    {
        // R0[10..15] / G0[10..15] / B0[10..15] are reversed — first bit
        // read goes to the higher target bit (bit 15 first, then 14, …).
        ep.R0 = b.Bits(5, 10)
              | (b.Bit(44) << 10) | (b.Bit(43) << 11) | (b.Bit(42) << 12)
              | (b.Bit(41) << 13) | (b.Bit(40) << 14) | (b.Bit(39) << 15);
        ep.G0 = b.Bits(15, 10)
              | (b.Bit(54) << 10) | (b.Bit(53) << 11) | (b.Bit(52) << 12)
              | (b.Bit(51) << 13) | (b.Bit(50) << 14) | (b.Bit(49) << 15);
        ep.B0 = b.Bits(25, 10)
              | (b.Bit(64) << 10) | (b.Bit(63) << 11) | (b.Bit(62) << 12)
              | (b.Bit(61) << 13) | (b.Bit(60) << 14) | (b.Bit(59) << 15);
        ep.R1 = b.Bits(35, 4);
        ep.G1 = b.Bits(45, 4);
        ep.B1 = b.Bits(55, 4);
    }

    private static void ExtractMode18(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 8);
        ep.G0 = b.Bits(15, 8);
        ep.B0 = b.Bits(25, 8);
        ep.R1 = b.Bits(35, 6);
        ep.G1 = b.Bits(45, 5);
        ep.B1 = b.Bits(55, 5);
        ep.R2 = b.Bits(65, 6);
        ep.R3 = b.Bits(71, 6);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4);
        ep.G3 = b.Bits(51, 4) | (b.Bit(13) << 4);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4);
        ep.B3 = b.Bit(50) | (b.Bit(60) << 1) | (b.Bit(23) << 2)
              | (b.Bit(33) << 3) | (b.Bit(34) << 4);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode22(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 8);
        ep.G0 = b.Bits(15, 8);
        ep.B0 = b.Bits(25, 8);
        ep.R1 = b.Bits(35, 5);
        ep.G1 = b.Bits(45, 6);
        ep.B1 = b.Bits(55, 5);
        ep.R2 = b.Bits(65, 5);
        ep.R3 = b.Bits(71, 5);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4) | (b.Bit(23) << 5);
        ep.G3 = b.Bits(51, 4) | (b.Bit(40) << 4) | (b.Bit(33) << 5);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4);
        ep.B3 = b.Bit(13) | (b.Bit(60) << 1) | (b.Bit(70) << 2)
              | (b.Bit(76) << 3) | (b.Bit(34) << 4);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode26(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 8);
        ep.G0 = b.Bits(15, 8);
        ep.B0 = b.Bits(25, 8);
        ep.R1 = b.Bits(35, 5);
        ep.G1 = b.Bits(45, 5);
        ep.B1 = b.Bits(55, 6);
        ep.R2 = b.Bits(65, 5);
        ep.R3 = b.Bits(71, 5);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4);
        ep.G3 = b.Bits(51, 4) | (b.Bit(40) << 4);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4) | (b.Bit(23) << 5);
        ep.B3 = b.Bit(50) | (b.Bit(13) << 1) | (b.Bit(70) << 2)
              | (b.Bit(76) << 3) | (b.Bit(34) << 4) | (b.Bit(33) << 5);
        partition = b.Bits(77, 5);
    }

    private static void ExtractMode30(BlockBits b, ref Bc6hEndpoints ep, out int partition)
    {
        ep.R0 = b.Bits(5, 6);
        ep.G0 = b.Bits(15, 6);
        ep.B0 = b.Bits(25, 6);
        ep.R1 = b.Bits(35, 6);
        ep.G1 = b.Bits(45, 6);
        ep.B1 = b.Bits(55, 6);
        ep.R2 = b.Bits(65, 6);
        ep.R3 = b.Bits(71, 6);
        ep.G2 = b.Bits(41, 4) | (b.Bit(24) << 4) | (b.Bit(21) << 5);
        ep.G3 = b.Bits(51, 4) | (b.Bit(11) << 4) | (b.Bit(31) << 5);
        ep.B2 = b.Bits(61, 4) | (b.Bit(14) << 4) | (b.Bit(22) << 5);
        ep.B3 = b.Bit(12) | (b.Bit(13) << 1) | (b.Bit(23) << 2)
              | (b.Bit(32) << 3) | (b.Bit(34) << 4) | (b.Bit(33) << 5);
        partition = b.Bits(77, 5);
    }

    // ─────────────────────────────────────────────────────────────────────
    // Endpoint transformation, sign-extension and unquantization.
    // Per spec § 20.2: E0 is sign-extended only if format is signed.
    // E1/E2/E3 are sign-extended if format is signed OR mode is transformed.
    // For transformed modes, E1/E2/E3 are stored as signed deltas relative
    // to E0; the wrapped sum modulo (1<<EPB) becomes the actual endpoint.
    // ─────────────────────────────────────────────────────────────────────

    private static void TransformAndUnquantize(ref Bc6hEndpoints ep, Bc6hModeInfo info, bool signed)
    {
        // Save raw E0 (unsigned EPB-bit value) for the transform addition.
        int r0raw = ep.R0, g0raw = ep.G0, b0raw = ep.B0;

        // E0: only sign-extended for signed format.
        ep.R0 = signed ? SignExtend(ep.R0, info.EpbR) : ep.R0;
        ep.G0 = signed ? SignExtend(ep.G0, info.EpbG) : ep.G0;
        ep.B0 = signed ? SignExtend(ep.B0, info.EpbB) : ep.B0;

        ep.R1 = ResolveSecondaryEndpoint(ep.R1, r0raw, info.EpbR, info.DeltaBitsR, info.Transformed, signed);
        ep.G1 = ResolveSecondaryEndpoint(ep.G1, g0raw, info.EpbG, info.DeltaBitsG, info.Transformed, signed);
        ep.B1 = ResolveSecondaryEndpoint(ep.B1, b0raw, info.EpbB, info.DeltaBitsB, info.Transformed, signed);

        if (info.TwoSubsets)
        {
            ep.R2 = ResolveSecondaryEndpoint(ep.R2, r0raw, info.EpbR, info.DeltaBitsR, info.Transformed, signed);
            ep.G2 = ResolveSecondaryEndpoint(ep.G2, g0raw, info.EpbG, info.DeltaBitsG, info.Transformed, signed);
            ep.B2 = ResolveSecondaryEndpoint(ep.B2, b0raw, info.EpbB, info.DeltaBitsB, info.Transformed, signed);
            ep.R3 = ResolveSecondaryEndpoint(ep.R3, r0raw, info.EpbR, info.DeltaBitsR, info.Transformed, signed);
            ep.G3 = ResolveSecondaryEndpoint(ep.G3, g0raw, info.EpbG, info.DeltaBitsG, info.Transformed, signed);
            ep.B3 = ResolveSecondaryEndpoint(ep.B3, b0raw, info.EpbB, info.DeltaBitsB, info.Transformed, signed);
        }

        // Now unquantize all endpoints to 16-bit integer interpolation space.
        ep.R0 = Unquantize(ep.R0, info.EpbR, signed);
        ep.G0 = Unquantize(ep.G0, info.EpbG, signed);
        ep.B0 = Unquantize(ep.B0, info.EpbB, signed);
        ep.R1 = Unquantize(ep.R1, info.EpbR, signed);
        ep.G1 = Unquantize(ep.G1, info.EpbG, signed);
        ep.B1 = Unquantize(ep.B1, info.EpbB, signed);
        if (info.TwoSubsets)
        {
            ep.R2 = Unquantize(ep.R2, info.EpbR, signed);
            ep.G2 = Unquantize(ep.G2, info.EpbG, signed);
            ep.B2 = Unquantize(ep.B2, info.EpbB, signed);
            ep.R3 = Unquantize(ep.R3, info.EpbR, signed);
            ep.G3 = Unquantize(ep.G3, info.EpbG, signed);
            ep.B3 = Unquantize(ep.B3, info.EpbB, signed);
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ResolveSecondaryEndpoint(int rawValue, int e0raw, int epb, int deltaBits, bool transformed, bool signed)
    {
        if (transformed)
        {
            // rawValue is a signed delta of width deltaBits; wrap-add to E0 modulo EPB.
            int delta = SignExtend(rawValue, deltaBits);
            int mask = (1 << epb) - 1;
            int combined = (e0raw + delta) & mask;
            return signed ? SignExtend(combined, epb) : combined;
        }
        // Untransformed: rawValue is already EPB-wide. Sign-extend only if signed format.
        return signed ? SignExtend(rawValue, epb) : rawValue;
    }

    // ─────────────────────────────────────────────────────────────────────
    // Pixel decode: read indices in y-major order, look up subset for
    // each pixel, interpolate endpoints, finalize to half-float bits.
    // ─────────────────────────────────────────────────────────────────────

    private static void DecodePixels(BlockBits b, in Bc6hEndpoints ep, Bc6hModeInfo info, int partition, bool signed, Span<float> outRgb)
    {
        ushort partitionMask = info.TwoSubsets ? s_partition2[partition] : (ushort)0;
        int anchor = info.TwoSubsets ? s_anchor2[partition] : -1;

        int indexPos = info.TwoSubsets ? 82 : 65;
        ReadOnlySpan<ushort> weights = info.TwoSubsets ? s_weights3 : s_weights4;
        int normalBits = info.TwoSubsets ? 3 : 4;

        for (int i = 0; i < 16; i++)
        {
            int bitsToRead = (i == 0 || i == anchor) ? normalBits - 1 : normalBits;
            int index = b.Bits(indexPos, bitsToRead);
            indexPos += bitsToRead;

            int subset = info.TwoSubsets ? ((partitionMask >> i) & 1) : 0;

            int e0r, e0g, e0b, e1r, e1g, e1b;
            if (subset == 0)
            {
                e0r = ep.R0; e0g = ep.G0; e0b = ep.B0;
                e1r = ep.R1; e1g = ep.G1; e1b = ep.B1;
            }
            else
            {
                e0r = ep.R2; e0g = ep.G2; e0b = ep.B2;
                e1r = ep.R3; e1g = ep.G3; e1b = ep.B3;
            }

            int w = weights[index];
            int r = Interpolate(e0r, e1r, w);
            int g = Interpolate(e0g, e1g, w);
            int bl = Interpolate(e0b, e1b, w);

            outRgb[i * 3 + 0] = Finalize(r, signed);
            outRgb[i * 3 + 1] = Finalize(g, signed);
            outRgb[i * 3 + 2] = Finalize(bl, signed);
        }
    }

    // ─────────────────────────────────────────────────────────────────────
    // Math primitives.
    // ─────────────────────────────────────────────────────────────────────

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Interpolate(int e0, int e1, int weight) =>
        ((64 - weight) * e0 + weight * e1 + 32) >> 6;

    /// <summary>BC6H endpoint unquantization per spec § 20.2 pseudocode.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Unquantize(int val, int epb, bool signed)
    {
        if (signed)
        {
            if (epb >= 16) return val;
            int sign = 0;
            int abs = val;
            if (abs < 0) { sign = 1; abs = -abs; }
            int unq;
            if (abs == 0) unq = 0;
            else if (abs >= ((1 << (epb - 1)) - 1)) unq = 0x7FFF;
            else unq = ((abs << 15) + 0x4000) >> (epb - 1);
            return sign != 0 ? -unq : unq;
        }
        else
        {
            if (epb >= 15) return val;
            if (val == 0) return 0;
            if (val == ((1 << epb) - 1)) return 0xFFFF;
            return ((val << 15) + 0x4000) >> (epb - 1);
        }
    }

    /// <summary>Convert the interpolated 16-bit integer to a half-float reinterpreted as float.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static float Finalize(int value, bool signed)
    {
        int bits;
        if (signed)
        {
            if (value < 0)
            {
                int abs = -value;
                bits = ((abs * 31) >> 5) | 0x8000;
            }
            else
            {
                bits = (value * 31) >> 5;
            }
        }
        else
        {
            // Clamp to non-negative — interpolation can briefly produce a
            // small negative value on rounding boundaries that the spec
            // formula handles via integer truncation. (i*31)>>6 still
            // produces a meaningful half bit-pattern for slightly negative
            // values, but unsigned half floats by definition have no sign
            // bit so we clamp first to match the format contract.
            int v = value < 0 ? 0 : value;
            bits = (v * 31) >> 6;
        }
        return (float)BitConverter.UInt16BitsToHalf((ushort)bits);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int SignExtend(int val, int bits)
    {
        int signMask = 1 << (bits - 1);
        if ((val & signMask) != 0) val -= (1 << bits);
        return val;
    }

    // ─────────────────────────────────────────────────────────────────────
    // Tables and types.
    // ─────────────────────────────────────────────────────────────────────

    private struct Bc6hEndpoints
    {
        public int R0, G0, B0;
        public int R1, G1, B1;
        public int R2, G2, B2;
        public int R3, G3, B3;
    }

    private readonly record struct Bc6hModeInfo(
        byte EpbR, byte EpbG, byte EpbB,
        byte DeltaBitsR, byte DeltaBitsG, byte DeltaBitsB,
        bool Transformed,
        bool TwoSubsets);

    private static readonly Bc6hModeInfo[] s_modeInfo = BuildModeInfoTable();

    private static Bc6hModeInfo[] BuildModeInfoTable()
    {
        var t = new Bc6hModeInfo[32];
        // 2-subset modes (Table 134):
        t[0]  = new(10, 10, 10, 5, 5, 5, Transformed: true,  TwoSubsets: true);
        t[1]  = new(7,  7,  7,  6, 6, 6, Transformed: true,  TwoSubsets: true);
        t[2]  = new(11, 11, 11, 5, 4, 4, Transformed: true,  TwoSubsets: true);
        t[6]  = new(11, 11, 11, 4, 5, 4, Transformed: true,  TwoSubsets: true);
        t[10] = new(11, 11, 11, 4, 4, 5, Transformed: true,  TwoSubsets: true);
        t[14] = new(9,  9,  9,  5, 5, 5, Transformed: true,  TwoSubsets: true);
        t[18] = new(8,  8,  8,  6, 5, 5, Transformed: true,  TwoSubsets: true);
        t[22] = new(8,  8,  8,  5, 6, 5, Transformed: true,  TwoSubsets: true);
        t[26] = new(8,  8,  8,  5, 5, 6, Transformed: true,  TwoSubsets: true);
        t[30] = new(6,  6,  6,  0, 0, 0, Transformed: false, TwoSubsets: true);
        // 1-subset modes:
        t[3]  = new(10, 10, 10, 0, 0, 0, Transformed: false, TwoSubsets: false);
        t[7]  = new(11, 11, 11, 9, 9, 9, Transformed: true,  TwoSubsets: false);
        t[11] = new(12, 12, 12, 8, 8, 8, Transformed: true,  TwoSubsets: false);
        t[15] = new(16, 16, 16, 4, 4, 4, Transformed: true,  TwoSubsets: false);
        return t;
    }

    private static readonly ushort[] s_weights3 = [0, 9, 18, 27, 37, 46, 55, 64];
    private static readonly ushort[] s_weights4 = [0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64];

    /// <summary>
    /// 2-subset BPTC partition table (Table 127 of the Khronos DF spec 1.4).
    /// For each of 64 partition patterns, bit <i>i</i> (LSB-first) indicates
    /// which subset (0 or 1) pixel <i>i</i> belongs to in the 4×4 block.
    /// </summary>
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

    /// <summary>
    /// 2-subset BPTC anchor index table (Table 131 of the Khronos DF spec 1.4):
    /// the pixel index that is the anchor (1 fewer bit) in the secondary
    /// subset for each partition pattern.
    /// </summary>
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

    /// <summary>Random-access bit reader over a 128-bit BC6H block, LSB-first.</summary>
    private readonly ref struct BlockBits
    {
        private readonly ulong _lo;
        private readonly ulong _hi;

        public BlockBits(ulong lo, ulong hi) { _lo = lo; _hi = hi; }

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public int Bit(int pos) =>
            (int)(((pos < 64 ? _lo : _hi) >> (pos & 63)) & 1UL);

        [MethodImpl(MethodImplOptions.AggressiveInlining)]
        public int Bits(int pos, int count)
        {
            int v = 0;
            for (int i = 0; i < count; i++)
            {
                v |= Bit(pos + i) << i;
            }
            return v;
        }
    }
}
