using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacInverseQuantizationTests
{
    [Fact]
    public void Dequantize_Zero_ReturnsZero()
    {
        Assert.Equal(0f, AacInverseQuantization.Dequantize(0));
    }

    [Fact]
    public void Dequantize_One_ReturnsOne()
    {
        // |1|^(4/3) = 1.
        Assert.Equal(1f, AacInverseQuantization.Dequantize(1), precision: 5);
    }

    [Fact]
    public void Dequantize_NegativeOne_ReturnsNegativeOne()
    {
        Assert.Equal(-1f, AacInverseQuantization.Dequantize(-1), precision: 5);
    }

    [Theory]
    [InlineData(2,    2.519842099789747)]      // 2^(4/3) = 2.5198421
    [InlineData(3,    4.326748710922225)]      // 3^(4/3) = 4.3267487
    [InlineData(4,    6.349604207872798)]      // 4^(4/3) = 6.3496042
    [InlineData(8,    16.0)]                    // 8^(4/3) = 16 exactly
    [InlineData(27,   80.99999999999999)]       // 27^(4/3) = 81
    [InlineData(64,   256.0)]                   // 64^(4/3) = 256
    [InlineData(125,  624.9999999999998)]       // 125^(4/3) = 625
    [InlineData(8191, 165113.4940829452)]       // max AAC quantised value
    public void Dequantize_PositiveValues_MatchPower43(int input, double expected)
    {
        float result = AacInverseQuantization.Dequantize(input);
        Assert.Equal(expected, result, tolerance: Math.Abs(expected) * 1e-4);
    }

    [Theory]
    [InlineData(-2,    -2.519842099789747)]
    [InlineData(-8,    -16.0)]
    [InlineData(-27,   -80.99999999999999)]
    [InlineData(-8191, -165113.4940829452)]
    public void Dequantize_NegativeValues_MatchSignedPower43(int input, double expected)
    {
        float result = AacInverseQuantization.Dequantize(input);
        Assert.Equal(expected, result, tolerance: Math.Abs(expected) * 1e-4);
    }

    [Fact]
    public void Dequantize_Symmetric_OutputsAreSignFlipped()
    {
        for (int i = 1; i <= 256; i++)
        {
            float pos = AacInverseQuantization.Dequantize(i);
            float neg = AacInverseQuantization.Dequantize(-i);
            Assert.Equal(pos, -neg, precision: 5);
        }
    }

    [Fact]
    public void Dequantize_Span_PopulatesDestinationElementwise()
    {
        int[] src = [-8, -1, 0, 1, 8, 27, 64];
        var dst = new float[src.Length];
        AacInverseQuantization.Dequantize(src, dst);
        Assert.Equal(-16f, dst[0], precision: 4);
        Assert.Equal(-1f, dst[1], precision: 5);
        Assert.Equal(0f, dst[2]);
        Assert.Equal(1f, dst[3], precision: 5);
        Assert.Equal(16f, dst[4], precision: 4);
        Assert.Equal(81f, dst[5], precision: 3);
        Assert.Equal(256f, dst[6], precision: 3);
    }

    [Fact]
    public void Dequantize_Span_TooShortDestination_Throws()
    {
        int[] src = [1, 2, 3];
        var dst = new float[2];
        Assert.Throws<ArgumentException>(() =>
            AacInverseQuantization.Dequantize(src, dst));
    }

    [Fact]
    public void Dequantize_Span_AllocatingOverload_ReturnsCorrectLength()
    {
        int[] src = [0, 1, 8];
        float[] result = AacInverseQuantization.Dequantize(src);
        Assert.Equal(3, result.Length);
        Assert.Equal(0f, result[0]);
        Assert.Equal(1f, result[1], precision: 5);
        Assert.Equal(16f, result[2], precision: 4);
    }

    [Fact]
    public void Dequantize_Span_EmptySource_LeavesDestinationUntouched()
    {
        var dst = new float[3];
        dst[0] = 1.25f;
        AacInverseQuantization.Dequantize(ReadOnlySpan<int>.Empty, dst);
        Assert.Equal(1.25f, dst[0]);
    }

    [Fact]
    public void Dequantize_Span_AllZeros_AllZeros()
    {
        var src = new int[1024];
        var dst = new float[1024];
        AacInverseQuantization.Dequantize(src, dst);
        foreach (float f in dst) Assert.Equal(0f, f);
    }

    [Fact]
    public void Dequantize_Exponent_IsFourThirds()
    {
        Assert.Equal(4f / 3f, AacInverseQuantization.Exponent);
    }

    [Fact]
    public void Dequantize_OutOfSpec_PositiveInput_StillProcessed()
    {
        // Spec limit is +/-8191; we do not saturate, just compute.
        float r = AacInverseQuantization.Dequantize(10_000);
        Assert.True(r > 200_000f && r < 250_000f);
    }

    [Fact]
    public void Dequantize_OutOfSpec_NegativeInput_StillProcessed()
    {
        float r = AacInverseQuantization.Dequantize(-10_000);
        Assert.True(r < -200_000f && r > -250_000f);
    }

    [Fact]
    public void Dequantize_Span_AllocatingOverload_Empty_Returns_Empty()
    {
        var result = AacInverseQuantization.Dequantize(ReadOnlySpan<int>.Empty);
        Assert.Empty(result);
    }

    [Fact]
    public void Dequantize_Span_AllocatingOverload_AgreesWithScalar()
    {
        int[] src = [-8191, -1, 0, 1, 8191];
        float[] viaSpan = AacInverseQuantization.Dequantize(src);
        for (int i = 0; i < src.Length; i++)
        {
            Assert.Equal(AacInverseQuantization.Dequantize(src[i]), viaSpan[i]);
        }
    }

    [Fact]
    public void Dequantize_Span_DestinationLargerThanSource_LeavesTail()
    {
        int[] src = [1];
        var dst = new float[3];
        dst[1] = 99f;
        dst[2] = -99f;
        AacInverseQuantization.Dequantize(src, dst);
        Assert.Equal(1f, dst[0], precision: 5);
        // Tail untouched.
        Assert.Equal(99f, dst[1]);
        Assert.Equal(-99f, dst[2]);
    }

    [Theory]
    [InlineData(1, 1.0)]
    [InlineData(8, 16.0)]
    [InlineData(27, 81.0)]
    [InlineData(64, 256.0)]
    [InlineData(125, 625.0)]
    [InlineData(216, 1296.0)]
    [InlineData(343, 2401.0)]
    [InlineData(512, 4096.0)]
    [InlineData(729, 6561.0)]
    [InlineData(1000, 10000.0)]
    public void Dequantize_PerfectCubes_AreIntegerPower43(int input, double expected)
    {
        // For inputs of the form n^3, n^(4/3) = n^4 is exact integer
        // arithmetic; this is the cleanest way to assert numerical
        // correctness across the full clip range.
        float result = AacInverseQuantization.Dequantize(input);
        Assert.Equal(expected, result, tolerance: Math.Abs(expected) * 1e-5);
    }

    [Fact]
    public void Dequantize_MonotonicallyIncreasing_OnPositives()
    {
        float prev = AacInverseQuantization.Dequantize(0);
        for (int i = 1; i <= 8191; i++)
        {
            float v = AacInverseQuantization.Dequantize(i);
            Assert.True(v > prev, $"Dequantize({i}) = {v} should exceed Dequantize({i - 1}) = {prev}.");
            prev = v;
        }
    }

    [Fact]
    public void Dequantize_IsPure_RepeatedCallsAreStable()
    {
        for (int i = -1000; i <= 1000; i += 37)
        {
            float a = AacInverseQuantization.Dequantize(i);
            float b = AacInverseQuantization.Dequantize(i);
            Assert.Equal(a, b);
        }
    }

    [Fact]
    public void Dequantize_Span_ExactLength_DoesNotThrow()
    {
        int[] src = [1, 2, 3];
        var dst = new float[3];
        AacInverseQuantization.Dequantize(src, dst);
        Assert.Equal(1f, dst[0], precision: 5);
        Assert.Equal(2.519842f, dst[1], precision: 4);
        Assert.Equal(4.326749f, dst[2], precision: 4);
    }

    [Fact]
    public void Dequantize_Span_EmptyDestination_NoThrowOnEmptySource()
    {
        AacInverseQuantization.Dequantize(ReadOnlySpan<int>.Empty, Span<float>.Empty);
        // No assert — verifying it doesn't throw or modify anything is enough.
    }

    [Fact]
    public void Dequantize_Span_TooShortBy_One_Throws_With_Destination_ParamName()
    {
        int[] src = [1, 2];
        var dst = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacInverseQuantization.Dequantize(src, dst));
        Assert.Equal("destination", ex.ParamName);
    }

    [Fact]
    public void Dequantize_Span_AllocatingOverload_DoesNotShareBuffer()
    {
        int[] src = [1, 2, 3];
        float[] a = AacInverseQuantization.Dequantize(src);
        float[] b = AacInverseQuantization.Dequantize(src);
        Assert.NotSame(a, b);
        // Same numeric content
        Assert.Equal(a[0], b[0]);
        Assert.Equal(a[1], b[1]);
        Assert.Equal(a[2], b[2]);
    }

    [Fact]
    public void Dequantize_AllValuesInFullClipRange_AreNonNegativeForNonNegativeInputs()
    {
        for (int i = 0; i <= 8191; i++)
        {
            float v = AacInverseQuantization.Dequantize(i);
            Assert.True(v >= 0f);
        }
    }

    [Fact]
    public void Dequantize_IntMinValue_NegatesViaDoubleWithoutOverflow()
    {
        // The implementation negates via `-(double)xQuant` so int.MinValue
        // produces a large finite negative result without overflowing.
        float v = AacInverseQuantization.Dequantize(int.MinValue);
        Assert.True(float.IsFinite(v));
        Assert.True(v < 0f);
    }

    [Fact]
    public void Dequantize_IntMaxValue_ProducesLargeFinitePositive()
    {
        float v = AacInverseQuantization.Dequantize(int.MaxValue);
        Assert.True(float.IsFinite(v));
        Assert.True(v > 0f);
    }

    [Fact]
    public void Dequantize_Exponent_Is_Approximately_1_3333()
    {
        Assert.InRange(AacInverseQuantization.Exponent, 1.333f, 1.334f);
    }
}
