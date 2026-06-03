using System.Buffers;
using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Baseline (SOF0) JPEG encoder per ITU-T Rec. T.81 (1992-09) Annex F.1.
/// Supports 8-bit grayscale and 8-bit RGB inputs and emits a JFIF
/// (Annex B) JPEG bitstream with optional Annex K standard tables,
/// optimised (K.2) Huffman tables, DRI restart markers, and embedded
/// EXIF / ICC / XMP metadata via <see cref="JpegMetadataWriter"/>.
/// </summary>
/// <remarks>
/// The pipeline is the canonical one described in Pennebaker &amp;
/// Mitchell (1992) ch. 7: RGB→YCbCr (Rec. 601), chroma subsampling,
/// block-level forward DCT (<see cref="JpegFdct"/>), divide-and-round
/// quantisation, zig-zag scan, DC differential + AC run-length Huffman
/// coding. All allocations after construction reuse a single
/// <see cref="ArrayPool{T}"/> rental per encode so that encoding a
/// frame is O(1) heap churn.
/// </remarks>
public static class JpegBaselineEncoder
{
    /// <summary>Encode <paramref name="frame"/> to <paramref name="output"/>.</summary>
    /// <exception cref="ArgumentNullException">A required argument is <c>null</c>.</exception>
    /// <exception cref="ArgumentException">The pixel format is not supported.</exception>
    public static void Encode(ImageFrame frame, Stream output, JpegEncodeOptions options)
    {
        ArgumentNullException.ThrowIfNull(frame);
        ArgumentNullException.ThrowIfNull(output);
        ArgumentNullException.ThrowIfNull(options);

        bool isGray = frame.PixelFormat == PixelFormat.Gray8;
        bool isRgb = frame.PixelFormat == PixelFormat.Rgb24;
        if (!isGray && !isRgb)
        {
            throw new ArgumentException(
                $"JpegBaselineEncoder supports Gray8 and Rgb24 only; got {frame.PixelFormat}.",
                nameof(frame));
        }

        int w = frame.Width;
        int h = frame.Height;
        int numComp = isGray ? 1 : 3;

        // Sampling factors.
        int hY = 1, vY = 1, hC = 1, vC = 1;
        if (!isGray)
        {
            switch (options.Subsampling)
            {
                case JpegSubsampling.Yuv444: hY = 1; vY = 1; hC = 1; vC = 1; break;
                case JpegSubsampling.Yuv422: hY = 2; vY = 1; hC = 1; vC = 1; break;
                case JpegSubsampling.Yuv420: hY = 2; vY = 2; hC = 1; vC = 1; break;
                default: throw new ArgumentException("Unsupported subsampling.", nameof(options));
            }
        }
        int hMax = hY, vMax = vY;

        int mcuW = hMax * 8;
        int mcuH = vMax * 8;
        int mcusX = (w + mcuW - 1) / mcuW;
        int mcusY = (h + mcuH - 1) / mcuH;
        int yPlaneW = mcusX * hY * 8;
        int yPlaneH = mcusY * vY * 8;
        int cPlaneW = mcusX * hC * 8;
        int cPlaneH = mcusY * vC * 8;

        // Single ArrayPool rent per encode: holds Y, Cb, Cr planes.
        int totalBytes = yPlaneW * yPlaneH + (isGray ? 0 : 2 * cPlaneW * cPlaneH);
        byte[] pool = ArrayPool<byte>.Shared.Rent(totalBytes);
        try
        {
            Span<byte> all = pool.AsSpan(0, totalBytes);
            all.Clear();
            Span<byte> yPlane = all[..(yPlaneW * yPlaneH)];
            Span<byte> cbPlane = isGray ? default : all.Slice(yPlaneW * yPlaneH, cPlaneW * cPlaneH);
            Span<byte> crPlane = isGray ? default : all.Slice(yPlaneW * yPlaneH + cPlaneW * cPlaneH, cPlaneW * cPlaneH);

            if (isGray)
            {
                FillGray(frame, yPlane, yPlaneW, yPlaneH);
            }
            else
            {
                FillYCbCr(frame, yPlane, yPlaneW, yPlaneH, cbPlane, crPlane, cPlaneW, cPlaneH, hY, vY);
            }

            // Quant tables (scaled per Annex K).
            byte[] qLum = JpegStandardTables.ScaleQuantTable(JpegStandardTables.LumQuantZigZag, options.Quality);
            byte[] qChr = isGray
                ? qLum
                : JpegStandardTables.ScaleQuantTable(JpegStandardTables.ChromQuantZigZag, options.Quality);

            // Huffman tables: standard, or optimised after a counting pass.
            JpegEncoderHuffmanTable dcLum, acLum, dcChr, acChr;
            byte[] dcLumBits, dcLumVals, acLumBits, acLumVals, dcChrBits, dcChrVals, acChrBits, acChrVals;

            if (options.OptimisedHuffman)
            {
                var (dcLumFreq, acLumFreq, dcChrFreq, acChrFreq) = CountSymbols(
                    isGray, numComp, mcusX, mcusY, hY, vY, hC, vC,
                    yPlane, yPlaneW, cbPlane, crPlane, cPlaneW, qLum, qChr);

                (dcLumBits, dcLumVals) = JpegOptimisedHuffman.Build(dcLumFreq);
                (acLumBits, acLumVals) = JpegOptimisedHuffman.Build(acLumFreq);
                if (isGray)
                {
                    dcChrBits = dcLumBits; dcChrVals = dcLumVals;
                    acChrBits = acLumBits; acChrVals = acLumVals;
                }
                else
                {
                    (dcChrBits, dcChrVals) = JpegOptimisedHuffman.Build(dcChrFreq);
                    (acChrBits, acChrVals) = JpegOptimisedHuffman.Build(acChrFreq);
                }
            }
            else
            {
                dcLumBits = JpegStandardTables.LumDcCounts; dcLumVals = JpegStandardTables.LumDcValues;
                acLumBits = JpegStandardTables.LumAcCounts; acLumVals = JpegStandardTables.LumAcValues;
                dcChrBits = JpegStandardTables.ChromDcCounts; dcChrVals = JpegStandardTables.ChromDcValues;
                acChrBits = JpegStandardTables.ChromAcCounts; acChrVals = JpegStandardTables.ChromAcValues;
            }

            dcLum = new JpegEncoderHuffmanTable(dcLumBits, dcLumVals);
            acLum = new JpegEncoderHuffmanTable(acLumBits, acLumVals);
            dcChr = new JpegEncoderHuffmanTable(dcChrBits, dcChrVals);
            acChr = new JpegEncoderHuffmanTable(acChrBits, acChrVals);

            // ---- Markers ----
            WriteMarker(output, 0xD8); // SOI
            WriteJfifApp0(output);
            JpegMetadataWriter.WriteOptionalMetadata(output, options.Exif, options.IccProfile, options.Xmp);
            WriteDqt(output, 0, qLum);
            if (!isGray) WriteDqt(output, 1, qChr);
            WriteSof0(output, w, h, numComp, hY, vY, hC, vC, isGray);
            WriteDht(output, 0, 0, dcLumBits, dcLumVals); // DC table 0
            WriteDht(output, 1, 0, acLumBits, acLumVals); // AC table 0
            if (!isGray)
            {
                WriteDht(output, 0, 1, dcChrBits, dcChrVals); // DC table 1
                WriteDht(output, 1, 1, acChrBits, acChrVals); // AC table 1
            }
            if (options.RestartInterval > 0)
            {
                WriteDri(output, options.RestartInterval);
            }
            WriteSos(output, numComp);

            // ---- Entropy-coded segment ----
            var bw = new JpegBitWriter(output);
            EncodeScan(
                ref bw, output, options.RestartInterval,
                isGray, mcusX, mcusY, hY, vY, hC, vC,
                yPlane, yPlaneW, cbPlane, crPlane, cPlaneW,
                qLum, qChr, dcLum, acLum, dcChr, acChr);
            bw.Flush();

            WriteMarker(output, 0xD9); // EOI
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(pool);
        }
    }

