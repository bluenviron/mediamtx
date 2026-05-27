using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Progressive-DCT JPEG decoder (SOF2). Replays each scan captured by
/// <see cref="JpegReader"/> into per-component zig-zag coefficient buffers,
/// then dequantises, runs the inverse 8×8 DCT and converts YCbCr → RGB.
/// </summary>
/// <remarks>
/// Implements the four progressive scan types defined by ITU-T T.81:
/// DC initial (interleaved or single-component), DC successive approximation,
/// AC initial (single-component only) and AC successive approximation
/// (single-component only). Restart markers (DRI / RSTn) reset both the
/// per-component DC predictors and the AC end-of-band run counter.
/// </remarks>
internal static class JpegProgressiveDecoder
{
    public static ImageFrame Decode(JpegFrame frame, JpegDecoderState state, List<JpegScan> scans)
    {
        int width = frame.Width;
        int height = frame.Height;
        int numComp = frame.NumberOfComponents;

        if (numComp is not (1 or 3))
        {
            throw new NotSupportedException(
                $"JPEG progressive decoder supports 1- and 3-component images; got {numComp}.");
        }

        int hMax = 0, vMax = 0;
        for (int i = 0; i < numComp; i++)
        {
            hMax = Math.Max(hMax, frame.Components[i].HSampling);
            vMax = Math.Max(vMax, frame.Components[i].VSampling);
        }

        int mcuWidth = hMax * 8;
        int mcuHeight = vMax * 8;
        int mcusX = (width + mcuWidth - 1) / mcuWidth;
        int mcusY = (height + mcuHeight - 1) / mcuHeight;

        var coefs = new short[numComp][];
        var compBlocksW = new int[numComp];
        var compBlocksH = new int[numComp];
        for (int i = 0; i < numComp; i++)
        {
            var c = frame.Components[i];
            compBlocksW[i] = mcusX * c.HSampling;
            compBlocksH[i] = mcusY * c.VSampling;
            coefs[i] = new short[compBlocksW[i] * compBlocksH[i] * 64];
        }

        foreach (var scan in scans)
        {
            DecodeScan(frame, scan, coefs, compBlocksW, compBlocksH, mcusX, mcusY);
        }

        var planes = new byte[numComp][];
        var planeWidths = new int[numComp];
        for (int i = 0; i < numComp; i++)
        {
            int w = compBlocksW[i] * 8;
            int h = compBlocksH[i] * 8;
            planeWidths[i] = w;
            planes[i] = new byte[w * h];
        }

        Span<short> natural = stackalloc short[64];
        for (int ci = 0; ci < numComp; ci++)
        {
            var comp = frame.Components[ci];
            var qTable = state.QuantTables[comp.QuantTableId]
                ?? throw new ImageFormatException("Missing quantisation table for progressive JPEG.");
            var compCoefs = coefs[ci];
            int blocksW = compBlocksW[ci];
            int blocksH = compBlocksH[ci];
            int planeStride = planeWidths[ci];
            var plane = planes[ci];

            for (int by = 0; by < blocksH; by++)
            {
                for (int bx = 0; bx < blocksW; bx++)
                {
                    int blockOff = (by * blocksW + bx) * 64;
                    natural.Clear();
                    for (int k = 0; k < 64; k++)
                    {
                        natural[JpegDecoderShared.Zigzag[k]] = (short)(compCoefs[blockOff + k] * qTable[k]);
                    }
                    JpegDecoderShared.Idct8x8(natural, plane, planeStride, by * 8 * planeStride + bx * 8);
                }
            }
        }

        return numComp == 1
            ? JpegDecoderShared.BuildGrayscaleFrame(width, height, planes[0], planeWidths[0])
            : JpegDecoderShared.BuildRgbFrame(width, height, planes, planeWidths, frame.Components, hMax, vMax);
    }

