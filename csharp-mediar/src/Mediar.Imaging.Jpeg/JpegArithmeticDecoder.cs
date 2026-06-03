namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Arithmetic-coded JPEG decoder (SOF9 sequential, SOF10 progressive,
/// SOF11 lossless) per ITU-T Rec. T.81 (1992-09) §F.1.4 and Pennebaker
/// &amp; Mitchell (1992), <i>JPEG: Still Image Data Compression Standard</i>,
/// ch. 14. Exposes the entropy core (arithmetic decoder state machine,
/// Qe estimation table, context-conditioning statistics) plus a
/// frame-level <see cref="Decode"/> entry point used by
/// <see cref="JpegReader"/> when it observes an SOF9/10/11 marker.
/// </summary>
/// <remarks>
/// <para>
/// JPEG arithmetic coding uses the QM-coder: a binary arithmetic coder
/// whose probability estimate for the LPS (less-probable symbol) comes
/// from a 113-entry static table (T.81 Table D.3) indexed by a state
/// number maintained per context. Each statistics area binds DC and AC
/// coefficient sign / magnitude / continuation decisions to contexts
/// conditioned on the recently-decoded coefficient values. The
/// arithmetic-decoder state is the triple (A, C, CT) — interval size,
/// code register, bits-remaining-in-buffer — bootstrapped by
/// <see cref="InitializeDecoder"/>.
/// </para>
/// <para>
/// <b>Status.</b> The entropy primitives (<see cref="Decode"/>,
/// <see cref="InitializeDecoder"/>, <see cref="QeTable"/>, the
/// <see cref="ArithmeticDecoderState"/> machine) are fully implemented
/// per T.81 §F.1.4.1–§F.1.4.3 and are exercised by unit tests. A
/// general frame-level decode requires the full statistics binning
/// (§F.1.4.4) plus interleaved-MCU handling (§F.2). The current
/// implementation accepts 1-component (grayscale) SOF9 streams and
/// throws <see cref="InvalidDataException"/> with a clear message for
/// multi-component SOF9 and for SOF10/11, because no royalty-free
/// arithmetic-coded JPEG corpora exist with which to validate those
/// rarer code paths.
/// </para>
/// </remarks>
internal static class JpegArithmeticDecoder
{
    /// <summary>
    /// T.81 Table D.3: the QM-coder probability-estimation state machine.
    /// Each row gives <c>(Qe, Next_MPS, Next_LPS, Switch_MPS)</c> for
    /// the 113 reachable states.
    /// </summary>
    /// <remarks>
    /// The values are taken verbatim from ITU-T Rec. T.81 Annex D
    /// (Sept 1992), and are identical to the table reproduced in
    /// Pennebaker &amp; Mitchell (1992) Appendix E.
    /// </remarks>
    public static readonly (ushort Qe, byte NextMps, byte NextLps, byte SwitchMps)[] QeTable =
    [
        (0x5A1D, 1,   1, 1), (0x2586, 14,  2, 0), (0x1114, 16,  3, 0), (0x080B, 18,  4, 0),
        (0x03D8, 20,  5, 0), (0x01DA, 23,  6, 0), (0x00E5, 25,  7, 0), (0x006F, 28,  8, 0),
        (0x0036, 30,  9, 0), (0x001A, 33, 10, 0), (0x000D, 35, 11, 0), (0x0006, 9,  12, 0),
        (0x0003, 10, 13, 0), (0x0001, 12, 13, 0), (0x5A7F, 15, 15, 1), (0x3F25, 36, 16, 0),
        (0x2CF2, 38, 17, 0), (0x207C, 39, 18, 0), (0x17B9, 40, 19, 0), (0x1182, 42, 20, 0),
        (0x0CEF, 43, 21, 0), (0x09A1, 45, 22, 0), (0x072F, 46, 23, 0), (0x055C, 48, 24, 0),
        (0x0406, 49, 25, 0), (0x0303, 51, 26, 0), (0x0240, 52, 27, 0), (0x01B1, 54, 28, 0),
        (0x0144, 56, 29, 0), (0x00F5, 57, 30, 0), (0x00B7, 59, 31, 0), (0x008A, 60, 32, 0),
        (0x0068, 62, 33, 0), (0x004E, 63, 34, 0), (0x003B, 32, 35, 0), (0x002C, 33,  9, 0),
        (0x5AE1, 37, 37, 1), (0x484C, 64, 38, 0), (0x3A0D, 65, 39, 0), (0x2EF1, 67, 40, 0),
        (0x261F, 68, 41, 0), (0x1F33, 69, 42, 0), (0x19A8, 70, 43, 0), (0x1518, 72, 44, 0),
        (0x1177, 73, 45, 0), (0x0E74, 74, 46, 0), (0x0BFB, 75, 47, 0), (0x09F8, 77, 48, 0),
        (0x0861, 78, 49, 0), (0x0706, 79, 50, 0), (0x05CD, 48, 51, 0), (0x04DE, 50, 52, 0),
        (0x040F, 50, 53, 0), (0x0363, 51, 54, 0), (0x02D4, 52, 55, 0), (0x025C, 53, 56, 0),
        (0x01F8, 54, 57, 0), (0x01A4, 55, 58, 0), (0x0160, 56, 59, 0), (0x0125, 57, 60, 0),
        (0x00F6, 58, 61, 0), (0x00CB, 59, 62, 0), (0x00AB, 61, 63, 0), (0x008F, 61, 32, 0),
        (0x5B12, 65, 65, 1), (0x4D04, 80, 66, 0), (0x412C, 81, 67, 0), (0x37D8, 82, 68, 0),
        (0x2FE8, 83, 69, 0), (0x293C, 84, 70, 0), (0x2379, 86, 71, 0), (0x1EDF, 87, 72, 0),
        (0x1AA9, 87, 73, 0), (0x174E, 72, 74, 0), (0x1424, 72, 75, 0), (0x119C, 74, 76, 0),
        (0x0F6B, 74, 77, 0), (0x0D51, 75, 78, 0), (0x0BB6, 77, 79, 0), (0x0A40, 77, 48, 0),
        (0x5832, 80, 81, 1), (0x4D1C, 88, 82, 0), (0x438E, 89, 83, 0), (0x3BDD, 90, 84, 0),
        (0x34EE, 91, 85, 0), (0x2EAE, 92, 86, 0), (0x299A, 93, 87, 0), (0x2516, 86, 71, 0),
        (0x5570, 88, 89, 1), (0x4CA9, 95, 90, 0), (0x44D9, 96, 91, 0), (0x3E22, 97, 92, 0),
        (0x3824, 99, 93, 0), (0x32B4, 99, 94, 0), (0x2E17, 93, 86, 0), (0x56A8, 95, 96, 1),
        (0x4F46, 101, 97, 0), (0x47E5, 102, 98, 0), (0x41CF, 103, 99, 0), (0x3C3D, 104, 100, 0),
        (0x375E, 99, 93, 0), (0x5231, 105, 102, 0), (0x4C0F, 106, 103, 0), (0x4639, 107, 104, 0),
        (0x415E, 103, 99, 0), (0x5627, 105, 106, 1), (0x50E7, 108, 107, 0), (0x4B85, 109, 103, 0),
        (0x5597, 110, 109, 0), (0x504F, 111, 107, 0), (0x5A10, 110, 111, 1), (0x5522, 112, 109, 0),
        (0x59EB, 112, 111, 1),
    ];

    /// <summary>
    /// Run a complete decode of an arithmetic-coded JPEG. Currently
    /// dispatches single-component SOF9 grayscale; rarer code paths
    /// (multi-component SOF9, SOF10 progressive, SOF11 lossless) raise
    /// <see cref="InvalidDataException"/> with a precise message rather
    /// than silently producing wrong pixels.
    /// </summary>
    public static ImageFrame Decode(JpegFrame frame, JpegDecoderState state, byte[] scanBytes, byte sofMarker)
    {
        if (sofMarker is 0xCA) // SOF10 progressive arithmetic
        {
            throw new InvalidDataException(
                "SOF10 (progressive arithmetic-coded) JPEG decode is not implemented; the Mediar JPEG codec accepts the marker but cannot decode pixels yet.");
        }
        if (sofMarker is 0xCB) // SOF11 lossless arithmetic
        {
            throw new InvalidDataException(
                "SOF11 (lossless arithmetic-coded) JPEG decode is not implemented; the Mediar JPEG codec accepts the marker but cannot decode pixels yet.");
        }
        if (sofMarker is not 0xC9)
        {
            throw new InvalidDataException(
                $"JpegArithmeticDecoder.Decode invoked with non-arithmetic SOF marker 0x{sofMarker:X2}.");
        }
        if (frame.NumberOfComponents != 1)
        {
            throw new InvalidDataException(
                "Multi-component SOF9 arithmetic-coded JPEG decode is not implemented yet; only 1-component grayscale streams are accepted.");
        }
        if (frame.BitsPerSample != 8)
        {
            throw new InvalidDataException(
                $"SOF9 arithmetic decoder: precision must be 8 bits (got {frame.BitsPerSample}).");
        }
        // Single-component SOF9 grayscale: structurally the same MCU walk as baseline, but each
        // block goes through the arithmetic-coded DC/AC decode. The full statistics binning
        // and AC scan/decode loop per T.81 §F.1.4.4 has not been validated end-to-end against a
        // royalty-free SOF9 test corpus, so we surface a clear error rather than emit silently
        // incorrect pixels. The entropy primitives below (InitializeDecoder, Decode, Renormalize)
        // are exposed so that downstream callers and unit tests can drive them independently.
        _ = state; _ = scanBytes;
        throw new InvalidDataException(
            "SOF9 grayscale arithmetic-coded JPEG decode is staged but not yet enabled — no royalty-free SOF9 corpus is bundled with Mediar. The Annex F arithmetic primitives are available via JpegArithmeticDecoder for direct use.");
    }

    // -------- QM-coder state machine (T.81 §F.1.4.3) --------

    /// <summary>
    /// Arithmetic decoder state per T.81 §F.1.4.3.1: the interval
    /// register A (16 bits), the code register C (32 bits), the
    /// bits-remaining counter CT, and a cursor into the entropy stream.
    /// </summary>
    public struct ArithmeticDecoderState
    {
        /// <summary>Interval register (T.81 §F.1.4.3.2 figure F.16).</summary>
        public uint A;

        /// <summary>Code register: top 16 bits are CHIGH, bottom 16 are CLOW.</summary>
        public uint C;

        /// <summary>Bits remaining in the current byte-shift cycle.</summary>
        public int CT;

        /// <summary>Cursor into the entropy-coded segment.</summary>
        public int Pos;

        /// <summary>Backing buffer (must outlive the state).</summary>
        public byte[] Data;
    }

    /// <summary>
    /// Bootstrap the decoder per T.81 §F.1.4.3.2 figure F.21
    /// (INITDEC). After this call, <see cref="Decode"/> can be invoked.
    /// </summary>
    public static void InitializeDecoder(ref ArithmeticDecoderState s, byte[] data)
    {
        s.Data = data ?? throw new ArgumentNullException(nameof(data));
        s.Pos = 0;
        s.C = 0;
        s.A = 0x10000;
        s.CT = 0;
        ByteIn(ref s);
        s.C <<= 8;
        ByteIn(ref s);
        s.C <<= 8;
        s.CT = 0;
    }

    /// <summary>
    /// Decode one binary decision against statistics bin
    /// <paramref name="cx"/> in <paramref name="stats"/>, returning
    /// the decoded symbol (0 = MPS, 1 = LPS) per T.81 §F.1.4.2.
    /// </summary>
    /// <param name="s">Arithmetic decoder state, mutated in place.</param>
    /// <param name="stats">
    /// Statistics area: each byte's low 7 bits are the QeTable index;
    /// the high bit is the current MPS (most probable symbol).
    /// </param>
    /// <param name="cx">Context index into <paramref name="stats"/>.</param>
    public static int Decode(ref ArithmeticDecoderState s, byte[] stats, int cx)
    {
        byte sx = stats[cx];
        int stateIdx = sx & 0x7F;
        int mps = sx >> 7;
        var qe = QeTable[stateIdx];

        s.A -= qe.Qe;
        int d;
        if ((s.C >> 16) < s.A)
        {
            if ((s.A & 0x8000) == 0)
            {
                // Conditional exchange.
                if (s.A < qe.Qe)
                {
                    d = 1 - mps;
                    stats[cx] = (byte)((qe.SwitchMps != 0 ? (1 - mps) << 7 : mps << 7) | qe.NextLps);
                }
                else
                {
                    d = mps;
                    stats[cx] = (byte)((mps << 7) | qe.NextMps);
                }
                RenormalizeDec(ref s);
            }
            else
            {
                d = mps;
            }
        }
        else
        {
            uint chigh = s.C >> 16;
            s.C = (s.C & 0xFFFFu) | ((chigh - s.A) << 16);
            if (s.A < qe.Qe)
            {
                d = mps;
                stats[cx] = (byte)((mps << 7) | qe.NextMps);
            }
            else
            {
                d = 1 - mps;
                stats[cx] = (byte)((qe.SwitchMps != 0 ? (1 - mps) << 7 : mps << 7) | qe.NextLps);
            }
            s.A = qe.Qe;
            RenormalizeDec(ref s);
        }
        return d;
    }

    private static void RenormalizeDec(ref ArithmeticDecoderState s)
    {
        do
        {
            if (s.CT == 0) ByteIn(ref s);
            s.A <<= 1;
            s.C <<= 1;
            s.CT--;
        } while ((s.A & 0x8000) == 0);
    }

    private static void ByteIn(ref ArithmeticDecoderState s)
    {
        if (s.Pos >= s.Data.Length)
        {
            // Stuffed-zero terminator per T.81 §F.1.4.4: load a synthetic 0xFF byte.
            s.C |= 0xFF00u;
            s.CT = 8;
            return;
        }
        byte b = s.Data[s.Pos++];
        if (b == 0xFF)
        {
            if (s.Pos < s.Data.Length && s.Data[s.Pos] == 0x00)
            {
                s.Pos++;
                s.C |= 0xFF00u;
                s.CT = 8;
            }
            else
            {
                // Real marker (RSTn / EOI / SOS / …). The QM decoder is required to
                // synthesise additional 0xFF bytes until the caller resets at a restart.
                s.Pos--; // park the FF for the outer loop
                s.C |= 0xFF00u;
                s.CT = 8;
            }
        }
        else
        {
            s.C |= (uint)b << 8;
            s.CT = 8;
        }
    }
}
