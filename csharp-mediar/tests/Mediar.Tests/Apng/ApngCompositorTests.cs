using System.Buffers.Binary;
using System.IO.Compression;
using Mediar.Codecs.Apng;
using Mediar.Imaging;
using Mediar.Imaging.Png;
using Xunit;

namespace Mediar.Tests.Apng;

/// <summary>
/// Tests for <see cref="PngReader.ReadComposedFramesAsync"/> driving
/// <see cref="ApngCompositor"/> per the APNG specification's
/// <c>dispose_op</c> and <c>blend_op</c> rules.
/// </summary>
public sealed class ApngCompositorTests
{
    [Fact]
    public async Task NonApng_Png_Yields_Single_Frame()
    {
        byte[] png = TestApngBuilder.BuildStaticRgba32(2, 2, 0xFF, 0x00, 0x00, 0xFF);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        Assert.Equal(ImageFormat.Png, reader.Format);
        int count = 0;
        await foreach (var f in reader.ReadComposedFramesAsync())
        {
            using (f)
            {
                Assert.Equal(2, f.Width);
                Assert.Equal(2, f.Height);
                count++;
            }
        }
        Assert.Equal(1, count);
    }

    [Fact]
    public async Task Apng_With_Source_Blend_Yields_FullCanvas_Composition()
    {
        var f0 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 4, height: 4,
            delayNum: 100, delayDen: 1000,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(4, 4, 0xFF, 0x00, 0x00, 0xFF));
        var f1 = TestApngBuilder.Frame(
            xOffset: 2, yOffset: 2, width: 2, height: 2,
            delayNum: 100, delayDen: 1000,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0x00, 0xFF, 0x00, 0xFF));

        byte[] png = TestApngBuilder.BuildApng(4, 4, numPlays: 0, [f0, f1]);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        Assert.Equal(ImageFormat.Apng, reader.Format);

        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadComposedFramesAsync())
        {
            frames.Add(f);
        }
        Assert.Equal(2, frames.Count);
        try
        {
            Assert.All(frames, f =>
            {
                Assert.Equal(4, f.Width);
                Assert.Equal(4, f.Height);
                Assert.Equal(PixelFormat.Rgba32, f.PixelFormat);
            });
            var c0 = frames[0].Pixels.Span;
            AssertPixel(c0, 0, 0, 4, 0xFF, 0x00, 0x00, 0xFF);
            AssertPixel(c0, 3, 3, 4, 0xFF, 0x00, 0x00, 0xFF);

            var c1 = frames[1].Pixels.Span;
            AssertPixel(c1, 0, 0, 4, 0xFF, 0x00, 0x00, 0xFF);
            AssertPixel(c1, 3, 3, 4, 0x00, 0xFF, 0x00, 0xFF);
            AssertPixel(c1, 2, 2, 4, 0x00, 0xFF, 0x00, 0xFF);

            Assert.Equal(TimeSpan.FromMilliseconds(100), frames[0].Duration);
            Assert.Equal(TimeSpan.FromMilliseconds(100), frames[1].Duration);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public async Task Apng_With_Dispose_Background_Clears_FrameRect()
    {
        var f0 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 4, height: 4,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.Background, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(4, 4, 0xFF, 0xFF, 0xFF, 0xFF));
        var f1 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0xAA, 0xBB, 0xCC, 0xFF));

        byte[] png = TestApngBuilder.BuildApng(4, 4, numPlays: 0, [f0, f1]);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadComposedFramesAsync()) frames.Add(f);
        try
        {
            var c1 = frames[1].Pixels.Span;
            AssertPixel(c1, 0, 0, 4, 0xAA, 0xBB, 0xCC, 0xFF);
            AssertPixel(c1, 3, 3, 4, 0x00, 0x00, 0x00, 0x00);
            AssertPixel(c1, 0, 3, 4, 0x00, 0x00, 0x00, 0x00);
            AssertPixel(c1, 3, 0, 4, 0x00, 0x00, 0x00, 0x00);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public async Task Apng_With_Dispose_Previous_Restores_Region()
    {
        var f0 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 4, height: 4,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(4, 4, 0xFF, 0x00, 0x00, 0xFF));
        var f1 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.Previous, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0x00, 0xFF, 0x00, 0xFF));
        var f2 = TestApngBuilder.Frame(
            xOffset: 2, yOffset: 2, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0x00, 0x00, 0xFF, 0xFF));

        byte[] png = TestApngBuilder.BuildApng(4, 4, numPlays: 0, [f0, f1, f2]);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadComposedFramesAsync()) frames.Add(f);
        try
        {
            var c1 = frames[1].Pixels.Span;
            AssertPixel(c1, 0, 0, 4, 0x00, 0xFF, 0x00, 0xFF);
            AssertPixel(c1, 3, 3, 4, 0xFF, 0x00, 0x00, 0xFF);

            var c2 = frames[2].Pixels.Span;
            AssertPixel(c2, 0, 0, 4, 0xFF, 0x00, 0x00, 0xFF);
            AssertPixel(c2, 1, 1, 4, 0xFF, 0x00, 0x00, 0xFF);
            AssertPixel(c2, 2, 2, 4, 0x00, 0x00, 0xFF, 0xFF);
            AssertPixel(c2, 3, 3, 4, 0x00, 0x00, 0xFF, 0xFF);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public async Task Apng_With_Blend_Over_Composites_Alpha()
    {
        var f0 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0xFF, 0x00, 0x00, 0xFF));
        var f1 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Over,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0x00, 0xFF, 0x00, 0x80));

        byte[] png = TestApngBuilder.BuildApng(2, 2, numPlays: 0, [f0, f1]);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadComposedFramesAsync()) frames.Add(f);
        try
        {
            var c1 = frames[1].Pixels.Span;
            int red = c1[0];
            int green = c1[1];
            int blue = c1[2];
            int alpha = c1[3];
            Assert.InRange(red, 100, 160);
            Assert.InRange(green, 100, 160);
            Assert.Equal(0, blue);
            Assert.Equal(0xFF, alpha);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public async Task Apng_Skips_DefaultImage_Outside_Animation()
    {
        var f0 = TestApngBuilder.Frame(
            xOffset: 0, yOffset: 0, width: 2, height: 2,
            delayNum: 1, delayDen: 10,
            dispose: ApngDisposeOp.None, blend: ApngBlendOp.Source,
            pixels: TestApngBuilder.FillRgba32(2, 2, 0x00, 0xFF, 0x00, 0xFF));

        byte[] defaultImage = TestApngBuilder.FillRgba32(2, 2, 0xFF, 0x00, 0x00, 0xFF);
        byte[] png = TestApngBuilder.BuildApngWithDefaultImage(2, 2, defaultImage, numPlays: 0, [f0]);
        using var reader = PngReader.Open(new MemoryStream(png), ownsStream: true);
        Assert.Equal(ImageFormat.Apng, reader.Format);

        var frames = new List<ImageFrame>();
        await foreach (var f in reader.ReadComposedFramesAsync()) frames.Add(f);
        try
        {
            Assert.Single(frames);
            var c = frames[0].Pixels.Span;
            AssertPixel(c, 0, 0, 2, 0x00, 0xFF, 0x00, 0xFF);
        }
        finally
        {
            foreach (var f in frames) f.Dispose();
        }
    }

    [Fact]
    public void Compositor_Throws_For_Out_Of_Bounds_Frame()
    {
        var compositor = new ApngCompositor(4, 4);
        var src = new byte[4 * 4 * 4];
        Assert.Throws<ArgumentException>(() =>
            compositor.Render(src, 4 * 4, 4, 4,
                offsetX: 2, offsetY: 0,
                ApngBlendOp.Source, ApngDisposeOp.None));
    }

    [Fact]
    public void Compositor_Clear_Resets_Canvas_And_State()
    {
        var compositor = new ApngCompositor(2, 2);
        var src = TestApngBuilder.FillRgba32(2, 2, 0xFF, 0x00, 0x00, 0xFF);
        compositor.Render(src, 2 * 4, 2, 2, 0, 0, ApngBlendOp.Source, ApngDisposeOp.Background);
        compositor.Clear();
        var canvas = compositor.Canvas;
        for (int i = 0; i < canvas.Length; i++)
        {
            Assert.Equal(0, canvas[i]);
        }
    }

    private static void AssertPixel(ReadOnlySpan<byte> rgba, int x, int y, int width, byte r, byte g, byte b, byte a)
    {
        int o = (y * width + x) * 4;
        Assert.Equal(r, rgba[o + 0]);
        Assert.Equal(g, rgba[o + 1]);
        Assert.Equal(b, rgba[o + 2]);
        Assert.Equal(a, rgba[o + 3]);
    }
}

