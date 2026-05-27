using Mediar.Imaging;
using Mediar.Imaging.Jpeg;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the progressive-DCT JPEG decoder (SOF2). Embeds a minimal
/// test-only progressive JPEG encoder (grayscale, identity quant, standard
/// Annex K Huffman tables) so we don't have to depend on platform image
/// codecs or external CLI tools to generate progressive fixtures — the
/// Windows GDI+ and WPF JPEG encoders cannot produce SOF2 output.
/// </summary>
public sealed class JpegProgressiveDecoderTests
{
    [Fact]
    public async Task SingleBlock_Uniform_DcThenAc_Roundtrips()
    {
        var pixels = new byte[8, 8];
        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 8; x++)
                pixels[y, x] = 128;

        byte[] jpeg = TestProgressiveEncoder.Encode(pixels, useApproximation: false);

        var decoded = await DecodeGrayAsync(jpeg);

        Assert.Equal(8, decoded.GetLength(1));
        Assert.Equal(8, decoded.GetLength(0));
        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 8; x++)
                Assert.InRange(decoded[y, x], 127, 129);
    }

    [Fact]
    public async Task TwoMcus_HorizontalGradient_DcThenAc_Roundtrips()
    {
        var pixels = new byte[8, 16];
        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 16; x++)
                pixels[y, x] = (byte)(x * 16 + 8);

        byte[] jpeg = TestProgressiveEncoder.Encode(pixels, useApproximation: false);

        var decoded = await DecodeGrayAsync(jpeg);

        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 16; x++)
                Assert.InRange(Math.Abs(decoded[y, x] - pixels[y, x]), 0, 4);
    }

    [Fact]
    public async Task SingleBlock_Uniform_WithSuccessiveApproximation_Roundtrips()
    {
        var pixels = new byte[8, 8];
        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 8; x++)
                pixels[y, x] = 200;

        // Encode with DC initial Al=1 + DC refinement Ah=1/Al=0,
        // and AC initial Al=1 + AC refinement Ah=1/Al=0 → exercises
        // both DC and AC successive-approximation paths in the decoder.
        byte[] jpeg = TestProgressiveEncoder.Encode(pixels, useApproximation: true);

        var decoded = await DecodeGrayAsync(jpeg);

        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 8; x++)
                Assert.InRange(decoded[y, x], 198, 202);
    }

    [Fact]
    public async Task ColorYCbCr444_2x2Mcus_DcInterleavedThenAc_Roundtrips()
    {
        // 16×16 4:4:4 image so each plane has 4 blocks → exercises the
        // interleaved DC scan path (3 components, MCU-ordered) and the
        // three separate single-component AC scans (one per channel).
        int w = 16, h = 16;
        var yPlane = new byte[h, w];
        var cbPlane = new byte[h, w];
        var crPlane = new byte[h, w];

        // Pure grey patch → Y ≈ original, Cb/Cr ≈ 128 (no chroma).
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
            {
                yPlane[y, x] = (byte)(40 + (x + y) * 4);
                cbPlane[y, x] = 128;
                crPlane[y, x] = 128;
            }

        byte[] jpeg = TestProgressiveEncoder.EncodeYCbCr444(yPlane, cbPlane, crPlane);

        var decoded = await DecodeRgbAsync(jpeg);
        Assert.Equal(h, decoded.GetLength(0));
        Assert.Equal(w, decoded.GetLength(1));

        // Reconstructed pixel should be near (Y, Y, Y) since Cb=Cr=128.
        for (int y = 0; y < h; y++)
        {
            for (int x = 0; x < w; x++)
            {
                int expected = yPlane[y, x];
                int r = decoded[y, x].r;
                int g = decoded[y, x].g;
                int b = decoded[y, x].b;
                Assert.InRange(Math.Abs(r - expected), 0, 3);
                Assert.InRange(Math.Abs(g - expected), 0, 3);
                Assert.InRange(Math.Abs(b - expected), 0, 3);
            }
        }
    }

    private static async Task<(byte r, byte g, byte b)[,]> DecodeRgbAsync(byte[] jpegBytes)
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

        int w = reader.Info.Width;
        int h = reader.Info.Height;
        Assert.Equal(PixelFormat.Rgb24, frame!.PixelFormat);
        var pixels = new (byte, byte, byte)[h, w];
        var data = frame.Pixels.Span;
        int stride = frame.Stride;
        for (int y = 0; y < h; y++)
        {
            int row = y * stride;
            for (int x = 0; x < w; x++)
            {
                int o = row + x * 3;
                pixels[y, x] = (data[o], data[o + 1], data[o + 2]);
            }
        }
        frame.Dispose();
        return pixels;
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

        int w = reader.Info.Width;
        int h = reader.Info.Height;
        Assert.Equal(PixelFormat.Gray8, frame!.PixelFormat);
        var pixels = new byte[h, w];
        var data = frame.Pixels.Span;
        int stride = frame.Stride;
        for (int y = 0; y < h; y++)
        {
            for (int x = 0; x < w; x++)
            {
                pixels[y, x] = data[y * stride + x];
            }
        }
        frame.Dispose();
        return pixels;
    }
}

