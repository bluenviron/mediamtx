using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacPcmFrameConverterTests
{
    [Fact]
    public void ToInt16Sample_Zero_IsZero()
    {
        Assert.Equal((short[])[0], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { 0f }));
    }

    [Fact]
    public void ToInt16Sample_PositiveOne_SaturatesToMax()
    {
        Assert.Equal((short[])[short.MaxValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { 1f }));
    }

    [Fact]
    public void ToInt16Sample_NegativeOne_HitsMin()
    {
        Assert.Equal((short[])[short.MinValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -1f }));
    }

    [Fact]
    public void ToInt16Sample_AboveOne_Saturates()
    {
        Assert.Equal((short[])[short.MaxValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { 2f }));
    }

    [Fact]
    public void ToInt16Sample_BelowNegativeOne_Saturates()
    {
        Assert.Equal((short[])[short.MinValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -2f }));
    }

    [Fact]
    public void ToInt16Sample_HalfPositive_RoundsToScaledHalf()
    {
        // 0.5 * 32767 = 16383.5 → round-half-to-even → 16384.
        Assert.Equal((short[])[16384], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { 0.5f }));
    }

    [Fact]
    public void ToInt16Sample_HalfNegative_UsesAsymmetricNegScale()
    {
        // -0.5 * 32768 = -16384 (exact).
        Assert.Equal((short[])[-16384], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -0.5f }));
    }

    [Fact]
    public void ToInt16Sample_Nan_BecomesZero()
    {
        Assert.Equal((short[])[0], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { float.NaN }));
    }

    [Fact]
    public void ToInt16Sample_PositiveInfinity_Saturates()
    {
        Assert.Equal((short[])[short.MaxValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { float.PositiveInfinity }));
    }

    [Fact]
    public void ToInt16Sample_NegativeInfinity_Saturates()
    {
        Assert.Equal((short[])[short.MinValue], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { float.NegativeInfinity }));
    }

    [Fact]
    public void ToInt16Samples_DestinationTooSmall_Throws()
    {
        var src = new float[4];
        var dst = new short[3];
        Assert.Throws<ArgumentException>(() => AacPcmFrameConverter.ToInt16Samples(src, dst));
    }

    [Fact]
    public void ToInt16Samples_SpanOverload_FillsDestination()
    {
        ReadOnlySpan<float> src = stackalloc float[] { 0f, 0.5f, -0.5f, 1f };
        Span<short> dst = stackalloc short[4];
        AacPcmFrameConverter.ToInt16Samples(src, dst);
        Assert.Equal(0, dst[0]);
        Assert.Equal(16384, dst[1]);
        Assert.Equal(-16384, dst[2]);
        Assert.Equal(short.MaxValue, dst[3]);
    }

    [Fact]
    public void ToInt16Frame_NullSource_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AacPcmFrameConverter.ToInt16Frame(null!));
    }

    [Fact]
    public void ToInt16Frame_PreservesShapeAndSpeakerOrder()
    {
        var source = new AacPcmFrame
        {
            Samples = new[] { 0f, 0.5f, -0.5f, 1f, -1f, 0f },
            ChannelCount = 2,
            SamplesPerChannel = 3,
            SampleRate = 48000,
            Speakers = new[] { AacSpeaker.FrontLeft, AacSpeaker.FrontRight },
        };

        var dst = AacPcmFrameConverter.ToInt16Frame(source);

        Assert.Equal(2, dst.ChannelCount);
        Assert.Equal(3, dst.SamplesPerChannel);
        Assert.Equal(48000, dst.SampleRate);
        Assert.Equal((short[])[0, 16384, -16384, short.MaxValue, short.MinValue, 0], dst.Samples);
        Assert.Equal(source.Speakers, dst.Speakers);
    }

    [Fact]
    public void ToInt16Samples_EmptySource_ReturnsEmpty()
    {
        var result = AacPcmFrameConverter.ToInt16Samples(ReadOnlySpan<float>.Empty);
        Assert.Empty(result);
    }

    [Fact]
    public void ToInt16Samples_EmptySpan_NoOp()
    {
        Span<short> dst = stackalloc short[0];
        AacPcmFrameConverter.ToInt16Samples(ReadOnlySpan<float>.Empty, dst);
        Assert.Equal(0, dst.Length);
    }

    [Fact]
    public void ToInt16Samples_DestinationEqualLength_Succeeds()
    {
        ReadOnlySpan<float> src = stackalloc float[] { 0.25f, -0.25f };
        Span<short> dst = stackalloc short[2];
        AacPcmFrameConverter.ToInt16Samples(src, dst);
        Assert.NotEqual(0, dst[0]);
        Assert.NotEqual(0, dst[1]);
    }

    [Fact]
    public void ToInt16Samples_DestinationLargerThanSource_LeavesTail()
    {
        ReadOnlySpan<float> src = stackalloc float[] { 0.5f };
        Span<short> dst = stackalloc short[3];
        dst[1] = 0x1234;
        dst[2] = unchecked((short)0xABCD);
        AacPcmFrameConverter.ToInt16Samples(src, dst);
        Assert.Equal(16384, dst[0]);
        // The tail is not touched.
        Assert.Equal(0x1234, dst[1]);
        Assert.Equal(unchecked((short)0xABCD), dst[2]);
    }

    [Theory]
    [InlineData(0.25f, 8192)]      // 0.25 * 32767 = 8191.75 -> round-to-even -> 8192
    [InlineData(-0.25f, -8192)]    // -0.25 * 32768 = -8192 (exact)
    [InlineData(0.75f, 24575)]     // 0.75 * 32767 = 24575.25 -> 24575
    [InlineData(-0.75f, -24576)]   // -0.75 * 32768 = -24576 (exact)
    public void ToInt16Sample_ParametricRounding(float input, short expected)
    {
        Assert.Equal((short[])[expected], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { input }));
    }

    [Theory]
    [InlineData(1.0e-9f, 0)]
    [InlineData(-1.0e-9f, 0)]
    [InlineData(2.0e-5f, 1)]   // ~0.65 -> 1
    [InlineData(-2.0e-5f, -1)] // ~-0.66 -> -1
    public void ToInt16Sample_SmallValuesRoundCorrectly(float input, short expected)
    {
        Assert.Equal((short[])[expected], AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { input }));
    }

    [Fact]
    public void ToInt16Samples_PreservesOrderAcrossBuffer()
    {
        ReadOnlySpan<float> src = stackalloc float[] { -1f, 0f, 1f };
        var result = AacPcmFrameConverter.ToInt16Samples(src);
        Assert.Equal(3, result.Length);
        Assert.Equal(short.MinValue, result[0]);
        Assert.Equal(0, result[1]);
        Assert.Equal(short.MaxValue, result[2]);
    }

    [Fact]
    public void ToInt16Samples_LongBuffer_HandlesAllValues()
    {
        var src = new float[1024];
        for (int i = 0; i < src.Length; i++)
        {
            src[i] = (i % 2 == 0) ? 0.5f : -0.5f;
        }
        var result = AacPcmFrameConverter.ToInt16Samples(src);
        Assert.Equal(src.Length, result.Length);
        for (int i = 0; i < result.Length; i++)
        {
            Assert.Equal((short)(i % 2 == 0 ? 16384 : -16384), result[i]);
        }
    }

    [Fact]
    public void ToInt16Frame_EmptyFrame_ReturnsEmptySamples()
    {
        var source = new AacPcmFrame
        {
            Samples = Array.Empty<float>(),
            ChannelCount = 1,
            SamplesPerChannel = 0,
            SampleRate = 44100,
            Speakers = new[] { AacSpeaker.FrontCentre },
        };

        var dst = AacPcmFrameConverter.ToInt16Frame(source);
        Assert.Empty(dst.Samples);
        Assert.Equal(1, dst.ChannelCount);
        Assert.Equal(0, dst.SamplesPerChannel);
        Assert.Equal(44100, dst.SampleRate);
    }

    [Fact]
    public void ToInt16Frame_MultiChannel_PreservesInterleavedOrder()
    {
        // 5.1: L, R, C, LFE, Ls, Rs
        var source = new AacPcmFrame
        {
            Samples = new[]
            {
                1f, -1f, 0.5f, -0.5f, 0.25f, -0.25f,
                0f, 0f, 0f, 0f, 0f, 0f,
            },
            ChannelCount = 6,
            SamplesPerChannel = 2,
            SampleRate = 48000,
            Speakers = new[]
            {
                AacSpeaker.FrontLeft, AacSpeaker.FrontRight,
                AacSpeaker.FrontCentre, AacSpeaker.Lfe,
                AacSpeaker.BackLeft, AacSpeaker.BackRight,
            },
        };

        var dst = AacPcmFrameConverter.ToInt16Frame(source);
        Assert.Equal(6, dst.ChannelCount);
        Assert.Equal(2, dst.SamplesPerChannel);
        Assert.Equal(6, dst.Speakers.Count);
        Assert.Equal(12, dst.Samples.Length);
        Assert.Equal(short.MaxValue, dst.Samples[0]);
        Assert.Equal(short.MinValue, dst.Samples[1]);
        Assert.Equal(16384, dst.Samples[2]);
        Assert.Equal(-16384, dst.Samples[3]);
    }

    [Fact]
    public void ToInt16Sample_ExactlyMinusOne_Returns_Minus32768()
    {
        // -1f * 32768 == -32768 exactly → short.MinValue.
        var result = AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -1f });
        Assert.Equal(short.MinValue, result[0]);
    }

    [Fact]
    public void ToInt16Sample_NegativeZero_Returns_Zero()
    {
        // -0f maps via the positive branch (>= 0f), result still 0.
        var result = AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -0f });
        Assert.Equal((short)0, result[0]);
    }

    [Fact]
    public void ToInt16Sample_Epsilon_Returns_Zero()
    {
        var result = AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { float.Epsilon });
        Assert.Equal((short)0, result[0]);
    }

    [Fact]
    public void ToInt16Samples_AllocatingOverload_AllocatesFreshArray()
    {
        ReadOnlySpan<float> src = stackalloc float[] { 0.1f, 0.2f };
        var a = AacPcmFrameConverter.ToInt16Samples(src);
        var b = AacPcmFrameConverter.ToInt16Samples(src);
        Assert.NotSame(a, b);
        Assert.Equal(a, b);
    }

    [Fact]
    public void ToInt16Samples_DestinationOverload_DoesNotMutateSource()
    {
        var src = new float[] { 0.5f, -0.5f, 1f };
        var copy = (float[])src.Clone();
        var dst = new short[3];
        AacPcmFrameConverter.ToInt16Samples(src, dst);
        Assert.Equal(copy, src);
    }

    [Fact]
    public void ToInt16Sample_PositiveJustUnderOne_DoesNotSaturate()
    {
        // 0.9999f * 32767 ≈ 32763.6 → rounds to nearest valid sample
        var result = AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { 0.9999f });
        Assert.InRange(result[0], (short)32750, (short)32766);
    }

    [Fact]
    public void ToInt16Sample_NegativeJustOverMinusOne_DoesNotSaturate()
    {
        // -0.9999f * 32768 ≈ -32764.7
        var result = AacPcmFrameConverter.ToInt16Samples(stackalloc float[] { -0.9999f });
        Assert.InRange(result[0], (short)-32766, (short)-32750);
    }

    [Fact]
    public void ToInt16Frame_FreshSamplesArray_DoesNotAliasSource()
    {
        var source = new AacPcmFrame
        {
            Samples = new[] { 0.5f, -0.5f },
            ChannelCount = 2,
            SamplesPerChannel = 1,
            SampleRate = 48000,
            Speakers = new[] { AacSpeaker.FrontLeft, AacSpeaker.FrontRight },
        };
        var first = AacPcmFrameConverter.ToInt16Frame(source);
        var second = AacPcmFrameConverter.ToInt16Frame(source);
        Assert.NotSame(first.Samples, second.Samples);
        Assert.Equal(first.Samples, second.Samples);
    }

    [Fact]
    public void ToInt16Frame_PassesSpeakersByReference()
    {
        // Speakers is IReadOnlyList<AacSpeaker> — converter just hands
        // the reference through, not a copy.
        var speakers = new[] { AacSpeaker.FrontCentre };
        var source = new AacPcmFrame
        {
            Samples = new[] { 0f },
            ChannelCount = 1,
            SamplesPerChannel = 1,
            SampleRate = 48000,
            Speakers = speakers,
        };
        var dst = AacPcmFrameConverter.ToInt16Frame(source);
        Assert.Same(speakers, dst.Speakers);
    }

    [Fact]
    public void ToInt16Samples_NamedDestinationArgument_ThrowsOnUndersized()
    {
        var src = new float[8];
        var dst = new short[7];
        var ex = Assert.Throws<ArgumentException>(
            () => AacPcmFrameConverter.ToInt16Samples(src, dst));
        Assert.Equal("destination", ex.ParamName);
    }
}