/// <summary>
/// Synthesises spec-conforming APNG byte streams for tests. Implements the
/// W3C PNG signature + IHDR + acTL + (default IDAT) + (fcTL + fdAT)+ + IEND
/// chunk grammar with correct CRC32 per the PNG specification.
/// </summary>
internal static class TestApngBuilder
{
    public static byte[] FillRgba32(int width, int height, byte r, byte g, byte b, byte a)
    {
        var pixels = new byte[width * height * 4];
        for (int i = 0; i < width * height; i++)
        {
            pixels[i * 4 + 0] = r;
            pixels[i * 4 + 1] = g;
            pixels[i * 4 + 2] = b;
            pixels[i * 4 + 3] = a;
        }
        return pixels;
    }

    public static ApngFrameSpec Frame(
        int xOffset, int yOffset, int width, int height,
        ushort delayNum, ushort delayDen,
        ApngDisposeOp dispose, ApngBlendOp blend,
        byte[] pixels)
    {
        return new ApngFrameSpec(xOffset, yOffset, width, height, delayNum, delayDen, dispose, blend, pixels);
    }

    public static byte[] BuildStaticRgba32(int width, int height, byte r, byte g, byte b, byte a)
    {
        byte[] pixels = FillRgba32(width, height, r, g, b, a);
        using var ms = new MemoryStream();
        WriteSignature(ms);
        WriteIhdr(ms, width, height);
        byte[] zRaw = ZlibCompress(BuildRawRows(pixels, width, height));
        WriteChunk(ms, "IDAT", zRaw);
        WriteChunk(ms, "IEND", []);
        return ms.ToArray();
    }