    // ---- Plane fill helpers ----

    private static void FillGray(ImageFrame frame, Span<byte> y, int yw, int yh)
    {
        var src = frame.Pixels.Span;
        int w = frame.Width;
        int h = frame.Height;
        int stride = frame.Stride;
        for (int row = 0; row < yh; row++)
        {
            int srcRow = Math.Min(row, h - 1);
            ReadOnlySpan<byte> srcLine = src.Slice(srcRow * stride, w);
            Span<byte> dstLine = y.Slice(row * yw, yw);
            srcLine.CopyTo(dstLine[..w]);
            byte edge = srcLine[w - 1];
            for (int x = w; x < yw; x++) dstLine[x] = edge;
        }
    }

    private static void FillYCbCr(
        ImageFrame frame,
        Span<byte> y, int yw, int yh,
        Span<byte> cb, Span<byte> cr, int cw, int ch,
        int hY, int vY)
    {
        var src = frame.Pixels.Span;
        int w = frame.Width;
        int h = frame.Height;
        int stride = frame.Stride;

        // Y at full resolution.
        for (int row = 0; row < yh; row++)
        {
            int sy = Math.Min(row, h - 1);
            int sOff = sy * stride;
            int dOff = row * yw;
            for (int col = 0; col < yw; col++)
            {
                int sx = Math.Min(col, w - 1);
                int o = sOff + sx * 3;
                int r = src[o];
                int g = src[o + 1];
                int b = src[o + 2];
                y[dOff + col] = (byte)((19595 * r + 38470 * g + 7471 * b + 32768) >> 16);
            }
        }

        // Cb/Cr at chroma resolution: average over hY x vY blocks of source pixels.
        int chromaXFactor = hY; // Y horizontal samples per chroma sample
        int chromaYFactor = vY;
        int divisor = chromaXFactor * chromaYFactor;
        for (int cy = 0; cy < ch; cy++)
        {
            for (int cx = 0; cx < cw; cx++)
            {
                int sumCb = 0, sumCr = 0;
                for (int dy = 0; dy < chromaYFactor; dy++)
                {
                    int srcY = Math.Min(cy * chromaYFactor + dy, h - 1);
                    for (int dx = 0; dx < chromaXFactor; dx++)
                    {
                        int srcX = Math.Min(cx * chromaXFactor + dx, w - 1);
                        int o = srcY * stride + srcX * 3;
                        int r = src[o];
                        int g = src[o + 1];
                        int b = src[o + 2];
                        sumCb += (-11059 * r - 21709 * g + 32768 * b + 8388608) >> 16;
                        sumCr += (32768 * r - 27439 * g - 5329 * b + 8388608) >> 16;
                    }
                }
                cb[cy * cw + cx] = (byte)Math.Clamp(sumCb / divisor, 0, 255);
                cr[cy * cw + cx] = (byte)Math.Clamp(sumCr / divisor, 0, 255);
            }
        }
    }

