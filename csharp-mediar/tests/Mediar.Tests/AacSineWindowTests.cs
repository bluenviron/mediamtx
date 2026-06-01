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

    [Theory]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(128)]
    [InlineData(1024)]
    public void ComputeFull_AllValuesInZeroToOne(int halfLength)
    {
        var window = AacSineWindow.ComputeFull(halfLength);
        foreach (var v in window)
        {
            Assert.InRange(v, 0f, 1f);
        }
    }

    [Fact]
    public void ComputeFull_FirstSample_ApproxSinPiOverTwoN()
    {
        // w[0] = sin(pi/(2*2N) * 0.5) = sin(pi/(4*halfLength))
        var window = AacSineWindow.ComputeFull(8);
        double expected = Math.Sin(Math.PI / 32.0);
        Assert.Equal(expected, window[0], 6);
    }

    [Fact]
    public void ComputeFull_LastSample_Mirrors_FirstSample()
    {
        var window = AacSineWindow.ComputeFull(8);
        Assert.Equal(window[0], window[^1], 6);
    }

    [Fact]
    public void ComputeFull_RightHalfDescendsMonotonically()
    {
        var window = AacSineWindow.ComputeFull(64);
        for (int i = 65; i < window.Length; i++)
        {
            Assert.True(window[i] < window[i - 1], $"Not descending at i={i}");
        }
    }

    [Fact]
    public void ComputeFull_MaxValueIsAtCenter()
    {
        var window = AacSineWindow.ComputeFull(16);
        int center = window.Length / 2 - 1;
        Assert.True(window[center] >= window[0], "Center sample must be >= edge");
        Assert.True(window[center] >= window[^1], "Center sample must be >= edge");
    }

    [Theory]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(128)]
    [InlineData(1024)]
    public void ComputeRisingHalf_SumOfSquares_EqualsHalfLength(int halfLength)
    {
        // For sine window: sum(sin^2((pi/2N)(n+0.5))) over n=0..N-1 = N/2.
        // Check sum of squares is close to halfLength/2.
        var window = AacSineWindow.ComputeRisingHalf(halfLength);
        double sumSq = 0;
        foreach (var v in window) sumSq += v * v;
        Assert.Equal(halfLength / 2.0, sumSq, 3);
    }

    [Fact]
    public void ComputeFull_ShortAndLongLengths_Independent_Instances()
    {
        var a = AacSineWindow.ComputeFull(128);
        var b = AacSineWindow.ComputeFull(128);
        // Two calls must produce equal but independent arrays.
        Assert.Equal(a.Length, b.Length);
        for (int i = 0; i < a.Length; i++) Assert.Equal(a[i], b[i]);
    }

    [Fact]
    public void WriteRisingHalf_LongSize_MatchesComputeRisingHalf()
    {
        var reference = AacSineWindow.ComputeRisingHalf(AacSineWindow.LongHalfLength);
        var buf = new float[AacSineWindow.LongHalfLength];
        AacSineWindow.WriteRisingHalf(buf);
        for (int i = 0; i < buf.Length; i++)
        {
            Assert.Equal(reference[i], buf[i], 6);
        }
    }

    [Fact]
    public void WriteRisingHalf_ShortSize_MatchesComputeRisingHalf()
    {
        var reference = AacSineWindow.ComputeRisingHalf(AacSineWindow.ShortHalfLength);
        var buf = new float[AacSineWindow.ShortHalfLength];
        AacSineWindow.WriteRisingHalf(buf);
        for (int i = 0; i < buf.Length; i++)
        {
            Assert.Equal(reference[i], buf[i], 6);
        }
    }

    [Fact]
    public void WriteRisingHalf_IsMonotonicallyIncreasing()
    {
        Span<float> buf = stackalloc float[32];
        AacSineWindow.WriteRisingHalf(buf);
        for (int i = 1; i < buf.Length; i++)
        {
            Assert.True(buf[i] > buf[i - 1]);
        }
    }

    [Fact]
    public void WriteRisingHalf_SingleSampleBuffer_FillsHalfStepSin()
    {
        // For N=1: w[0] = sin(pi/(2*1) * 0.5) = sin(pi/4) ≈ 0.7071
        Span<float> buf = stackalloc float[1];
        AacSineWindow.WriteRisingHalf(buf);
        Assert.Equal((float)Math.Sin(Math.PI / 4.0), buf[0], 6);
    }

    [Fact]
    public void ComputeRisingHalf_LongLength_FirstAndLast_ApproxSpec()
    {
        var window = AacSineWindow.ComputeRisingHalf(1024);
        double firstExpected = Math.Sin(Math.PI / (2.0 * 1024) * 0.5);
        double lastExpected = Math.Sin(Math.PI / (2.0 * 1024) * (1023 + 0.5));
        Assert.Equal(firstExpected, window[0], 6);
        Assert.Equal(lastExpected, window[^1], 6);
        // First should be near 0, last near 1.
        Assert.True(window[0] < 0.01f);
        Assert.True(window[^1] > 0.99f);
    }

    [Fact]
    public void ComputeFull_PerfectReconstruction_LongWindow()
    {
        var full = AacSineWindow.ComputeFull(AacSineWindow.LongHalfLength);
        for (int n = 0; n < AacSineWindow.LongHalfLength; n++)
        {
            double a = full[n];
            double b = full[AacSineWindow.LongHalfLength + n];
            Assert.Equal(1.0, a * a + b * b, 4);
        }
    }
}