/// <summary>
/// Minimal grayscale progressive JPEG encoder used only by tests. Identity
/// quant table, standard Annex K luminance Huffman tables. Emits either a
/// 2-scan stream (DC initial + AC initial) or a 4-scan stream that exercises
/// both DC and AC successive-approximation refinement.
/// </summary>
internal static class TestProgressiveEncoder
{
    public static byte[] Encode(byte[,] pixels, bool useApproximation)
    {
        int h = pixels.GetLength(0);
        int w = pixels.GetLength(1);
        Assert.True(w % 8 == 0 && h % 8 == 0, "Test only supports multiples of 8.");

        int blocksW = w / 8;
        int blocksH = h / 8;

        // Forward DCT every 8×8 block (centered, identity quant, integer rounded).
        var coefs = new short[blocksH * blocksW * 64];
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                ForwardDct(pixels, bx * 8, by * 8, coefs.AsSpan(blockOff, 64));
            }
        }

        using var ms = new MemoryStream();
        // SOI
        ms.WriteByte(0xFF); ms.WriteByte(0xD8);

        // DQT (identity quantization, table 0, 8-bit values all 1)
        ms.WriteByte(0xFF); ms.WriteByte(0xDB);
        WriteUInt16BE(ms, 67);
        ms.WriteByte(0x00); // Pq=0 (8-bit), Tq=0
        for (int k = 0; k < 64; k++) ms.WriteByte(1);

        // SOF2 — Progressive DCT, single grayscale component, sampling 1×1
        ms.WriteByte(0xFF); ms.WriteByte(0xC2);
        WriteUInt16BE(ms, 11);
        ms.WriteByte(8);                    // P = 8 bits
        WriteUInt16BE(ms, (ushort)h);       // Y
        WriteUInt16BE(ms, (ushort)w);       // X
        ms.WriteByte(1);                    // Nf
        ms.WriteByte(1);                    // component id
        ms.WriteByte(0x11);                 // 1x1 sampling
        ms.WriteByte(0);                    // quant table 0

        // DHT — Annex K luminance DC (table 0)
        WriteDht(ms, tc: 0, th: 0, AnnexK.DcLuminanceBits, AnnexK.DcLuminanceValues);
        // DHT — Annex K luminance AC (table 0)
        WriteDht(ms, tc: 1, th: 0, AnnexK.AcLuminanceBits, AnnexK.AcLuminanceValues);

        if (!useApproximation)
        {
            WriteScan(ms, coefs, blocksW, blocksH, ss: 0, se: 0, ah: 0, al: 0); // DC initial
            WriteScan(ms, coefs, blocksW, blocksH, ss: 1, se: 63, ah: 0, al: 0); // AC initial
        }
        else
        {
            WriteScan(ms, coefs, blocksW, blocksH, ss: 0, se: 0, ah: 0, al: 1); // DC initial, Al=1
            WriteScan(ms, coefs, blocksW, blocksH, ss: 0, se: 0, ah: 1, al: 0); // DC refinement
            WriteScan(ms, coefs, blocksW, blocksH, ss: 1, se: 63, ah: 0, al: 1); // AC initial, Al=1
            WriteScan(ms, coefs, blocksW, blocksH, ss: 1, se: 63, ah: 1, al: 0); // AC refinement
        }

        // EOI
        ms.WriteByte(0xFF); ms.WriteByte(0xD9);
        return ms.ToArray();
    }

    /// <summary>Encode a 3-component YCbCr 4:4:4 progressive JPEG (no chroma subsampling).</summary>
    public static byte[] EncodeYCbCr444(byte[,] yPlane, byte[,] cbPlane, byte[,] crPlane)
    {
        int h = yPlane.GetLength(0);
        int w = yPlane.GetLength(1);
        Assert.True(w % 8 == 0 && h % 8 == 0, "Test only supports multiples of 8.");
        Assert.Equal(h, cbPlane.GetLength(0));
        Assert.Equal(w, cbPlane.GetLength(1));
        Assert.Equal(h, crPlane.GetLength(0));
        Assert.Equal(w, crPlane.GetLength(1));

        int blocksW = w / 8;
        int blocksH = h / 8;

        var coefY = ForwardDctPlane(yPlane, blocksW, blocksH);
        var coefCb = ForwardDctPlane(cbPlane, blocksW, blocksH);
        var coefCr = ForwardDctPlane(crPlane, blocksW, blocksH);

        using var ms = new MemoryStream();
        ms.WriteByte(0xFF); ms.WriteByte(0xD8); // SOI

        // DQT (identity, table 0)
        ms.WriteByte(0xFF); ms.WriteByte(0xDB);
        WriteUInt16BE(ms, 67);
        ms.WriteByte(0x00);
        for (int k = 0; k < 64; k++) ms.WriteByte(1);

        // SOF2 — 3-component 4:4:4
        ms.WriteByte(0xFF); ms.WriteByte(0xC2);
        WriteUInt16BE(ms, 17); // 2 (len) + 1 (P) + 2 (Y) + 2 (X) + 1 (Nf) + 3*3 (comps) = 17
        ms.WriteByte(8);
        WriteUInt16BE(ms, (ushort)h);
        WriteUInt16BE(ms, (ushort)w);
        ms.WriteByte(3); // Nf
        ms.WriteByte(1); ms.WriteByte(0x11); ms.WriteByte(0); // Y, 1×1, qtab 0
        ms.WriteByte(2); ms.WriteByte(0x11); ms.WriteByte(0); // Cb, 1×1, qtab 0
        ms.WriteByte(3); ms.WriteByte(0x11); ms.WriteByte(0); // Cr, 1×1, qtab 0

        // Two Huffman tables: DC table 0 + AC table 0 (Annex K luminance, used for all components).
        WriteDht(ms, tc: 0, th: 0, AnnexK.DcLuminanceBits, AnnexK.DcLuminanceValues);
        WriteDht(ms, tc: 1, th: 0, AnnexK.AcLuminanceBits, AnnexK.AcLuminanceValues);

        // Scan 1: DC initial, interleaved (3 components).
        WriteInterleavedDcScan(ms, coefY, coefCb, coefCr, blocksW, blocksH);
        // Scans 2-4: AC initial per component (AC scans must be single-component).
        WriteScan(ms, coefY, blocksW, blocksH, ss: 1, se: 63, ah: 0, al: 0, compId: 1);
        WriteScan(ms, coefCb, blocksW, blocksH, ss: 1, se: 63, ah: 0, al: 0, compId: 2);
        WriteScan(ms, coefCr, blocksW, blocksH, ss: 1, se: 63, ah: 0, al: 0, compId: 3);

        ms.WriteByte(0xFF); ms.WriteByte(0xD9); // EOI
        return ms.ToArray();
    }

    private static short[] ForwardDctPlane(byte[,] plane, int blocksW, int blocksH)
    {
        var coefs = new short[blocksH * blocksW * 64];
        for (int by = 0; by < blocksH; by++)
            for (int bx = 0; bx < blocksW; bx++)
                ForwardDct(plane, bx * 8, by * 8, coefs.AsSpan((by * blocksW + bx) * 64, 64));
        return coefs;
    }

    /// <summary>Write a 3-component interleaved DC-initial scan (Ss=0, Se=0, Ah=0, Al=0).</summary>
    private static void WriteInterleavedDcScan(
        MemoryStream ms, short[] coefY, short[] coefCb, short[] coefCr,
        int blocksW, int blocksH)
    {
        // SOS header for 3-component DC-only scan.
        ms.WriteByte(0xFF); ms.WriteByte(0xDA);
        WriteUInt16BE(ms, 12); // 2 (len) + 1 (Ns) + 3*2 (comps) + 3 (Ss/Se/AhAl) = 12
        ms.WriteByte(3);                       // Ns
        ms.WriteByte(1); ms.WriteByte(0x00);   // Y:  DC=0 AC=0
        ms.WriteByte(2); ms.WriteByte(0x00);   // Cb: DC=0 AC=0
        ms.WriteByte(3); ms.WriteByte(0x00);   // Cr: DC=0 AC=0
        ms.WriteByte(0);                       // Ss = 0
        ms.WriteByte(0);                       // Se = 0
        ms.WriteByte(0x00);                    // Ah=0, Al=0

        var bw = new BitWriter(ms);
        int prevY = 0, prevCb = 0, prevCr = 0;
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int dcY = coefY[blockOff];
                int dcCb = coefCb[blockOff];
                int dcCr = coefCr[blockOff];
                EncodeHuffmanCoef(bw, dcY - prevY, AnnexK.DcLuminanceCodes);  prevY = dcY;
                EncodeHuffmanCoef(bw, dcCb - prevCb, AnnexK.DcLuminanceCodes); prevCb = dcCb;
                EncodeHuffmanCoef(bw, dcCr - prevCr, AnnexK.DcLuminanceCodes); prevCr = dcCr;
            }
        }
        bw.Flush();
    }

    private static void WriteScan(
        MemoryStream ms, short[] coefs, int blocksW, int blocksH,
        int ss, int se, int ah, int al, byte compId = 1)
    {
        // SOS header
        ms.WriteByte(0xFF); ms.WriteByte(0xDA);
        WriteUInt16BE(ms, 8);
        ms.WriteByte(1);                       // Ns
        ms.WriteByte(compId);                  // component id
        ms.WriteByte(0x00);                    // DC=0, AC=0
        ms.WriteByte((byte)ss);
        ms.WriteByte((byte)se);
        ms.WriteByte((byte)((ah << 4) | al));

        var bw = new BitWriter(ms);

        if (ss == 0)
        {
            if (ah == 0)
            {
                EncodeDcInitial(bw, coefs, blocksW, blocksH, al);
            }
            else
            {
                EncodeDcRefinement(bw, coefs, blocksW, blocksH, al);
            }
        }
        else
        {
            if (ah == 0)
            {
                EncodeAcInitial(bw, coefs, blocksW, blocksH, ss, se, al);
            }
            else
            {
                EncodeAcRefinement(bw, coefs, blocksW, blocksH, ss, se, al);
            }
        }

        bw.Flush();
    }

    private static void EncodeDcInitial(
        BitWriter bw, short[] coefs, int blocksW, int blocksH, int al)
    {
        int prev = 0;
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int dc = coefs[blockOff] >> al;
                int diff = dc - prev;
                prev = dc;
                EncodeHuffmanCoef(bw, diff, AnnexK.DcLuminanceCodes);
            }
        }
    }

    private static void EncodeDcRefinement(
        BitWriter bw, short[] coefs, int blocksW, int blocksH, int al)
    {
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int bit = (coefs[blockOff] >> al) & 1;
                bw.WriteBit(bit);
            }
        }
    }

    private static void EncodeAcInitial(
        BitWriter bw, short[] coefs, int blocksW, int blocksH, int ss, int se, int al)
    {
        // No EOB-run consolidation in the test encoder — emit an explicit EOB
        // for every block. Simple, correct, and exercises the decoder's
        // single-block EOB path.
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int run = 0;
                for (int k = ss; k <= se; k++)
                {
                    int coef = coefs[blockOff + JpegDecoderHelpersShared.Zigzag[k]] >> al;
                    if (coef == 0)
                    {
                        run++;
                    }
                    else
                    {
                        while (run >= 16)
                        {
                            EncodeHuffmanRaw(bw, 0xF0, AnnexK.AcLuminanceCodes); // ZRL
                            run -= 16;
                        }
                        int s = MagnitudeBits(coef);
                        int rs = (run << 4) | s;
                        EncodeHuffmanRaw(bw, rs, AnnexK.AcLuminanceCodes);
                        WriteExtendValue(bw, coef, s);
                        run = 0;
                    }
                }
                if (run > 0)
                {
                    EncodeHuffmanRaw(bw, 0x00, AnnexK.AcLuminanceCodes); // EOB
                }
            }
        }
    }

    private static void EncodeAcRefinement(
        BitWriter bw, short[] coefs, int blocksW, int blocksH, int ss, int se, int al)
    {
        // Encode per-block, walking [ss..se] in zig-zag order. For each
        // already-nonzero coef emit its refinement bit. For each newly
        // appearing coef (set in this approximation level) emit a (run, 1)
        // RS pair followed by a sign bit. If the block has no new coefs,
        // emit EOB (RS = 0x00) at the end.
        for (int by = 0; by < blocksH; by++)
        {
            for (int bx = 0; bx < blocksW; bx++)
            {
                int blockOff = (by * blocksW + bx) * 64;
                int run = 0;
                for (int k = ss; k <= se; k++)
                {
                    int natural = JpegDecoderHelpersShared.Zigzag[k];
                    int prev = coefs[blockOff + natural] >> (al + 1);
                    int curr = coefs[blockOff + natural] >> al;
                    bool wasNonzero = prev != 0;
                    if (wasNonzero)
                    {
                        // Refinement: emit the new LSB.
                        // (Cannot emit refinement bits without first emitting an RS pair
                        //  for the run-since-last-decoded-coef. The decoder reads
                        //  refinement bits inside the inner zero-run loop after each RS
                        //  decode, so we must flush the run as a (run, 1) RS pair pointing
                        //  at the next *new* nonzero. Simpler: handle ahead-of-time.)
                    }
                    _ = curr;
                }

                // Simpler correct strategy: split coefficients into (zero, was-nonzero, new-nonzero).
                // Iterate k from ss to se. Maintain run = count of zero coefs since last RS.
                // For each k:
                //   if prev != 0: this is a "skip with refinement" → refinement bit goes in
                //                 the inner loop, doesn't terminate the run.
                //   else if curr != 0: this is "newly nonzero" → emit RS (run, 1) + sign bit.
                //                       Then run := 0.
                //   else: run++.
                // After the loop, if run > 0 or there are remaining was-nonzero coefs, emit EOB
                // and put refinement bits in the EOB-tail loop.
                run = 0;
                int lastNewK = ss - 1;
                // First pass: count new-nonzero positions to know whether we need EOB at end.
                int newCount = 0;
                for (int k = ss; k <= se; k++)
                {
                    int natural = JpegDecoderHelpersShared.Zigzag[k];
                    int prev0 = coefs[blockOff + natural] >> (al + 1);
                    int curr0 = coefs[blockOff + natural] >> al;
                    if (prev0 == 0 && curr0 != 0) newCount++;
                }

                // Second pass: encode.
                int emitted = 0;
                for (int k = ss; k <= se; k++)
                {
                    int natural = JpegDecoderHelpersShared.Zigzag[k];
                    int prev0 = coefs[blockOff + natural] >> (al + 1);
                    int curr0 = coefs[blockOff + natural] >> al;
                    bool wasNz = prev0 != 0;
                    bool newNz = !wasNz && curr0 != 0;

                    if (newNz)
                    {
                        // Skip r = run zeros (run counts pure zero coefs since the last RS pair).
                        // For each was-nonzero we encountered while accumulating run, the decoder
                        // will consume a refinement bit. So we have to interleave: every time we
                        // pass over a was-nonzero we must remember to emit a refinement bit
                        // *after* the RS pair for the next new-nonzero.
                        // The libjpeg encoder does this by tracking which positions need
                        // refinement bits during the inner skip-r-zeros loop.
                        // Simpler approach: emit RS = (run, 1), value sign bit, then for
                        // each was-nonzero in the band [lastNewK+1 .. k-1] emit a refinement bit
                        // immediately after the RS pair (the decoder reads refinement bits
                        // *during* the skip loop, between each skip-step).
                        EncodeHuffmanRaw(bw, (run << 4) | 1, AnnexK.AcLuminanceCodes);
                        // Now emit refinement bits + sign bit interleaved exactly as the
                        // decoder consumes them. Decoder reads from k = lastNewK+1, looking
                        // for r-th zero, refining nonzeros along the way.
                        int kk = lastNewK + 1;
                        int rDown = run;
                        while (true)
                        {
                            int nat = JpegDecoderHelpersShared.Zigzag[kk];
                            int p2 = coefs[blockOff + nat] >> (al + 1);
                            if (p2 != 0)
                            {
                                // Was-nonzero. Decoder reads 1 refinement bit.
                                int rb = (coefs[blockOff + nat] >> al) & 1;
                                bw.WriteBit(rb);
                            }
                            else
                            {
                                if (--rDown < 0) break;
                            }
                            kk++;
                            if (kk > se) break;
                        }
                        // After exiting, kk should equal k (the new-nonzero position).
                        // Emit the sign bit: 1 = positive, 0 = negative.
                        bw.WriteBit(curr0 > 0 ? 1 : 0);
                        run = 0;
                        lastNewK = k;
                        emitted++;
                        if (emitted == newCount)
                        {
                            // No more new-nonzeros in the rest of the band. We still need to
                            // emit refinement bits for any was-nonzero in (k .. se].
                            for (int k2 = k + 1; k2 <= se; k2++)
                            {
                                int nat2 = JpegDecoderHelpersShared.Zigzag[k2];
                                int p3 = coefs[blockOff + nat2] >> (al + 1);
                                if (p3 != 0)
                                {
                                    // These refinement bits belong inside the EOB-tail loop
                                    // of the decoder. We'll emit them after the EOB code.
                                }
                            }
                        }
                    }
                    else if (!wasNz)
                    {
                        run++;
                    }
                    // wasNz with no new value: nothing to do here; the decoder reads a
                    // refinement bit for it inside the next skip-loop iteration.
                }

                // If we emitted all new-nonzeros (or had none), but there are still
                // positions after lastNewK with refinement bits to emit, emit EOB then
                // the refinement bits for any was-nonzero in (lastNewK .. se].
                int tailStart = lastNewK + 1;
                bool needTail = tailStart <= se;
                if (needTail)
                {
                    EncodeHuffmanRaw(bw, 0x00, AnnexK.AcLuminanceCodes); // EOB with run 0 → eobRun=1
                    for (int k2 = tailStart; k2 <= se; k2++)
                    {
                        int nat2 = JpegDecoderHelpersShared.Zigzag[k2];
                        int p2 = coefs[blockOff + nat2] >> (al + 1);
                        if (p2 != 0)
                        {
                            int rb = (coefs[blockOff + nat2] >> al) & 1;
                            bw.WriteBit(rb);
                        }
                    }
                }
            }
        }
    }

    private static void EncodeHuffmanCoef(BitWriter bw, int v, (int len, int code)[] table)
    {
        int s = MagnitudeBits(v);
        var (len, code) = table[s];
        bw.WriteBits(code, len);
        if (s > 0) WriteExtendValue(bw, v, s);
    }

    private static void EncodeHuffmanRaw(BitWriter bw, int symbol, (int len, int code)[] table)
    {
        var (len, code) = table[symbol];
        bw.WriteBits(code, len);
    }

    private static void WriteExtendValue(BitWriter bw, int v, int s)
    {
        int code = v < 0 ? v + (1 << s) - 1 : v;
        bw.WriteBits(code, s);
    }

    private static int MagnitudeBits(int v)
    {
        int a = v < 0 ? -v : v;
        int bits = 0;
        while (a > 0) { bits++; a >>= 1; }
        return bits;
    }

    private static void ForwardDct(byte[,] pixels, int x0, int y0, Span<short> outCoefs)
    {
        Span<float> f = stackalloc float[64];
        for (int y = 0; y < 8; y++)
            for (int x = 0; x < 8; x++)
                f[y * 8 + x] = pixels[y0 + y, x0 + x] - 128f;

        Span<float> temp = stackalloc float[64];
        for (int y = 0; y < 8; y++)
        {
            for (int u = 0; u < 8; u++)
            {
                float sum = 0;
                for (int x = 0; x < 8; x++)
                    sum += f[y * 8 + x] * MathF.Cos((2 * x + 1) * u * MathF.PI / 16f);
                float cu = u == 0 ? MathF.Sqrt(0.5f) : 1f;
                temp[y * 8 + u] = sum * cu * 0.5f;
            }
        }
        for (int u = 0; u < 8; u++)
        {
            for (int v = 0; v < 8; v++)
            {
                float sum = 0;
                for (int y = 0; y < 8; y++)
                    sum += temp[y * 8 + u] * MathF.Cos((2 * y + 1) * v * MathF.PI / 16f);
                float cv = v == 0 ? MathF.Sqrt(0.5f) : 1f;
                int rounded = (int)MathF.Round(sum * cv * 0.5f);
                if (rounded < short.MinValue) rounded = short.MinValue;
                else if (rounded > short.MaxValue) rounded = short.MaxValue;
                // Natural order [v * 8 + u].
                outCoefs[v * 8 + u] = (short)rounded;
            }
        }
    }

    private static void WriteDht(MemoryStream ms, int tc, int th, byte[] bits, byte[] values)
    {
        int len = 2 + 1 + 16 + values.Length;
        ms.WriteByte(0xFF); ms.WriteByte(0xC4);
        WriteUInt16BE(ms, (ushort)len);
        ms.WriteByte((byte)((tc << 4) | th));
        ms.Write(bits, 0, 16);
        ms.Write(values, 0, values.Length);
    }

    private static void WriteUInt16BE(MemoryStream ms, ushort v)
    {
        ms.WriteByte((byte)(v >> 8));
        ms.WriteByte((byte)(v & 0xFF));
    }
}

