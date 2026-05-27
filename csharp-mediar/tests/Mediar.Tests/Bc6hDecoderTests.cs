using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the BC6H (BPTC half-float) decoder. Mode numbers follow the
/// Khronos KDF Specification 1.4 § 20.2 (Table 134):
///
///   • Mode 3  — 1 subset, untransformed, 10-bit endpoints (≡ DXGI "Mode 11")
///   • Mode 15 — 1 subset, transformed, 16-bit endpoints + 4-bit deltas (≡ DXGI "Mode 14")
///   • Mode 0  — 2 subsets, transformed, 10-bit endpoints + 5-bit deltas
///   • Mode 30 — 2 subsets, untransformed, 6-bit endpoints
///
/// The hand-crafted blocks exercise all four representative modes plus
/// both half-float pipelines (UF16 unsigned, SF16 signed).
/// </summary>
public sealed class Bc6hDecoderTests
{
    /// <summary>LSB-first bit packer matching the format expected by BC6H.</summary>
    private sealed class BitWriter
    {
        public byte[] Buffer { get; } = new byte[16];
        private int _pos;

        public void Write(int value, int bits)
        {
            for (int i = 0; i < bits; i++)
            {
                int bp = _pos + i;
                int bit = (value >> i) & 1;
                Buffer[bp >> 3] |= (byte)(bit << (bp & 7));
            }
            _pos += bits;
        }

        public int Position => _pos;
    }

