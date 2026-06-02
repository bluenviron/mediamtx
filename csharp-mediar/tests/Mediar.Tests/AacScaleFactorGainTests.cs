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

    [Fact]
    public void Gain_Is_Monotonically_Increasing_In_ScaleFactor()
    {
        float prev = AacScaleFactorGain.Gain(0);
        for (int sf = 1; sf <= 255; sf++)
        {
            float current = AacScaleFactorGain.Gain(sf);
            Assert.True(current > prev, $"Gain({sf}) = {current} not greater than Gain({sf - 1}) = {prev}");
            prev = current;
        }
    }

    [Theory]
    [InlineData(0)]
    [InlineData(50)]
    [InlineData(100)]
    [InlineData(150)]
    [InlineData(200)]
    [InlineData(255)]
    public void Gain_Matches_Pow_Formula_For_Sample_Values(int sf)
    {
        double expected = Math.Pow(2.0, (sf - 100) / 4.0);
        Assert.Equal(expected, AacScaleFactorGain.Gain(sf),
            tolerance: Math.Max(expected * 1e-5, 1e-37));
    }

    [Fact]
    public void Gain_Always_Positive_Across_Full_Range()
    {
        for (int sf = 0; sf <= 255; sf++)
        {
            float g = AacScaleFactorGain.Gain(sf);
            Assert.True(g > 0f, $"Gain({sf}) = {g} is not positive");
            Assert.True(float.IsFinite(g), $"Gain({sf}) = {g} is not finite");
        }
    }

    [Fact]
    public void Gain_Doubles_When_Sf_Increases_By_Four()
    {
        for (int sf = 100; sf < 200; sf++)
        {
            float a = AacScaleFactorGain.Gain(sf);
            float b = AacScaleFactorGain.Gain(sf + 4);
            Assert.Equal(2.0, b / a, tolerance: 1e-4);
        }
    }

    [Fact]
    public void Apply_NaN_Source_Yields_NaN_Destination()
    {
        var src = new float[] { float.NaN, 1f };
        var dst = new float[2];
        AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 104);
        Assert.True(float.IsNaN(dst[0]));
        Assert.Equal(2f, dst[1], precision: 5);
    }

    [Fact]
    public void Apply_Inf_Source_Stays_Positive_Infinity()
    {
        var src = new float[] { float.PositiveInfinity };
        var dst = new float[1];
        AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 100);
        Assert.True(float.IsPositiveInfinity(dst[0]));
    }

    [Fact]
    public void Apply_Destination_Exactly_Same_Length_Works()
    {
        var src = new float[] { 1f, 2f, 3f };
        var dst = new float[3];
        AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 100);
        Assert.Equal(1f, dst[0], precision: 5);
        Assert.Equal(2f, dst[1], precision: 5);
        Assert.Equal(3f, dst[2], precision: 5);
    }

    [Fact]
    public void ApplyTo_Length_One_Band_Scales_Single_Value()
    {
        var band = new float[] { 5f };
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 108); // gain=4
        Assert.Equal(20f, band[0], precision: 5);
    }

    [Fact]
    public void Gain_Multiplicative_Property_Holds()
    {
        // Gain(a) * Gain(b) == Gain(a + b - SfOffset) since both reduce
        // to 2^((a - 100)/4 + (b - 100)/4) == 2^((a + b - 200)/4).
        int a = 120, b = 84;
        float product = AacScaleFactorGain.Gain(a) * AacScaleFactorGain.Gain(b);
        float combined = AacScaleFactorGain.Gain(a + b - AacScaleFactorGain.SfOffset);
        Assert.Equal(product, combined, tolerance: product * 1e-4f);
    }

    [Fact]
    public void Apply_Source_Empty_With_NonEmpty_Destination_Untouched()
    {
        var dst = new float[] { 1f, 2f, 3f };
        AacScaleFactorGain.Apply(ReadOnlySpan<float>.Empty, dst, absoluteScaleFactor: 200);
        Assert.Equal(1f, dst[0], precision: 5);
        Assert.Equal(2f, dst[1], precision: 5);
        Assert.Equal(3f, dst[2], precision: 5);
    }

    [Fact]
    public void ApplyTo_Zero_Length_Band_NoOp_Any_Sf()
    {
        Span<float> band = Span<float>.Empty;
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 0);
        AacScaleFactorGain.ApplyTo(band, absoluteScaleFactor: 255);
    }

    [Fact]
    public void Apply_Destination_Shorter_By_One_Throws()
    {
        var src = new float[] { 1f, 2f, 3f, 4f };
        var dst = new float[3];
        Assert.Throws<ArgumentException>(() =>
            AacScaleFactorGain.Apply(src, dst, absoluteScaleFactor: 108));
    }

    [Fact]
    public void Gain_Zero_Sf_Below_Negative_Sf_Magnitude_Limit_Still_Finite()
    {
        // Even at int.MinValue/+1 (well below the spec range) the result
        // should be a representable float (0 or a finite tiny value).
        float g = AacScaleFactorGain.Gain(-1000);
        Assert.True(g >= 0f);
        Assert.False(float.IsNaN(g));
    }

    [Fact]
    public void Gain_Max_ScaleFactor_Is_Finite_Float()
    {
        // sf = 255 -> exponent 38.75 -> ~4.6e11; well within single-precision range.
        float g = AacScaleFactorGain.Gain(255);
        Assert.True(float.IsFinite(g));
        Assert.True(g > 1e10f);
    }
}