/// <summary>Bit writer for JPEG entropy data: MSB-first, with FF stuffing.</summary>
internal sealed class BitWriter
{
    private readonly MemoryStream _ms;
    private uint _buffer;
    private int _bits;

    public BitWriter(MemoryStream ms) { _ms = ms; }

    public void WriteBit(int b) => WriteBits(b & 1, 1);

    public void WriteBits(int value, int len)
    {
        _buffer = (_buffer << len) | (uint)(value & ((1 << len) - 1));
        _bits += len;
        while (_bits >= 8)
        {
            byte b = (byte)((_buffer >> (_bits - 8)) & 0xFF);
            _ms.WriteByte(b);
            if (b == 0xFF) _ms.WriteByte(0x00); // byte stuffing
            _bits -= 8;
        }
    }

    public void Flush()
    {
        if (_bits > 0)
        {
            byte b = (byte)((_buffer << (8 - _bits)) | (0xFFu >> _bits));
            _ms.WriteByte(b);
            if (b == 0xFF) _ms.WriteByte(0x00);
            _bits = 0;
            _buffer = 0;
        }
    }
}

/// <summary>Mirror of <see cref="JpegDecoderShared"/>.Zigzag — duplicated here so we
/// don't need to expose internals of Mediar.Imaging.Jpeg.</summary>
internal static class JpegDecoderHelpersShared
{
    public static readonly int[] Zigzag =
    [
         0,  1,  8, 16,  9,  2,  3, 10,
        17, 24, 32, 25, 18, 11,  4,  5,
        12, 19, 26, 33, 40, 48, 41, 34,
        27, 20, 13,  6,  7, 14, 21, 28,
        35, 42, 49, 56, 57, 50, 43, 36,
        29, 22, 15, 23, 30, 37, 44, 51,
        58, 59, 52, 45, 38, 31, 39, 46,
        53, 60, 61, 54, 47, 55, 62, 63,
    ];
}

