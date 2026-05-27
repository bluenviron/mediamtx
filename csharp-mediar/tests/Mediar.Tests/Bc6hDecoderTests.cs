using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the BC6H (BPTC half-float) decoder. The hand-crafted blocks
/// exercise the two pure 1-subset modes (11 and 14) and verify that the
/// 1-subset / partitioned modes either decode correctly or raise a clean
/// <see cref="NotSupportedException"/>.
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

    /// <summary>Build a Mode 11 BC6H block (1 subset, no transform, 10-bit endpoints, 4-bit indices).</summary>
    private static byte[] BuildMode11Block(
        int rw, int gw, int bw,
        int rx, int gx, int bx,
        int[] indices)
    {
        var w = new BitWriter();
        // Mode 11 → 5-bit value '00011' LSB-first = 0b00011 = 3.
        // Bits: pos0=1, pos1=1, pos2=0, pos3=0, pos4=0
        w.Write(3, 5);
        // 6 × 10-bit endpoint components (rw, gw, bw, rx, gx, bx).
        w.Write(rw, 10); w.Write(gw, 10); w.Write(bw, 10);
        w.Write(rx, 10); w.Write(gx, 10); w.Write(bx, 10);
        // 16 indices: pixel 0 (anchor) = 3 bits, rest = 4 bits → 3 + 15*4 = 63 bits.
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 3 : 4;
            w.Write(indices[i], bits);
        }
        return w.Buffer;
    }

    /// <summary>Build a Mode 14 BC6H block (1 subset, 16-bit base + 4-bit deltas, 4-bit indices).</summary>
    private static byte[] BuildMode14Block(
        int rw, int gw, int bw,
        int drx, int dgx, int dbx,
        int[] indices)
    {
        var w = new BitWriter();
        // Mode 14 → 5-bit value '01111' LSB-first = 0b01111 = 15.
        w.Write(15, 5);
        // 3 × 16-bit base + 3 × 4-bit delta.
        w.Write(rw, 16); w.Write(gw, 16); w.Write(bw, 16);
        w.Write(drx, 4); w.Write(dgx, 4); w.Write(dbx, 4);
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 3 : 4;
            w.Write(indices[i], bits);
        }
        return w.Buffer;
    }

    [Fact]
    public async Task Bc6h_Uf16_Mode11_ZeroEndpoints_ProducesZeroPixels()
    {
        // All endpoint components = 0, all indices = 0 → every pixel should be 0.0f.
        int[] indices = new int[16];
        var block = BuildMode11Block(0, 0, 0, 0, 0, 0, indices);

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
    public async Task Bc6h_Uf16_Mode11_EqualMaxEndpoints_ProducesHalfMax()
    {
        // Both endpoints = (1023, 1023, 1023). For 10-bit unsigned, val == ((1<<10)-1)
        // unquantizes to 0xFFFF, then finalize: (65535 * 31) >> 6 = 31743 = 0x7BFF,
        // which is Half.MaxValue (≈ 65504.0).
        int[] indices = new int[16];
        var block = BuildMode11Block(1023, 1023, 1023, 1023, 1023, 1023, indices);

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
    public async Task Bc6h_Uf16_Mode11_RedOnlyAtMaxIndex_RedNonzero()
    {
        // e0 = (0, 0, 0), e1 = (512, 0, 0). All non-anchor indices = 15 → use e1.
        // Pixel 0 is anchor with 3-bit index → set to 0 → uses e0 → black.
        int[] indices = new int[16];
        indices[0] = 0;
        for (int i = 1; i < 16; i++) indices[i] = 15;

        var block = BuildMode11Block(0, 0, 0, 512, 0, 0, indices);
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
        var block = BuildMode11Block(0, 0, 0, 0, 0, 0, indices);

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
    public async Task Bc6h_Uf16_Mode14_ZeroBaseZeroDelta_ProducesZero()
    {
        // Base = (0, 0, 0), delta = (0, 0, 0) → e0 = e1 = 0 → all pixels = 0.
        int[] indices = new int[16];
        var block = BuildMode14Block(0, 0, 0, 0, 0, 0, indices);

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
    public async Task Bc6h_PartitionedMode_ThrowsNotSupported()
    {
        // 2-bit mode prefix '00' = mode 1 (partitioned). The decoder must raise
        // a clean NotSupportedException instead of producing garbage pixels.
        var block = new byte[16]; // all zero → first 2 bits = 0,0 → mode 1
        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(95), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync()) { }
        });
    }

    private static byte[] Pad(byte[] src, int len)
    {
        if (src.Length == len) return src;
        var r = new byte[len];
        Buffer.BlockCopy(src, 0, r, 0, Math.Min(src.Length, len));
        return r;
    }
}