    public static byte[] BuildApng(int width, int height, int numPlays, ApngFrameSpec[] frames)
    {
        using var ms = new MemoryStream();
        WriteSignature(ms);
        WriteIhdr(ms, width, height);
        WriteActl(ms, numFrames: (uint)frames.Length, numPlays: (uint)numPlays);
        uint seq = 0;
        for (int i = 0; i < frames.Length; i++)
        {
            var f = frames[i];
            WriteFctl(ms, seq++, f);
            byte[] zRaw = ZlibCompress(BuildRawRows(f.Pixels, f.Width, f.Height));
            if (i == 0)
            {
                WriteChunk(ms, "IDAT", zRaw);
            }
            else
            {
                WriteFdat(ms, seq++, zRaw);
            }
        }
        WriteChunk(ms, "IEND", []);
        return ms.ToArray();
    }

    public static byte[] BuildApngWithDefaultImage(
        int width, int height, byte[] defaultPixels, int numPlays, ApngFrameSpec[] animationFrames)
    {
        using var ms = new MemoryStream();
        WriteSignature(ms);
        WriteIhdr(ms, width, height);
        WriteActl(ms, numFrames: (uint)animationFrames.Length, numPlays: (uint)numPlays);
        byte[] defaultRaw = ZlibCompress(BuildRawRows(defaultPixels, width, height));
        WriteChunk(ms, "IDAT", defaultRaw);
        uint seq = 0;
        foreach (var f in animationFrames)
        {
            WriteFctl(ms, seq++, f);
            byte[] zRaw = ZlibCompress(BuildRawRows(f.Pixels, f.Width, f.Height));
            WriteFdat(ms, seq++, zRaw);
        }
        WriteChunk(ms, "IEND", []);
        return ms.ToArray();
    }