/// <summary>Annex K standard JPEG Huffman tables (luminance only — sufficient
/// for the test encoder's grayscale output).</summary>
internal static class AnnexK
{
    public static readonly byte[] DcLuminanceBits =
        [0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0];

    public static readonly byte[] DcLuminanceValues =
        [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11];

    public static readonly byte[] AcLuminanceBits =
        [0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 0x7D];

    public static readonly byte[] AcLuminanceValues =
    [
        0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12,
        0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07,
        0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
        0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0,
        0x24, 0x33, 0x62, 0x72, 0x82, 0x09, 0x0A, 0x16,
        0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
        0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39,
        0x3A, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49,
        0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
        0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69,
        0x6A, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79,
        0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
        0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98,
        0x99, 0x9A, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
        0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
        0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5,
        0xC6, 0xC7, 0xC8, 0xC9, 0xCA, 0xD2, 0xD3, 0xD4,
        0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
        0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA,
        0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8,
        0xF9, 0xFA,
    ];

    public static readonly (int len, int code)[] DcLuminanceCodes =
        BuildEncoderTable(DcLuminanceBits, DcLuminanceValues, 256);

    public static readonly (int len, int code)[] AcLuminanceCodes =
        BuildEncoderTable(AcLuminanceBits, AcLuminanceValues, 256);

    private static (int len, int code)[] BuildEncoderTable(byte[] bits, byte[] values, int size)
    {
        var table = new (int, int)[size];
        int code = 0;
        int idx = 0;
        for (int L = 1; L <= 16; L++)
        {
            int count = bits[L - 1];
            for (int n = 0; n < count; n++)
            {
                byte sym = values[idx++];
                table[sym] = (L, code);
                code++;
            }
            code <<= 1;
        }
        return table;
    }
}
