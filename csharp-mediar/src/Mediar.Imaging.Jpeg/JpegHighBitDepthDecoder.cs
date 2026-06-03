using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Extended-sequential 12-bit JPEG decoder (SOF1) per ITU-T Rec. T.81
/// (1992-09) §B.2.2 and §F.1.1. Reuses the Huffman / DC-differential /
/// AC-run-length entropy pipeline of <see cref="JpegBaselineDecoder"/>
/// but with 12-bit sample precision, 16-bit dequantised coefficients,
/// and a floating-point IDCT that produces 12-bit samples in
/// <c>[0, 4095]</c>. Output is emitted as <see cref="PixelFormat.Gray16"/>
/// (left-justified to 16-bit) or <see cref="PixelFormat.Rgb48"/>.
/// </summary>
/// <remarks>
/// 12-bit JPEG is used by medical imaging (DICOM JPEG transfer syntax
/// 1.2.840.10008.1.2.4.50 with P=12), some camera RAW thumbnails, and
/// scientific imaging. The decoder validates <c>P == 12</c> at SOF1
/// time and rejects anything else with <see cref="InvalidDataException"/>.
/// </remarks>
internal static class JpegHighBitDepthDecoder
{
    private const int MaxValue = 4095;     // 2^12 - 1
    private const int LevelShift = 2048;   // 2^(P-1) for P = 12

    public static ImageFrame Decode(JpegFrame frame, JpegDecoderState state, byte[] scanBytes)
    {
        if (frame.BitsPerSample != 12)
        {
            throw new InvalidDataException(
                $"JpegHighBitDepthDecoder: expected SOF1 P=12, got P={frame.BitsPerSample}.");
        }
        int width = frame.Width;
        int height = frame.Height;
        int numComp = frame.NumberOfComponents;
        if (numComp is not (1 or 3))
        {
            throw new NotSupportedException(
                $"12-bit JPEG: only 1- and 3-component frames are supported (got {numComp}).");
        }
        if (width <= 0 || height <= 0)
        {
            throw new InvalidDataException("12-bit JPEG: bad SOF dimensions.");
        }

        int hMax = 0, vMax = 0;
        for (int i = 0; i < numComp; i++)
        {
            hMax = Math.Max(hMax, frame.Components[i].HSampling);
            vMax = Math.Max(vMax, frame.Components[i].VSampling);
        }
        if (hMax == 0 || vMax == 0)
        {
            throw new InvalidDataException("12-bit JPEG: zero sampling factor.");
        }
        int mcuWidth = hMax * 8;
        int mcuHeight = vMax * 8;
        int mcusX = (width + mcuWidth - 1) / mcuWidth;
        int mcusY = (height + mcuHeight - 1) / mcuHeight;

        var planes = new ushort[numComp][];
        var planeStrides = new int[numComp];
        for (int i = 0; i < numComp; i++)
        {
            var c = frame.Components[i];
            int w = mcusX * c.HSampling * 8;
            int h = mcusY * c.VSampling * 8;
            planeStrides[i] = w;
            planes[i] = new ushort[w * h];
        }

        var prevDc = new int[numComp];
        int restartCounter = state.RestartInterval;

        Span<short> block = stackalloc short[64];
        Span<short> dequant = stackalloc short[64];

        var reader = new JpegBitReader(scanBytes);

        for (int my = 0; my < mcusY; my++)
        {
            for (int mx = 0; mx < mcusX; mx++)
            {
                for (int ci = 0; ci < numComp; ci++)
                {
                    var comp = frame.Components[ci];
                    var dcTable = state.DcHuffman[state.ScanDcTables[ci]]
                        ?? throw new InvalidDataException("12-bit JPEG: missing DC Huffman table.");
                    var acTable = state.AcHuffman[state.ScanAcTables[ci]]
                        ?? throw new InvalidDataException("12-bit JPEG: missing AC Huffman table.");
                    var qTable = state.QuantTables[comp.QuantTableId]
                        ?? throw new InvalidDataException("12-bit JPEG: missing quantisation table.");
                    int planeStride = planeStrides[ci];
                    var plane = planes[ci];

                    for (int by = 0; by < comp.VSampling; by++)
                    {
                        for (int bx = 0; bx < comp.HSampling; bx++)
                        {
                            block.Clear();
                            DecodeBlock12(ref reader, dcTable, acTable, ref prevDc[ci], block);

                            dequant.Clear();
                            for (int k = 0; k < 64; k++)
                            {
                                dequant[JpegDecoderShared.Zigzag[k]] = (short)(block[k] * qTable[k]);
                            }

                            int outX = (mx * comp.HSampling + bx) * 8;
                            int outY = (my * comp.VSampling + by) * 8;
                            Idct8x8_12bit(dequant, plane, planeStride, outY * planeStride + outX);
                        }
                    }
                }

                if (state.RestartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && !(my == mcusY - 1 && mx == mcusX - 1))
                    {
                        restartCounter = state.RestartInterval;
                        reader.ResetAfterRestart();
                        Array.Clear(prevDc, 0, prevDc.Length);
                    }
                }
            }
        }

