using System.Buffers.Binary;

namespace Mediar.Codecs.Astc;

/// <summary>
/// ASTC (Adaptive Scalable Texture Compression) block decoder per Khronos
/// KDF 1.4 section 19. Currently implements the void-extent (constant-
/// colour) fast path — the most common ASTC block type for skybox edges,
/// atlas padding, and any region that stores a single colour. Non-void-
/// extent blocks (ordinary partitioned colour-endpoint blocks) are
/// recognised but not yet decoded; callers can use <see cref="IsVoidExtent"/>
/// to gate the fast path.
/// </summary>
/// <remarks>
/// All ASTC blocks are exactly 16 bytes (128 bits). The block bit layout
/// is little-endian within each byte; bit 0 of byte 0 is the LSB of the
/// 128-bit value. Void-extent blocks encode a single RGBA colour as either
/// four 16-bit UNORM values (LDR) or four IEEE half-float values (HDR),
/// plus an extent rectangle that this implementation does not use (the
/// constant colour applies to every texel of the block).
/// </remarks>
public static class AstcDecoder
{
    /// <summary>
    /// Return the (X, Y) pixel footprint of an ASTC block for the given format.
    /// Throws <see cref="ArgumentOutOfRangeException"/> for <see cref="AstcFormat.None"/>.
    /// </summary>
    public static (int X, int Y) BlockDimensions(AstcFormat format) => format switch
    {
        AstcFormat.Astc4x4Unorm or AstcFormat.Astc4x4Srgb => (4, 4),
        AstcFormat.Astc5x4Unorm or AstcFormat.Astc5x4Srgb => (5, 4),
        AstcFormat.Astc5x5Unorm or AstcFormat.Astc5x5Srgb => (5, 5),
        AstcFormat.Astc6x5Unorm or AstcFormat.Astc6x5Srgb => (6, 5),
        AstcFormat.Astc6x6Unorm or AstcFormat.Astc6x6Srgb => (6, 6),
        AstcFormat.Astc8x5Unorm or AstcFormat.Astc8x5Srgb => (8, 5),
        AstcFormat.Astc8x6Unorm or AstcFormat.Astc8x6Srgb => (8, 6),
        AstcFormat.Astc8x8Unorm or AstcFormat.Astc8x8Srgb => (8, 8),
        AstcFormat.Astc10x5Unorm or AstcFormat.Astc10x5Srgb => (10, 5),
        AstcFormat.Astc10x6Unorm or AstcFormat.Astc10x6Srgb => (10, 6),
        AstcFormat.Astc10x8Unorm or AstcFormat.Astc10x8Srgb => (10, 8),
        AstcFormat.Astc10x10Unorm or AstcFormat.Astc10x10Srgb => (10, 10),
        AstcFormat.Astc12x10Unorm or AstcFormat.Astc12x10Srgb => (12, 10),
        AstcFormat.Astc12x12Unorm or AstcFormat.Astc12x12Srgb => (12, 12),
        _ => throw new ArgumentOutOfRangeException(nameof(format)),
    };

    /// <summary>
    /// Every ASTC block is exactly 16 bytes (128 bits) regardless of footprint.
    /// </summary>
    public static int BytesPerBlock(AstcFormat format) => format == AstcFormat.None ? 0 : 16;

