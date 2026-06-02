using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the DDS BCn block-decompression paths (BC1-BC5).
/// Each test constructs a minimal 4×4 DDS file with hand-crafted blocks and
/// verifies the decoded output exactly matches the expected pixels.
/// </summary>
public sealed class DdsBcnDecoderTests
{
    // DDS pixel-format flags
    private const uint DDPF_FOURCC = 0x4;

    private static byte[] BuildDdsHeader(int width, int height, string fourCC, bool compressed)
    {
        // 128-byte DDS header.
        var hdr = new byte[128];
        hdr[0] = (byte)'D'; hdr[1] = (byte)'D'; hdr[2] = (byte)'S'; hdr[3] = (byte)' ';
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4), 124);   // size
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(8), 0x1007); // flags (caps | width | height | pixelformat)
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(12), (uint)height);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(16), (uint)width);
        // pitch / linear size at offset 20 — leave 0.
        // depth + mipMapCount = 0.
        // 11 reserved DWORDs.

        // Pixel format starts at offset 76, size 32 bytes.
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(76), 32); // size of pixel format
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(80), compressed ? DDPF_FOURCC : 0);
        if (compressed)
        {
            var f = Encoding.ASCII.GetBytes(fourCC);
            hdr[84] = f[0]; hdr[85] = f[1]; hdr[86] = f[2]; hdr[87] = f[3];
        }
        // caps at offset 108.
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(108), 0x1000); // DDSCAPS_TEXTURE
        return hdr;
    }

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    [Fact]
    public async Task Bc1_AllZeroIndices_ProducesEndpointZero()
    {
        // c0 = RGB565 red (0xF800), c1 = RGB565 black (0x0000), all indices = 0.
        // Since c0 (0xF800) > c1 (0x0000), opaque mode; index 0 = c0 = red.
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0xF800); // c0 = red 5:6:5
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0x0000); // c1 = black
        // indices = all-zero → every pixel = c0.

        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.Equal(4, reader.Info.Width);
        Assert.Equal(4, reader.Info.Height);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(PixelFormat.Bgra32, captured!.PixelFormat);
            Assert.Equal(4 * 4 * 4, captured.Pixels.Length);
            var s = captured.Pixels.Span;
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                // BGRA: B=0, G=0, R=255, A=255
                Assert.Equal(0x00, s[i + 0]);
                Assert.Equal(0x00, s[i + 1]);
                Assert.Equal(0xFF, s[i + 2]);
                Assert.Equal(0xFF, s[i + 3]);
            }
        }
    }

    [Fact]
    public async Task Bc1_AlphaMode_AllOnes_ProducesEndpointOne()
    {
        // c0 = 0x0000, c1 = 0xF800 — c0 <= c1 → 1-bit-alpha mode.
        // Index 1 = c1 = red.
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0x0000); // c0 = black
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0xF800); // c1 = red
        // indices = all-ones (every 2-bit pair = 01) → 0x55555555
        BinaryPrimitives.WriteUInt32LittleEndian(block.AsSpan(4), 0x55555555);

        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);

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
                Assert.Equal(0x00, s[i + 0]); // B
                Assert.Equal(0x00, s[i + 1]); // G
                Assert.Equal(0xFF, s[i + 2]); // R
                Assert.Equal(0xFF, s[i + 3]); // A (opaque)
            }
        }
    }

    [Fact]
    public async Task Bc2_ExplicitAlpha_NibblesPromotedTo8Bit()
    {
        // BC2 block = 8 bytes alpha + 8 bytes BC1-style color.
        // Alpha nibbles all 0xF → 0xFF.
        var block = new byte[16];
        for (int i = 0; i < 8; i++) block[i] = 0xFF;
        // Color: c0 > c1 → opaque mode, c0 = red, c1 = black, indices = all-zero → red.
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(8), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(10), 0x0000);

        var file = Concat(BuildDdsHeader(4, 4, "DXT3", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(PixelFormat.Bgra32, captured!.PixelFormat);
            var s = captured.Pixels.Span;
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                Assert.Equal(0x00, s[i + 0]);  // B
                Assert.Equal(0x00, s[i + 1]);  // G
                Assert.Equal(0xFF, s[i + 2]);  // R
                Assert.Equal(0xFF, s[i + 3]);  // A
            }
        }
    }

    [Fact]
    public async Task Bc2_HalfAlpha_NibbleEightBecomes88Hex()
    {
        // Each alpha nibble = 0x8 → 0x88 (replicated nibble).
        var block = new byte[16];
        for (int i = 0; i < 8; i++) block[i] = 0x88;
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(8), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(10), 0x0000);

        var file = Concat(BuildDdsHeader(4, 4, "DXT3", compressed: true), block);

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
                Assert.Equal(0x88, s[i + 3]);  // alpha
            }
        }
    }

    [Fact]
    public async Task Bc3_InterpolatedAlpha_AllZeroIndicesProducesEndpoint0()
    {
        // BC3 block: 8-byte BC4-style alpha + 8-byte BC1-style color.
        // a0=0xFF, a1=0x00 → 8-step mode. Indices = all-zero → alpha = a0 = 0xFF.
        var block = new byte[16];
        block[0] = 0xFF;
        block[1] = 0x00;
        // bytes 2..7 = indices = all 0 → every pixel = a0
        for (int i = 2; i < 8; i++) block[i] = 0;
        // Color block: c0=red, c1=black, indices=0 → red.
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(8), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(10), 0x0000);

        var file = Concat(BuildDdsHeader(4, 4, "DXT5", compressed: true), block);

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
                Assert.Equal(0xFF, s[i + 2]);  // R
                Assert.Equal(0xFF, s[i + 3]);  // A
            }
        }
    }

    [Fact]
    public async Task Bc4_RedChannel_AllZeroIndicesProducesA0()
    {
        // 8-byte BC4 block: a0=200, a1=50, indices = all-zero → output = 200 everywhere.
        var block = new byte[8];
        block[0] = 200;
        block[1] = 50;
        // bytes 2..7 = 0

        var file = Concat(BuildDdsHeader(4, 4, "ATI1", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(PixelFormat.Gray8, captured!.PixelFormat);
            Assert.Equal(16, captured.Pixels.Length);
            var s = captured.Pixels.Span;
            for (int i = 0; i < s.Length; i++)
            {
                Assert.Equal(200, s[i]);
            }
        }
    }

    [Fact]
    public async Task Bc4_AllOnesIndices_ProducesA1()
    {
        // a0=200, a1=50, indices = all-ones (16 × 3-bit value 1) → output = a1 = 50.
        var block = new byte[8];
        block[0] = 200;
        block[1] = 50;
        // 16 × 3-bit ones = 0x249249249249 (48 bits)
        // Bit pattern: 001 001 001 001 ... 16 times, little-endian byte order.
        // bits = 0x249249249249 → byte 2 = 0x49, byte 3 = 0x92, byte 4 = 0x24,
        // byte 5 = 0x49, byte 6 = 0x92, byte 7 = 0x24.
        block[2] = 0x49; block[3] = 0x92; block[4] = 0x24;
        block[5] = 0x49; block[6] = 0x92; block[7] = 0x24;

        var file = Concat(BuildDdsHeader(4, 4, "ATI1", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int i = 0; i < s.Length; i++)
            {
                Assert.Equal(50, s[i]);
            }
        }
    }

    [Fact]
    public async Task Bc5_TwoChannels_AllZeroIndicesProducesA0A0_0()
    {
        // 16-byte BC5 block = two BC4 blocks back-to-back (red, then green).
        var block = new byte[16];
        block[0] = 180; block[1] = 30;   // red endpoints
        // bytes 2..7 = 0 → all indices 0 → red = 180
        block[8] = 220; block[9] = 40;   // green endpoints
        // bytes 10..15 = 0 → green = 220

        var file = Concat(BuildDdsHeader(4, 4, "ATI2", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        Assert.Equal(PixelFormat.Rgb24, reader.Info.PixelFormat);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(PixelFormat.Rgb24, captured!.PixelFormat);
            Assert.Equal(4 * 4 * 3, captured.Pixels.Length);
            var s = captured.Pixels.Span;
            for (int i = 0; i + 2 < s.Length; i += 3)
            {
                Assert.Equal(180, s[i + 0]);  // R
                Assert.Equal(220, s[i + 1]);  // G
                Assert.Equal(0, s[i + 2]);    // B
            }
        }
    }

    [Fact]
    public async Task Bcn_Bc6h_AllZeroBlock_DecodesAsMode0AllZeroPixels()
    {
        // An all-zero 128-bit block has low2 = 0 → Khronos Mode 0 (2 subsets,
        // transformed). With every endpoint, delta, and index equal to zero,
        // the entire 4×4 tile must decode to (0, 0, 0). Previously this test
        // expected NotSupportedException because the decoder rejected
        // partitioned modes; the full 14-mode decoder now handles it.
        var file = BuildDdsHeader(4, 4, "DX10", compressed: true);
        file = Concat(file, BuildDx10TailLocal(95)); // BC6H_UF16
        var block = new byte[16];
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
            Assert.Equal(4 * 4 * 12, s.Length);
            for (int i = 0; i < s.Length; i++) Assert.Equal((byte)0, s[i]);
        }
    }

    [Fact]
    public void Open_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => DdsReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_TruncatedHeader_Throws()
    {
        // < 128 bytes is rejected as truncated.
        var bytes = new byte[64];
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => DdsReader.Open(ms, ownsStream: false));
    }

    [Fact]
    public void Open_BadMagic_Throws()
    {
        // Header is full size but magic isn't "DDS ".
        var bytes = BuildDdsHeader(4, 4, "DXT1", compressed: true);
        bytes[0] = (byte)'X'; bytes[1] = (byte)'X'; bytes[2] = (byte)'X'; bytes[3] = (byte)'X';
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => DdsReader.Open(ms, ownsStream: false));
    }

    [Fact]
    public void Open_BadHeaderSize_Throws()
    {
        // Magic is right but size field isn't 124.
        var bytes = BuildDdsHeader(4, 4, "DXT1", compressed: true);
        BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(4), 100);
        using var ms = new MemoryStream(bytes);
        Assert.Throws<ImageFormatException>(() => DdsReader.Open(ms, ownsStream: false));
    }

    [Fact]
    public async Task Bc1_OpaqueMode_AllIndicesOne_ProducesC1()
    {
        // c0=red (0xF800) > c1=black (0x0000) → opaque mode, index 1 = c1 = black.
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0x0000);
        BinaryPrimitives.WriteUInt32LittleEndian(block.AsSpan(4), 0x55555555);

        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);
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
                Assert.Equal(0x00, s[i + 0]); // B
                Assert.Equal(0x00, s[i + 1]); // G
                Assert.Equal(0x00, s[i + 2]); // R (c1 = black)
                Assert.Equal(0xFF, s[i + 3]); // A opaque
            }
        }
    }

    [Fact]
    public async Task Bc1_AlphaMode_Index3_ProducesTransparentBlack()
    {
        // c0 <= c1 → 1-bit-alpha mode. Index 3 yields transparent black
        // (RGBA = 0, 0, 0, 0).
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0x0000);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0xF800);
        // All-ones index pairs (11 binary = 3) -> 0xFFFFFFFF
        BinaryPrimitives.WriteUInt32LittleEndian(block.AsSpan(4), 0xFFFFFFFF);

        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);
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
                Assert.Equal(0x00, s[i + 0]);
                Assert.Equal(0x00, s[i + 1]);
                Assert.Equal(0x00, s[i + 2]);
                Assert.Equal(0x00, s[i + 3]); // transparent
            }
        }
    }

    [Fact]
    public async Task Bc1_EightByEight_Tile_Has_Sixty_Four_Pixels()
    {
        // 8x8 -> 2x2 BC1 blocks -> 4 blocks @ 8 bytes each = 32 bytes payload.
        var blocks = new byte[32];
        for (int b = 0; b < 4; b++)
        {
            BinaryPrimitives.WriteUInt16LittleEndian(blocks.AsSpan(b * 8 + 0), 0xF800);
            BinaryPrimitives.WriteUInt16LittleEndian(blocks.AsSpan(b * 8 + 2), 0x0000);
            // indices = 0 -> every pixel = c0 = red
        }
        var file = Concat(BuildDdsHeader(8, 8, "DXT1", compressed: true), blocks);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.Equal(8, reader.Info.Width);
        Assert.Equal(8, reader.Info.Height);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            Assert.Equal(8 * 8 * 4, captured!.Pixels.Length);
            var s = captured.Pixels.Span;
            for (int i = 0; i + 3 < s.Length; i += 4)
            {
                Assert.Equal(0xFF, s[i + 2]); // R
                Assert.Equal(0xFF, s[i + 3]); // A
            }
        }
    }

    [Fact]
    public async Task Bc2_FullyTransparentAlpha_ZeroNibbles()
    {
        // All-zero alpha nibbles -> alpha 0 everywhere.
        var block = new byte[16];
        // bytes 0..7 = 0 -> alpha = 0
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(8), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(10), 0x0000);
        var file = Concat(BuildDdsHeader(4, 4, "DXT3", compressed: true), block);

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
                Assert.Equal(0x00, s[i + 3]);
            }
        }
    }

    [Fact]
    public async Task Bc4_a0EqualsA1_AllPixelsTakeOnSameValue()
    {
        // a0 == a1 → 6-step mode but with identical endpoints, every interpolated
        // value collapses to that single endpoint regardless of indices.
        var block = new byte[8];
        block[0] = 128; block[1] = 128;
        for (int i = 2; i < 8; i++) block[i] = 0xFF;
        var file = Concat(BuildDdsHeader(4, 4, "ATI1", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            byte first = s[0];
            for (int i = 0; i < s.Length; i++)
            {
                Assert.Equal(first, s[i]);
            }
        }
    }

    [Fact]
    public async Task Bc5_Different_R_And_G_Endpoints_Surface_Independently()
    {
        // Independent red/green endpoints with all-zero indices: every pixel
        // takes red=a0_red, green=a0_green, blue=0.
        var block = new byte[16];
        block[0] = 175; block[1] = 60;
        for (int i = 2; i < 8; i++) block[i] = 0;
        block[8] = 25; block[9] = 240;
        for (int i = 10; i < 16; i++) block[i] = 0;

        var file = Concat(BuildDdsHeader(4, 4, "ATI2", compressed: true), block);
        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        ImageFrame? captured = null;
        await foreach (var f in reader.ReadFramesAsync()) { captured = f; break; }
        Assert.NotNull(captured);
        using (captured)
        {
            var s = captured!.Pixels.Span;
            for (int i = 0; i + 2 < s.Length; i += 3)
            {
                Assert.Equal(175, s[i + 0]);
                Assert.Equal(25, s[i + 1]);
                Assert.Equal(0, s[i + 2]);
            }
        }
    }

    [Fact]
    public void OwnsStream_True_Disposes_Underlying_Stream_On_Dispose()
    {
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0x0000);
        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);

        var ms = new MemoryStream(file);
        var reader = DdsReader.Open(ms, ownsStream: true);
        reader.Dispose();
        // Underlying stream must be disposed; Position throws ObjectDisposedException.
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Position);
    }

    [Fact]
    public void OwnsStream_False_Default_Leaves_Stream_Usable()
    {
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0x0000);
        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);

        using var ms = new MemoryStream(file);
        var reader = DdsReader.Open(ms, ownsStream: false);
        reader.Dispose();
        // Stream still usable.
        Assert.Equal(file.Length, ms.Length);
    }

    [Fact]
    public void Format_Property_Is_Dds()
    {
        var block = new byte[8];
        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);
        using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.Equal(ImageFormat.Dds, reader.Format);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        var block = new byte[8];
        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);
        using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.Same(ImageMetadata.Empty, reader.Metadata);
    }

    [Fact]
    public void Info_Width_Height_Reflect_Header()
    {
        var block = new byte[8];
        var file = Concat(BuildDdsHeader(16, 8, "DXT1", compressed: true),
            new byte[8 * 8]); // 16x8 -> 4x2 blocks * 8 bytes
        using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);
        Assert.Equal(16, reader.Info.Width);
        Assert.Equal(8, reader.Info.Height);
    }

    [Fact]
    public async Task Bc1_ReadFramesAsync_Yields_Exactly_One_Frame()
    {
        var block = new byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(0), 0xF800);
        BinaryPrimitives.WriteUInt16LittleEndian(block.AsSpan(2), 0x0000);
        var file = Concat(BuildDdsHeader(4, 4, "DXT1", compressed: true), block);

        await using var ms = new MemoryStream(file);
        using var reader = DdsReader.Open(ms, ownsStream: false);

        int count = 0;
        await foreach (var f in reader.ReadFramesAsync()) { f.Dispose(); count++; }
        Assert.Equal(1, count);
    }

    private static byte[] BuildDx10TailLocal(uint dxgiFormat)
    {
        var tail = new byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(0, 4), dxgiFormat);
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(4, 4), 3); // resourceDimension = TEXTURE2D
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(8, 4), 0); // miscFlag
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(12, 4), 1); // arraySize
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(16, 4), 0); // miscFlags2
        return tail;
    }
}