    private static void DecodeScan(
        JpegFrame frame, JpegScan scan,
        short[][] coefs, int[] compBlocksW, int[] compBlocksH,
        int mcusX, int mcusY)
    {
        var reader = new JpegBitReader(scan.EntropyData);
        bool isDc = scan.Ss == 0;
        bool isRefine = scan.Ah > 0;
        bool isInterleaved = scan.ComponentIds.Length > 1;

        if (isDc)
        {
            if (isInterleaved)
            {
                DecodeInterleavedDc(frame, scan, ref reader, coefs, compBlocksW, mcusX, mcusY, isRefine);
            }
            else
            {
                int ci = FindComponent(frame, scan.ComponentIds[0]);
                DecodeNoninterleavedDc(scan, ref reader, coefs[ci], compBlocksW[ci], compBlocksH[ci], isRefine);
            }
        }
        else
        {
            if (isInterleaved)
            {
                throw new ImageFormatException("Progressive AC scans must be single-component.");
            }
            int ci = FindComponent(frame, scan.ComponentIds[0]);
            if (isRefine)
            {
                DecodeAcRefinement(scan, ref reader, coefs[ci], compBlocksW[ci], compBlocksH[ci]);
            }
            else
            {
                DecodeAcInitial(scan, ref reader, coefs[ci], compBlocksW[ci], compBlocksH[ci]);
            }
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int FindComponent(JpegFrame frame, byte id)
    {
        for (int i = 0; i < frame.NumberOfComponents; i++)
        {
            if (frame.Components[i].Id == id) return i;
        }
        throw new ImageFormatException($"Unknown component id {id} in scan.");
    }

    private static void DecodeInterleavedDc(
        JpegFrame frame, JpegScan scan, ref JpegBitReader reader,
        short[][] coefs, int[] compBlocksW, int mcusX, int mcusY, bool refine)
    {
        int ns = scan.ComponentIds.Length;
        var compIdx = new int[ns];
        var hs = new int[ns];
        var vs = new int[ns];
        for (int i = 0; i < ns; i++)
        {
            compIdx[i] = FindComponent(frame, scan.ComponentIds[i]);
            hs[i] = frame.Components[compIdx[i]].HSampling;
            vs[i] = frame.Components[compIdx[i]].VSampling;
        }

        Span<int> prevDc = stackalloc int[ns];
        int restartCounter = scan.RestartInterval;

        for (int my = 0; my < mcusY; my++)
        {
            for (int mx = 0; mx < mcusX; mx++)
            {
                for (int i = 0; i < ns; i++)
                {
                    int ci = compIdx[i];
                    int blocksW = compBlocksW[ci];
                    var dcTable = scan.DcHuffmanSnapshot[scan.DcTables[i]];
                    for (int by = 0; by < vs[i]; by++)
                    {
                        for (int bx = 0; bx < hs[i]; bx++)
                        {
                            int blockY = my * vs[i] + by;
                            int blockX = mx * hs[i] + bx;
                            int blockOff = (blockY * blocksW + blockX) * 64;
                            if (!refine)
                            {
                                int t = JpegDecoderShared.DecodeHuffman(ref reader, dcTable
                                    ?? throw new ImageFormatException("Missing DC Huffman table."));
                                int diff = t == 0 ? 0 : JpegDecoderShared.ReceiveExtend(ref reader, t);
                                prevDc[i] += diff;
                                coefs[ci][blockOff] = (short)(prevDc[i] << scan.Al);
                            }
                            else
                            {
                                int bit = reader.ReadBit();
                                if (bit != 0)
                                {
                                    coefs[ci][blockOff] |= (short)(1 << scan.Al);
                                }
                            }
                        }
                    }
                }

                if (scan.RestartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && !(my == mcusY - 1 && mx == mcusX - 1))
                    {
                        restartCounter = scan.RestartInterval;
                        reader.ResetAfterRestart();
                        prevDc.Clear();
                    }
                }
            }
        }
    }

    private static void DecodeNoninterleavedDc(
        JpegScan scan, ref JpegBitReader reader,
        short[] coefs, int blocksW, int blocksH, bool refine)
    {
        var dcTable = scan.DcHuffmanSnapshot[scan.DcTables[0]]
            ?? throw new ImageFormatException("Missing DC Huffman table.");

        int prevDc = 0;
        int restartCounter = scan.RestartInterval;
        int blockCount = 0;
        int totalBlocks = blocksW * blocksH;

        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                if (!refine)
                {
                    int t = JpegDecoderShared.DecodeHuffman(ref reader, dcTable);
                    int diff = t == 0 ? 0 : JpegDecoderShared.ReceiveExtend(ref reader, t);
                    prevDc += diff;
                    coefs[blockOff] = (short)(prevDc << scan.Al);
                }
                else
                {
                    int bit = reader.ReadBit();
                    if (bit != 0)
                    {
                        coefs[blockOff] |= (short)(1 << scan.Al);
                    }
                }

                blockCount++;
                if (scan.RestartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && blockCount < totalBlocks)
                    {
                        restartCounter = scan.RestartInterval;
                        reader.ResetAfterRestart();
                        prevDc = 0;
                    }
                }
            }
        }
    }

    private static void DecodeAcInitial(
        JpegScan scan, ref JpegBitReader reader,
        short[] coefs, int blocksW, int blocksH)
    {
        var acTable = scan.AcHuffmanSnapshot[scan.AcTables[0]]
            ?? throw new ImageFormatException("Missing AC Huffman table.");

        int eobRun = 0;
        int restartCounter = scan.RestartInterval;
        int blockCount = 0;
        int totalBlocks = blocksW * blocksH;

        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;

                if (eobRun > 0)
                {
                    eobRun--;
                }
                else
                {
                    int k = scan.Ss;
                    while (k <= scan.Se)
                    {
                        int rs = JpegDecoderShared.DecodeHuffman(ref reader, acTable);
                        int r = rs >> 4;
                        int s = rs & 0x0F;
                        if (s == 0)
                        {
                            if (r == 15)
                            {
                                k += 16;
                                continue;
                            }
                            eobRun = (1 << r) + (r > 0 ? reader.ReadBits(r) : 0) - 1;
                            break;
                        }
                        k += r;
                        if (k > scan.Se) break;
                        int v = JpegDecoderShared.ReceiveExtend(ref reader, s);
                        coefs[blockOff + k] = (short)(v << scan.Al);
                        k++;
                    }
                }

                blockCount++;
                if (scan.RestartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && blockCount < totalBlocks)
                    {
                        restartCounter = scan.RestartInterval;
                        reader.ResetAfterRestart();
                        eobRun = 0;
                    }
                }
            }
        }
    }

    private static void DecodeAcRefinement(
        JpegScan scan, ref JpegBitReader reader,
        short[] coefs, int blocksW, int blocksH)
    {
        var acTable = scan.AcHuffmanSnapshot[scan.AcTables[0]]
            ?? throw new ImageFormatException("Missing AC Huffman table.");

        int eobRun = 0;
        int restartCounter = scan.RestartInterval;
        int blockCount = 0;
        int totalBlocks = blocksW * blocksH;
        int p1 = 1 << scan.Al;
        int m1 = (-1) << scan.Al;

        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int k = scan.Ss;

                if (eobRun == 0)
                {
                    for (; k <= scan.Se; k++)
                    {
                        int rs = JpegDecoderShared.DecodeHuffman(ref reader, acTable);
                        int r = rs >> 4;
                        int s = rs & 0x0F;
                        int newCoef;
                        if (s != 0)
                        {
                            if (s != 1) throw new ImageFormatException("Bad AC refinement value (s != 1).");
                            newCoef = reader.ReadBit() != 0 ? p1 : m1;
                        }
                        else
                        {
                            if (r != 15)
                            {
                                eobRun = (1 << r) + (r > 0 ? reader.ReadBits(r) : 0);
                                break;
                            }
                            newCoef = 0;
                        }

                        while (true)
                        {
                            if (coefs[blockOff + k] != 0)
                            {
                                if (reader.ReadBit() != 0 && (coefs[blockOff + k] & p1) == 0)
                                {
                                    coefs[blockOff + k] = coefs[blockOff + k] > 0
                                        ? (short)(coefs[blockOff + k] + p1)
                                        : (short)(coefs[blockOff + k] + m1);
                                }
                            }
                            else
                            {
                                if (--r < 0) break;
                            }
                            k++;
                            if (k > scan.Se) break;
                        }

                        if (s != 0 && k <= scan.Se)
                        {
                            coefs[blockOff + k] = (short)newCoef;
                        }
                    }
                }

                if (eobRun > 0)
                {
                    while (k <= scan.Se)
                    {
                        if (coefs[blockOff + k] != 0)
                        {
                            if (reader.ReadBit() != 0 && (coefs[blockOff + k] & p1) == 0)
                            {
                                coefs[blockOff + k] = coefs[blockOff + k] > 0
                                    ? (short)(coefs[blockOff + k] + p1)
                                    : (short)(coefs[blockOff + k] + m1);
                            }
                        }
                        k++;
                    }
                    eobRun--;
                }

                blockCount++;
                if (scan.RestartInterval > 0)
                {
                    restartCounter--;
                    if (restartCounter == 0 && blockCount < totalBlocks)
                    {
                        restartCounter = scan.RestartInterval;
                        reader.ResetAfterRestart();
                        eobRun = 0;
                    }
                }
            }
        }
    }
}