        return numComp == 1
            ? BuildGray16(width, height, planes[0], planeStrides[0])
            : BuildRgb48(width, height, planes, planeStrides, frame.Components, hMax, vMax);
    }

    /// <summary>
    /// 12-bit baseline-style block decode. The DC/AC value tables can
    /// carry a magnitude category up to 15 (vs 11 for 8-bit baseline).
    /// </summary>
    private static void DecodeBlock12(
        ref JpegBitReader reader, HuffmanTable dc, HuffmanTable ac,
        ref int prevDc, Span<short> block)
    {
        int t = JpegDecoderShared.DecodeHuffman(ref reader, dc);
        if (t > 15) throw new InvalidDataException("12-bit JPEG: DC magnitude category > 15.");
        int diff = t == 0 ? 0 : JpegDecoderShared.ReceiveExtend(ref reader, t);
        prevDc += diff;
        block[0] = (short)prevDc;

        int k = 1;
        while (k < 64)
        {
            int rs = JpegDecoderShared.DecodeHuffman(ref reader, ac);
            int run = rs >> 4;
            int s = rs & 0x0F;
            if (s == 0)
            {
                if (run == 15) { k += 16; continue; }
                break;
            }
            k += run;
            if (k >= 64) break;
            int v = JpegDecoderShared.ReceiveExtend(ref reader, s);
            block[k++] = (short)v;
        }
    }

    /// <summary>
    /// Float-IDCT producing 12-bit samples in <c>[0, 4095]</c>. Same
    /// math as <c>JpegDecoderShared.Idct8x8</c> but with a 2048 level
    /// shift and a 12-bit clamp.
    /// </summary>
    private static void Idct8x8_12bit(ReadOnlySpan<short> input, ushort[] outShorts, int outStride, int outOff)
    {
        Span<float> temp = stackalloc float[64];
        var cos = JpegDecoderShared.IdctCos;

        for (int row = 0; row < 8; row++)
        {
            int ri = row * 8;
            for (int x = 0; x < 8; x++)
            {
                float sum = 0;
                for (int u = 0; u < 8; u++)
                {
                    sum += input[ri + u] * cos[u * 8 + x];
                }
                temp[ri + x] = sum;
            }
        }

        for (int col = 0; col < 8; col++)
        {
            for (int y = 0; y < 8; y++)
            {
                float sum = 0;
                for (int v = 0; v < 8; v++)
                {
                    sum += temp[v * 8 + col] * cos[v * 8 + y];
                }
                int s = (int)MathF.Round(sum + LevelShift);
                if (s < 0) s = 0;
                else if (s > MaxValue) s = MaxValue;
                outShorts[outOff + y * outStride + col] = (ushort)s;
            }
        }
    }

    private static ImageFrame BuildGray16(int w, int h, ushort[] plane, int planeStride)
    {
        var (frame, buf) = ImageFrame.Rent(w, h, PixelFormat.Gray16, w * 2);
        for (int y = 0; y < h; y++)
        {
            int srcOff = y * planeStride;
            int dstOff = y * w * 2;
            for (int x = 0; x < w; x++)
            {
                // Left-justify 12 bits into 16 by shifting up 4.
                ushort v = (ushort)(plane[srcOff + x] << 4);
                BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(dstOff + x * 2, 2), v);
            }
        }
        return frame;
    }

    private static ImageFrame BuildRgb48(
        int w, int h, ushort[][] planes, int[] planeStrides,
        JpegComponent[] components, int hMax, int vMax)
    {
        var (frame, buf) = ImageFrame.Rent(w, h, PixelFormat.Rgb48, w * 6);
        var yPlane = planes[0]; var cbPlane = planes[1]; var crPlane = planes[2];
        int yStride = planeStrides[0]; int cbStride = planeStrides[1]; int crStride = planeStrides[2];
        int yhDiv = hMax / components[0].HSampling, yvDiv = vMax / components[0].VSampling;
        int cbhDiv = hMax / components[1].HSampling, cbvDiv = vMax / components[1].VSampling;
        int crhDiv = hMax / components[2].HSampling, crvDiv = vMax / components[2].VSampling;
        const int Half = 2048;
        const int Max = 4095;

        for (int py = 0; py < h; py++)
        {
            int yOff = (py / yvDiv) * yStride;
            int cbOff = (py / cbvDiv) * cbStride;
            int crOff = (py / crvDiv) * crStride;
            int rowOff = py * w * 6;
            for (int px = 0; px < w; px++)
            {
                int y = yPlane[yOff + px / yhDiv];
                int cb = cbPlane[cbOff + px / cbhDiv] - Half;
                int cr = crPlane[crOff + px / crhDiv] - Half;
                int r = y + (int)MathF.Round(1.402f * cr);
                int g = y - (int)MathF.Round(0.344136f * cb + 0.714136f * cr);
                int b = y + (int)MathF.Round(1.772f * cb);
                r = Math.Clamp(r, 0, Max);
                g = Math.Clamp(g, 0, Max);
                b = Math.Clamp(b, 0, Max);
                int o = rowOff + px * 6;
                BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o + 0, 2), (ushort)(r << 4));
                BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o + 2, 2), (ushort)(g << 4));
                BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(o + 4, 2), (ushort)(b << 4));
            }
        }
        return frame;
    }
}
