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
/// automatically picking the cheapest representation per channel. Stereo
/// decorrelation is reserved for a follow-up phase. The output is a valid FLAC
/// frame that the existing <see cref="FlacFrameHeaderParser"/> /
/// <see cref="FlacSubframeDecoder"/> pair round-trips back to the exact input
/// samples.
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
    /// <c>C × (1 + ⌈N × bps / 8⌉)</c> bytes of subframes (1 B subframe header
    /// + verbatim sample words) plus 3 B for byte-alignment padding + CRC-16.
    /// </remarks>
    public static int MaxFrameSize(FlacEncoderParameters parameters, int samplesPerChannel)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        parameters.Validate();
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(samplesPerChannel);
        long sampleBytes = parameters.Channels * (1L + (samplesPerChannel * (long)parameters.BitsPerSample + 7) / 8);
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
        var bw = new BitWriter(output);

        // -- Frame header (§10.1) --
        bw.WriteBits(0b11111111111110u, 14); // sync code
        bw.WriteBit(false);                  // reserved
        bw.WriteBit(false);                  // blocking_strategy: 0 = fixed → use frame number

        (int blockSizeCode, int blockSizeExtraBits) = SelectBlockSizeCode(samplesPerChannel);
        bw.WriteBits((uint)blockSizeCode, 4);

        // Sample-rate code 0b0000 = use STREAMINFO. Avoids encoding the rate per frame.
        bw.WriteBits(0u, 4);

        // Channel assignment 0..7 = independent C+1 channels. No stereo decorrelation here.
        bw.WriteBits((uint)(parameters.Channels - 1), 4);

        bw.WriteBits((uint)SelectSampleSizeCode(parameters.BitsPerSample), 3);
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
        // No sample-rate extension bytes (code 0).

        // CRC-8 over the byte-aligned header so far.
        int headerBytes = bw.BytesWritten;
        byte crc8 = FlacCrc.Crc8(output[..headerBytes]);
        bw.WriteBits(crc8, 8);

        // -- Subframes (§10.3) --
        int[] channelBuf = ArrayPool<int>.Shared.Rent(samplesPerChannel);
        int[] residualBuf = ArrayPool<int>.Shared.Rent(samplesPerChannel);
        double[] windowedBuf = ArrayPool<double>.Shared.Rent(samplesPerChannel);
        int[] qcoefBuf = ArrayPool<int>.Shared.Rent(FlacLpcPredictor.MaxOrder);
        int[] ksBuf = ArrayPool<int>.Shared.Rent(1 << FlacRice.MaxPartitionOrder);
        try
        {
            for (int c = 0; c < parameters.Channels; c++)
            {
                EncodeSubframe(
                    parameters,
                    interleavedSamples,
                    samplesPerChannel,
                    c,
                    channelBuf.AsSpan(0, samplesPerChannel),
                    residualBuf.AsSpan(0, samplesPerChannel),
                    windowedBuf.AsSpan(0, samplesPerChannel),
                    qcoefBuf.AsSpan(0, FlacLpcPredictor.MaxOrder),
                    ksBuf.AsSpan(0, 1 << FlacRice.MaxPartitionOrder),
                    ref bw);
            }
        }
        finally
        {
            ArrayPool<int>.Shared.Return(ksBuf);
            ArrayPool<int>.Shared.Return(qcoefBuf);
            ArrayPool<double>.Shared.Return(windowedBuf);
            ArrayPool<int>.Shared.Return(residualBuf);
            ArrayPool<int>.Shared.Return(channelBuf);
        }

        // -- Frame footer (§10.4): align + CRC-16 over everything written so far. --
        bw.AlignToByte();
        int bodyBytes = bw.BytesWritten;
        ushort crc16 = FlacCrc.Crc16(output[..bodyBytes]);
        bw.WriteBits((uint)(crc16 >> 8), 8);
        bw.WriteBits((uint)(crc16 & 0xFF), 8);

        return bw.BytesWritten;
    }

    private static void EncodeSubframe(
        FlacEncoderParameters parameters,
        ReadOnlySpan<int> interleaved,
        int samplesPerChannel,
        int channelIndex,
        Span<int> channelBuf,
        Span<int> residualBuf,
        Span<double> windowedBuf,
        Span<int> qcoefBuf,
        Span<int> ksBuf,
        ref BitWriter bw)
    {
        int channels = parameters.Channels;
        int bps = parameters.BitsPerSample;

        // Deinterleave this channel into a contiguous workspace.
        for (int i = 0; i < samplesPerChannel; i++)
        {
            channelBuf[i] = interleaved[i * channels + channelIndex];
        }

        // CONSTANT detection (cheapest possible subframe at 8 + bps bits).
        int first = channelBuf[0];
        bool constant = true;
        for (int i = 1; i < samplesPerChannel; i++)
        {
            if (channelBuf[i] != first)
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

        ReadOnlySpan<int> samples = channelBuf[..samplesPerChannel];
        long verbatimBodyBits = 8L + (long)samplesPerChannel * bps;
        long bestBits = verbatimBodyBits;
        int maxPo = FlacRice.DefaultMaxPartitionOrder;

        // Estimate FIXED — cheap (5 orders × multi-partition sweep).
        bool fixedOk = FlacFixedPredictor.TryEstimateBest(
            samples, bps, samplesPerChannel, maxPo, residualBuf, ksBuf, bestBits,
            out int fixedOrder, out long fixedBits);
        if (fixedOk) bestBits = fixedBits;

        // Estimate LPC — more expensive (autocorrelation + Levinson + per-order
        // quantise + per-order multi-partition Rice sweep) but lets LPC challenge FIXED.
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
            WriteSignedSample(ref bw, channelBuf[i], bps);
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