    private static byte[] BuildDdsHeader(int width, int height, string fourCC)
    {
        var hdr = new byte[128];
        hdr[0] = (byte)'D'; hdr[1] = (byte)'D'; hdr[2] = (byte)'S'; hdr[3] = (byte)' ';
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4), 124);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(8), 0x1007);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(12), (uint)height);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(16), (uint)width);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(76), 32);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(80), 0x4); // DDPF_FOURCC
        var f = Encoding.ASCII.GetBytes(fourCC);
        hdr[84] = f[0]; hdr[85] = f[1]; hdr[86] = f[2]; hdr[87] = f[3];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(108), 0x1000);
        return hdr;
    }

    private static byte[] BuildDx10Tail(uint dxgiFormat)
    {
        var t = new byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(0), dxgiFormat);
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(4), 3);  // 2D texture
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(8), 0);
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(12), 1);
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(16), 0);
        return t;
    }

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    /// <summary>
    /// Build a Khronos Mode 3 BC6H block (1 subset, no transform, 10-bit
    /// endpoints, 4-bit indices). Equivalent to DXGI "BC6H Mode 11".
    /// </summary>
    private static byte[] BuildMode3Block(
        int rw, int gw, int bw,
        int rx, int gx, int bx,
        int[] indices)
    {
        var w = new BitWriter();
        // Khronos Mode 3 → 5-bit value '00011' written LSB-first = 3.
        w.Write(3, 5);
        // 6 × 10-bit endpoint components (rw, gw, bw, rx, gx, bx).
        w.Write(rw, 10); w.Write(gw, 10); w.Write(bw, 10);
        w.Write(rx, 10); w.Write(gx, 10); w.Write(bx, 10);
        // 16 indices: pixel 0 (implicit anchor) = 3 bits, rest = 4 bits.
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 3 : 4;
            w.Write(indices[i], bits);
        }
        return w.Buffer;
    }

    /// <summary>
    /// Build a Khronos Mode 15 BC6H block (1 subset, 16-bit base + 4-bit
    /// deltas, 4-bit indices). Endpoints' high six bits (R0[15..10],
    /// G0[15..10], B0[15..10]) are stored *reversed* between R1/G1/B1 in
    /// the bit-layout dictated by the spec — see KDF 1.4 Table 135 mode 15.
    /// Equivalent to DXGI "BC6H Mode 14".
    /// </summary>
    private static byte[] BuildMode15Block(
        int rw, int gw, int bw,
        int drx, int dgx, int dbx,
        int[] indices)
    {
        // Direct byte writes — bit positions are scattered, so the linear
        // BitWriter helper is awkward. Build the 128-bit block as two ulongs.
        ulong lo = 0, hi = 0;
        WriteBit(ref lo, ref hi, 0, 1); WriteBit(ref lo, ref hi, 1, 1);  // mode = 15
        WriteBit(ref lo, ref hi, 2, 1); WriteBit(ref lo, ref hi, 3, 1);
        WriteBit(ref lo, ref hi, 4, 0);
        WriteBits(ref lo, ref hi, 5, 10, rw & 0x3FF);     // R0[9..0]
        WriteBits(ref lo, ref hi, 15, 10, gw & 0x3FF);    // G0[9..0]
        WriteBits(ref lo, ref hi, 25, 10, bw & 0x3FF);    // B0[9..0]
        WriteBits(ref lo, ref hi, 35, 4, drx & 0xF);      // R1[3..0]
        // R0[15..10] reversed: bit39 ← R0[15], bit40 ← R0[14], …
        for (int i = 0; i < 6; i++)
            WriteBit(ref lo, ref hi, 39 + i, (rw >> (15 - i)) & 1);
        WriteBits(ref lo, ref hi, 45, 4, dgx & 0xF);      // G1[3..0]
        for (int i = 0; i < 6; i++)
            WriteBit(ref lo, ref hi, 49 + i, (gw >> (15 - i)) & 1);
        WriteBits(ref lo, ref hi, 55, 4, dbx & 0xF);      // B1[3..0]
        for (int i = 0; i < 6; i++)
            WriteBit(ref lo, ref hi, 59 + i, (bw >> (15 - i)) & 1);
        // Indices: pixel 0 = 3 bits at pos 65, pixels 1..15 = 4 bits.
        int idxPos = 65;
        for (int i = 0; i < 16; i++)
        {
            int n = (i == 0) ? 3 : 4;
            WriteBits(ref lo, ref hi, idxPos, n, indices[i]);
            idxPos += n;
        }
        return PackBlock(lo, hi);
    }

    /// <summary>
    /// Build a Khronos Mode 0 BC6H block (2 subsets, transformed,
    /// 10-bit base + 5-bit deltas, 3-bit indices). See KDF 1.4 Table 135.
    /// </summary>
    private static byte[] BuildMode0Block(
        int r0, int g0, int b0,
        int dr1, int dg1, int db1,
        int dr2, int dg2, int db2,
        int dr3, int dg3, int db3,
        int partition, int[] indices)
    {
        ulong lo = 0, hi = 0;
        // Mode 0 → 2-bit prefix '00' (low bits 0,1) selects mode 0 directly.
        WriteBit(ref lo, ref hi, 0, 0); WriteBit(ref lo, ref hi, 1, 0);
        // (bits 2,3,4 carry endpoint payload — written below)
        WriteBits(ref lo, ref hi, 5, 10, r0 & 0x3FF);
        WriteBits(ref lo, ref hi, 15, 10, g0 & 0x3FF);
        WriteBits(ref lo, ref hi, 25, 10, b0 & 0x3FF);
        WriteBits(ref lo, ref hi, 35, 5, dr1 & 0x1F);
        WriteBits(ref lo, ref hi, 45, 5, dg1 & 0x1F);
        WriteBits(ref lo, ref hi, 55, 5, db1 & 0x1F);
        WriteBits(ref lo, ref hi, 65, 5, dr2 & 0x1F);
        WriteBits(ref lo, ref hi, 71, 5, dr3 & 0x1F);
        // G2 = bits(41,4) | bit(2)<<4 ; G3 = bits(51,4) | bit(40)<<4
        WriteBits(ref lo, ref hi, 41, 4, dg2 & 0xF);
        WriteBit(ref lo, ref hi, 2, (dg2 >> 4) & 1);
        WriteBits(ref lo, ref hi, 51, 4, dg3 & 0xF);
        WriteBit(ref lo, ref hi, 40, (dg3 >> 4) & 1);
        // B2 = bits(61,4) | bit(3)<<4
        WriteBits(ref lo, ref hi, 61, 4, db2 & 0xF);
        WriteBit(ref lo, ref hi, 3, (db2 >> 4) & 1);
        // B3 = bit(50) | bit(60)<<1 | bit(70)<<2 | bit(76)<<3 | bit(4)<<4
        WriteBit(ref lo, ref hi, 50, (db3 >> 0) & 1);
        WriteBit(ref lo, ref hi, 60, (db3 >> 1) & 1);
        WriteBit(ref lo, ref hi, 70, (db3 >> 2) & 1);
        WriteBit(ref lo, ref hi, 76, (db3 >> 3) & 1);
        WriteBit(ref lo, ref hi, 4,  (db3 >> 4) & 1);
        WriteBits(ref lo, ref hi, 77, 5, partition & 0x1F);
        // Indices start at bit 82. Pixel 0 + anchor pixel use 2 bits each;
        // all others use 3 bits.
        EmitIndices2Subset(ref lo, ref hi, partition, indices);
        return PackBlock(lo, hi);
    }

    /// <summary>
    /// Build a Khronos Mode 30 BC6H block (2 subsets, untransformed,
    /// 6-bit endpoints, 3-bit indices). See KDF 1.4 Table 135.
    /// </summary>
    private static byte[] BuildMode30Block(
        int r0, int g0, int b0,
        int r1, int g1, int b1,
        int r2, int g2, int b2,
        int r3, int g3, int b3,
        int partition, int[] indices)
    {
        ulong lo = 0, hi = 0;
        // Mode 30 → 5-bit value '01111' wait — 30 = 0b11110 LSB-first.
        // Bit 0 = 0, bit 1 = 1, bit 2 = 1, bit 3 = 1, bit 4 = 1.
        WriteBits(ref lo, ref hi, 0, 5, 30);
        WriteBits(ref lo, ref hi, 5, 6, r0 & 0x3F);
        WriteBits(ref lo, ref hi, 15, 6, g0 & 0x3F);
        WriteBits(ref lo, ref hi, 25, 6, b0 & 0x3F);
        WriteBits(ref lo, ref hi, 35, 6, r1 & 0x3F);
        WriteBits(ref lo, ref hi, 45, 6, g1 & 0x3F);
        WriteBits(ref lo, ref hi, 55, 6, b1 & 0x3F);
        WriteBits(ref lo, ref hi, 65, 6, r2 & 0x3F);
        WriteBits(ref lo, ref hi, 71, 6, r3 & 0x3F);
        // G2 = bits(41,4) | bit(24)<<4 | bit(21)<<5
        WriteBits(ref lo, ref hi, 41, 4, g2 & 0xF);
        WriteBit(ref lo, ref hi, 24, (g2 >> 4) & 1);
        WriteBit(ref lo, ref hi, 21, (g2 >> 5) & 1);
        // G3 = bits(51,4) | bit(11)<<4 | bit(31)<<5
        WriteBits(ref lo, ref hi, 51, 4, g3 & 0xF);
        WriteBit(ref lo, ref hi, 11, (g3 >> 4) & 1);
        WriteBit(ref lo, ref hi, 31, (g3 >> 5) & 1);
        // B2 = bits(61,4) | bit(14)<<4 | bit(22)<<5
        WriteBits(ref lo, ref hi, 61, 4, b2 & 0xF);
        WriteBit(ref lo, ref hi, 14, (b2 >> 4) & 1);
        WriteBit(ref lo, ref hi, 22, (b2 >> 5) & 1);
        // B3 = bit(12) | bit(13)<<1 | bit(23)<<2 | bit(32)<<3 | bit(34)<<4 | bit(33)<<5
        WriteBit(ref lo, ref hi, 12, (b3 >> 0) & 1);
        WriteBit(ref lo, ref hi, 13, (b3 >> 1) & 1);
        WriteBit(ref lo, ref hi, 23, (b3 >> 2) & 1);
        WriteBit(ref lo, ref hi, 32, (b3 >> 3) & 1);
        WriteBit(ref lo, ref hi, 34, (b3 >> 4) & 1);
        WriteBit(ref lo, ref hi, 33, (b3 >> 5) & 1);
        WriteBits(ref lo, ref hi, 77, 5, partition & 0x1F);
        EmitIndices2Subset(ref lo, ref hi, partition, indices);
        return PackBlock(lo, hi);
    }

    private static void EmitIndices2Subset(ref ulong lo, ref ulong hi, int partition, int[] indices)
    {
        // Anchor pixel index per partition (Khronos KDF 1.4 Table 131).
        ReadOnlySpan<byte> anchor =
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
        int idxPos = 82;
        int a = anchor[partition];
        for (int i = 0; i < 16; i++)
        {
            int n = (i == 0 || i == a) ? 2 : 3;
            WriteBits(ref lo, ref hi, idxPos, n, indices[i]);
            idxPos += n;
        }
    }

    private static void WriteBit(ref ulong lo, ref ulong hi, int pos, int bit)
    {
        if (bit == 0) return;
        if (pos < 64) lo |= 1UL << pos;
        else hi |= 1UL << (pos - 64);
    }

    private static void WriteBits(ref ulong lo, ref ulong hi, int pos, int count, int value)
    {
        for (int i = 0; i < count; i++)
            WriteBit(ref lo, ref hi, pos + i, (value >> i) & 1);
    }

    private static byte[] PackBlock(ulong lo, ulong hi)
    {
        var block = new byte[16];
        BinaryPrimitives.WriteUInt64LittleEndian(block.AsSpan(0, 8), lo);
        BinaryPrimitives.WriteUInt64LittleEndian(block.AsSpan(8, 8), hi);
        return block;
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode3_ZeroEndpoints_ProducesZeroPixels()
    {
        // All endpoint components = 0, all indices = 0 → every pixel should be 0.0f.
        int[] indices = new int[16];
        var block = BuildMode3Block(0, 0, 0, 0, 0, 0, indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(PixelFormat.Rgb96Float, captured!.PixelFormat);
            Assert.Equal(4 * 4 * 12, captured.Pixels.Length);
            var s = captured.Pixels.Span;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                float r = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4));
                float g = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4));
                float b = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4));
                Assert.Equal(0.0f, r);
                Assert.Equal(0.0f, g);
                Assert.Equal(0.0f, b);
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode3_EqualMaxEndpoints_ProducesHalfMax()
    {
        // Both endpoints = (1023, 1023, 1023). For 10-bit unsigned, val == ((1<<10)-1)
        // unquantizes to 0xFFFF, then finalize: (65535 * 31) >> 6 = 31743 = 0x7BFF,
        // which is Half.MaxValue (≈ 65504.0).
        int[] indices = new int[16];
        var block = BuildMode3Block(1023, 1023, 1023, 1023, 1023, 1023, indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            var expected = (float)Half.MaxValue;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                float r = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4));
                float g = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4));
                float b = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4));
                Assert.Equal(expected, r);
                Assert.Equal(expected, g);
                Assert.Equal(expected, b);
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode3_RedOnlyAtMaxIndex_RedNonzero()
    {
        // e0 = (0, 0, 0), e1 = (512, 0, 0). All non-anchor indices = 15 → use e1.
        // Pixel 0 is anchor with 3-bit index → set to 0 → uses e0 → black.
        int[] indices = new int[16];
        indices[0] = 0;
        for (int i = 1; i < 16; i++) indices[i] = 15;

        var block = BuildMode3Block(0, 0, 0, 512, 0, 0, indices);
        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            // Pixel 0 (anchor, index 0): should be black.
            int o0 = 0;
            float r0 = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o0, 4));
            float g0 = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o0 + 4, 4));
            float b0 = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o0 + 8, 4));
            Assert.Equal(0.0f, r0);
            Assert.Equal(0.0f, g0);
            Assert.Equal(0.0f, b0);

            // Pixels 1-15 (index 15 → e1): red should be a positive non-zero value, G/B = 0.
            for (int p = 1; p < 16; p++)
            {
                int o = p * 12;
                float r = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4));
                float g = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4));
                float b = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4));
                Assert.True(r > 0.0f, $"pixel {p} expected r>0, got {r}");
                Assert.Equal(0.0f, g);
                Assert.Equal(0.0f, b);
            }
        }
    }

    [Fact]
    public async Task Bc6h_Sf16_DxgiFormat96_IsRecognised()
    {
        // DXGI 96 = BC6H_SF16. The block contents don't matter for this test —
        // we only verify the format identification + pipeline wiring up to decode.
        int[] indices = new int[16]; // zeros
        var block = BuildMode3Block(0, 0, 0, 0, 0, 0, indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(96), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Rgb96Float, reader.Info.PixelFormat);
        Assert.Equal("Bc6hSf16", reader.Info.ColorSpace);

        // The signed pipeline must produce 0 for all-zero endpoints (sign-extended = 0).
        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4)));
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode15_ZeroBaseZeroDelta_ProducesZero()
    {
        // Base = (0, 0, 0), delta = (0, 0, 0) → e0 = e1 = 0 → all pixels = 0.
        int[] indices = new int[16];
        var block = BuildMode15Block(0, 0, 0, 0, 0, 0, indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4)));
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode15_NonZeroBase_LayoutRoundTrips()
    {
        // Sanity-check the scattered Mode 15 layout: a non-trivial base
        // (R0=0xABCD, G0=0x1234, B0=0xCAFE) with zero deltas must produce
        // 16 identical pixels equal to that base unquantized to half-float.
        // Per Unquantize(epb=16, unsigned): val is returned as-is, then
        // Finalize() does (val*31)>>6 → reinterpret as Half bits.
        int[] indices = new int[16];
        const int R0 = 0xABCD, G0 = 0x1234, B0 = 0xCAFE;
        var block = BuildMode15Block(R0, G0, B0, 0, 0, 0, indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            float expR = (float)BitConverter.UInt16BitsToHalf((ushort)((R0 * 31) >> 6));
            float expG = (float)BitConverter.UInt16BitsToHalf((ushort)((G0 * 31) >> 6));
            float expB = (float)BitConverter.UInt16BitsToHalf((ushort)((B0 * 31) >> 6));
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                Assert.Equal(expR, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4)));
                Assert.Equal(expG, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4)));
                Assert.Equal(expB, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4)));
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode0_AllZero_ProducesZero()
    {
        // An all-zero 128-bit block: low2 = 0 → Khronos Mode 0 (2-subset, transformed).
        // Every endpoint, every delta, every index = 0 → all pixels = 0.
        var block = new byte[16];
        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4)));
            }
        }
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode30_Partition0_SubsetSplit_ProducesExpectedColors()
    {
        // Mode 30: 2-subset, untransformed, 6-bit endpoints. Partition 0
        // mask = 0xCCCC (1100 1100 1100 1100b LSB-first), so pixels
        // 0,1,4,5,8,9,12,13 are subset 0, the rest are subset 1.
        //
        // Subset 0: endpoints (0,0,0)/(0,0,0) → black.
        // Subset 1: endpoints (63,63,63)/(63,63,63) → Half.MaxValue
        //   because Unquantize(63, epb=6) = ((63<<15)+0x4000) >> 5 = 65024,
        //   then Finalize(unsigned) = (65024*31)>>6 = 31496 = 0x7B08,
        //   reinterpreting as Half bits gives 60992.0. Close enough to
        //   confirm subset-1 pixels are large positive values.
        int[] indices = new int[16];

        var block = BuildMode30Block(
            r0: 0, g0: 0, b0: 0,
            r1: 0, g1: 0, b1: 0,
            r2: 63, g2: 63, b2: 63,
            r3: 63, g3: 63, b3: 63,
            partition: 0, indices: indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            // Partition 0: subset bits per pixel index 0..15 (LSB-first of 0xCCCC):
            // pixel:    0 1 2 3 4 5 6 7 8 9 a b c d e f
            // subset:   0 0 1 1 0 0 1 1 0 0 1 1 0 0 1 1
            ReadOnlySpan<int> subset = [0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 1, 1];
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                float r = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4));
                float g = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4));
                float b = BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4));
                if (subset[p] == 0)
                {
                    Assert.Equal(0.0f, r);
                    Assert.Equal(0.0f, g);
                    Assert.Equal(0.0f, b);
                }
                else
                {
                    Assert.True(r > 1000.0f, $"pixel {p}: expected large R, got {r}");
                    Assert.True(g > 1000.0f, $"pixel {p}: expected large G, got {g}");
                    Assert.True(b > 1000.0f, $"pixel {p}: expected large B, got {b}");
                }
            }
        }
    }

    [Fact]
    public async Task Bc6h_ReservedMode_DecodesToZero()
    {
        // Mode 19 is reserved per KDF 1.4 § 20.2. The spec mandates that
        // reserved modes decode to (0, 0, 0, 1) — for our RGB output we
        // verify the three components are zero.
        // Mode 19 = 0b10011 LSB-first: bit0=1, bit1=1, bit2=0, bit3=0, bit4=1.
        ulong lo = 0, hi = 0;
        // bit0=1
        lo |= 1UL << 0;
        // bit1=1
        lo |= 1UL << 1;
        // bit2=0, bit3=0
        // bit4=1
        lo |= 1UL << 4;
        var block = new byte[16];
        BinaryPrimitives.WriteUInt64LittleEndian(block.AsSpan(0, 8), lo);
        BinaryPrimitives.WriteUInt64LittleEndian(block.AsSpan(8, 8), hi);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int p = 0; p < 16; p++)
            {
                int o = p * 12;
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 4, 4)));
                Assert.Equal(0.0f, BinaryPrimitives.ReadSingleLittleEndian(s.Slice(o + 8, 4)));
            }
        }
    }

    private static byte[] Pad(byte[] src, int len)
    {
        if (src.Length == len) return src;
        var r = new byte[len];
        Buffer.BlockCopy(src, 0, r, 0, Math.Min(src.Length, len));
        return r;
    }
}
