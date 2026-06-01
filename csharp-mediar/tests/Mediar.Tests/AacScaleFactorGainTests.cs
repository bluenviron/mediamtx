using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacScaleFactorGainTests
{
    [Fact]
    public void Gain_AtSfOffset_ReturnsOne()
    {
        // sf=100 → exponent = 0 → 2^0 = 1.
        Assert.Equal(1f, AacScaleFactorGain.Gain(100), precision: 6);
    }

    [Fact]
    public void Gain_FourAboveOffset_DoublesAmplitude()
    {
        // sf=104 → (104-100)/4 = 1 → 2^1 = 2.
        Assert.Equal(2f, AacScaleFactorGain.Gain(104), precision: 6);
    }

    [Fact]
    public void Gain_FourBelowOffset_HalvesAmplitude()
    {
        // sf=96 → (96-100)/4 = -1 → 2^-1 = 0.5.
        Assert.Equal(0.5f, AacScaleFactorGain.Gain(96), precision: 6);
    }

    [Theory]
    [InlineData(100, 1.0)]
    [InlineData(101, 1.189207115)]              // 2^(1/4)
    [InlineData(102, 1.414213562)]              // 2^(1/2) = sqrt(2)
    [InlineData(103, 1.681792831)]              // 2^(3/4)
    [InlineData(104, 2.0)]
    [InlineData(108, 4.0)]
    [InlineData(116, 16.0)]
    [InlineData(140, 1024.0)]
    [InlineData(96, 0.5)]
    [InlineData(92, 0.25)]
    [InlineData(76, 0.015625)]                  // 2^-6
    [InlineData(0, 2.9802322387695312e-08)]     // 2^-25
    [InlineData(255, 4.622876999680e11)]        // 2^38.75
    public void Gain_KnownPowerOfTwo_MatchesSpec(int sf, double expected)
    {
        float result = AacScaleFactorGain.Gain(sf);
        double tolerance = Math.Max(expected * 1e-5, 1e-37);
        Assert.Equal(expected, result, tolerance: tolerance);
    }

    [Fact]
    public void Gain_NegativeScaleFactor_StillFinite()
    {
        // PNS bands can produce small negative absolute scale factors.
        float result = AacScaleFactorGain.Gain(-10);
        Assert.True(float.IsFinite(result));
        Assert.True(result > 0);
        Assert.True(result < 1f);
    }

    [Fact]
    public void ApplyTo_InPlace_MultipliesEveryCoefficient()
    {
        var band = new float[4] { 1f, 2f, -3f, 4f };
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 104); // gain=2
        Assert.Equal(2f, band[0], precision: 5);
        Assert.Equal(4f, band[1], precision: 5);
        Assert.Equal(-6f, band[2], precision: 5);
        Assert.Equal(8f, band[3], precision: 5);
    }

    [Fact]
    public void ApplyTo_AtOffset_LeavesValuesUnchanged()
    {
        var band = new float[3] { 1.5f, -2.25f, 0.125f };
        var original = band.ToArray();
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 100);
        for (int i = 0; i < band.Length; i++)
        {
            Assert.Equal(original[i], band[i], precision: 5);
        }
    }

    [Fact]
    public void ApplyTo_EmptyBand_NoOp()
    {
        Span<float> band = Span<float>.Empty;
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 120);
        // No exception thrown.
    }

    [Fact]
    public void Apply_SrcToDst_MultipliesAndWrites()
    {
        var src = new float[] { 1f, 2f, 4f };
        var dst = new float[3];
        AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 108); // gain=4
        Assert.Equal(4f, dst[0], precision: 5);
        Assert.Equal(8f, dst[1], precision: 5);
        Assert.Equal(16f, dst[2], precision: 5);
    }

    [Fact]
    public void Apply_DestinationTooShort_Throws()
    {
        var src = new float[3];
        var dst = new float[2];
        Assert.Throws<ArgumentException>(() =>
            AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 100));
    }

    [Fact]
    public void Apply_DestinationLonger_OnlyFirstNTouched()
    {
        var src = new float[] { 1f, 1f };
        var dst = new float[] { 7f, 7f, 7f };
        AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 100);
        Assert.Equal(1f, dst[0], precision: 5);
        Assert.Equal(1f, dst[1], precision: 5);
        Assert.Equal(7f, dst[2]);
    }

    [Fact]
    public void SfOffset_IsOneHundred()
    {
        Assert.Equal(100, AacScaleFactorGain.SfOffset);
    }

    [Fact]
    public void ApplyTo_Zeros_RemainZeros_For_AnyScaleFactor()
    {
        Span<float> band = stackalloc float[] { 0f, 0f, 0f };
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 200);
        Assert.Equal(0f, band[0]);
        Assert.Equal(0f, band[1]);
        Assert.Equal(0f, band[2]);
    }

    [Fact]
    public void ApplyTo_Negative_ScaleFactor_Shrinks_Towards_Zero()
    {
        var band = new float[] { 1f, -1f };
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: -100);
        Assert.True(Math.Abs(band[0]) < 1e-10f);
        Assert.True(Math.Abs(band[1]) < 1e-10f);
    }

    [Fact]
    public void Apply_EmptySource_NoOp()
    {
        Span<float> dst = stackalloc float[3];
        dst[0] = 7f;
        AacScaleFactorGain.Apply(ReadOnlySpan<float>.Empty, dst, absoluteScaleFactor: 100);
        Assert.Equal(7f, dst[0]);
    }

    [Fact]
    public void Apply_Matches_ApplyTo_When_Source_Equals_Destination_Buffer()
    {
        var inplace = new float[] { 1f, 2f, 3f, 4f };
        var copy = new float[] { 1f, 2f, 3f, 4f };
        var dst = new float[4];
        AacScaleFactorGain.ApplyTo(inplace, 108); // gain = 4
        AacScaleFactorGain.Apply(copy, dst, 108);
        for (int i = 0; i < dst.Length; i++)
        {
            Assert.Equal(inplace[i], dst[i], precision: 5);
        }
    }

    [Fact]
    public void Gain_Symmetric_Around_Offset()
    {
        for (int delta = 1; delta <= 50; delta++)
        {
            float up = AacScaleFactorGain.Gain(100 + delta);
            float dn = AacScaleFactorGain.Gain(100 - delta);
            Assert.Equal(1.0, up * dn, tolerance: 1e-4);
        }
    }

    [Fact]
    public void Apply_Reverses_With_Negated_Exponent()
    {
        var src = new float[] { 1.5f, -3.25f, 0.125f };
        var forward = new float[3];
        AacScaleFactorGain.Apply(src, forward, 116); // gain=16

        var roundTrip = new float[3];
        AacScaleFactorGain.Apply(forward, roundTrip, 84); // gain=1/16
        for (int i = 0; i < src.Length; i++)
        {
            Assert.Equal(src[i], roundTrip[i], precision: 4);
        }
    }
}
