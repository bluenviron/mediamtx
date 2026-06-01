using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacKbdWindowTests
{
    [Theory]
    [InlineData(0, 4.0)]
    [InlineData(-1, 4.0)]
    public void ComputeRisingHalf_BadLength_Throws(int n, double alpha)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacKbdWindow.ComputeRisingHalf(n, alpha));
    }

    [Theory]
    [InlineData(8, 0.0)]
    [InlineData(8, -1.0)]
    public void ComputeRisingHalf_BadAlpha_Throws(int n, double alpha)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => AacKbdWindow.ComputeRisingHalf(n, alpha));
    }

    [Theory]
    [InlineData(128, 4.0)]
    [InlineData(1024, 4.0)]
    [InlineData(128, 6.0)]
    public void ComputeRisingHalf_ReturnsExpectedLength(int halfLength, double alpha)
    {
        var window = AacKbdWindow.ComputeRisingHalf(halfLength, alpha);
        Assert.Equal(halfLength, window.Length);
    }

    [Theory]
    [InlineData(8, 4.0)]
    [InlineData(128, 4.0)]
    [InlineData(128, 6.0)]
    [InlineData(1024, 4.0)]
    public void ComputeRisingHalf_AllValuesInZeroToOne(int halfLength, double alpha)
    {
        var window = AacKbdWindow.ComputeRisingHalf(halfLength, alpha);
        foreach (var v in window)
        {
            Assert.InRange(v, 0f, 1f);
        }
    }

    [Theory]
    [InlineData(16, 4.0)]
    [InlineData(128, 4.0)]
    [InlineData(128, 6.0)]
    public void ComputeRisingHalf_IsMonotonicallyIncreasing(int halfLength, double alpha)
    {
        var window = AacKbdWindow.ComputeRisingHalf(halfLength, alpha);
        for (int i = 1; i < window.Length; i++)
        {
            Assert.True(window[i] >= window[i - 1], $"Not monotonic at i={i} (alpha={alpha})");
        }
    }

    [Theory]
    [InlineData(16, 4.0)]
    [InlineData(128, 6.0)]
    public void ComputeFull_IsSymmetric(int halfLength, double alpha)
    {
        var window = AacKbdWindow.ComputeFull(halfLength, alpha);
        int fullLength = window.Length;
        for (int n = 0; n < halfLength; n++)
        {
            Assert.Equal(window[n], window[fullLength - 1 - n], 6);
        }
    }

    [Theory]
    [InlineData(16, 4.0)]
    [InlineData(128, 4.0)]
    [InlineData(128, 6.0)]
    public void ComputeFull_TdacPerfectReconstructionHolds(int halfLength, double alpha)
    {
        // KBD satisfies w(n)^2 + w(N + n)^2 = 1 for all n in [0, N).
        var full = AacKbdWindow.ComputeFull(halfLength, alpha);
        for (int n = 0; n < halfLength; n++)
        {
            double a = full[n];
            double b = full[halfLength + n];
            Assert.Equal(1.0, a * a + b * b, 4);
        }
    }

    [Theory]
    [InlineData(128, 4.0)]
    [InlineData(128, 6.0)]
    public void ComputeFull_LeftHalfMatchesComputeRisingHalf(int halfLength, double alpha)
    {
        var full = AacKbdWindow.ComputeFull(halfLength, alpha);
        var half = AacKbdWindow.ComputeRisingHalf(halfLength, alpha);
        for (int n = 0; n < halfLength; n++)
        {
            Assert.Equal(half[n], full[n], 6);
        }
    }

    [Fact]
    public void ModifiedBesselI0_ZeroArgument_ReturnsOne()
    {
        Assert.Equal(1.0, AacKbdWindow.ModifiedBesselI0(0.0), 12);
    }

    [Fact]
    public void ModifiedBesselI0_KnownValues()
    {
        // I0(1) ≈ 1.2660658...
        Assert.Equal(1.2660658777520084, AacKbdWindow.ModifiedBesselI0(1.0), 9);
        // I0(2) ≈ 2.2795853...
        Assert.Equal(2.2795853023360673, AacKbdWindow.ModifiedBesselI0(2.0), 9);
        // I0(5) ≈ 27.2398718...
        Assert.Equal(27.239871823604442, AacKbdWindow.ModifiedBesselI0(5.0), 6);
    }

    [Fact]
    public void ModifiedBesselI0_LargeArgument_HandlesPiAlpha4()
    {
        // I0(pi*4) ≈ 24159.0 — covers the worst case for long-block alpha=4.
        double val = AacKbdWindow.ModifiedBesselI0(Math.PI * 4.0);
        Assert.True(val > 1e3, $"I0(4pi) should be large, got {val}");
        Assert.False(double.IsInfinity(val));
        Assert.False(double.IsNaN(val));
    }

    [Fact]
    public void Constants_MatchAacSpecValues()
    {
        Assert.Equal(4.0, AacKbdWindow.LongAlpha);
        Assert.Equal(6.0, AacKbdWindow.ShortAlpha);
        Assert.Equal(1024, AacKbdWindow.LongHalfLength);
        Assert.Equal(128, AacKbdWindow.ShortHalfLength);
    }

    [Fact]
    public void WriteRisingHalf_MatchesComputeRisingHalf()
    {
        var refValues = AacKbdWindow.ComputeRisingHalf(128, 4.0);
        Span<float> buf = stackalloc float[128];
        AacKbdWindow.WriteRisingHalf(buf, 4.0);
        for (int i = 0; i < 128; i++)
        {
            Assert.Equal(refValues[i], buf[i], 6);
        }
    }

    [Fact]
    public void WriteRisingHalf_EmptySpan_NoOp()
    {
        AacKbdWindow.WriteRisingHalf(Span<float>.Empty, 4.0);
    }

    [Fact]
    public void WriteRisingHalf_NegativeAlpha_Throws()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            float[] buf = new float[16];
            AacKbdWindow.WriteRisingHalf(buf.AsSpan(), -1.0);
        });
    }

    [Fact]
    public void ComputeRisingHalf_FirstSampleIsSmall_LastSampleNearOne()
    {
        var window = AacKbdWindow.ComputeRisingHalf(128, 4.0);
        Assert.True(window[0] < 0.01f, $"First sample should be small, got {window[0]}");
        Assert.True(window[127] > 0.95f, $"Last sample should be near 1, got {window[127]}");
    }

    [Fact]
    public void ComputeRisingHalf_LongAlpha4_DiffersFromShortAlpha6()
    {
        var a = AacKbdWindow.ComputeRisingHalf(128, 4.0);
        var b = AacKbdWindow.ComputeRisingHalf(128, 6.0);
        bool anyDiff = false;
        for (int i = 0; i < a.Length; i++)
        {
            if (Math.Abs(a[i] - b[i]) > 1e-5f) { anyDiff = true; break; }
        }
        Assert.True(anyDiff);
    }
}
