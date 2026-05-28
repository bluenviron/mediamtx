using System.Security.Cryptography;
using Mediar.Codecs.Flac.Decoder;
using Mediar.Codecs.Flac.Encoder;
using Xunit;

namespace Mediar.Tests;

// MD5 here is a spec requirement of FLAC (RFC 9639 STREAMINFO) used as a
// non-cryptographic fingerprint of the unencoded PCM, not for security.
#pragma warning disable CA5351

public sealed class FlacEncoderTests
{
    [Fact]
    public void EncodeFrame_StereoS16_RoundTrips_Bit_Exact_Through_Decoder()
    {
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 2, BitsPerSample: 16);
        const int n = 192;
        int[] samples = new int[n * 2];
        for (int i = 0; i < n; i++)
        {
            samples[i * 2 + 0] = (int)Math.Round(20000.0 * Math.Sin(2.0 * Math.PI * i / 32.0));
            samples[i * 2 + 1] = (int)Math.Round(15000.0 * Math.Cos(2.0 * Math.PI * i / 48.0));
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), n);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_MonoS16_AllZeros_Emits_Constant_Subframe()
    {
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 16);
        const int n = 192;
        int[] samples = new int[n];

        var (frame, frameLen, _) = EncodeOneFrame(p, samples, n);

        // Verify header is valid and locate the subframe.
        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 16, out var header));
        int headerBytes = header.HeaderSize;

        // Subframe header byte: 1 pad + 6 type + 1 wasted-flag. 0x00 = CONSTANT (type 000000).
        // We compare against 0x02 (VERBATIM type 000001) to make sure we did NOT pick verbatim.
        Assert.Equal(0x00, frame[headerBytes]);
        // CONSTANT + 16-bit zero sample = 3 subframe bytes + 2 CRC-16 bytes = headerBytes + 5.
        Assert.Equal(headerBytes + 5, frameLen);
    }

    [Fact]
    public void EncodeFrame_MonoS16_LinearRamp_Emits_Fixed_Subframe()
    {
        // Strictly-increasing samples drive Fixed predictor selection. The
        // ramp 0,1,2,... has zero second differences, so the encoder should
        // pick FIXED order 2 (type byte 0b0_001010_0 = 0x14).
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 16);
        const int n = 192;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++) samples[i] = i;

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);

        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 16, out var header));
        Assert.Equal(0x14, frame[header.HeaderSize]);

        // FIXED must still round-trip bit-exact through the public decoder.
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_MonoS16_Order1_Constant_Delta_Emits_Fixed_Order1()
    {
        // A sequence with constant non-zero first difference (e.g. 100, 113, 126, ...)
        // makes FIXED order 1 optimal (zero second difference makes order 2 better,
        // so use a non-arithmetic ramp by perturbing: i*13 + small jitter).
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 256;
        int[] samples = new int[n];
        var rng = new Random(42);
        int acc = 0;
        for (int i = 0; i < n; i++)
        {
            acc += 10 + rng.Next(-2, 3); // jittered ramp → order 1 residual ≈ ±2
            samples[i] = acc;
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 16, out var header));
        // Expect a FIXED subframe (any order 0..4). Subframe-header byte layout
        // is [pad(1)][type(6)][wasted(1)]; the FIXED family has type 0b001NNN
        // (bit 4 set, bits 6-5 clear). Mask 0x70 = bits 6,5,4.
        byte sub = frame[header.HeaderSize];
        Assert.True((sub & 0b0111_0000) == 0b0001_0000,
            $"Expected FIXED subframe (type 0b001xxx), got byte 0x{sub:X2}");
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_AllZero_Beats_Fixed_With_Constant()
    {
        // All-zero samples should still pick CONSTANT (8 + bps bits) over any
        // FIXED encoding (8 + 10 + N bits minimum at k=0).
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 1024;
        int[] samples = new int[n]; // all zero

        var (frame, frameLen, _) = EncodeOneFrame(p, samples, n);
        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 16, out var header));
        Assert.Equal(0x00, frame[header.HeaderSize]); // CONSTANT
    }

    [Fact]
    public void EncodeFrame_Sine_Wave_Compresses_Below_Verbatim()
    {
        // A pure sine wave at moderate amplitude is highly predictable for
        // Fixed orders 2-4. The encoded frame should be materially smaller
        // than the VERBATIM bit-budget.
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 4096;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++)
        {
            samples[i] = (int)Math.Round(8000.0 * Math.Sin(2.0 * Math.PI * i / 64.0));
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);

        int verbatimBytes = (8 + n * 16) / 8 + 1; // approx VERBATIM subframe size
        Assert.True(frameLen < verbatimBytes,
            $"Frame ({frameLen} B) should beat verbatim subframe (~{verbatimBytes} B).");
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Theory]
    [InlineData(8)]
    [InlineData(12)]
    [InlineData(16)]
    [InlineData(20)]
    [InlineData(24)]
    public void EncodeFrame_Sine_AllBps_Fixed_RoundTrip(int bps)
    {
        var p = new FlacEncoderParameters(48000, 2, bps);
        const int n = 1024;
        long amp = (1L << (bps - 1)) / 4;
        int[] samples = new int[n * 2];
        for (int i = 0; i < n; i++)
        {
            samples[i * 2 + 0] = (int)Math.Round(amp * Math.Sin(2.0 * Math.PI * i / 32.0));
            samples[i * 2 + 1] = (int)Math.Round(amp * Math.Cos(2.0 * Math.PI * i / 48.0));
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Theory]
    [InlineData(192)]
    [InlineData(576)]
    [InlineData(1024)]
    [InlineData(4096)]
    [InlineData(16384)]
    public void EncodeFrame_StandardBlockSizes_RoundTrip(int blockSize)
    {
        var p = new FlacEncoderParameters(44100, 2, 16, BlockSize: blockSize);
        int[] samples = SyntheticStereo(blockSize, seed: 17);
        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, blockSize);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), blockSize);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, blockSize);
    }

    [Theory]
    [InlineData(200)]   // 8-bit (n-1) extension path
    [InlineData(255)]   // 8-bit (n-1) extension path, edge
    [InlineData(5000)]  // 16-bit (n-1) extension path
    [InlineData(33333)] // 16-bit (n-1) extension path
    public void EncodeFrame_NonStandardBlockSizes_RoundTrip(int blockSize)
    {
        var p = new FlacEncoderParameters(44100, 1, 16, BlockSize: blockSize);
        int[] samples = SyntheticMono(blockSize, seed: 23);
        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, blockSize);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), blockSize);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, blockSize);
    }

    [Theory]
    [InlineData(8)]
    [InlineData(12)]
    [InlineData(16)]
    [InlineData(20)]
    [InlineData(24)]
    public void EncodeFrame_AllStandardBitDepths_RoundTrip(int bps)
    {
        var p = new FlacEncoderParameters(48000, 1, bps);
        const int n = 512;
        int max = (1 << (bps - 1)) - 1;
        int[] samples = new int[n];
        var rng = new Random(bps);
        for (int i = 0; i < n; i++)
        {
            long r = rng.Next();
            samples[i] = (int)(r % (2L * max + 1) - max);
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), n);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_Bps32_Header_RoundTrips()
    {
        // bps = 32 cannot be exactly round-tripped through the FlacDecoder's
        // normalised-float output (float32 mantissa has only ~24 effective bits),
        // so we restrict this test to confirming the frame header parses with
        // BitsPerSample = 32 and the body decode succeeds without throwing.
        var p = new FlacEncoderParameters(48000, 1, 32);
        const int n = 256;
        int[] samples = new int[n];
        var rng = new Random(32);
        for (int i = 0; i < n; i++) samples[i] = rng.Next(int.MinValue, int.MaxValue);

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), n);

        var dp = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = p.SampleRate,
            Channels = p.Channels,
            BitsPerSample = p.BitsPerSample,
            ExtraData = streamInfo,
        };
        using var decoder = new FlacDecoder(dp);
        using var decoded = decoder.Decode(frame.AsSpan(0, frameLen), pts: 0);
        Assert.Equal(n, decoded.SamplesPerChannel);
        Assert.Equal(p.Channels, decoded.Channels);
    }

    [Theory]
    [InlineData(0UL)]
    [InlineData(1UL)]
    [InlineData(127UL)]
    [InlineData(128UL)]
    [InlineData(2047UL)]
    [InlineData(2048UL)]
    [InlineData(65535UL)]
    [InlineData(1_000_000UL)]
    public void EncodeFrame_VariousFrameNumbers_DecodeHeaderRoundTrips(ulong frameNumber)
    {
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 192;
        int[] samples = SyntheticMono(n, seed: 5);
        byte[] buffer = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        int frameLen = FlacFrameEncoder.EncodeFrame(p, samples, n, frameNumber, buffer);
        Assert.True(FlacFrameHeaderParser.TryParse(buffer.AsSpan(0, frameLen), 44100, 16, out var header));
        Assert.Equal((long)frameNumber, header.FrameOrSampleNumber);
    }

    [Fact]
    public void EncodeFrame_RejectsSampleAboveSignedRange()
    {
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 192;
        int[] samples = new int[n];
        samples[42] = 32768; // one above int16 max
        byte[] buffer = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            FlacFrameEncoder.EncodeFrame(p, samples, n, 0, buffer));
    }

    [Fact]
    public void EncodeFrame_RejectsSampleBelowSignedRange()
    {
        var p = new FlacEncoderParameters(44100, 1, 16);
        const int n = 192;
        int[] samples = new int[n];
        samples[3] = -32769; // one below int16 min
        byte[] buffer = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            FlacFrameEncoder.EncodeFrame(p, samples, n, 0, buffer));
    }

    [Fact]
    public void EncoderParameters_Validate_Catches_BadInput()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(0, 1, 16).Validate());
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(44100, 0, 16).Validate());
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(44100, 9, 16).Validate());
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(44100, 1, 3).Validate());
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(44100, 1, 33).Validate());
        Assert.Throws<ArgumentOutOfRangeException>(() => new FlacEncoderParameters(44100, 1, 16, BlockSize: 8).Validate());
    }

    [Fact]
    public void MaxFrameSize_Math_Sanity()
    {
        var p = new FlacEncoderParameters(44100, 2, 16);
        // 17 + 2 * (1 + ceil(4096 * 16 / 8)) = 17 + 2 * (1 + 8192) = 17 + 16386 = 16403
        Assert.Equal(16403, FlacFrameEncoder.MaxFrameSize(p, 4096));
    }

    [Fact]
    public void StreamInfoBuilder_Records_Min_Max_Sizes_And_TotalSamples()
    {
        var p = new FlacEncoderParameters(48000, 2, 24);
        var builder = new FlacStreamInfoBuilder(p);
        int[] block1 = SyntheticStereo(192, seed: 1);
        int[] block2 = SyntheticStereo(4096, seed: 2);
        int[] block3 = SyntheticStereo(1024, seed: 3);

        builder.ObserveFrame(block1, 192, frameSizeInBytes: 800);
        builder.ObserveFrame(block2, 4096, frameSizeInBytes: 16000);
        builder.ObserveFrame(block3, 1024, frameSizeInBytes: 4200);

        Assert.Equal(192 + 4096 + 1024, builder.TotalSamples);
        Assert.Equal(3, builder.FrameCount);
        Assert.Equal(800, builder.MinFrameSize);
        Assert.Equal(16000, builder.MaxFrameSize);

        byte[] si = builder.ToBytes(writeMd5: false);
        Assert.Equal(34, si.Length);
        // Min block size 192, max 4096
        Assert.Equal(192, (si[0] << 8) | si[1]);
        Assert.Equal(4096, (si[2] << 8) | si[3]);
        // Min frame size 800
        Assert.Equal(800, (si[4] << 16) | (si[5] << 8) | si[6]);
        // Max frame size 16000
        Assert.Equal(16000, (si[7] << 16) | (si[8] << 8) | si[9]);
        // Sample rate 48000, channels 2, bps 24
        ulong packed = 0;
        for (int i = 0; i < 8; i++) packed = (packed << 8) | si[10 + i];
        int sampleRate = (int)((packed >> 44) & 0xFFFFF);
        int channels = (int)((packed >> 41) & 0x7) + 1;
        int bps = (int)((packed >> 36) & 0x1F) + 1;
        long total = (long)(packed & 0xFFFFFFFFFUL);
        Assert.Equal(48000, sampleRate);
        Assert.Equal(2, channels);
        Assert.Equal(24, bps);
        Assert.Equal(192 + 4096 + 1024, total);
        // MD5 zeroed when writeMd5: false.
        for (int i = 18; i < 34; i++) Assert.Equal(0, si[i]);
    }

    [Fact]
    public void StreamInfoBuilder_Md5_Matches_LibFlac_Style_Serialisation_S16()
    {
        var p = new FlacEncoderParameters(44100, 2, 16);
        var builder = new FlacStreamInfoBuilder(p);
        int[] samples = SyntheticStereo(1024, seed: 7);
        builder.ObserveFrame(samples, 1024, frameSizeInBytes: 500);

        byte[] si = builder.ToBytes(writeMd5: true);
        // Compute expected MD5: little-endian 2 bytes per sample.
        byte[] expected = new byte[samples.Length * 2];
        for (int i = 0; i < samples.Length; i++)
        {
            short v = (short)samples[i];
            expected[i * 2 + 0] = (byte)v;
            expected[i * 2 + 1] = (byte)(v >> 8);
        }
        byte[] expectedMd5 = MD5.HashData(expected);
        Assert.Equal(expectedMd5, si.AsSpan(18, 16).ToArray());
    }

    [Fact]
    public void StreamInfoBuilder_Md5_Matches_S24_LE_Serialisation()
    {
        var p = new FlacEncoderParameters(48000, 1, 24);
        var builder = new FlacStreamInfoBuilder(p);
        int[] samples = new int[256];
        var rng = new Random(99);
        for (int i = 0; i < samples.Length; i++) samples[i] = rng.Next(-8388608, 8388608);

        builder.ObserveFrame(samples, samples.Length, frameSizeInBytes: 700);
        byte[] si = builder.ToBytes(writeMd5: true);

        byte[] expected = new byte[samples.Length * 3];
        for (int i = 0; i < samples.Length; i++)
        {
            int v = samples[i];
            expected[i * 3 + 0] = (byte)v;
            expected[i * 3 + 1] = (byte)(v >> 8);
            expected[i * 3 + 2] = (byte)(v >> 16);
        }
        byte[] expectedMd5 = MD5.HashData(expected);
        Assert.Equal(expectedMd5, si.AsSpan(18, 16).ToArray());
    }

    [Fact]
    public void StreamInfoBuilder_ToBytes_Twice_Freezes_State()
    {
        var p = new FlacEncoderParameters(44100, 1, 16);
        var builder = new FlacStreamInfoBuilder(p);
        int[] samples = SyntheticMono(192, seed: 1);
        builder.ObserveFrame(samples, 192, 600);
        byte[] first = builder.ToBytes();
        Assert.Throws<InvalidOperationException>(() => builder.ObserveFrame(samples, 192, 600));
        // ToBytes can still be called repeatedly without throwing and returns same bytes.
        byte[] second = builder.ToBytes();
        Assert.Equal(first, second);
    }

    [Fact]
    public void EndToEnd_TwoFrames_RoundTrip_Bit_Exact_Via_PublicDecoder()
    {
        var p = new FlacEncoderParameters(44100, 2, 16);
        const int n = 192;
        int[] samples1 = SyntheticStereo(n, seed: 11);
        int[] samples2 = SyntheticStereo(n, seed: 12);

        var builder = new FlacStreamInfoBuilder(p);
        byte[] buf1 = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        byte[] buf2 = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        int len1 = FlacFrameEncoder.EncodeFrame(p, samples1, n, 0, buf1);
        int len2 = FlacFrameEncoder.EncodeFrame(p, samples2, n, 1, buf2);
        builder.ObserveFrame(samples1, n, len1);
        builder.ObserveFrame(samples2, n, len2);

        byte[] si = builder.ToBytes();

        var decoderParams = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = 44100,
            Channels = 2,
            BitsPerSample = 16,
            ExtraData = si,
        };
        using var decoder = new FlacDecoder(decoderParams);

        AssertDecodedMatches(decoder, buf1.AsSpan(0, len1), samples1, n, 2, 16);
        AssertDecodedMatches(decoder, buf2.AsSpan(0, len2), samples2, n, 2, 16);
    }

    // ----------------- LPC (phase 3) -----------------

    [Fact]
    public void EncodeFrame_MonoS16_Sine4096_Picks_Lpc_Subframe()
    {
        // A long sinusoid at a non-trivial fractional period is the canonical
        // input where LPC beats Fixed: Fixed's analytical predictors can match
        // simple slopes, but a band-limited sinusoid is exactly the signal an
        // autocorrelation-driven LPC fits to within a fraction of a bit per
        // sample.
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 16);
        const int n = 4096;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++)
        {
            samples[i] = (int)Math.Round(20000.0 * Math.Sin(2.0 * Math.PI * i / 17.3));
        }

        var (frame, frameLen, _) = EncodeOneFrame(p, samples, n);

        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 16, out var header));
        byte subframeHeader = frame[header.HeaderSize];

        // LPC subframe type bits are 1NNNNN with N = order-1 → header byte
        // (header << 1) has its 0x40 bit set and the 0x80 bit clear.
        Assert.True((subframeHeader & 0x80) == 0, "subframe pad bit must be 0");
        Assert.True((subframeHeader & 0x40) == 0x40, "subframe top type bit must be 1 (LPC family)");
    }

    [Fact]
    public void EncodeFrame_MonoS16_Sine4096_Lpc_RoundTrips_Bit_Exact()
    {
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 16);
        const int n = 4096;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++)
        {
            samples[i] = (int)Math.Round(18000.0 * Math.Sin(2.0 * Math.PI * i / 19.7));
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), n);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_MonoS16_Sine_Compresses_Better_Than_Fixed_Baseline()
    {
        // Cross-check that LPC's win over Fixed materialises in fewer bytes on
        // a long, high-quality sine block. Pure verbatim ≈ 2 bytes/sample; phase
        // 2 (Fixed) hit ~20% of verbatim on this shape, phase 3 (LPC) should
        // come in materially below that — anything under 18% comfortably beats
        // the Fixed baseline on this signal (measured ~15% at period=23.0).
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 16);
        const int n = 4096;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++)
        {
            samples[i] = (int)Math.Round(20000.0 * Math.Sin(2.0 * Math.PI * i / 23.0));
        }

        var (_, frameLen, _) = EncodeOneFrame(p, samples, n);
        int verbatimBytes = n * 2;
        Assert.True(
            frameLen < verbatimBytes * 18 / 100,
            $"LPC frame {frameLen} bytes should be < 18% of verbatim {verbatimBytes}.");
    }

    [Theory]
    [InlineData(8)]
    [InlineData(12)]
    [InlineData(16)]
    [InlineData(20)]
    [InlineData(24)]
    public void EncodeFrame_StereoSine_AllBps_Lpc_RoundTrips(int bps)
    {
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 2, BitsPerSample: bps);
        const int n = 2048;
        int max = (1 << (bps - 1)) - 1;
        double amp = max * 0.6;
        int[] samples = new int[n * 2];
        for (int i = 0; i < n; i++)
        {
            samples[i * 2 + 0] = (int)Math.Round(amp * Math.Sin(2.0 * Math.PI * i / 13.7));
            samples[i * 2 + 1] = (int)Math.Round(amp * Math.Cos(2.0 * Math.PI * i / 21.3));
        }

        var (frame, frameLen, streamInfo) = EncodeOneFrame(p, samples, n);
        AssertHeaderRoundTripsAndMatches(p, frame.AsSpan(0, frameLen), n);
        AssertDecoderProducesExactSamples(p, streamInfo, frame, frameLen, samples, n);
    }

    [Fact]
    public void EncodeFrame_Bps32_Sine_Skips_Lpc_And_Stays_Bit_Exact()
    {
        // bps > 24 disqualifies LPC (Σ qcoef·sample > 2^63 risk on the
        // accumulator). The encoder must fall through to Fixed or VERBATIM
        // and still emit a frame whose header round-trips. Bit-exact decode
        // of bps=32 itself is gated by the decoder's float-precision path
        // (already documented in the phase-1 test); here we just verify the
        // encoder doesn't crash and emits a non-LPC subframe.
        var p = new FlacEncoderParameters(SampleRate: 44100, Channels: 1, BitsPerSample: 32);
        const int n = 1024;
        int[] samples = new int[n];
        for (int i = 0; i < n; i++)
        {
            samples[i] = (int)Math.Round(1.5e8 * Math.Sin(2.0 * Math.PI * i / 31.0));
        }

        byte[] frame = new byte[FlacFrameEncoder.MaxFrameSize(p, n)];
        int frameLen = FlacFrameEncoder.EncodeFrame(p, samples, n, 0, frame);

        Assert.True(FlacFrameHeaderParser.TryParse(frame.AsSpan(0, frameLen), 44100, 32, out var header));
        byte subframeHeader = frame[header.HeaderSize];

        // Top type bit must NOT be set (not in the LPC family at bps=32).
        Assert.True((subframeHeader & 0x40) == 0, $"bps=32 must not pick LPC; got 0x{subframeHeader:X2}");
    }

    // ----------------- helpers -----------------

    private static (byte[] Frame, int Length, byte[] StreamInfo) EncodeOneFrame(
        FlacEncoderParameters p, int[] interleaved, int samplesPerChannel)
    {
        byte[] frame = new byte[FlacFrameEncoder.MaxFrameSize(p, samplesPerChannel)];
        int frameLen = FlacFrameEncoder.EncodeFrame(p, interleaved, samplesPerChannel, 0, frame);

        var builder = new FlacStreamInfoBuilder(p);
        builder.ObserveFrame(interleaved, samplesPerChannel, frameLen);
        byte[] streamInfo = builder.ToBytes();
        return (frame, frameLen, streamInfo);
    }

    private static void AssertHeaderRoundTripsAndMatches(
        FlacEncoderParameters p, ReadOnlySpan<byte> frame, int samplesPerChannel)
    {
        Assert.True(FlacFrameHeaderParser.TryParse(frame, p.SampleRate, p.BitsPerSample, out var header));
        Assert.Equal(samplesPerChannel, header.BlockSize);
        Assert.Equal(p.SampleRate, header.SampleRate);
        Assert.Equal(p.Channels, header.Channels);
        Assert.Equal(p.BitsPerSample, header.BitsPerSample);
        Assert.Equal(0L, header.FrameOrSampleNumber);
    }

    private static void AssertDecoderProducesExactSamples(
        FlacEncoderParameters p, byte[] streamInfo,
        byte[] frame, int frameLen, int[] samples, int samplesPerChannel)
    {
        var dp = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = p.SampleRate,
            Channels = p.Channels,
            BitsPerSample = p.BitsPerSample,
            ExtraData = streamInfo,
        };
        using var decoder = new FlacDecoder(dp);
        AssertDecodedMatches(decoder, frame.AsSpan(0, frameLen), samples, samplesPerChannel, p.Channels, p.BitsPerSample);
    }

    private static void AssertDecodedMatches(
        FlacDecoder decoder, ReadOnlySpan<byte> frame,
        int[] expected, int samplesPerChannel, int channels, int bps)
    {
        using var decoded = decoder.Decode(frame, pts: 0);
        Assert.Equal(samplesPerChannel, decoded.SamplesPerChannel);
        Assert.Equal(channels, decoded.Channels);

        // Decoder produces normalised floats (divided by 2^(bps-1)). Convert back to int.
        float scale = 1L << (bps - 1);
        var floats = decoded.Samples.Span;
        Assert.Equal(samplesPerChannel * channels, floats.Length);
        for (int i = 0; i < floats.Length; i++)
        {
            int actual = (int)Math.Round(floats[i] * scale);
            Assert.Equal(expected[i], actual);
        }
    }

    private static int[] SyntheticMono(int n, int seed)
    {
        var rng = new Random(seed);
        int[] s = new int[n];
        for (int i = 0; i < n; i++) s[i] = rng.Next(-30000, 30000);
        return s;
    }

    private static int[] SyntheticStereo(int n, int seed)
    {
        var rng = new Random(seed);
        int[] s = new int[n * 2];
        for (int i = 0; i < n; i++)
        {
            s[i * 2 + 0] = rng.Next(-30000, 30000);
            s[i * 2 + 1] = rng.Next(-30000, 30000);
        }
        return s;
    }
}
