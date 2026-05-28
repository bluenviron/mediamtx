using System.Buffers;
using Mediar.Codecs.Flac.Decoder;
using Mediar.IO;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Encodes a single FLAC frame from interleaved integer PCM samples into the
/// byte representation defined by RFC 9639 §10. The encoder emits
/// <c>CONSTANT</c>, <c>VERBATIM</c>, <c>FIXED</c> (orders 0..4) and <c>LPC</c>
/// (orders 1..12 via Welch-windowed autocorrelation + Levinson-Durbin) subframes
/// with multi-partition Rice / Rice2 residual coding (partition_order ∈ [0, 6]),
/// automatically picking the cheapest representation per channel. For stereo
/// input (Channels == 2, bps &lt; 32) the encoder additionally picks the
/// cheapest of independent / left-side / side-right / mid-side decorrelation.
/// The output is a valid FLAC frame that the existing
/// <see cref="FlacFrameHeaderParser"/> / <see cref="FlacSubframeDecoder"/> pair
/// round-trips back to the exact input samples.
/// </summary>
public static class FlacFrameEncoder
{
    /// <summary>
    /// Worst-case byte count for a single frame at the given block size. Use
    /// this to size the output buffer passed to <see cref="EncodeFrame"/>.
    /// </summary>
    /// <remarks>
    /// Layout budget: ≤14 byte header (sync 2 B + UTF-8 frame number ≤7 B +
    /// block-size extension ≤2 B + CRC-8 1 B + slack 2 B) plus
    /// <c>C × (1 + ⌈N × (bps+1) / 8⌉)</c> bytes of subframes (1 B subframe header
    /// + verbatim sample words; the +1 covers a side-channel at bps+1) plus 3 B
    /// for byte-alignment padding + CRC-16.
    /// </remarks>
    public static int MaxFrameSize(FlacEncoderParameters parameters, int samplesPerChannel)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        parameters.Validate();
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(samplesPerChannel);
        // Use bps + 1 in the per-channel budget so a stereo side channel
        // (which carries bps + 1 bits per residual sample in the worst case)
        // still fits.
        int budgetBps = parameters.BitsPerSample == 32 ? 32 : parameters.BitsPerSample + 1;
        long sampleBytes = parameters.Channels * (1L + (samplesPerChannel * (long)budgetBps + 7) / 8);
        return checked((int)(17 + sampleBytes));
    }

    /// <summary>
    /// Encode one frame.
    /// </summary>
    /// <param name="parameters">Stream parameters (channels / sample rate / bps / blockSize).</param>
    /// <param name="interleavedSamples">Channel-interleaved samples
    /// (<c>samplesPerChannel × parameters.Channels</c> entries; <c>samples[i*C + c]</c>
    /// is the <c>c</c>-th channel's <c>i</c>-th sample).</param>
    /// <param name="samplesPerChannel">Block size of THIS frame in samples per
    /// channel. May be less than <see cref="FlacEncoderParameters.BlockSize"/>
    /// for the final frame.</param>
    /// <param name="frameNumber">Fixed-blocksize frame number (zero-based).</param>
    /// <param name="output">Output buffer. Must be zero-initialised; size with
    /// <see cref="MaxFrameSize"/>.</param>
    /// <returns>Number of bytes written.</returns>
    public static int EncodeFrame(
        FlacEncoderParameters parameters,
        ReadOnlySpan<int> interleavedSamples,
        int samplesPerChannel,
        ulong frameNumber,
        Span<byte> output)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        parameters.Validate();
        if (samplesPerChannel is < 1 or > 65535)
        {
            throw new ArgumentOutOfRangeException(nameof(samplesPerChannel), samplesPerChannel, "Frame block size must be in [1, 65535].");
        }
        int expected = checked(samplesPerChannel * parameters.Channels);
        if (interleavedSamples.Length < expected)
        {
            throw new ArgumentException($"Need {expected} interleaved samples (got {interleavedSamples.Length}).", nameof(interleavedSamples));
        }
        if (parameters.BitsPerSample < 32)
        {
            ValidateSampleRange(interleavedSamples[..expected], parameters.BitsPerSample);
        }
        int needed = MaxFrameSize(parameters, samplesPerChannel);
        if (output.Length < needed)
        {
            throw new ArgumentException($"Output buffer too small (need {needed}, have {output.Length}).", nameof(output));
        }
        output.Clear();

        int bps = parameters.BitsPerSample;

        // Rent scratches once at frame entry and reuse across all channels.
        int[] channelBuf0 = ArrayPool<int>.Shared.Rent(samplesPerChannel);
        int[] residualBuf = ArrayPool<int>.Shared.Rent(samplesPerChannel);
        double[] windowedBuf = ArrayPool<double>.Shared.Rent(samplesPerChannel);
        int[] qcoefBuf = ArrayPool<int>.Shared.Rent(FlacLpcPredictor.MaxOrder);
        int[] ksBuf = ArrayPool<int>.Shared.Rent(1 << FlacRice.MaxPartitionOrder);
        // Stereo decorrelation buffers (only rented when applicable).
        bool stereoMode = parameters.Channels == 2 && bps < 32;
        int[]? channelBuf1 = stereoMode ? ArrayPool<int>.Shared.Rent(samplesPerChannel) : null;
        int[]? sideBuf = stereoMode ? ArrayPool<int>.Shared.Rent(samplesPerChannel) : null;
        int[]? midBuf = stereoMode ? ArrayPool<int>.Shared.Rent(samplesPerChannel) : null;
        try
        {
            // -- Channel-assignment selection (§10.1.7) --
            int chanAssignmentNibble = parameters.Channels - 1;
            if (stereoMode)
            {
                // Deinterleave L, R into the first two buffers.
                for (int i = 0; i < samplesPerChannel; i++)
                {
                    channelBuf0[i] = interleavedSamples[i * 2 + 0];
                    channelBuf1![i] = interleavedSamples[i * 2 + 1];
                }
                // Derive Side = L - R and Mid = (L + R) >> 1 (arithmetic shift,
                // not divide — matches the decoder reconstruction L = M + ((S+S0)>>1)+(S&1)
                // when S=L-R, M=(L+R)>>1).
                for (int i = 0; i < samplesPerChannel; i++)
                {
                    int l = channelBuf0[i];
                    int r = channelBuf1![i];
                    sideBuf![i] = l - r;
                    midBuf![i] = (l + r) >> 1;
                }
                long costL = EstimateSubframeBits(channelBuf0.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf);
                long costR = EstimateSubframeBits(channelBuf1.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf);
                long costS = EstimateSubframeBits(sideBuf.AsSpan(0, samplesPerChannel), bps + 1, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf);
                long costM = EstimateSubframeBits(midBuf!.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf);

                long costInd = costL + costR;
                long costLs = costL + costS;
                long costSr = costS + costR;
                long costMs = costM + costS;

                long bestCost = costInd; chanAssignmentNibble = 0b0001;
                if (costLs < bestCost) { bestCost = costLs; chanAssignmentNibble = 0b1000; }
                if (costSr < bestCost) { bestCost = costSr; chanAssignmentNibble = 0b1001; }
                if (costMs < bestCost) { chanAssignmentNibble = 0b1010; }
            }

            // -- Frame header (§10.1) --
            var bw = new BitWriter(output);
            bw.WriteBits(0b11111111111110u, 14); // sync code
            bw.WriteBit(false);                  // reserved
            bw.WriteBit(false);                  // blocking_strategy: 0 = fixed → use frame number

            (int blockSizeCode, int blockSizeExtraBits) = SelectBlockSizeCode(samplesPerChannel);
            bw.WriteBits((uint)blockSizeCode, 4);
            bw.WriteBits(0u, 4); // sample-rate code 0b0000 = use STREAMINFO
            bw.WriteBits((uint)chanAssignmentNibble, 4);
            bw.WriteBits((uint)SelectSampleSizeCode(bps), 3);
            bw.WriteBit(false); // reserved

            // UTF-8 frame number is byte-aligned at this point (we've written exactly 32 bits).
            Span<byte> utf8 = stackalloc byte[FlacUtf8Writer.MaxBytes];
            int utf8Len = FlacUtf8Writer.Write(frameNumber, utf8);
            for (int i = 0; i < utf8Len; i++)
            {
                bw.WriteBits(utf8[i], 8);
            }

            if (blockSizeExtraBits == 8)
            {
                bw.WriteBits((uint)(samplesPerChannel - 1), 8);
            }
            else if (blockSizeExtraBits == 16)
            {
                bw.WriteBits((uint)(samplesPerChannel - 1), 16);
            }

            int headerBytes = bw.BytesWritten;
            byte crc8 = FlacCrc.Crc8(output[..headerBytes]);
            bw.WriteBits(crc8, 8);

            // -- Subframes (§10.3) --
            if (stereoMode)
            {
                // The channel-assignment nibble dictates which pair of (samples,
                // bps) we emit. Side channel always carries one extra bit.
                switch (chanAssignmentNibble)
                {
                    case 0b1000: // Left + Side
                        EncodeSubframeFromSamples(channelBuf0.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        EncodeSubframeFromSamples(sideBuf!.AsSpan(0, samplesPerChannel), bps + 1, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        break;
                    case 0b1001: // Side + Right
                        EncodeSubframeFromSamples(sideBuf!.AsSpan(0, samplesPerChannel), bps + 1, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        EncodeSubframeFromSamples(channelBuf1!.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        break;
                    case 0b1010: // Mid + Side
                        EncodeSubframeFromSamples(midBuf!.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        EncodeSubframeFromSamples(sideBuf!.AsSpan(0, samplesPerChannel), bps + 1, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        break;
                    default: // 0b0001 independent stereo
                        EncodeSubframeFromSamples(channelBuf0.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        EncodeSubframeFromSamples(channelBuf1!.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                        break;
                }
            }
            else
            {
                // Mono / multi-channel / bps==32: independent channels, deinterleave per channel.
                for (int c = 0; c < parameters.Channels; c++)
                {
                    for (int i = 0; i < samplesPerChannel; i++)
                    {
                        channelBuf0[i] = interleavedSamples[i * parameters.Channels + c];
                    }
                    EncodeSubframeFromSamples(channelBuf0.AsSpan(0, samplesPerChannel), bps, samplesPerChannel, residualBuf, windowedBuf, qcoefBuf, ksBuf, ref bw);
                }
            }

            // -- Frame footer (§10.4): align + CRC-16 over everything written so far. --
            bw.AlignToByte();
            int bodyBytes = bw.BytesWritten;
            ushort crc16 = FlacCrc.Crc16(output[..bodyBytes]);
            bw.WriteBits((uint)(crc16 >> 8), 8);
            bw.WriteBits((uint)(crc16 & 0xFF), 8);
            return bw.BytesWritten;
        }
        finally
        {
            if (midBuf is not null) ArrayPool<int>.Shared.Return(midBuf);
            if (sideBuf is not null) ArrayPool<int>.Shared.Return(sideBuf);
            if (channelBuf1 is not null) ArrayPool<int>.Shared.Return(channelBuf1);
            ArrayPool<int>.Shared.Return(ksBuf);
            ArrayPool<int>.Shared.Return(qcoefBuf);
            ArrayPool<double>.Shared.Return(windowedBuf);
            ArrayPool<int>.Shared.Return(residualBuf);
            ArrayPool<int>.Shared.Return(channelBuf0);
        }
    }

    /// <summary>
    /// Estimate the encoded bit cost of one subframe carrying the given
    /// (already-deinterleaved, possibly side- or mid-derived) samples at the
    /// given bps. Returns the cheapest of CONSTANT, FIXED-orders-0..4 with
    /// multi-partition Rice, LPC-orders-1..12 with multi-partition Rice, and
    /// VERBATIM. Does not write any bits. Mutates the residual / windowed /
    /// qcoef / ks scratches in passing — callers must not rely on their
    /// contents across this call.
    /// </summary>
    private static long EstimateSubframeBits(
        ReadOnlySpan<int> samples,
        int bps,
        int samplesPerChannel,
        Span<int> residualBuf,
        Span<double> windowedBuf,
        Span<int> qcoefBuf,
        Span<int> ksBuf)
    {
        int first = samples[0];
        bool constant = true;
        for (int i = 1; i < samplesPerChannel; i++)
        {
            if (samples[i] != first)
            {
                constant = false;
                break;
            }
        }
        if (constant)
        {
            // CONSTANT subframe: 8-bit header + bps-bit sample.
            return 8L + bps;
        }

        long verbatimBits = 8L + (long)samplesPerChannel * bps;
        long bestBits = verbatimBits;
        int maxPo = FlacRice.DefaultMaxPartitionOrder;

        if (FlacFixedPredictor.TryEstimateBest(
                samples, bps, samplesPerChannel, maxPo, residualBuf, ksBuf, bestBits,
                out _, out long fixedBits))
        {
            bestBits = fixedBits;
        }
        if (FlacLpcPredictor.TryEstimateBest(
                samples, bps, samplesPerChannel, maxPo, residualBuf, windowedBuf, qcoefBuf, ksBuf, bestBits,
                out _, out _, out _, out long lpcBits))
        {
            bestBits = lpcBits;
        }
        return bestBits;
    }

    /// <summary>
    /// Emit one subframe carrying the given (already-deinterleaved, possibly
    /// side- or mid-derived) samples at the given bps. Picks the cheapest of
    /// CONSTANT, FIXED-orders-0..4, LPC-orders-1..12, or VERBATIM and writes
    /// it through <paramref name="bw"/>. Re-estimates internally so does not
    /// depend on prior <see cref="EstimateSubframeBits"/> state.
    /// </summary>
    private static void EncodeSubframeFromSamples(
        ReadOnlySpan<int> samples,
        int bps,
        int samplesPerChannel,
        Span<int> residualBuf,
        Span<double> windowedBuf,
        Span<int> qcoefBuf,
        Span<int> ksBuf,
        ref BitWriter bw)
    {
        // CONSTANT detection (cheapest possible subframe at 8 + bps bits).
        int first = samples[0];
        bool constant = true;
        for (int i = 1; i < samplesPerChannel; i++)
        {
            if (samples[i] != first)
            {
                constant = false;
                break;
            }
        }
        if (constant)
        {
            // Subframe header: pad-bit 0 + type 000000 (CONSTANT) + wasted-bits 0 = 0x00
            bw.WriteBits(0u, 8);
            WriteSignedSample(ref bw, first, bps);
            return;
        }

        long verbatimBodyBits = 8L + (long)samplesPerChannel * bps;
        long bestBits = verbatimBodyBits;
        int maxPo = FlacRice.DefaultMaxPartitionOrder;

        bool fixedOk = FlacFixedPredictor.TryEstimateBest(
            samples, bps, samplesPerChannel, maxPo, residualBuf, ksBuf, bestBits,
            out int fixedOrder, out long fixedBits);
        if (fixedOk) bestBits = fixedBits;

        bool lpcOk = FlacLpcPredictor.TryEstimateBest(
            samples, bps, samplesPerChannel, maxPo, residualBuf, windowedBuf, qcoefBuf, ksBuf, bestBits,
            out int lpcOrder, out int lpcPrecision, out int lpcShift, out long lpcBits);
        if (lpcOk) bestBits = lpcBits;

        if (lpcOk)
        {
            FlacLpcPredictor.WriteSubframe(
                ref bw, samples, bps,
                lpcOrder, lpcPrecision, lpcShift, samplesPerChannel, maxPo,
                qcoefBuf[..lpcOrder], residualBuf, ksBuf);
            return;
        }

        if (fixedOk)
        {
            FlacFixedPredictor.WriteSubframe(
                ref bw, samples, bps,
                fixedOrder, samplesPerChannel, maxPo, residualBuf, ksBuf);
            return;
        }

        // VERBATIM: pad 0 + type 000001 + wasted 0 = 0b00000010
        bw.WriteBits(0b00000010u, 8);
        for (int i = 0; i < samplesPerChannel; i++)
        {
            WriteSignedSample(ref bw, samples[i], bps);
        }
    }

    /// <summary>
    /// Emit <paramref name="value"/> as a <paramref name="bps"/>-bit two's-
    /// complement signed integer (MSB first).
    /// </summary>
    private static void WriteSignedSample(ref BitWriter bw, int value, int bps)
    {
        if (bps >= 32)
        {
            bw.WriteBits((uint)value, 32);
            return;
        }
        uint mask = (1u << bps) - 1u;
        bw.WriteBits((uint)value & mask, bps);
    }

    private static void ValidateSampleRange(ReadOnlySpan<int> samples, int bps)
    {
        int max = (1 << (bps - 1)) - 1;
        int min = -(1 << (bps - 1));
        for (int i = 0; i < samples.Length; i++)
        {
            int v = samples[i];
            if (v < min || v > max)
            {
                throw new ArgumentOutOfRangeException(nameof(samples), v, $"Sample at index {i} is outside the signed {bps}-bit range [{min}, {max}].");
            }
        }
    }

    private static (int Code, int ExtraBits) SelectBlockSizeCode(int blockSize)
    {
        return blockSize switch
        {
            192 => (0b0001, 0),
            576 => (0b0010, 0),
            1152 => (0b0011, 0),
            2304 => (0b0100, 0),
            4608 => (0b0101, 0),
            256 => (0b1000, 0),
            512 => (0b1001, 0),
            1024 => (0b1010, 0),
            2048 => (0b1011, 0),
            4096 => (0b1100, 0),
            8192 => (0b1101, 0),
            16384 => (0b1110, 0),
            32768 => (0b1111, 0),
            _ when blockSize <= 256 => (0b0110, 8),
            _ => (0b0111, 16),
        };
    }

    private static int SelectSampleSizeCode(int bps)
    {
        return bps switch
        {
            8 => 0b001,
            12 => 0b010,
            16 => 0b100,
            20 => 0b101,
            24 => 0b110,
            32 => 0b111,
            _ => 0b000, // resolve via STREAMINFO
        };
    }
}
