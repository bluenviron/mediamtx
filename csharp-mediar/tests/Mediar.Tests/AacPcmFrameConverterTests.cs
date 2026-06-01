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
}
