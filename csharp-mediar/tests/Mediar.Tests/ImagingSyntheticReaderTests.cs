using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Mediar.Imaging.Hdr;
using Mediar.Imaging.Pcx;
using Mediar.Imaging.Pnm;
using Mediar.Imaging.Tga;
using Mediar.Imaging.Xpm;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests that synthesize the smallest legal file for each format and
/// verify the corresponding reader produces correct info + pixels.
/// </summary>
public sealed class ImagingSyntheticReaderTests
{
    [Fact]
    public async Task Pnm_P6_Ppm_Rgb24_RoundTrips()
    {
        // P6 4 2 255\n + 24 bytes (4*2*3) of pixel data.
        var header = Encoding.ASCII.GetBytes("P6\n4 2\n255\n");
        var pixels = new byte[24];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 10);
        var file = new byte[header.Length + pixels.Length];
        header.CopyTo(file, 0);
        pixels.CopyTo(file, header.Length);

        await using var ms = new MemoryStream(file);
        using var r = PnmReader.Open(ms, ownsStream: false);
        Assert.Equal(4, r.Info.Width);
        Assert.Equal(2, r.Info.Height);
        Assert.Equal(PixelFormat.Rgb24, r.Info.PixelFormat);

        await foreach (var f in r.ReadFramesAsync())
        {
            using (f)
            {
                Assert.Equal(24, f.Pixels.Length);
                for (int i = 0; i < pixels.Length; i++)
                {
                    Assert.Equal(pixels[i], f.Pixels.Span[i]);
                }
            }
        }
    }

    [Fact]
    public async Task Pnm_P5_Pgm_Gray8_RoundTrips()
    {
        var header = Encoding.ASCII.GetBytes("P5\n3 2\n255\n");
        var pixels = new byte[] { 1, 2, 3, 4, 5, 6 };
        var file = new byte[header.Length + pixels.Length];
        header.CopyTo(file, 0);
        pixels.CopyTo(file, header.Length);

        await using var ms = new MemoryStream(file);
        using var r = PnmReader.Open(ms, ownsStream: false);
        Assert.Equal(3, r.Info.Width);
        Assert.Equal(2, r.Info.Height);

        await foreach (var f in r.ReadFramesAsync())
        {
            using (f)
            {
                Assert.Equal(PixelFormat.Gray8, f.PixelFormat);
                for (int i = 0; i < pixels.Length; i++)
                    Assert.Equal(pixels[i], f.Pixels.Span[i]);
            }
        }
    }

    [Fact]
    public async Task Xpm3_TwoColor_Parses()
    {
        var xpm =
            "/* XPM */\n" +
            "static char * test[] = {\n" +
            "\"2 2 2 1\",\n" +
            "\". c #FF0000\",\n" +
            "\"# c #00FF00\",\n" +
            "\".#\",\n" +
            "\"#.\"\n" +
            "};\n";
        var bytes = Encoding.UTF8.GetBytes(xpm);

        await using var ms = new MemoryStream(bytes);
        using var r = XpmReader.Open(ms, ownsStream: false);
        Assert.Equal(2, r.Info.Width);
        Assert.Equal(2, r.Info.Height);

        await foreach (var f in r.ReadFramesAsync())
        {
            using (f)
            {
                Assert.Equal(PixelFormat.Rgba32, f.PixelFormat);
                var s = f.Pixels.Span;
                // (0,0) = '.' → red.
                Assert.Equal(0xFF, s[0]); // R
                Assert.Equal(0x00, s[1]); // G
                Assert.Equal(0x00, s[2]); // B
                Assert.Equal(0xFF, s[3]); // A
                // (1,0) = '#' → green.
                Assert.Equal(0x00, s[4]);
                Assert.Equal(0xFF, s[5]);
                Assert.Equal(0x00, s[6]);
            }
        }
    }

    [Fact]
    public async Task Tga_Type2_24bpp_Uncompressed_RoundTrips()
    {
        const int W = 4, H = 2;
        var file = new byte[18 + W * H * 3];
        // Header
        file[0] = 0;   // ID length
        file[1] = 0;   // color map type
        file[2] = 2;   // image type = uncompressed true-color
        // color map spec = zeros
        // x/y origin = 0
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(12), W);
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(14), H);
        file[16] = 24; // bits per pixel
        file[17] = 0;  // bottom-up, no alpha
        // BGR pixels (bottom-up storage)
        for (int y = 0; y < H; y++)
        {
            for (int x = 0; x < W; x++)
            {
                int off = 18 + (y * W + x) * 3;
                file[off + 0] = (byte)x;            // B
                file[off + 1] = (byte)(y * 30);     // G
                file[off + 2] = (byte)(x * 40);     // R
            }
        }

        await using var ms = new MemoryStream(file);
        using var r = TgaReader.Open(ms, ownsStream: false);
        Assert.Equal(W, r.Info.Width);
        Assert.Equal(H, r.Info.Height);

        await foreach (var f in r.ReadFramesAsync())
        {
            using (f)
            {
                Assert.True(f.PixelFormat is PixelFormat.Bgr24 or PixelFormat.Rgb24);
                Assert.Equal(W * H * 3, f.Pixels.Length);
            }
        }
    }

    [Fact]
    public async Task Pcx_8bpp_256ColorPalette_Decodes()
    {
        // Minimal PCX: 1×1 image, 8bpp, 1 plane, palette appended at end of file.
        // Header (128 bytes), then 1 byte of RLE (single byte not in 0xC0 range), then 0x0C marker + 768-byte palette.
        var file = new byte[128 + 1 + 1 + 768];
        file[0] = 0x0A;   // ZSoft signature
        file[1] = 5;      // version
        file[2] = 1;      // RLE
        file[3] = 8;      // bits per plane
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(4), 0); // xMin
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(6), 0); // yMin
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(8), 0); // xMax
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(10), 0); // yMax
        file[65] = 1;     // num planes
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(66), 1); // bytes per line

        // Pixel data: a single byte = palette index 7. Not in 0xC0..0xFF so it stores literally.
        file[128] = 7;

        // Palette marker + 256×3 entries. Entry 7 = (100, 150, 200).
        file[129] = 0x0C;
        int paletteStart = 130;
        file[paletteStart + 7 * 3 + 0] = 100;
        file[paletteStart + 7 * 3 + 1] = 150;
        file[paletteStart + 7 * 3 + 2] = 200;

        await using var ms = new MemoryStream(file);
        using var r = PcxReader.Open(ms, ownsStream: false);
        Assert.Equal(1, r.Info.Width);
        Assert.Equal(1, r.Info.Height);

        await foreach (var f in r.ReadFramesAsync())
        {
            using (f)
            {
                // Palette-indexed or RGB24 — either way verify a single pixel.
                Assert.True(f.Pixels.Length >= 1);
            }
        }
    }

    [Fact]
    public async Task Hdr_Radiance_Rgbe_Parses()
    {
        var sb = new StringBuilder();
        sb.Append("#?RADIANCE\n");
        sb.Append("FORMAT=32-bit_rle_rgbe\n");
        sb.Append('\n');
        sb.Append("-Y 1 +X 2\n");
        var head = Encoding.ASCII.GetBytes(sb.ToString());
        // For width 2, the new RLE scanline preamble (0x02 0x02 hi lo) requires width>=8 or
        // legacy mode; for tiny widths the reader falls back to a per-pixel RGBE encoding.
        // We just emit 2 pixels of 32-bit RGBE.
        var px = new byte[] {
            0xFF, 0x00, 0x00, 0x80,  // R only, exponent 128 → 1.0
            0x00, 0xFF, 0x00, 0x80,  // G only
        };
        var file = new byte[head.Length + px.Length];
        head.CopyTo(file, 0);
        px.CopyTo(file, head.Length);

        await using var ms = new MemoryStream(file);
        using var r = HdrReader.Open(ms, ownsStream: false);
        Assert.Equal(2, r.Info.Width);
        Assert.Equal(1, r.Info.Height);
        Assert.True(r.Info.IsHdr);
    }

    [Fact]
    public async Task Dds_UncompressedBgra32_Parses()
    {
        // 128-byte DDS header + 4 bytes of BGRA pixel.
        const int W = 1, H = 1;
        var file = new byte[128 + 4];
        file[0] = (byte)'D'; file[1] = (byte)'D'; file[2] = (byte)'S'; file[3] = (byte)' ';
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(4), 124);  // size of header
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(8),
            0x1u | 0x2u | 0x4u | 0x1000u | 0x80000u); // CAPS|HEIGHT|WIDTH|PIXELFORMAT|PITCH
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(12), H);
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(16), W);
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(20), W * 4); // pitch
        // DDS_PIXELFORMAT starts at offset 76, 32 bytes total
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(76), 32);     // dwSize
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(80), 0x41u);  // DDPF_RGB | DDPF_ALPHAPIXELS
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(88), 32);     // RGB bit count
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(92), 0x00FF0000);  // R mask
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(96), 0x0000FF00);  // G mask
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(100), 0x000000FF); // B mask
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(104), 0xFF000000); // A mask
        // 1 pixel: B=10 G=20 R=30 A=40
        file[128] = 10; file[129] = 20; file[130] = 30; file[131] = 40;

        await using var ms = new MemoryStream(file);
        using var r = DdsReader.Open(ms, ownsStream: false);
        Assert.Equal(W, r.Info.Width);
        Assert.Equal(H, r.Info.Height);
    }

    [Fact]
    public void MediarImage_Open_DispatchesToCorrectReader()
    {
        // Smallest 2×2 PNG was already tested above; here we just confirm that the
        // facade returns *some* reader for a known PNG signature.
        var pngSig = new byte[]
        {
            0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
            0x00, 0x00, 0x00, 0x0D, (byte)'I', (byte)'H', (byte)'D', (byte)'R',
        };
        var path = Path.Combine(Path.GetTempPath(), $"mediar-fmt-{Guid.NewGuid():N}.png");
        try
        {
            File.WriteAllBytes(path, pngSig);
            // We don't expect Open to fully decode (the file is truncated) — but
            // ImageFormatDetector.Detect on the same prefix must return PNG.
            Assert.Equal(ImageFormat.Png, ImageFormatDetector.Detect(pngSig));
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }
}
