using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Baseline-DCT JPEG decoder (SOF0). Implements the full Huffman / dequant /
/// inverse-DCT / YCbCr → RGB pipeline against the marker state captured by
/// <see cref="JpegReader"/>.
/// </summary>
/// <remarks>
/// Supports 1-component (grayscale) and 3-component (YCbCr) baseline scans
/// with any combination of 1×1, 1×2, 2×1 or 2×2 horizontal/vertical sampling
/// (i.e. 4:4:4 / 4:4:0 / 4:2:2 / 4:2:0). Restart markers (DRI / RSTn) are
/// honoured. Progressive (SOF2) scans are decoded by
/// <see cref="JpegProgressiveDecoder"/>; lossless and arithmetic-coded
/// scans are out of scope.
/// </remarks>
internal static class JpegBaselineDecoder
{
    public static ImageFrame Decode(JpegFrame frame, JpegDecoderState state, byte[] scanBytes)
    {
        int width = frame.Width;
        int height = frame.Height;
        int numComp = frame.NumberOfComponents;

        if (numComp is not (1 or 3))
        {
            throw new NotSupportedException(
                $"JPEG baseline decoder supports 1- and 3-component images; got {numComp}.");
        }

        int hMax = 0, vMax = 0;
        for (int i = 0; i < numComp; i++)
        {
            hMax = Math.Max(hMax, frame.Components[i].HSampling);
            vMax = Math.Max(vMax, frame.Components[i].VSampling);
        }
        if (hMax == 0 || vMax == 0)
        {
            throw new ImageFormatException("JPEG SOF declared zero sampling factor.");
        }

        int mcuWidth = hMax * 8;
        int mcuHeight = vMax * 8;
        int mcusX = (width + mcuWidth - 1) / mcuWidth;
        int mcusY = (height + mcuHeight - 1) / mcuHeight;

        var planes = new byte[numComp][];
        var planeWidths = new int[numComp];
        for (int i = 0; i < numComp; i++)
        {
            var c = frame.Components[i];
            int w = mcusX * c.HSampling * 8;
            int h = mcusY * c.VSampling * 8;
            planeWidths[i] = w;
            planes[i] = new byte[w * h];
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
                        ?? throw new ImageFormatException("Missing DC Huffman table.");
                    var acTable = state.AcHuffman[state.ScanAcTables[ci]]
                        ?? throw new ImageFormatException("Missing AC Huffman table.");
                    var qTable = state.QuantTables[comp.QuantTableId]
                        ?? throw new ImageFormatException("Missing quantisation table.");
                    int planeStride = planeWidths[ci];
                    var plane = planes[ci];

                    for (int by = 0; by < comp.VSampling; by++)
                    {
                        for (int bx = 0; bx < comp.HSampling; bx++)
                        {
                            block.Clear();
                            DecodeBlock(ref reader, dcTable, acTable, ref prevDc[ci], block);

                            dequant.Clear();
                            for (int k = 0; k < 64; k++)
                            {
                                dequant[JpegDecoderShared.Zigzag[k]] = (short)(block[k] * qTable[k]);
                            }

                            int outX = (mx * comp.HSampling + bx) * 8;
                            int outY = (my * comp.VSampling + by) * 8;
                            JpegDecoderShared.Idct8x8(dequant, plane, planeStride, outY * planeStride + outX);
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
            ? JpegDecoderShared.BuildGrayscaleFrame(width, height, planes[0], planeWidths[0])
            : JpegDecoderShared.BuildRgbFrame(width, height, planes, planeWidths, frame.Components, hMax, vMax);
    }

    private static void DecodeBlock(
        ref JpegBitReader reader, HuffmanTable dc, HuffmanTable ac,
        ref int prevDc, Span<short> block)
    {
        int t = JpegDecoderShared.DecodeHuffman(ref reader, dc);
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
                if (run == 15) { k += 16; continue; } // ZRL
                break;                                 // EOB
            }
            k += run;
            if (k >= 64) break;
            int v = JpegDecoderShared.ReceiveExtend(ref reader, s);
            block[k++] = (short)v;
        }
    }
}

/// <summary>
/// Bit reader over the entropy-coded segment of a JPEG scan. Handles
/// <c>FF 00</c> stuffing and stops at any other <c>FF xx</c> marker.
/// </summary>
internal struct JpegBitReader
{
    private readonly byte[] _data;
    private int _pos;
    private ulong _buffer;
    private int _bits;

