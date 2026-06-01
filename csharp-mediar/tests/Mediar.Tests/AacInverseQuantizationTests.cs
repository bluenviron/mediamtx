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
}
