using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the BC7 (BPTC unorm) decoder. Each test programmatically builds
/// a 128-bit BC7 block bit-by-bit, then verifies the decoded BGRA output
/// pixel-by-pixel against the expected colour.
/// </summary>
public sealed class Bc7DecoderTests
{
    /// <summary>Helper to LSB-first pack arbitrary bit fields into a 16-byte BC7 block.</summary>
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

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    /// <summary>Builds a Mode 6 BC7 block (1 subset, 7-bit R/G/B/A + 1 P-bit, 4-bit indices).</summary>
    private static byte[] BuildMode6Block(byte r0, byte r1, byte g0, byte g1, byte b0, byte b1,
                                          byte a0, byte a1, int p0, int p1, int[] indices)
    {
        var w = new BitWriter();
        // Mode = 6: 6 zero bits + 1 → bit position 6 is set.
        w.Write(0, 6);
        w.Write(1, 1);
        // R[0], R[1], G[0], G[1], B[0], B[1], A[0], A[1] (7 bits each).
        w.Write(r0, 7); w.Write(r1, 7);
        w.Write(g0, 7); w.Write(g1, 7);
        w.Write(b0, 7); w.Write(b1, 7);
        w.Write(a0, 7); w.Write(a1, 7);
        // P-bits.
        w.Write(p0, 1); w.Write(p1, 1);
        // 16 × 4-bit indices, pixel 0 = anchor → 3 bits, rest 4 bits.
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 3 : 4;
            w.Write(indices[i], bits);
        }
        return w.Buffer;
    }

    [Fact]
    public async Task Bc7_Mode6_SolidRed_AllIndicesZero_ProducesRed()
    {
        // Both endpoints encode pure red:
        //   R = 0x7F | P=1 → 0xFF, G = 0x00 | P=1 → 0x01, B = 0x00 | P=1 → 0x01, A = 0x7F | P=1 → 0xFF.
        // All indices = 0 → every pixel = endpoint 0.
        var block = BuildMode6Block(
            r0: 0x7F, r1: 0x7F,
            g0: 0x00, g1: 0x00,
            b0: 0x00, b1: 0x00,
            a0: 0x7F, a1: 0x7F,
            p0: 1, p1: 1,
            indices: new int[16]);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(98 /* BC7_UNORM */), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            Assert.Equal(4 * 4 * 4, s.Length);
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                // BGRA layout
                Assert.Equal(0x01, s[i + 0]);  // B
                Assert.Equal(0x01, s[i + 1]);  // G
                Assert.Equal(0xFF, s[i + 2]);  // R
                Assert.Equal(0xFF, s[i + 3]);  // A
            }
        }
    }

    [Fact]
    public async Task Bc7_Mode6_AllIndicesMax_ProducesEndpoint1()
    {
        // Endpoint 0 = (0,0,0,0), Endpoint 1 = (FF,FF,FF,FF). Index 15 selects endpoint 1.
        var indices = new int[16];
        for (int i = 0; i < 16; i++) indices[i] = (i == 0) ? 7 : 15;
        // Anchor index 0 uses only 3 bits, so max anchor value = 7. But to truly select endpoint 1
        // we want weight 64, which corresponds to index 15. Since pixel 0 only has 3 bits, max is 7
        // (weight 30). For simplicity make pixel 0 also explicitly anchored to endpoint 1's halfway
        // and skip verifying pixel 0. We'll just verify pixels 1..15.

        var block = BuildMode6Block(
            r0: 0x00, r1: 0x7F,
            g0: 0x00, g1: 0x7F,
            b0: 0x00, b1: 0x7F,
            a0: 0x00, a1: 0x7F,
            p0: 0, p1: 1,
            indices: indices);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(98), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            // Skip pixel 0 (anchor only allows 3 bits). Verify pixels 1..15 are endpoint 1 = 0xFF.
            for (int p = 1; p < 16; p++)
            {
                int o = p * 4;
                Assert.Equal(0xFF, s[o + 0]);  // B
                Assert.Equal(0xFF, s[o + 1]);  // G
                Assert.Equal(0xFF, s[o + 2]);  // R
                Assert.Equal(0xFF, s[o + 3]);  // A
            }
        }
    }

    [Fact]
    public async Task Bc7_RecognisedAsBgra32_ViaDxgiFormatCode()
    {
        // BC7 fourCC "BC7" via DXGI format 98.
        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(98), 20));
        // Add a dummy zero block (which decodes to transparent black via mode 8/reserved).
        file = Concat(file, new byte[16]);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);
        Assert.Equal("Bc7", reader.Info.ColorSpace);
    }

    /// <summary>Builds a Mode 5 BC7 block (1 subset, rotation, 7-bit RGB, 8-bit A, 2-bit color + 2-bit alpha indices).</summary>
    private static byte[] BuildMode5Block(int rotation,
                                          byte r0, byte r1, byte g0, byte g1, byte b0, byte b1,
                                          byte a0, byte a1, int[] colorIndices, int[] alphaIndices)
    {
        var w = new BitWriter();
        // Mode = 5: 5 zero bits + 1 → bit position 5 is set.
        w.Write(0, 5);
        w.Write(1, 1);
        // 2 rotation bits.
        w.Write(rotation, 2);
        // R[0], R[1], G[0], G[1], B[0], B[1] (7 bits each).
        w.Write(r0, 7); w.Write(r1, 7);
        w.Write(g0, 7); w.Write(g1, 7);
        w.Write(b0, 7); w.Write(b1, 7);
        // A[0], A[1] (8 bits each).
        w.Write(a0, 8); w.Write(a1, 8);
        // 16 × 2-bit colour indices, pixel 0 = anchor → 1 bit, rest 2 bits.
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 1 : 2;
            w.Write(colorIndices[i], bits);
        }
        // 16 × 2-bit alpha indices, pixel 0 = 1 bit.
        for (int i = 0; i < 16; i++)
        {
            int bits = (i == 0) ? 1 : 2;
            w.Write(alphaIndices[i], bits);
        }
        return w.Buffer;
    }

    [Fact]
    public async Task Bc7_Mode5_SolidGreen_AllIndicesZero()
    {
        // Mode 5: 7-bit RGB endpoints (bit-replicated), 8-bit alpha endpoints.
        // R[0]=R[1]=0 → 0x00, G[0]=G[1]=0x7F → 0xFF (bit replication), B[0]=B[1]=0 → 0x00,
        // A[0]=A[1]=0xFF → 0xFF (already 8-bit). Indices all 0 → endpoint 0.
        var colorIdx = new int[16];
        var alphaIdx = new int[16];
        var block = BuildMode5Block(
            rotation: 0,
            r0: 0x00, r1: 0x00,
            g0: 0x7F, g1: 0x7F,
            b0: 0x00, b1: 0x00,
            a0: 0xFF, a1: 0xFF,
            colorIndices: colorIdx,
            alphaIndices: alphaIdx);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(98), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                Assert.Equal(0x00, s[i + 0]);  // B
                Assert.Equal(0xFF, s[i + 1]);  // G
                Assert.Equal(0x00, s[i + 2]);  // R
                Assert.Equal(0xFF, s[i + 3]);  // A
            }
        }
    }

    [Fact]
    public async Task Bc7_Mode5_AlphaRotation_SwapsAlphaWithBlue()
    {
        // Rotation 3 swaps alpha channel with the blue channel.
        // Set up so that "colour" endpoints encode RGB=(0xFF,0,0xFF) and alpha endpoint=0x00.
        // After rotation 3: blue channel ↔ alpha. So B becomes alpha value (0), A becomes blue (0xFF).
        // Expected output BGRA = (0x00, 0x00, 0xFF, 0xFF) — same as red (since blue is swapped out).
        var colorIdx = new int[16];
        var alphaIdx = new int[16];
        var block = BuildMode5Block(
            rotation: 3,
            r0: 0x7F, r1: 0x7F,
            g0: 0x00, g1: 0x00,
            b0: 0x7F, b1: 0x7F,
            a0: 0x00, a1: 0x00,
            colorIndices: colorIdx,
            alphaIndices: alphaIdx);

        var file = Concat(BuildDdsHeader(4, 4, "DX10"), Pad(BuildDx10Tail(98), 20));
        file = Concat(file, block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                // Rotation 3: blue ↔ alpha.
                // Original interpolated colour: B=0xFF, G=0x00, R=0xFF, A=0x00.
                // After swap (B,A): B=0x00, A=0xFF.
                Assert.Equal(0x00, s[i + 0]);  // B (was A)
                Assert.Equal(0x00, s[i + 1]);  // G
                Assert.Equal(0xFF, s[i + 2]);  // R
                Assert.Equal(0xFF, s[i + 3]);  // A (was B)
            }
        }
    }

    private static byte[] BuildDx10Tail(uint dxgiFormat)
    {
        var t = new byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(0), dxgiFormat);
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(4), 3);  // resourceDimension = 2D
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(8), 0);  // miscFlag
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(12), 1); // arraySize
        BinaryPrimitives.WriteUInt32LittleEndian(t.AsSpan(16), 0); // miscFlags2
        return t;
    }

    private static byte[] Pad(byte[] src, int len)
    {
        if (src.Length == len) return src;
        var r = new byte[len];
        Buffer.BlockCopy(src, 0, r, 0, Math.Min(src.Length, len));
        return r;
    }
}