    private static void WriteSignature(Stream s)
    {
        ReadOnlySpan<byte> sig = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
        s.Write(sig);
    }

    private static void WriteIhdr(Stream s, int width, int height)
    {
        var data = new byte[13];
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(0, 4), (uint)width);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(4, 4), (uint)height);
        data[8] = 8;
        data[9] = 6;
        data[10] = 0;
        data[11] = 0;
        data[12] = 0;
        WriteChunk(s, "IHDR", data);
    }

    private static void WriteActl(Stream s, uint numFrames, uint numPlays)
    {
        var data = new byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(0, 4), numFrames);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(4, 4), numPlays);
        WriteChunk(s, "acTL", data);
    }

    private static void WriteFctl(Stream s, uint sequenceNumber, ApngFrameSpec f)
    {
        var data = new byte[26];
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(0, 4), sequenceNumber);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(4, 4), (uint)f.Width);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(8, 4), (uint)f.Height);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(12, 4), (uint)f.XOffset);
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(16, 4), (uint)f.YOffset);
        BinaryPrimitives.WriteUInt16BigEndian(data.AsSpan(20, 2), f.DelayNum);
        BinaryPrimitives.WriteUInt16BigEndian(data.AsSpan(22, 2), f.DelayDen);
        data[24] = (byte)f.Dispose;
        data[25] = (byte)f.Blend;
        WriteChunk(s, "fcTL", data);
    }

    private static void WriteFdat(Stream s, uint sequenceNumber, byte[] zlibData)
    {
        var data = new byte[4 + zlibData.Length];
        BinaryPrimitives.WriteUInt32BigEndian(data.AsSpan(0, 4), sequenceNumber);
        zlibData.CopyTo(data, 4);
        WriteChunk(s, "fdAT", data);
    }

    private static byte[] BuildRawRows(byte[] pixels, int width, int height)
    {
        int rowBytes = width * 4;
        var raw = new byte[(rowBytes + 1) * height];
        for (int y = 0; y < height; y++)
        {
            int srcRow = y * rowBytes;
            int dstRow = y * (rowBytes + 1);
            raw[dstRow] = 0;
            Array.Copy(pixels, srcRow, raw, dstRow + 1, rowBytes);
        }
        return raw;
    }

    private static byte[] ZlibCompress(byte[] raw)
    {
        using var ms = new MemoryStream();
        using (var zs = new ZLibStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            zs.Write(raw, 0, raw.Length);
        }
        return ms.ToArray();
    }

    private static void WriteChunk(Stream s, string typeAscii, byte[] payload)
    {
        var lenBuf = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(lenBuf, (uint)payload.Length);
        s.Write(lenBuf);
        var type = System.Text.Encoding.ASCII.GetBytes(typeAscii);
        s.Write(type);
        s.Write(payload);
        uint crc = Crc32(0xFFFFFFFFu, type);
        crc = Crc32(crc, payload);
        crc ^= 0xFFFFFFFFu;
        var crcOut = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(crcOut, crc);
        s.Write(crcOut);
    }

    private static readonly uint[] s_crcTable = BuildCrcTable();

    private static uint[] BuildCrcTable()
    {
        var t = new uint[256];
        for (uint n = 0; n < 256; n++)
        {
            uint c = n;
            for (int k = 0; k < 8; k++)
            {
                c = (c & 1) != 0 ? 0xEDB88320u ^ (c >> 1) : c >> 1;
            }
            t[n] = c;
        }
        return t;
    }

    private static uint Crc32(uint seed, ReadOnlySpan<byte> data)
    {
        uint c = seed;
        for (int i = 0; i < data.Length; i++)
        {
            c = s_crcTable[(c ^ data[i]) & 0xFF] ^ (c >> 8);
        }
        return c;
    }
}

internal sealed record ApngFrameSpec(
    int XOffset, int YOffset, int Width, int Height,
    ushort DelayNum, ushort DelayDen,
    ApngDisposeOp Dispose, ApngBlendOp Blend,
    byte[] Pixels);
