using Mediar.Codecs.Lzw;
using Mediar.Imaging;
using Mediar.Imaging.Gif;
using Mediar.Imaging.Hdr;
using Mediar.Imaging.Icns;
using Mediar.Imaging.Pcx;
using Mediar.Imaging.Pnm;
using Mediar.Imaging.Tga;
using Mediar.Imaging.Tiff;
using Mediar.Imaging.Xpm;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Round-trip + writer-guard coverage for the family of image writers added
/// alongside the existing BMP / PNG writers: PNM, TGA, HDR, PCX, XPM, ICNS,
/// TIFF, GIF. Each writer is exercised against the matching reader to ensure
/// the output is decodable and pixel-identical (where the format allows it).
/// </summary>
public sealed class ImagingNewWritersTests
{
    // ---------- PNM (PBM / PGM / PPM raw) ----------

    [Fact]
    public async Task Pnm_writes_and_reads_back_Gray8_pgm()
    {
        var pixels = MakeRamp(8, 6, 1);
        using var frame = new ImageFrame(8, 6, PixelFormat.Gray8, 8, pixels);
        var ms = new MemoryStream();
        await using (var w = new PnmWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = PnmReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Gray8));
    }

    [Fact]
    public async Task Pnm_writes_and_reads_back_Rgb24_ppm()
    {
        var pixels = MakeRamp(4, 3, 3);
        using var frame = new ImageFrame(4, 3, PixelFormat.Rgb24, 12, pixels);
        var ms = new MemoryStream();
        await using (var w = new PnmWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = PnmReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Rgb24));
    }

    [Fact]
    public async Task Pnm_writer_rejects_unsupported_format()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Rgba32, 8, new byte[16]);
        await using var w = new PnmWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Pnm_writer_rejects_second_frame()
    {
        var pixels = new byte[8];
        using var f1 = new ImageFrame(4, 2, PixelFormat.Gray8, 4, pixels);
        using var f2 = new ImageFrame(4, 2, PixelFormat.Gray8, 4, pixels);
        await using var w = new PnmWriter(new MemoryStream());
        await w.WriteFrameAsync(f1);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(f2));
    }

    [Fact]
    public void Pnm_writer_validates_constructor()
    {
        Assert.Throws<ArgumentNullException>(() => new PnmWriter(null!));
    }

    // ---------- TGA ----------

    [Fact]
    public async Task Tga_round_trips_uncompressed_Bgra32()
    {
        var pixels = MakeRamp(6, 4, 4);
        using var frame = new ImageFrame(6, 4, PixelFormat.Bgra32, 24, pixels);

        var ms = new MemoryStream();
        await using (var w = new TgaWriter(ms, ownsStream: false))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = TgaReader.Open(ms);
        byte[] decoded = RoundTripBytes(reader, PixelFormat.Bgra32);
        Assert.Equal(pixels, decoded);
    }

    [Fact]
    public async Task Tga_round_trips_rle_Gray8()
    {
        // Pattern with long runs to exercise the RLE branch.
        var pixels = new byte[64];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i / 8);
        using var frame = new ImageFrame(8, 8, PixelFormat.Gray8, 8, pixels);
        var ms = new MemoryStream();
        await using (var w = new TgaWriter(ms, ownsStream: false, compression: TgaCompression.Rle))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = TgaReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Gray8));
    }

    [Fact]
    public async Task Tga_writer_rejects_second_frame()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Bgr24, 6, new byte[12]);
        using var f2 = new ImageFrame(2, 2, PixelFormat.Bgr24, 6, new byte[12]);
        await using var w = new TgaWriter(new MemoryStream());
        await w.WriteFrameAsync(frame);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(f2));
    }

    // ---------- HDR ----------

    [Fact]
    public async Task Hdr_round_trips_Rgb96Float()
    {
        var pixels = new byte[16 * 8 * 12]; // Rgb96Float = 12 bytes per pixel
        Random rnd = new(1234);
        for (int i = 0; i < pixels.Length; i += 4)
        {
            float v = (float)(rnd.NextDouble() * 4.0);
            BitConverter.GetBytes(v).CopyTo(pixels, i);
        }
        using var frame = new ImageFrame(16, 8, PixelFormat.Rgb96Float, 16 * 12, pixels);
        var ms = new MemoryStream();
        await using (var w = new HdrWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = HdrReader.Open(ms);
        byte[] decoded = RoundTripBytes(reader, PixelFormat.Rgb96Float);

        // HDR is lossy: the 8-bit shared exponent reduces each channel to ~1
        // part in 256 of the per-pixel maximum. Verify relative agreement.
        var srcF = System.Runtime.InteropServices.MemoryMarshal.Cast<byte, float>(pixels);
        var dstF = System.Runtime.InteropServices.MemoryMarshal.Cast<byte, float>(decoded);
        Assert.Equal(srcF.Length, dstF.Length);
        for (int i = 0; i < srcF.Length; i++)
        {
            float tol = Math.Max(0.02f, Math.Abs(srcF[i]) * 0.02f);
            Assert.InRange(dstF[i] - srcF[i], -tol, tol);
        }
    }

    [Fact]
    public async Task Hdr_writer_rejects_non_float_format()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Rgb24, 6, new byte[12]);
        await using var w = new HdrWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    // ---------- PCX ----------

    [Fact]
    public async Task Pcx_round_trips_Rgb24()
    {
        var pixels = MakeRamp(8, 4, 3);
        using var frame = new ImageFrame(8, 4, PixelFormat.Rgb24, 24, pixels);
        var ms = new MemoryStream();
        await using (var w = new PcxWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = PcxReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Rgb24));
    }

    [Fact]
    public async Task Pcx_round_trips_Indexed8_palette()
    {
        var palette = new uint[256];
        for (int i = 0; i < 256; i++)
            palette[i] = 0xFF000000u | ((uint)(255 - i) << 16) | ((uint)i << 8) | (uint)(i ^ 0x55); // R in low byte
        var pixels = new byte[16 * 6];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i % 256);
        using var frame = new ImageFrame(16, 6, PixelFormat.Indexed8, 16, pixels, palette: palette);
        var ms = new MemoryStream();
        await using (var w = new PcxWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = PcxReader.Open(ms);
        var (decoded, decodedPalette) = RoundTripBytesAndPalette(reader, PixelFormat.Indexed8);
        Assert.Equal(pixels, decoded);
        Assert.Equal(palette, decodedPalette.ToArray());
    }

    [Fact]
    public async Task Pcx_writer_rejects_unsupported_format()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Rgba32, 8, new byte[16]);
        await using var w = new PcxWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    // ---------- XPM ----------

    [Fact]
    public async Task Xpm_round_trips_small_palette()
    {
        // 4 distinct colors, easy to round-trip via XPM.
        byte[] rgb = new byte[]
        {
            // row 0: R, G, B, R
            255,0,0,  0,255,0,  0,0,255,  255,0,0,
            // row 1: B, R, G, B
            0,0,255,  255,0,0,  0,255,0,  0,0,255,
        };
        using var frame = new ImageFrame(4, 2, PixelFormat.Rgb24, 12, rgb);
        var ms = new MemoryStream();
        await using (var w = new XpmWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = XpmReader.Open(ms);
        // XpmReader returns Rgba32; rebuild Rgb24 from those pixels for comparison.
        byte[] decoded = RoundTripBytes(reader, PixelFormat.Rgba32);
        Assert.Equal(rgb.Length, decoded.Length / 4 * 3);
        for (int i = 0, j = 0; i < decoded.Length; i += 4, j += 3)
        {
            Assert.Equal(rgb[j + 0], decoded[i + 0]); // R
            Assert.Equal(rgb[j + 1], decoded[i + 1]); // G
            Assert.Equal(rgb[j + 2], decoded[i + 2]); // B
        }
    }

    [Fact]
    public async Task Xpm_writer_rejects_unsupported_format()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Gray16, 4, new byte[8]);
        await using var w = new XpmWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    // ---------- ICNS ----------

    [Fact]
    public async Task Icns_writer_emits_decodable_icp4_entry()
    {
        // 16x16 RGBA frame → icp4 (16x16) entry encoded as PNG.
        var pixels = MakeRamp(16, 16, 4);
        using var frame = new ImageFrame(16, 16, PixelFormat.Rgba32, 64, pixels);

        var ms = new MemoryStream();
        await using (var w = new IcnsWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        Assert.True(ms.Length > 8);
        ms.Position = 0;
        var header = new byte[8];
        Assert.Equal(8, await ms.ReadAsync(header));
        Assert.Equal((byte)'i', header[0]);
        Assert.Equal((byte)'c', header[1]);
        Assert.Equal((byte)'n', header[2]);
        Assert.Equal((byte)'s', header[3]);
        // Total length recorded in header matches stream length.
        uint total = (uint)header[4] << 24 | (uint)header[5] << 16 | (uint)header[6] << 8 | header[7];
        Assert.Equal((uint)ms.Length, total);

        // The icp4 payload is a complete PNG file. IcnsReader hands us the raw
        // payload bytes via PixelFormat.Unknown; decode them with PngReader
        // and verify the round-trip is byte-identical.
        ms.Position = 0;
        byte[] pngBytes;
        using (var reader = IcnsReader.Open(ms))
        {
            var en = reader.ReadFramesAsync().GetAsyncEnumerator();
            try
            {
                Assert.True(await en.MoveNextAsync());
                var f = en.Current;
                pngBytes = f.Pixels.ToArray();
                f.Dispose();
            }
            finally { await en.DisposeAsync(); }
        }

        using var pngStream = new MemoryStream(pngBytes);
        using var pngReader = Mediar.Imaging.Png.PngReader.Open(pngStream);
        byte[] decoded = RoundTripBytes(pngReader, PixelFormat.Rgba32);
        Assert.Equal(pixels, decoded);
    }

    [Fact]
    public async Task Icns_writer_rejects_non_standard_size()
    {
        using var frame = new ImageFrame(17, 17, PixelFormat.Rgba32, 17 * 4, new byte[17 * 17 * 4]);
        await using var w = new IcnsWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Icns_writer_requires_at_least_one_frame()
    {
        await using var w = new IcnsWriter(new MemoryStream());
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.FinishAsync());
    }

    // ---------- TIFF ----------

    [Theory]
    [InlineData(TiffWriterCompression.None)]
    [InlineData(TiffWriterCompression.Deflate)]
    [InlineData(TiffWriterCompression.PackBits)]
    [InlineData(TiffWriterCompression.Lzw)]
    public async Task Tiff_round_trips_Rgba32_under_all_compressions(TiffWriterCompression c)
    {
        var pixels = MakeRamp(12, 10, 4);
        using var frame = new ImageFrame(12, 10, PixelFormat.Rgba32, 12 * 4, pixels);
        var ms = new MemoryStream();
        await using (var w = new TiffWriter(ms, ownsStream: false, compression: c))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = TiffReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Rgba32));
    }

    [Fact]
    public async Task Tiff_round_trips_Gray16_deflate()
    {
        var pixels = new byte[6 * 4 * 2];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i * 37);
        using var frame = new ImageFrame(6, 4, PixelFormat.Gray16, 6 * 2, pixels);
        var ms = new MemoryStream();
        await using (var w = new TiffWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }
        ms.Position = 0;
        using var reader = TiffReader.Open(ms);
        Assert.Equal(pixels, RoundTripBytes(reader, PixelFormat.Gray16));
    }

    [Fact]
    public async Task Tiff_writer_rejects_unsupported_format()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Indexed8, 2, new byte[4]);
        await using var w = new TiffWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Tiff_writer_rejects_second_frame()
    {
        using var f = new ImageFrame(2, 2, PixelFormat.Rgb24, 6, new byte[12]);
        using var f2 = new ImageFrame(2, 2, PixelFormat.Rgb24, 6, new byte[12]);
        await using var w = new TiffWriter(new MemoryStream());
        await w.WriteFrameAsync(f);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(f2));
    }

    // ---------- GIF ----------

    [Fact]
    public async Task Gif_round_trips_Indexed8_through_decoder()
    {
        var palette = new uint[16];
        for (int i = 0; i < 16; i++)
            palette[i] = 0xFF000000u | ((uint)(255 - i * 16) << 16) | ((uint)(i * 16) << 8) | (uint)(i * 16); // R in low byte
        var pixels = new byte[10 * 6];
        for (int i = 0; i < pixels.Length; i++) pixels[i] = (byte)(i % 16);
        using var frame = new ImageFrame(10, 6, PixelFormat.Indexed8, 10, pixels, palette: palette);

        var ms = new MemoryStream();
        await using (var w = new GifWriter(ms))
        {
            await w.WriteFrameAsync(frame);
            await w.FinishAsync();
        }

        // Verify the file structure: GIF89a header + 0x3B trailer.
        ms.Position = 0;
        byte[] head = new byte[6];
        Assert.Equal(6, await ms.ReadAsync(head));
        Assert.Equal("GIF89a", System.Text.Encoding.ASCII.GetString(head));
        Assert.Equal(0x3B, ms.ToArray()[^1]);

        // Decode back and compare composited RGBA pixels to the expected palette lookup.
        ms.Position = 0;
        using var reader = GifReader.Open(ms);
        byte[] decoded = RoundTripBytes(reader, PixelFormat.Rgba32);
        Assert.Equal(10 * 6 * 4, decoded.Length);
        for (int p = 0; p < pixels.Length; p++)
        {
            uint expectedRgba = palette[pixels[p]];
            int o = p * 4;
            // GifReader produces (a<<24)|(b<<16)|(g<<8)|r, so R is in the low byte.
            Assert.Equal((byte)(expectedRgba & 0xFF),         decoded[o + 0]);
            Assert.Equal((byte)((expectedRgba >> 8) & 0xFF),  decoded[o + 1]);
            Assert.Equal((byte)((expectedRgba >> 16) & 0xFF), decoded[o + 2]);
            Assert.Equal((byte)((expectedRgba >> 24) & 0xFF), decoded[o + 3]);
        }
    }

    [Fact]
    public async Task Gif_writer_rejects_non_indexed_input()
    {
        using var frame = new ImageFrame(2, 2, PixelFormat.Rgb24, 6, new byte[12]);
        await using var w = new GifWriter(new MemoryStream());
        await Assert.ThrowsAsync<NotSupportedException>(async () => await w.WriteFrameAsync(frame));
    }

    [Fact]
    public async Task Gif_writer_rejects_second_frame()
    {
        var palette = new uint[2] { 0xFF000000u, 0xFFFFFFFFu };
        using var f = new ImageFrame(2, 2, PixelFormat.Indexed8, 2, new byte[4], palette: palette);
        using var f2 = new ImageFrame(2, 2, PixelFormat.Indexed8, 2, new byte[4], palette: palette);
        await using var w = new GifWriter(new MemoryStream());
        await w.WriteFrameAsync(f);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await w.WriteFrameAsync(f2));
    }

    // ---------- LZW encoder ↔ decoder round-trip ----------

    [Theory]
    [InlineData("simple")]
    [InlineData("zeros")]
    [InlineData("ramp")]
    [InlineData("random")]
    public void LzwEncoder_round_trips_through_decoder(string kind)
    {
        byte[] data = kind switch
        {
            "simple" => System.Text.Encoding.ASCII.GetBytes("TOBEORNOTTOBEORTOBEORNOT#"),
            "zeros" => new byte[2048],
            "ramp" => Enumerable.Range(0, 256).Select(i => (byte)i).ToArray(),
            "random" => RandomBytes(seed: 42, length: 5000),
            _ => throw new ArgumentException(kind),
        };
        byte[] gif = LzwEncoder.EncodeGif(data, 8);
        byte[] decGif = LzwDecoder.DecodeGif(gif, 8, data.Length);
        Assert.Equal(data, decGif);

        byte[] tiff = LzwEncoder.EncodeTiff(data);
        byte[] decTiff = LzwDecoder.DecodeTiff(tiff);
        Assert.Equal(data, decTiff);
    }

    [Fact]
    public void LzwEncoder_handles_empty_input()
    {
        byte[] gif = LzwEncoder.EncodeGif(Array.Empty<byte>(), 8);
        Assert.Equal(Array.Empty<byte>(), LzwDecoder.DecodeGif(gif, 8, 0));
        byte[] tiff = LzwEncoder.EncodeTiff(Array.Empty<byte>());
        Assert.Equal(Array.Empty<byte>(), LzwDecoder.DecodeTiff(tiff));
    }

    // ---------- helpers ----------

    private static byte[] MakeRamp(int width, int height, int channels)
    {
        var buf = new byte[width * height * channels];
        for (int i = 0; i < buf.Length; i++) buf[i] = (byte)((i * 17 + 19) & 0xFF);
        return buf;
    }

    private static byte[] RandomBytes(int seed, int length)
    {
        var rnd = new Random(seed);
        var b = new byte[length];
        rnd.NextBytes(b);
        return b;
    }

    private static byte[] RoundTripBytes(IImageReader reader, PixelFormat expected)
    {
        var enumerator = reader.ReadFramesAsync().GetAsyncEnumerator();
        try
        {
            Assert.True(enumerator.MoveNextAsync().AsTask().GetAwaiter().GetResult());
            var f = enumerator.Current;
            using (f)
            {
                Assert.Equal(expected, f.PixelFormat);
                int rowBytes = f.PixelFormat switch
                {
                    PixelFormat.Gray8 or PixelFormat.Indexed8 => f.Width,
                    PixelFormat.Gray16 => f.Width * 2,
                    PixelFormat.Rgb24 or PixelFormat.Bgr24 => f.Width * 3,
                    PixelFormat.Rgba32 or PixelFormat.Bgra32 => f.Width * 4,
                    PixelFormat.Rgb96Float => f.Width * 12,
                    _ => f.Stride,
                };
                byte[] tightlyPacked = new byte[rowBytes * f.Height];
                ReadOnlySpan<byte> src = f.Pixels.Span;
                for (int y = 0; y < f.Height; y++)
                    src.Slice(y * f.Stride, rowBytes).CopyTo(tightlyPacked.AsSpan(y * rowBytes));
                return tightlyPacked;
            }
        }
        finally
        {
            enumerator.DisposeAsync().AsTask().GetAwaiter().GetResult();
        }
    }

    private static (byte[] Pixels, ReadOnlyMemory<uint> Palette) RoundTripBytesAndPalette(PcxReader reader, PixelFormat expected)
    {
        var enumerator = reader.ReadFramesAsync().GetAsyncEnumerator();
        try
        {
            Assert.True(enumerator.MoveNextAsync().AsTask().GetAwaiter().GetResult());
            var f = enumerator.Current;
            using (f)
            {
                Assert.Equal(expected, f.PixelFormat);
                byte[] tightlyPacked = new byte[f.Width * f.Height];
                ReadOnlySpan<byte> src = f.Pixels.Span;
                for (int y = 0; y < f.Height; y++)
                    src.Slice(y * f.Stride, f.Width).CopyTo(tightlyPacked.AsSpan(y * f.Width));
                return (tightlyPacked, f.Palette.ToArray());
            }
        }
        finally
        {
            enumerator.DisposeAsync().AsTask().GetAwaiter().GetResult();
        }
    }
}
