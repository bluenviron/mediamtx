using System.Buffers.Binary;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Lossless JPEG decoder (SOF3, ITU-T T.81 Annex H). Implements the predictor
/// Huffman / spatial-domain pipeline used by DNG raw files and a few medical
/// imaging containers.
/// </summary>
/// <remarks>
/// Supports the seven predictors defined by the spec (Ra, Rb, Rc, Ra+Rb-Rc,
/// Ra+((Rb-Rc)>>1), Rb+((Ra-Rc)>>1), (Ra+Rb)>>1), precisions of 2 to 16
/// bits-per-sample, and the point transform (Al = Pt). Restart markers
/// (DRI / RSTn) are honoured by resetting the bit reader and re-seeding
/// the per-row predictor with the row-start initial value.
/// <para>
/// Only non-interleaved scans (Ns = 1) are supported, which covers the
/// overwhelming majority of real-world lossless JPEG files (DNG raw,
/// per-component DICOM tiles). Multi-component interleaved lossless scans
/// raise <see cref="NotSupportedException"/>.
/// </para>
/// </remarks>
internal static class JpegLosslessDecoder
{
    public static ImageFrame Decode(JpegFrame frame, JpegDecoderState state, byte[] scanBytes)
    {
        int p = frame.BitsPerSample;
        if (p is < 2 or > 16)
        {
            throw new ImageFormatException(
                $"JPEG lossless: precision {p} out of range (must be 2..16).");
        }
        if (state.ScanComponentIds.Length != 1)
        {
            throw new NotSupportedException(
                "JPEG lossless: only non-interleaved (single-component) scans are supported.");
        }
        if (frame.NumberOfComponents is not (1 or 3))
        {
            throw new NotSupportedException(
                $"JPEG lossless decoder supports 1- and 3-component images; got {frame.NumberOfComponents}.");
        }
        int psv = state.ScanSs;
        if (psv is < 1 or > 7)
        {
            throw new ImageFormatException(
                $"JPEG lossless: predictor selector {psv} out of range (must be 1..7).");
        }
        int pt = state.ScanAl;
        if (pt < 0 || pt >= p)
        {
            throw new ImageFormatException(
                $"JPEG lossless: point transform {pt} out of range for precision {p}.");
        }

        // Find the component this scan is for. For non-interleaved scans the
        // single SOS component id selects exactly one entry in the SOF table.
        byte scanCompId = state.ScanComponentIds[0];
        int compIndex = -1;
        for (int i = 0; i < frame.Components.Length; i++)
        {
            if (frame.Components[i].Id == scanCompId)
            {
                compIndex = i;
                break;
            }
        }
        if (compIndex < 0)
        {
            throw new ImageFormatException(
                $"JPEG lossless: SOS component id {scanCompId} not declared in SOF.");
        }

        var dcTable = state.DcHuffman[state.ScanDcTables[0]]
            ?? throw new ImageFormatException("JPEG lossless: missing DC Huffman table.");

        // For non-interleaved lossless scans the component is sampled at its full
        // dimensions (sampling factors are ignored per H.1.2.1.4).
        int width = frame.Width;
        int height = frame.Height;

        // Initial prediction at start of scan and after every restart-interval boundary.
        int initialPrediction = 1 << (p - pt - 1);
        // Mask for "modulo 2^P" / "low P bits" arithmetic.
        int sampleMask = (1 << p) - 1;

        var samples = new int[width * height];
        var reader = new JpegBitReader(scanBytes);

        // Decode samples row-major. The predictor for the very first sample
        // is the initial value; subsequent first-row samples use Ra; subsequent
        // first-column samples use Rb; everywhere else uses the selector.
        int restartCounter = state.RestartInterval;
        int restartMcuCount = 0;
        bool justRestarted = true;

        for (int y = 0; y < height; y++)
        {
            int rowOff = y * width;
            for (int x = 0; x < width; x++)
            {
                int px;
                if (justRestarted)
                {
                    // First sample of scan or of any restart interval.
                    px = initialPrediction;
                    justRestarted = false;
                }
                else if (y == 0)
                {
                    // First row (no Rb / Rc) → predictor 1 (Ra).
                    px = samples[rowOff + x - 1] & sampleMask;
                }
                else if (x == 0)
                {
                    // First column of subsequent rows (no Ra / Rc) → predictor 2 (Rb).
                    px = samples[rowOff - width] & sampleMask;
                }
                else
                {
                    int ra = samples[rowOff + x - 1];
                    int rb = samples[rowOff - width + x];
                    int rc = samples[rowOff - width + x - 1];
                    px = psv switch
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

                int s = JpegDecoderShared.DecodeHuffman(ref reader, dcTable);
                int diff;
                if (s == 0)
                {
                    diff = 0;
                }
                else if (s == 16)
                {
                    // Special escape: diff = 32768 (per H.1.2.2).
                    diff = 32768;
                }
                else
                {
                    diff = JpegDecoderShared.ReceiveExtend(ref reader, s);
                }

                samples[rowOff + x] = (px + diff) & sampleMask;

                if (state.RestartInterval > 0)
                {
                    restartMcuCount++;
                    if (restartMcuCount >= state.RestartInterval)
                    {
                        reader.ResetAfterRestart();
                        restartMcuCount = 0;
                        justRestarted = true;
                    }
                }
                _ = restartCounter;
            }
        }

        // Apply inverse point transform: output sample = decoded << Pt.
        return BuildSingleComponentFrame(samples, width, height, p, pt);
    }

    private static ImageFrame BuildSingleComponentFrame(
        int[] samples, int width, int height, int p, int pt)
    {
        if (p <= 8)
        {
            int stride = width;
            var (f, buf) = ImageFrame.Rent(width, height, PixelFormat.Gray8, stride);
            for (int y = 0; y < height; y++)
            {
                int row = y * width;
                int dst = y * stride;
                for (int x = 0; x < width; x++)
                {
                    int v = (samples[row + x] << pt) & 0xFF;
                    buf[dst + x] = (byte)v;
                }
            }
            return f;
        }
        else
        {
            int stride = width * 2;
            int outMask = (1 << Math.Min(p + pt, 16)) - 1;
            var (f, buf) = ImageFrame.Rent(width, height, PixelFormat.Gray16, stride);
            for (int y = 0; y < height; y++)
            {
                int row = y * width;
                int dst = y * stride;
                for (int x = 0; x < width; x++)
                {
                    int v = (samples[row + x] << pt) & outMask;
                    BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(dst + x * 2, 2), (ushort)v);
                }
            }
            return f;
        }
    }
}