    // ---- First-pass symbol counting for optimised Huffman ----

    private static (int[] DcLum, int[] AcLum, int[] DcChr, int[] AcChr) CountSymbols(
        bool isGray, int numComp, int mcusX, int mcusY,
        int hY, int vY, int hC, int vC,
        Span<byte> yPlane, int yw,
        Span<byte> cbPlane, Span<byte> crPlane, int cw,
        byte[] qLum, byte[] qChr)
    {
        var dcLum = new int[257]; var acLum = new int[257];
        var dcChr = new int[257]; var acChr = new int[257];

        Span<short> block = stackalloc short[64];
        Span<short> coef = stackalloc short[64];
        Span<short> qzz = stackalloc short[64];
        int prevDcY = 0, prevDcCb = 0, prevDcCr = 0;

        for (int my = 0; my < mcusY; my++)
        {
            for (int mx = 0; mx < mcusX; mx++)
            {
                for (int by = 0; by < vY; by++)
                {
                    for (int bx = 0; bx < hY; bx++)
                    {
                        ExtractAndTransform(yPlane, yw, (mx * hY + bx) * 8, (my * vY + by) * 8, block, coef);
                        QuantiseZigZag(coef, qLum, qzz);
                        CountBlock(qzz, ref prevDcY, dcLum, acLum);
                    }
                }
                if (!isGray)
                {
                    ExtractAndTransform(cbPlane, cw, mx * hC * 8, my * vC * 8, block, coef);
                    QuantiseZigZag(coef, qChr, qzz);
                    CountBlock(qzz, ref prevDcCb, dcChr, acChr);

                    ExtractAndTransform(crPlane, cw, mx * hC * 8, my * vC * 8, block, coef);
                    QuantiseZigZag(coef, qChr, qzz);
                    CountBlock(qzz, ref prevDcCr, dcChr, acChr);
                }
            }
        }
        return (dcLum, acLum, dcChr, acChr);
    }

    private static void CountBlock(ReadOnlySpan<short> qzz, ref int prevDc, int[] dcFreq, int[] acFreq)
    {
        int dc = qzz[0];
        int diff = dc - prevDc;
        prevDc = dc;
        int magCat = MagnitudeCategory(diff);
        dcFreq[magCat]++;

        int run = 0;
        for (int k = 1; k < 64; k++)
        {
            int v = qzz[k];
            if (v == 0)
            {
                run++;
            }
            else
            {
                while (run > 15) { acFreq[0xF0]++; run -= 16; }
                int s = MagnitudeCategory(v);
                acFreq[(run << 4) | s]++;
                run = 0;
            }
        }
        if (run > 0) acFreq[0x00]++; // EOB
    }

