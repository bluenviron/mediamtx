using System.Collections.Immutable;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsInverseQuantTests
{
    private static AacTnsFilter Filter(int order, int coefBits, params int[] raw)
        => new()
        {
            Length = 16,
            Order = order,
            Direction = false,
            CoefCompress = false,
            CoefBits = coefBits,
            Coefficients = raw.ToImmutableArray(),
        };

    private static float ExpectedParcor(int signed, bool coefResHigh)
    {
        double basePower = coefResHigh ? 8.0 : 4.0;
        double iqfac = (basePower - 0.5) / (Math.PI / 2.0);
        double iqfacM = (basePower + 0.5) / (Math.PI / 2.0);
        double tmp = signed >= 0 ? signed / iqfac : signed / iqfacM;
        return (float)Math.Sin(tmp);
    }

    [Fact]
    public void Compute_NullFilter_Throws()
    {
        Span<float> parcor = stackalloc float[1];
        Assert.Throws<ArgumentNullException>(() =>
        {
            // Action wrapping not allowed for span - use a local function pattern.
            float[] arr = new float[1];
            AacTnsInverseQuant.Compute(null!, false, arr);
        });
    }

    [Fact]
    public void Compute_WrongParcorLength_Throws()
    {
        var f = Filter(order: 2, coefBits: 3, 0, 0);
        float[] parcor = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsInverseQuant.Compute(f, coefResHigh: false, parcor));
        Assert.Equal("parcor", ex.ParamName);
    }

    [Fact]
    public void Compute_InvalidCoefBits_Throws()
    {
        var f = Filter(order: 1, coefBits: 5, 0);
        float[] parcor = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsInverseQuant.Compute(f, coefResHigh: true, parcor));
        Assert.Equal("filter", ex.ParamName);
    }

    [Fact]
    public void Compute_RawOutOfRange_Throws()
    {
        // 3-bit field max raw value is 7; 8 is out of range.
        var f = Filter(order: 1, coefBits: 3, 8);
        float[] parcor = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsInverseQuant.Compute(f, coefResHigh: false, parcor));
        Assert.Equal("filter", ex.ParamName);
    }

    [Fact]
    public void Compute_OrderZero_NoOp()
    {
        var f = Filter(order: 0, coefBits: 0);
        // CoefBits=0 isn't checked when order is zero (early return).
        float[] parcor = Array.Empty<float>();
        AacTnsInverseQuant.Compute(f, coefResHigh: false, parcor);
        Assert.Empty(parcor);
    }

    [Fact]
    public void Compute_AllocatingOverload_ReturnsCorrectLength()
    {
        var f = Filter(order: 3, coefBits: 3, 0, 1, 7);
        var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: false);
        Assert.Equal(3, parcor.Length);
    }

    [Fact]
    public void Compute_ZeroRawValue_YieldsZeroParcor()
    {
        var f = Filter(order: 1, coefBits: 4, 0);
        var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true);
        Assert.Equal(0f, parcor[0]);
    }

    [Fact]
    public void Compute_3BitBasePositiveSweep_MatchesFormula()
    {
        // 3-bit field: signed range is -4..3 (raw 4..7 are negative).
        // Sweep positive half (raw 0..3, signed = raw).
        for (int raw = 0; raw <= 3; raw++)
        {
            var f = Filter(order: 1, coefBits: 3, raw);
            var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: false);
            float expected = ExpectedParcor(raw, coefResHigh: false);
            Assert.Equal(expected, parcor[0], 6);
        }
    }

    [Fact]
    public void Compute_3BitBaseNegativeSweep_MatchesFormula()
    {
        // 3-bit field: raw 4..7 → signed -4..-1.
        for (int raw = 4; raw <= 7; raw++)
        {
            int signed = raw - 8;
            var f = Filter(order: 1, coefBits: 3, raw);
            var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: false);
            float expected = ExpectedParcor(signed, coefResHigh: false);
            Assert.Equal(expected, parcor[0], 6);
        }
    }

    [Fact]
    public void Compute_4BitBasePositiveSweep_MatchesFormula()
    {
        // 4-bit field: signed range is -8..7.
        for (int raw = 0; raw <= 7; raw++)
        {
            var f = Filter(order: 1, coefBits: 4, raw);
            var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true);
            float expected = ExpectedParcor(raw, coefResHigh: true);
            Assert.Equal(expected, parcor[0], 6);
        }
    }

    [Fact]
    public void Compute_4BitBaseNegativeSweep_MatchesFormula()
    {
        for (int raw = 8; raw <= 15; raw++)
        {
            int signed = raw - 16;
            var f = Filter(order: 1, coefBits: 4, raw);
            var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true);
            float expected = ExpectedParcor(signed, coefResHigh: true);
            Assert.Equal(expected, parcor[0], 6);
        }
    }

    [Fact]
    public void Compute_ParcorMagnitudeAlwaysStrictlyLessThanOne()
    {
        // Stability invariant: |parcor| < 1 across the full quantization range.
        // Test both resolutions.
        foreach (bool res in new[] { false, true })
        {
            int coefBits = res ? 4 : 3;
            int range = 1 << coefBits;
            for (int raw = 0; raw < range; raw++)
            {
                var f = Filter(order: 1, coefBits: coefBits, raw);
                var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: res);
                Assert.True(Math.Abs(parcor[0]) < 1.0f,
                    $"raw={raw} res={res} produced |parcor|={Math.Abs(parcor[0])}");
            }
        }
    }

    [Fact]
    public void Compute_PositiveAndNegativeSameMagnitude_DifferInScale()
    {
        // The +/- step sizes are asymmetric: positive uses 7.5, negative uses 8.5
        // (for 4-bit), so a raw value of +1 and its sign-flipped counterpart (signed -1, raw=15)
        // should NOT produce parcor values that are exact negatives of each other.
        var fPos = Filter(order: 1, coefBits: 4, 1);
        var fNeg = Filter(order: 1, coefBits: 4, 15);   // signed -1

        var parcorPos = AacTnsInverseQuant.Compute(fPos, coefResHigh: true);
        var parcorNeg = AacTnsInverseQuant.Compute(fNeg, coefResHigh: true);

        Assert.True(parcorPos[0] > 0);
        Assert.True(parcorNeg[0] < 0);
        // |parcor_pos| should be GREATER than |parcor_neg| because positive
        // step size denominator is smaller (7.5 vs 8.5).
        Assert.True(Math.Abs(parcorPos[0]) > Math.Abs(parcorNeg[0]),
            $"pos={parcorPos[0]} neg={parcorNeg[0]}");
    }

    [Fact]
    public void Compute_MultipleCoefficients_PreservesOrder()
    {
        var f = Filter(order: 4, coefBits: 4, 1, 2, 14, 15);
        var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true);

        Assert.Equal(ExpectedParcor(1,  true), parcor[0], 6);
        Assert.Equal(ExpectedParcor(2,  true), parcor[1], 6);
        Assert.Equal(ExpectedParcor(-2, true), parcor[2], 6);
        Assert.Equal(ExpectedParcor(-1, true), parcor[3], 6);
    }

    [Fact]
    public void Compute_LpcStepUpRoundTrip_ProducesStableFilter()
    {
        // PARCOR → step-up → direct-form. Apply inverse filter to a
        // delta input and confirm the resulting IIR doesn't blow up.
        var f = Filter(order: 4, coefBits: 4, 7, 6, 5, 4); // strong positive PARCOR
        var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true);

        // Stability: every PARCOR must be in (-1, 1).
        foreach (var k in parcor)
        {
            Assert.True(Math.Abs(k) < 1.0f);
        }

        // Step-up should not throw and should produce LPCs that, when used
        // in the inverse filter on a unit impulse, give a bounded impulse
        // response.
        float[] lpc = AacTnsLpcStepUp.Compute(parcor);
        float[] impulse = new float[64];
        impulse[0] = 1.0f;
        AacTnsInverseFilter.Apply(impulse, lpc, reverseDirection: false);

        // Bounded impulse response: max absolute value should stay reasonable.
        // For PARCOR ~0.5..0.9 the IIR can be peaky but must not explode.
        float maxAbs = 0;
        foreach (var v in impulse) maxAbs = Math.Max(maxAbs, Math.Abs(v));
        Assert.True(maxAbs < 1000f, $"impulse response exploded: maxAbs={maxAbs}");
    }

    [Fact]
    public void Compute_2BitCompressedField_WorksForBothPolarities()
    {
        // 2-bit field: raw 0..3 → signed -2..1. raw=0,1 positive; raw=2,3 negative.
        var f0 = Filter(order: 1, coefBits: 2, 0);
        var f1 = Filter(order: 1, coefBits: 2, 1);
        var f2 = Filter(order: 1, coefBits: 2, 2);  // signed -2
        var f3 = Filter(order: 1, coefBits: 2, 3);  // signed -1

        Assert.Equal(0f, AacTnsInverseQuant.Compute(f0, coefResHigh: false)[0], 6);
        Assert.Equal(ExpectedParcor(1, false),  AacTnsInverseQuant.Compute(f1, coefResHigh: false)[0], 6);
        Assert.Equal(ExpectedParcor(-2, false), AacTnsInverseQuant.Compute(f2, coefResHigh: false)[0], 6);
        Assert.Equal(ExpectedParcor(-1, false), AacTnsInverseQuant.Compute(f3, coefResHigh: false)[0], 6);
    }

    [Fact]
    public void Compute_InPlaceMatchesAllocatingOverload()
    {
        var f = Filter(order: 5, coefBits: 4, 3, 12, 0, 7, 9);
        var alloc = AacTnsInverseQuant.Compute(f, coefResHigh: true);
        Span<float> inplace = stackalloc float[5];
        AacTnsInverseQuant.Compute(f, coefResHigh: true, inplace);
        for (int i = 0; i < 5; i++)
        {
            Assert.Equal(alloc[i], inplace[i], 6);
        }
    }

    [Fact]
    public void Compute_AllocatingOverload_NullFilter_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AacTnsInverseQuant.Compute(null!, false));
    }

    [Fact]
    public void Compute_AllocatingOverload_RawCountMismatch_Throws()
    {
        var f = new AacTnsFilter
        {
            Length = 16,
            Order = 3,
            Direction = false,
            CoefCompress = false,
            CoefBits = 3,
            Coefficients = ImmutableArray.Create(1, 2), // only 2 raws for order 3
        };
        var ex = Assert.Throws<ArgumentException>(() => AacTnsInverseQuant.Compute(f, false));
        Assert.Equal("filter", ex.ParamName);
    }

    [Fact]
    public void Compute_SpanOverload_CoefBitsBelowRange_Throws()
    {
        var f = Filter(order: 1, coefBits: 1, 0);
        float[] parcor = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsInverseQuant.Compute(f, coefResHigh: false, parcor));
        Assert.Equal("filter", ex.ParamName);
    }

    [Theory]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    public void Compute_RawZero_AlwaysProducesZero_AcrossAllSupportedWidths(int coefBits)
    {
        var f = Filter(order: 1, coefBits: coefBits, 0);
        Assert.Equal(0f, AacTnsInverseQuant.Compute(f, coefResHigh: false)[0], 6);
        Assert.Equal(0f, AacTnsInverseQuant.Compute(f, coefResHigh: true)[0], 6);
    }

    [Fact]
    public void Compute_4BitMostNegative_ProducesLargestNegativeMagnitude()
    {
        // raw=8 (signed=-8) is the most-negative encoding for a 4-bit field.
        var f = Filter(order: 1, coefBits: 4, 8);
        float parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true)[0];
        Assert.True(parcor < 0);
        // Verify formula: sin(-8 * (π/2) / 8.5)
        double expected = Math.Sin(-8 * (Math.PI / 2) / 8.5);
        Assert.Equal((float)expected, parcor, 6);
        // Still inside the unit interval.
        Assert.True(Math.Abs(parcor) < 1f);
    }

    [Fact]
    public void Compute_4BitMostPositive_ProducesLargestPositiveMagnitude()
    {
        // raw=7 (signed=7) is the most-positive encoding for a 4-bit field.
        var f = Filter(order: 1, coefBits: 4, 7);
        float parcor = AacTnsInverseQuant.Compute(f, coefResHigh: true)[0];
        Assert.True(parcor > 0);
        double expected = Math.Sin(7 * (Math.PI / 2) / 7.5);
        Assert.Equal((float)expected, parcor, 6);
        Assert.True(parcor < 1f);
    }

    [Fact]
    public void Compute_PositiveMonotonicAcross3BitRange()
    {
        // Positive half (raw 0..3) is strictly increasing in parcor[0].
        float prev = float.NegativeInfinity;
        for (int raw = 0; raw <= 3; raw++)
        {
            var f = Filter(order: 1, coefBits: 3, raw);
            float val = AacTnsInverseQuant.Compute(f, coefResHigh: false)[0];
            Assert.True(val > prev, $"raw={raw} val={val} prev={prev}");
            prev = val;
        }
    }

    [Fact]
    public void Compute_SpanOverload_IsDeterministic()
    {
        var f = Filter(order: 3, coefBits: 4, 5, 10, 2);
        Span<float> first = stackalloc float[3];
        Span<float> second = stackalloc float[3];
        AacTnsInverseQuant.Compute(f, coefResHigh: true, first);
        AacTnsInverseQuant.Compute(f, coefResHigh: true, second);
        for (int i = 0; i < 3; i++)
        {
            Assert.Equal(first[i], second[i]);
        }
    }

    [Fact]
    public void Compute_AllocatingOverload_OrderZero_ReturnsEmptyArray()
    {
        var f = Filter(order: 0, coefBits: 0);
        var parcor = AacTnsInverseQuant.Compute(f, coefResHigh: false);
        Assert.Empty(parcor);
    }

    [Fact]
    public void Compute_2BitMostNegative_Sign_Is_Negative()
    {
        // 2-bit field: raw=2 (signed=-2) is the most-negative encoding.
        var f = Filter(order: 1, coefBits: 2, 2);
        float parcor = AacTnsInverseQuant.Compute(f, coefResHigh: false)[0];
        Assert.True(parcor < 0);
        // Formula: sin(-2 * π/2 / 4.5)
        double expected = Math.Sin(-2 * (Math.PI / 2) / 4.5);
        Assert.Equal((float)expected, parcor, 6);
    }

    [Fact]
    public void Compute_SpanOverload_DoesNotMutate_BeyondOrder()
    {
        var f = Filter(order: 2, coefBits: 3, 1, 2);
        float[] parcor = new float[2];
        AacTnsInverseQuant.Compute(f, coefResHigh: false, parcor);
        // Only 2 entries are valid; both should be non-zero finite values.
        Assert.True(float.IsFinite(parcor[0]));
        Assert.True(float.IsFinite(parcor[1]));
        Assert.NotEqual(0f, parcor[0]);
        Assert.NotEqual(0f, parcor[1]);
    }

    [Fact]
    public void Compute_AllocatingOverload_DifferentResolutions_ProduceDifferentMagnitudes()
    {
        // Same raw value at different resolution flags must use different step sizes.
        var f = Filter(order: 1, coefBits: 4, 3);
        float low = AacTnsInverseQuant.Compute(f, coefResHigh: false)[0];
        float high = AacTnsInverseQuant.Compute(f, coefResHigh: true)[0];
        // Low-res uses denominator 3.5 vs high-res 7.5, so low-res parcor
        // should be LARGER in magnitude (raw 3 maps to bigger angle).
        Assert.True(Math.Abs(low) > Math.Abs(high), $"low={low} high={high}");
    }

    [Fact]
    public void Compute_RawAboveRangeFor4Bit_Throws()
    {
        // 4-bit field max raw = 15. 16 is out of range.
        var f = Filter(order: 1, coefBits: 4, 16);
        float[] parcor = new float[1];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsInverseQuant.Compute(f, coefResHigh: true, parcor));
        Assert.Equal("filter", ex.ParamName);
    }
}