    /// <summary>
    /// Returns true if the 16-byte block is a void-extent (constant-colour)
    /// block per KDF 1.4 §19. A void-extent block has its low 9 bits equal
    /// to <c>0x1FC</c> (binary <c>111111100</c>); bit 9 selects LDR (0) vs
    /// HDR (1).
    /// </summary>
    public static bool IsVoidExtent(ReadOnlySpan<byte> block)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(block.Length, 16, nameof(block));
        // Read bits [10:0] of the 128-bit block (block mode field).
        // We only need the low 9 bits for void-extent detection.
        int b0 = block[0];
        int b1 = block[1];
        int blockMode9 = (b0 | (b1 << 8)) & 0x1FF;
        return blockMode9 == 0x1FC;
    }

    /// <summary>
    /// Returns true if a void-extent block is HDR (half-float colour)
    /// rather than LDR (UNORM16 colour). The result is unspecified when
    /// <see cref="IsVoidExtent"/> would return false.
    /// </summary>
    public static bool IsVoidExtentHdr(ReadOnlySpan<byte> block)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(block.Length, 16, nameof(block));
        // Bit 9 (mask 0x200) selects U16 vs F16.
        int b0 = block[0];
        int b1 = block[1];
        return ((b0 | (b1 << 8)) & 0x200) != 0;
    }

    /// <summary>
    /// Decode a single 16-byte ASTC block into a per-pixel Rgba32 buffer of
    /// <c>blockX * blockY * 4</c> bytes, where <c>(blockX, blockY)</c> is
    /// <see cref="BlockDimensions"/> for the given format. Only void-extent
    /// blocks are currently decoded; non-void-extent blocks leave the
    /// destination untouched and return false.
    /// </summary>
    /// <remarks>
    /// HDR (half-float) void-extent blocks are tone-mapped by simple
    /// clamping into [0, 1] then scaled to 8-bit — callers needing
    /// preserved HDR range should use <see cref="TryDecodeVoidExtentHdr"/>.
    /// </remarks>
    public static bool TryDecodeBlock(ReadOnlySpan<byte> block, AstcFormat format, Span<byte> rgba)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(block.Length, 16, nameof(block));
        if (format == AstcFormat.None) return false;
        var (bx, by) = BlockDimensions(format);
        int needed = bx * by * 4;
        ArgumentOutOfRangeException.ThrowIfLessThan(rgba.Length, needed, nameof(rgba));

        if (!IsVoidExtent(block)) return false;
        if (!ValidateVoidExtent(block, twoDimensional: true)) return false;

        bool hdr = IsVoidExtentHdr(block);
        byte r, g, b, a;
        if (hdr)
        {
            ushort r16 = BinaryPrimitives.ReadUInt16LittleEndian(block[8..]);
            ushort g16 = BinaryPrimitives.ReadUInt16LittleEndian(block[10..]);
            ushort b16 = BinaryPrimitives.ReadUInt16LittleEndian(block[12..]);
            ushort a16 = BinaryPrimitives.ReadUInt16LittleEndian(block[14..]);
            r = HalfToByteClamped(r16);
            g = HalfToByteClamped(g16);
            b = HalfToByteClamped(b16);
            a = HalfToByteClamped(a16);
        }
        else
        {
            // UNORM16 -> top 8 bits = display byte
            r = (byte)(BinaryPrimitives.ReadUInt16LittleEndian(block[8..]) >> 8);
            g = (byte)(BinaryPrimitives.ReadUInt16LittleEndian(block[10..]) >> 8);
            b = (byte)(BinaryPrimitives.ReadUInt16LittleEndian(block[12..]) >> 8);
            a = (byte)(BinaryPrimitives.ReadUInt16LittleEndian(block[14..]) >> 8);
        }

        for (int p = 0; p < bx * by; p++)
        {
            int o = p * 4;
            rgba[o + 0] = r;
            rgba[o + 1] = g;
            rgba[o + 2] = b;
            rgba[o + 3] = a;
        }
        return true;
    }

    /// <summary>
    /// Decode a single void-extent block into 4 IEEE half-float values
    /// (R/G/B/A). Returns false for non-void-extent blocks. Each ushort
    /// element is the IEEE 754 binary16 bit pattern of one channel.
    /// </summary>
    /// <remarks>
    /// For LDR void-extents, the stored UNORM16 channel value is converted
    /// to its half-float bit pattern via <c>float -&gt; Half</c> with the
    /// linear scale c16 / 65535.
    /// </remarks>
    public static bool TryDecodeVoidExtentHdr(ReadOnlySpan<byte> block, Span<ushort> rgba4)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(block.Length, 16, nameof(block));
        ArgumentOutOfRangeException.ThrowIfLessThan(rgba4.Length, 4, nameof(rgba4));

        if (!IsVoidExtent(block)) return false;
        if (!ValidateVoidExtent(block, twoDimensional: true)) return false;

        bool hdr = IsVoidExtentHdr(block);
        if (hdr)
        {
            rgba4[0] = BinaryPrimitives.ReadUInt16LittleEndian(block[8..]);
            rgba4[1] = BinaryPrimitives.ReadUInt16LittleEndian(block[10..]);
            rgba4[2] = BinaryPrimitives.ReadUInt16LittleEndian(block[12..]);
            rgba4[3] = BinaryPrimitives.ReadUInt16LittleEndian(block[14..]);
        }
        else
        {
            ushort r16 = BinaryPrimitives.ReadUInt16LittleEndian(block[8..]);
            ushort g16 = BinaryPrimitives.ReadUInt16LittleEndian(block[10..]);
            ushort b16 = BinaryPrimitives.ReadUInt16LittleEndian(block[12..]);
            ushort a16 = BinaryPrimitives.ReadUInt16LittleEndian(block[14..]);
            rgba4[0] = HalfFromUnorm16(r16);
            rgba4[1] = HalfFromUnorm16(g16);
            rgba4[2] = HalfFromUnorm16(b16);
            rgba4[3] = HalfFromUnorm16(a16);
        }
        return true;
    }

    /// <summary>
    /// Decode a full ASTC payload (a packed grid of 16-byte blocks) into
    /// <paramref name="rgba"/>, an Rgba32 framebuffer of
    /// <c>width * height * 4</c> bytes. Only void-extent blocks are
    /// decoded; any non-void-extent block leaves its corresponding output
    /// region as transparent black (0, 0, 0, 0) and increments
    /// <paramref name="undecodedBlocks"/>. Returns the number of blocks
    /// successfully decoded.
    /// </summary>
    public static int DecodeImage(
        ReadOnlySpan<byte> payload,
        AstcFormat format,
        int width,
        int height,
        Span<byte> rgba,
        out int undecodedBlocks)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        if (format == AstcFormat.None) throw new ArgumentOutOfRangeException(nameof(format));

        var (bx, by) = BlockDimensions(format);
        int blocksX = (width + bx - 1) / bx;
        int blocksY = (height + by - 1) / by;
        int totalBlocks = blocksX * blocksY;
        ArgumentOutOfRangeException.ThrowIfLessThan(payload.Length, totalBlocks * 16, nameof(payload));
        ArgumentOutOfRangeException.ThrowIfLessThan(rgba.Length, width * height * 4, nameof(rgba));

        // Wipe the destination so non-void-extent blocks contribute zeros.
        rgba[..(width * height * 4)].Clear();

        Span<byte> blockOut = stackalloc byte[12 * 12 * 4]; // largest footprint
        int decoded = 0;
        int skipped = 0;

        for (int by_idx = 0; by_idx < blocksY; by_idx++)
        {
            for (int bx_idx = 0; bx_idx < blocksX; bx_idx++)
            {
                int blockIndex = by_idx * blocksX + bx_idx;
                var block = payload.Slice(blockIndex * 16, 16);
                int bw = Math.Min(bx, width - bx_idx * bx);
                int bh = Math.Min(by, height - by_idx * by);
                int blockOutLen = bx * by * 4;

                if (TryDecodeBlock(block, format, blockOut[..blockOutLen]))
                {
                    for (int py = 0; py < bh; py++)
                    {
                        int srcRow = py * bx * 4;
                        int dstRow = ((by_idx * by + py) * width + bx_idx * bx) * 4;
                        blockOut.Slice(srcRow, bw * 4).CopyTo(rgba[dstRow..]);
                    }
                    decoded++;
                }
                else
                {
                    skipped++;
                }
            }
        }

        undecodedBlocks = skipped;
        return decoded;
    }

    private static bool ValidateVoidExtent(ReadOnlySpan<byte> block, bool twoDimensional)
    {
        // Bits [11:10] must be 0b11 ("reserved bits") for a 2D void-extent.
        // We don't currently expose 3D, but we accept either reading.
        int b1 = block[1];
        int rsv = (b1 >> 2) & 0x3;
        if (twoDimensional && rsv != 3) return false;

        // Extent fields: vx_low_s @12 (13 bits), vx_high_s @25, vx_low_t @38, vx_high_t @51.
        int vxLowS = ReadBits(block, 12, 13);
        int vxHighS = ReadBits(block, 25, 13);
        int vxLowT = ReadBits(block, 38, 13);
        int vxHighT = ReadBits(block, 51, 13);
        bool allOnes = vxLowS == 0x1FFF && vxHighS == 0x1FFF && vxLowT == 0x1FFF && vxHighT == 0x1FFF;
        if ((vxLowS >= vxHighS || vxLowT >= vxHighT) && !allOnes) return false;
        return true;
    }

    private static int ReadBits(ReadOnlySpan<byte> data, int bitOffset, int bitCount)
    {
        // Read up to 16 bits at an arbitrary bit offset, LSB-first within bytes.
        int result = 0;
        int produced = 0;
        while (produced < bitCount)
        {
            int byteIdx = bitOffset >> 3;
            int bitInByte = bitOffset & 7;
            int avail = 8 - bitInByte;
            int take = Math.Min(avail, bitCount - produced);
            int chunk = (data[byteIdx] >> bitInByte) & ((1 << take) - 1);
            result |= chunk << produced;
            produced += take;
            bitOffset += take;
        }
        return result;
    }

    private static byte HalfToByteClamped(ushort h16)
    {
        // Treat NaN as 0, clamp to [0, 1], scale.
        float f = (float)BitConverter.UInt16BitsToHalf(h16);
        if (float.IsNaN(f) || f <= 0f) return 0;
        if (f >= 1f) return 255;
        return (byte)(f * 255f + 0.5f);
    }

    private static ushort HalfFromUnorm16(ushort c16)
    {
        // Convert UNORM16 channel to half-float bit pattern.
        float f = c16 / 65535f;
        return BitConverter.HalfToUInt16Bits((Half)f);
    }
}