    // ---- Main encoder scan ----

    private static void EncodeScan(
        ref JpegBitWriter bw, Stream output, int restartInterval,
        bool isGray, int mcusX, int mcusY,
        int hY, int vY, int hC, int vC,
        Span<byte> yPlane, int yw,
        Span<byte> cbPlane, Span<byte> crPlane, int cw,
        byte[] qLum, byte[] qChr,
        JpegEncoderHuffmanTable dcLum, JpegEncoderHuffmanTable acLum,
        JpegEncoderHuffmanTable dcChr, JpegEncoderHuffmanTable acChr)
    {
        Span<short> block = stackalloc short[64];
        Span<short> coef = stackalloc short[64];
        Span<short> qzz = stackalloc short[64];
        int prevDcY = 0, prevDcCb = 0, prevDcCr = 0;
        int restartCounter = restartInterval;
        int rstIndex = 0;

        for (int my = 0; my < mcusY; my++)
        {
            for (int mx = 0; mx < mcusX; mx++)
            {
                for (int by = 0; by < vY; by++)
                {
                    for (int bx = 0; bx < hY; bx++)
                    {
                        ExtractAndTransform(yPlane, yw, (mx * hY + bx) * 8, (my * vY + by) * 8, block, coef);
                        QuantiseZigZag(coef, qLum, qzz);
                        EncodeBlock(ref bw, qzz, ref prevDcY, dcLum, acLum);
                    }
                }
                if (!isGray)
                {
                    ExtractAndTransform(cbPlane, cw, mx * hC * 8, my * vC * 8, block, coef);
                    QuantiseZigZag(coef, qChr, qzz);
                    EncodeBlock(ref bw, qzz, ref prevDcCb, dcChr, acChr);

                    ExtractAndTransform(crPlane, cw, mx * hC * 8, my * vC * 8, block, coef);
                    QuantiseZigZag(coef, qChr, qzz);
                    EncodeBlock(ref bw, qzz, ref prevDcCr, dcChr, acChr);
                }

                if (restartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && !(my == mcusY - 1 && mx == mcusX - 1))
                    {
                        restartCounter = restartInterval;
                        bw.Flush();
                        output.WriteByte(0xFF);
                        output.WriteByte((byte)(0xD0 + (rstIndex & 7)));
                        rstIndex++;
                        bw = new JpegBitWriter(output);
                        prevDcY = prevDcCb = prevDcCr = 0;
                    }
                }
            }
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void ExtractAndTransform(
        Span<byte> plane, int planeStride, int x0, int y0,
        Span<short> blockOut, Span<short> coefOut)
    {
        for (int y = 0; y < 8; y++)
        {
            int rowOff = (y0 + y) * planeStride + x0;
            int dst = y * 8;
            for (int x = 0; x < 8; x++)
            {
                blockOut[dst + x] = (short)(plane[rowOff + x] - 128);
            }
        }
        JpegFdct.Forward(blockOut, coefOut);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void QuantiseZigZag(ReadOnlySpan<short> coef, byte[] qZigZag, Span<short> qzzOut)
    {
        // q is in zig-zag order, coef is in natural order. For each
        // scan-order index k, fetch the natural-order coefficient at
        // Zigzag[k] and divide by q[k] with banker-rounding-to-zero.
        for (int k = 0; k < 64; k++)
        {
            int natural = JpegDecoderShared.Zigzag[k];
            int v = coef[natural];
            int q = qZigZag[k];
            int rounded = v >= 0
                ? (v + (q >> 1)) / q
                : -((-v + (q >> 1)) / q);
            qzzOut[k] = (short)rounded;
        }
    }

    private static void EncodeBlock(
        ref JpegBitWriter bw, ReadOnlySpan<short> qzz,
        ref int prevDc, JpegEncoderHuffmanTable dc, JpegEncoderHuffmanTable ac)
    {
        int diff = qzz[0] - prevDc;
        prevDc = qzz[0];
        int s = MagnitudeCategory(diff);
        bw.WriteBits(dc.Codes[s], dc.Sizes[s]);
        if (s > 0)
        {
            int v = diff;
            if (v < 0) v += (1 << s) - 1;
            bw.WriteBits(v, s);
        }

        int run = 0;
        for (int k = 1; k < 64; k++)
        {
            int coef = qzz[k];
            if (coef == 0)
            {
                run++;
            }
            else
            {
                while (run > 15)
                {
                    bw.WriteBits(ac.Codes[0xF0], ac.Sizes[0xF0]); // ZRL
                    run -= 16;
                }
                int sc = MagnitudeCategory(coef);
                int rs = (run << 4) | sc;
                bw.WriteBits(ac.Codes[rs], ac.Sizes[rs]);
                int v = coef;
                if (v < 0) v += (1 << sc) - 1;
                bw.WriteBits(v, sc);
                run = 0;
            }
        }
        if (run > 0)
        {
            bw.WriteBits(ac.Codes[0x00], ac.Sizes[0x00]); // EOB
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int MagnitudeCategory(int v)
    {
        if (v == 0) return 0;
        if (v < 0) v = -v;
        int s = 0;
        while (v != 0) { s++; v >>= 1; }
        return s;
    }

    // ---- Marker writers ----

    private static void WriteMarker(Stream s, byte marker)
    {
        s.WriteByte(0xFF);
        s.WriteByte(marker);
    }

    private static void WriteSegmentHeader(Stream s, byte marker, int payloadLength)
    {
        WriteMarker(s, marker);
        Span<byte> len = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(len, (ushort)(payloadLength + 2));
        s.Write(len);
    }

    private static void WriteJfifApp0(Stream s)
    {
        // JFIF 1.02 (T.81 Annex B): identifier + version + units + density + thumb dims.
        WriteSegmentHeader(s, 0xE0, 16);
        s.Write("JFIF\0"u8);
        s.WriteByte(0x01); s.WriteByte(0x02);          // version 1.02
        s.WriteByte(0x00);                              // density units = 0 (aspect ratio only)
        s.WriteByte(0x00); s.WriteByte(0x01);          // X density = 1
        s.WriteByte(0x00); s.WriteByte(0x01);          // Y density = 1
        s.WriteByte(0x00); s.WriteByte(0x00);          // thumb 0×0
    }

    private static void WriteDqt(Stream s, int tableId, byte[] qZigZag)
    {
        WriteSegmentHeader(s, 0xDB, 65);
        s.WriteByte((byte)(tableId & 0x0F)); // Pq=0 (8-bit), Tq=tableId
        s.Write(qZigZag);
    }

    private static void WriteSof0(Stream s, int w, int h, int numComp,
        int hY, int vY, int hC, int vC, bool isGray)
    {
        int payload = 6 + 3 * numComp;
        WriteSegmentHeader(s, 0xC0, payload);
        s.WriteByte(8); // P = 8
        s.WriteByte((byte)(h >> 8)); s.WriteByte((byte)h);
        s.WriteByte((byte)(w >> 8)); s.WriteByte((byte)w);
        s.WriteByte((byte)numComp);
        // Component spec: Ci, Hi Vi, Tqi
        s.WriteByte(1); s.WriteByte((byte)((hY << 4) | vY)); s.WriteByte(0);
        if (!isGray)
        {
            s.WriteByte(2); s.WriteByte((byte)((hC << 4) | vC)); s.WriteByte(1);
            s.WriteByte(3); s.WriteByte((byte)((hC << 4) | vC)); s.WriteByte(1);
        }
    }

    private static void WriteDht(Stream s, int tc, int th, byte[] bits, byte[] values)
    {
        int payload = 1 + 16 + values.Length;
        WriteSegmentHeader(s, 0xC4, payload);
        s.WriteByte((byte)((tc << 4) | (th & 0x0F)));
        s.Write(bits);
        s.Write(values);
    }

    private static void WriteDri(Stream s, int interval)
    {
        WriteSegmentHeader(s, 0xDD, 2);
        s.WriteByte((byte)(interval >> 8));
        s.WriteByte((byte)interval);
    }

    private static void WriteSos(Stream s, int numComp)
    {
        int payload = 1 + 2 * numComp + 3;
        WriteSegmentHeader(s, 0xDA, payload);
        s.WriteByte((byte)numComp);
        s.WriteByte(1); s.WriteByte(0x00); // Y uses DC0/AC0
        if (numComp == 3)
        {
            s.WriteByte(2); s.WriteByte(0x11);
            s.WriteByte(3); s.WriteByte(0x11);
        }
        s.WriteByte(0);   // Ss = 0
        s.WriteByte(63);  // Se = 63
        s.WriteByte(0);   // Ah/Al = 0
    }
}
