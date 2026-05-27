using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests;

public class JpegLosslessDecoderTests
{
    [Fact]
    public async Task SingleComponent8Bit_Predictor1_RoundTrips()
    {
        int w = 32, h = 24;
        var pixels = new byte[h, w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                pixels[y, x] = (byte)((x * 7 + y * 3) & 0xFF);

        byte[] jpeg = TestLosslessEncoder.Encode(pixels, predictorSelector: 1);

        var decoded = await DecodeGrayAsync(jpeg);

        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                Assert.Equal(pixels[y, x], decoded[y, x]);
    }

    [Fact]
    public async Task SingleComponent8Bit_Predictor6_RoundTrips()
    {
        int w = 16, h = 16;
        var pixels = new byte[h, w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                pixels[y, x] = (byte)((x + y) * 8 & 0xFF);

        byte[] jpeg = TestLosslessEncoder.Encode(pixels, predictorSelector: 6);

        var decoded = await DecodeGrayAsync(jpeg);

        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                Assert.Equal(pixels[y, x], decoded[y, x]);
    }

    [Fact]
    public async Task SingleComponent12Bit_Predictor1_RoundTrips()
    {
        int w = 16, h = 8;
        var pixels = new ushort[h, w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                pixels[y, x] = (ushort)((x * 137 + y * 211) & 0x0FFF);

        byte[] jpeg = TestLosslessEncoder.Encode16(pixels, precision: 12, predictorSelector: 1);

        var decoded = await DecodeGray16Async(jpeg);

        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                Assert.Equal(pixels[y, x], decoded[y, x]);
    }

    [Fact]
    public async Task SingleComponent8Bit_WithRestartInterval_RoundTrips()
    {
        int w = 32, h = 8;
        var pixels = new byte[h, w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                pixels[y, x] = (byte)(((x * 5) ^ (y * 11)) & 0xFF);

        // Restart every 16 samples → forces multiple RST markers within scan.
        byte[] jpeg = TestLosslessEncoder.Encode(pixels, predictorSelector: 1, restartInterval: 16);

        var decoded = await DecodeGrayAsync(jpeg);

        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                Assert.Equal(pixels[y, x], decoded[y, x]);
    }

    private static async Task<byte[,]> DecodeGrayAsync(byte[] jpegBytes)
    {
        await using var ms = new MemoryStream(jpegBytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Gray8, frame!.PixelFormat);

        int w = reader.Info.Width;
        int h = reader.Info.Height;
        var pixels = new byte[h, w];
        var data = frame.Pixels.Span;
        int stride = frame.Stride;
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                pixels[y, x] = data[y * stride + x];
        frame.Dispose();
        return pixels;
    }

    private static async Task<ushort[,]> DecodeGray16Async(byte[] jpegBytes)
    {
        await using var ms = new MemoryStream(jpegBytes);
        using var reader = JpegReader.Open(ms, ownsStream: false);
        Assert.True(reader.CanDecodePixels);

        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            frame = f;
            break;
        }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Gray16, frame!.PixelFormat);

        int w = reader.Info.Width;
        int h = reader.Info.Height;
        var pixels = new ushort[h, w];
        var data = frame.Pixels.Span;
        int stride = frame.Stride;
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
            {
                int o = y * stride + x * 2;
                pixels[y, x] = (ushort)(data[o] | (data[o + 1] << 8));
            }
        frame.Dispose();
        return pixels;
    }
}

/// <summary>
/// Test-only encoder that produces minimal but spec-conformant lossless JPEG
/// (SOF3) streams. Uses a fixed Huffman table tuned for natural images
/// (libjpeg's default) and the predictor selector chosen by the caller.
/// </summary>
internal static class TestLosslessEncoder
{
    public static byte[] Encode(byte[,] pixels, int predictorSelector, int restartInterval = 0)
    {
        int h = pixels.GetLength(0);
        int w = pixels.GetLength(1);
        var samples = new int[h * w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                samples[y * w + x] = pixels[y, x];
        return Build(samples, w, h, precision: 8, predictorSelector, restartInterval);
    }

    public static byte[] Encode16(ushort[,] pixels, int precision, int predictorSelector, int restartInterval = 0)
    {
        int h = pixels.GetLength(0);
        int w = pixels.GetLength(1);
        var samples = new int[h * w];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                samples[y * w + x] = pixels[y, x];
        return Build(samples, w, h, precision, predictorSelector, restartInterval);
    }

    private static byte[] Build(int[] samples, int w, int h, int precision, int predictorSelector, int restartInterval)
    {
        using var ms = new MemoryStream();
        ms.WriteByte(0xFF); ms.WriteByte(0xD8); // SOI

        // SOF3 — lossless, single component.
        ms.WriteByte(0xFF); ms.WriteByte(0xC3);
        WriteUInt16BE(ms, 11);                 // 2 + 1 + 2 + 2 + 1 + 3 = 11
        ms.WriteByte((byte)precision);
        WriteUInt16BE(ms, (ushort)h);
        WriteUInt16BE(ms, (ushort)w);
        ms.WriteByte(1);                       // Nf
        ms.WriteByte(1);                       // component id
        ms.WriteByte(0x11);                    // sampling 1×1
        ms.WriteByte(0);                       // qtab (unused for lossless)

        // DRI if requested.
        if (restartInterval > 0)
        {
            ms.WriteByte(0xFF); ms.WriteByte(0xDD);
            WriteUInt16BE(ms, 4);
            WriteUInt16BE(ms, (ushort)restartInterval);
        }

        // DHT — single DC Huffman table (tc=0, th=0). Use a canonical table that
        // covers all magnitude categories 0..16 with reasonable code lengths.
        // Layout: counts[16], then values in length-order.
        // We use libjpeg's default DC luminance distribution stretched to cover
        // category 16 (which lossless uses as a special escape per H.1.2.2).
        WriteDht(ms, LosslessDcBits, LosslessDcValues);

        // SOS — Ns=1, predictor selector = Ss, Se=0, Ah=0, Al=0 (no point xform).
        ms.WriteByte(0xFF); ms.WriteByte(0xDA);
        WriteUInt16BE(ms, 8);
        ms.WriteByte(1);                       // Ns
        ms.WriteByte(1);                       // component id
        ms.WriteByte(0x00);                    // Td=0, Ta=0
        ms.WriteByte((byte)predictorSelector); // Ss = predictor selector (1..7)
        ms.WriteByte(0);                       // Se = 0
        ms.WriteByte(0x00);                    // Ah=0, Al=0 (Pt = 0)

        // Entropy data.
        var bw = new BitWriter(ms);
        int initialPrediction = 1 << (precision - 1);
        int sampleMask = (1 << precision) - 1;
        int rstNext = 0;
        int restartCount = 0;
        bool justRestarted = true;

        for (int y = 0; y < h; y++)
        {
            for (int x = 0; x < w; x++)
            {
                int px;
                if (justRestarted)
                {
                    px = initialPrediction;
                    justRestarted = false;
                }
                else if (y == 0)
                {
                    px = samples[y * w + x - 1] & sampleMask;
                }
                else if (x == 0)
                {
                    px = samples[(y - 1) * w] & sampleMask;
                }
                else
                {
                    int ra = samples[y * w + x - 1];
                    int rb = samples[(y - 1) * w + x];
                    int rc = samples[(y - 1) * w + x - 1];
                    px = predictorSelector switch
                    {
                        1 => ra,
                        2 => rb,
                        3 => rc,
                        4 => ra + rb - rc,
                        5 => ra + ((rb - rc) >> 1),
                        6 => rb + ((ra - rc) >> 1),
                        7 => (ra + rb) >> 1,
                        _ => ra,
                    } & sampleMask;
                }

                int actual = samples[y * w + x] & sampleMask;
                int diff = actual - px;
                // Wrap diff into [-2^(P-1), 2^(P-1)-1] for the mod-2^P arithmetic
                // that the decoder will undo via (px + diff) & sampleMask.
                int half = 1 << (precision - 1);
                if (diff > half - 1) diff -= 1 << precision;
                else if (diff < -half) diff += 1 << precision;

                EncodeDiff(bw, diff, precision);

                if (restartInterval > 0)
                {
                    restartCount++;
                    if (restartCount >= restartInterval &&
                        !(y == h - 1 && x == w - 1))
                    {
                        bw.Flush();
                        ms.WriteByte(0xFF);
                        ms.WriteByte((byte)(0xD0 + (rstNext & 7)));
                        rstNext++;
                        restartCount = 0;
                        justRestarted = true;
                    }
                }
            }
        }
        bw.Flush();

        ms.WriteByte(0xFF); ms.WriteByte(0xD9); // EOI
        return ms.ToArray();
    }

    private static void EncodeDiff(BitWriter bw, int diff, int precision)
    {
        int s;
        int v;
        if (diff == 0)
        {
            s = 0;
            v = 0;
        }
        else if (diff == 32768 && precision == 16)
        {
            // Special escape from H.1.2.2.
            s = 16;
            v = 0;
            EmitHuffmanSymbol(bw, s);
            return;
        }
        else
        {
            int abs = diff < 0 ? -diff : diff;
            s = 0;
            int t = abs;
            while (t > 0) { s++; t >>= 1; }
            int mask = (1 << s) - 1;
            v = diff < 0 ? (diff - 1) & mask : diff & mask;
        }

        EmitHuffmanSymbol(bw, s);
        if (s > 0 && s < 16) bw.WriteBits(v, s);
    }

    private static void EmitHuffmanSymbol(BitWriter bw, int s)
    {
        ushort code = LosslessDcCodes[s].Code;
        int len = LosslessDcCodes[s].Length;
        bw.WriteBits(code, len);
    }

    private static void WriteDht(MemoryStream ms, byte[] bits, byte[] vals)
    {
        ms.WriteByte(0xFF); ms.WriteByte(0xC4);
        int len = 2 + 1 + 16 + vals.Length;
        WriteUInt16BE(ms, (ushort)len);
        ms.WriteByte(0x00); // tc=0 (DC), th=0
        ms.Write(bits, 0, 16);
        ms.Write(vals, 0, vals.Length);
    }

    private static void WriteUInt16BE(MemoryStream ms, ushort v)
    {
        ms.WriteByte((byte)(v >> 8));
        ms.WriteByte((byte)v);
    }

    // 16 length counts (lengths 1..16). Custom canonical Huffman that covers
    // categories 0..12 with short codes (typical lossless diff magnitudes)
    // plus category 16 (the rare H.1.2.2 escape) with a 16-bit code.
    private static readonly byte[] LosslessDcBits =
        [0, 1, 5, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 1];
    private static readonly byte[] LosslessDcValues =
        [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 16];

    private static readonly (ushort Code, int Length)[] LosslessDcCodes = BuildCanonicalCodes();

    private static (ushort, int)[] BuildCanonicalCodes()
    {
        var lookup = new (ushort, int)[17];
        int code = 0;
        int valIdx = 0;
        for (int l = 1; l <= 16; l++)
        {
            for (int i = 0; i < LosslessDcBits[l - 1]; i++)
            {
                int sym = LosslessDcValues[valIdx++];
                lookup[sym] = ((ushort)code, l);
                code++;
            }
            code <<= 1;
        }
        return lookup;
    }

    private sealed class BitWriter
    {
        private readonly MemoryStream _ms;
        private uint _buf;
        private int _bits;

        public BitWriter(MemoryStream ms) { _ms = ms; }

        public void WriteBit(int b)
        {
            _buf = (_buf << 1) | ((uint)b & 1);
            _bits++;
            WhileFull();
        }

        public void WriteBits(int v, int n)
        {
            if (n == 0) return;
            _buf = (_buf << n) | ((uint)v & ((1u << n) - 1));
            _bits += n;
            WhileFull();
        }

        private void WhileFull()
        {
            while (_bits >= 8)
            {
                byte by = (byte)((_buf >> (_bits - 8)) & 0xFF);
                _bits -= 8;
                _ms.WriteByte(by);
                if (by == 0xFF) _ms.WriteByte(0x00);
            }
        }

        public void Flush()
        {
            if (_bits > 0)
            {
                int pad = 8 - _bits;
                _buf = (_buf << pad) | ((1u << pad) - 1);
                _bits = 8;
                WhileFull();
            }
            _buf = 0;
            _bits = 0;
        }
    }
}
