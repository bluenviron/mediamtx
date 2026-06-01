using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSineWindowTests
{
    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(-128)]
    public void ComputeRisingHalf_NonPositiveLength_Throws(int n)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacSineWindow.ComputeRisingHalf(n));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    public void ComputeFull_NonPositiveLength_Throws(int n)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacSineWindow.ComputeFull(n));
    }

    [Theory]
    [InlineData(8)]
    [InlineData(128)]
    [InlineData(1024)]
    public void ComputeRisingHalf_ReturnsExpectedLength(int halfLength)
    {
        var window = AacSineWindow.ComputeRisingHalf(halfLength);
        Assert.Equal(halfLength, window.Length);
    }

    [Theory]
    [InlineData(8)]
    [InlineData(128)]
    [InlineData(1024)]
    public void ComputeFull_ReturnsDoubleLength(int halfLength)
    {
        var window = AacSineWindow.ComputeFull(halfLength);
        Assert.Equal(halfLength * 2, window.Length);
    }

    [Fact]
    public void ComputeRisingHalf_FirstSample_ApproxSinOfHalfStep()
    {
        // For halfLength = 8: w[0] = sin(pi/16 * 0.5) = sin(pi/32) ≈ 0.0980
        var window = AacSineWindow.ComputeRisingHalf(8);
        double expected = Math.Sin(Math.PI / 32.0);
        Assert.Equal(expected, window[0], 6);
    }

    [Fact]
    public void ComputeRisingHalf_LastSample_ApproxSinNearPiOverTwo()
    {
        // For halfLength = 8: w[7] = sin(pi/16 * 7.5) = sin(15pi/32) ≈ 0.9952
        var window = AacSineWindow.ComputeRisingHalf(8);
        double expected = Math.Sin(15.0 * Math.PI / 32.0);
        Assert.Equal(expected, window[7], 6);
    }

    [Theory]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(128)]
    public void ComputeRisingHalf_IsMonotonicallyIncreasing(int halfLength)
    {
        var window = AacSineWindow.ComputeRisingHalf(halfLength);
        for (int i = 1; i < window.Length; i++)
        {
            Assert.True(window[i] > window[i - 1], $"Not monotonic at i={i}");
        }
    }

    [Theory]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(128)]
    public void ComputeFull_IsSymmetric(int halfLength)
    {
        var window = AacSineWindow.ComputeFull(halfLength);
        int fullLength = window.Length;
        for (int n = 0; n < halfLength; n++)
        {
            Assert.Equal(window[n], window[fullLength - 1 - n], 6);
        }
    }

    [Theory]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(128)]
    public void ComputeFull_LeftHalfMatchesComputeRisingHalf(int halfLength)
    {
        var full = AacSineWindow.ComputeFull(halfLength);
        var half = AacSineWindow.ComputeRisingHalf(halfLength);
        for (int n = 0; n < halfLength; n++)
        {
            Assert.Equal(half[n], full[n], 6);
        }
    }

    [Theory]
    [InlineData(8)]
    [InlineData(128)]
    public void ComputeFull_PerfectReconstructionOverlapSquaredSumsToOne(int halfLength)
    {
        // For sine window: w(n)^2 + w(N + n)^2 = 1 for all n in [0, N).
        // This is the MDCT TDAC condition.
        var full = AacSineWindow.ComputeFull(halfLength);
        for (int n = 0; n < halfLength; n++)
        {
            double a = full[n];
            double b = full[halfLength + n];
            Assert.Equal(1.0, a * a + b * b, 5);
        }
    }

    [Fact]
    public void ComputeRisingHalf_LongConstant_MatchesSpec()
    {
        Assert.Equal(1024, AacSineWindow.LongHalfLength);
        var window = AacSineWindow.ComputeRisingHalf(AacSineWindow.LongHalfLength);
        Assert.Equal(1024, window.Length);
    }

    [Fact]
    public void ComputeRisingHalf_ShortConstant_MatchesSpec()
    {
        Assert.Equal(128, AacSineWindow.ShortHalfLength);
        var window = AacSineWindow.ComputeRisingHalf(AacSineWindow.ShortHalfLength);
        Assert.Equal(128, window.Length);
    }

    [Fact]
    public void WriteRisingHalf_FillsDestination()
    {
        Span<float> buf = stackalloc float[8];
        AacSineWindow.WriteRisingHalf(buf);
        for (int i = 0; i < buf.Length; i++)
        {
            Assert.True(buf[i] > 0f);
        }
    }

    [Fact]
    public void WriteRisingHalf_MatchesComputeRisingHalf()
    {
        var ref1 = AacSineWindow.ComputeRisingHalf(16);
        Span<float> buf = stackalloc float[16];
        AacSineWindow.WriteRisingHalf(buf);
        for (int i = 0; i < 16; i++)
        {
            Assert.Equal(ref1[i], buf[i], 6);
        }
    }

    [Fact]
    public void WriteRisingHalf_EmptySpan_NoOp()
    {
        AacSineWindow.WriteRisingHalf(Span<float>.Empty);
    }

    [Fact]
    public void ComputeRisingHalf_Length128_AllValuesInZeroToOne()
    {
        var window = AacSineWindow.ComputeRisingHalf(128);
        foreach (var v in window)
        {
            Assert.InRange(v, 0f, 1f);
        }
    }
}