    public JpegBitReader(byte[] data)
    {
        _data = data;
        _pos = 0;
        _buffer = 0;
        _bits = 0;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public int Peek9()
    {
        EnsureBits(9);
        if (_bits < 9) return -1;
        return (int)((_buffer >> (_bits - 9)) & 0x1FF);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void Consume(int n)
    {
        _bits -= n;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public int ReadBit()
    {
        EnsureBits(1);
        if (_bits == 0) return 0;
        _bits--;
        return (int)((_buffer >> _bits) & 1);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public int ReadBits(int n)
    {
        if (n == 0) return 0;
        EnsureBits(n);
        if (_bits < n) return 0;
        _bits -= n;
        return (int)((_buffer >> _bits) & ((1u << n) - 1));
    }

    private void EnsureBits(int n)
    {
        while (_bits < 48 && _pos < _data.Length)
        {
            byte b = _data[_pos];
            if (b == 0xFF)
            {
                if (_pos + 1 >= _data.Length)
                {
                    _pos++;
                    break;
                }
                byte next = _data[_pos + 1];
                if (next == 0x00)
                {
                    _pos += 2;
                }
                else
                {
                    // Restart / EOI / other marker — leave it for ResetAfterRestart.
                    break;
                }
            }
            else
            {
                _pos++;
            }
            _buffer = (_buffer << 8) | b;
            _bits += 8;
        }
        _ = n;
    }

    public void ResetAfterRestart()
    {
        while (_pos < _data.Length && _data[_pos] != 0xFF) _pos++;
        if (_pos + 1 < _data.Length && _data[_pos] == 0xFF
            && _data[_pos + 1] >= 0xD0 && _data[_pos + 1] <= 0xD7)
        {
            _pos += 2;
        }
        _buffer = 0;
        _bits = 0;
    }
}

/// <summary>
/// JPEG Huffman table built from a DHT marker. Encapsulates the canonical
/// min/max/valptr decoder, plus a 9-bit fast-path lookup that covers the
/// short codes (which dominate real JPEG streams).
/// </summary>
internal sealed class HuffmanTable
{
    public ushort[] MinCode { get; } = new ushort[17];
    public int[] MaxCode { get; } = new int[17];
    public int[] ValPtr { get; } = new int[17];
    public byte[] HuffVal { get; private set; } = [];
    public short[] FastTable { get; } = new short[512];

    private HuffmanTable() { }

    public static HuffmanTable Build(ReadOnlySpan<byte> lengths, ReadOnlySpan<byte> values)
    {
        var t = new HuffmanTable();
        t.HuffVal = values.ToArray();

        Span<int> huffCode = stackalloc int[256];
        Span<int> huffSize = stackalloc int[256];

        int total = 0;
        for (int i = 0; i < 16; i++)
        {
            for (int j = 0; j < lengths[i]; j++)
            {
                huffSize[total++] = i + 1;
            }
        }

        int code = 0;
        int si = total > 0 ? huffSize[0] : 0;
        int k = 0;
        while (k < total)
        {
            while (k < total && huffSize[k] == si)
            {
                huffCode[k++] = code++;
            }
            if (k >= total) break;
            while (huffSize[k] != si)
            {
                code <<= 1;
                si++;
            }
        }

        int j2 = 0;
        for (int l = 1; l <= 16; l++)
        {
            t.MaxCode[l] = -1;
            if (lengths[l - 1] == 0) continue;
            t.ValPtr[l] = j2;
            t.MinCode[l] = (ushort)huffCode[j2];
            j2 += lengths[l - 1];
            t.MaxCode[l] = huffCode[j2 - 1];
        }

        Array.Fill(t.FastTable, (short)-1);
        for (int kk = 0; kk < total; kk++)
        {
            int len = huffSize[kk];
            if (len > 9) continue;
            int cc = huffCode[kk] << (9 - len);
            int upper = cc + (1 << (9 - len));
            short entry = (short)((len << 8) | t.HuffVal[kk]);
            for (int x = cc; x < upper; x++)
            {
                t.FastTable[x] = entry;
            }
        }

        return t;
    }
}

