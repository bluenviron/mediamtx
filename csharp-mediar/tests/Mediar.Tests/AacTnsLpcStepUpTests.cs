using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsLpcStepUpTests
{
    [Fact]
    public void Compute_LengthMismatch_Throws()
    {
        var parcor = new float[3];
        var lpc = new float[2];
        Assert.Throws<ArgumentException>(() =>
            AacTnsLpcStepUp.Compute(parcor, lpc));
    }

    [Fact]
    public void Compute_OrderExceedsMax_Throws()
    {
        var parcor = new float[AacTnsLpcStepUp.MaxOrder + 1];
        Assert.Throws<ArgumentException>(() => AacTnsLpcStepUp.Compute(parcor));
    }

    [Fact]
    public void Compute_OrderZero_NoOp()
    {
        var lpc = Array.Empty<float>();
        AacTnsLpcStepUp.Compute(ReadOnlySpan<float>.Empty, lpc);
        Assert.Empty(lpc);
    }

    [Fact]
    public void Compute_AllocatingOverload_ReturnsLength()
    {
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0.5f, 0.25f, 0.125f });
        Assert.Equal(3, lpc.Length);
    }

    [Fact]
    public void Compute_OrderOne_MatchesParcor()
    {
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0.5f });
        Assert.Equal(0.5f, lpc[0], precision: 6);
    }

    [Fact]
    public void Compute_OrderTwo_MatchesHandWorkedRecurrence()
    {
        // k1=0.5, k2=0.25 → a[0] = k1*(1+k2) = 0.625, a[1] = k2 = 0.25
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0.5f, 0.25f });
        Assert.Equal(0.625f, lpc[0], precision: 6);
        Assert.Equal(0.25f, lpc[1], precision: 6);
    }

    [Fact]
    public void Compute_OrderThree_MatchesHandWorkedRecurrence()
    {
        // k1=0.5, k2=0.25, k3=0.125
        //   After step 2: lpc = [0.625, 0.25]
        //   Step 3:
        //     tmp[0] = 0.625 + 0.125 * 0.25 = 0.65625
        //     tmp[1] = 0.25  + 0.125 * 0.625 = 0.328125
        //   lpc = [0.65625, 0.328125, 0.125]
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0.5f, 0.25f, 0.125f });
        Assert.Equal(0.65625f, lpc[0], precision: 6);
        Assert.Equal(0.328125f, lpc[1], precision: 6);
        Assert.Equal(0.125f, lpc[2], precision: 6);
    }

    [Fact]
    public void Compute_AllZeroParcor_ProducesAllZeroLpc()
    {
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0f, 0f, 0f, 0f, 0f });
        foreach (var c in lpc) Assert.Equal(0f, c);
    }

    [Fact]
    public void Compute_NegativeParcor_PreservesSignInTail()
    {
        // k = [-0.5] → a[0] = -0.5.
        var lpc = AacTnsLpcStepUp.Compute(new float[] { -0.5f });
        Assert.Equal(-0.5f, lpc[0], precision: 6);

        // k = [-0.5, -0.5] →
        //   a[0] = -0.5 * (1 + -0.5) = -0.5 * 0.5 = -0.25
        //   a[1] = -0.5
        var lpc2 = AacTnsLpcStepUp.Compute(new float[] { -0.5f, -0.5f });
        Assert.Equal(-0.25f, lpc2[0], precision: 6);
        Assert.Equal(-0.5f, lpc2[1], precision: 6);
    }

    [Fact]
    public void Compute_MaxOrder_DoesNotThrowOrAllocate()
    {
        var parcor = new float[AacTnsLpcStepUp.MaxOrder];
        for (int i = 0; i < parcor.Length; i++) parcor[i] = 0.01f * (i + 1);
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        Assert.Equal(AacTnsLpcStepUp.MaxOrder, lpc.Length);
        // Last coefficient is always equal to the last PARCOR.
        Assert.Equal(parcor[^1], lpc[^1], precision: 5);
    }

    [Fact]
    public void Compute_LastCoefficientAlwaysEqualsLastParcor()
    {
        var parcor = new float[] { 0.3f, -0.4f, 0.6f, -0.2f };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        Assert.Equal(parcor[^1], lpc[^1], precision: 6);
    }

    [Fact]
    public void Compute_FilterRoundTrip_IirInverseOfFirRecoversInput()
    {
        // Apply forward FIR (encoder side): y[n] = x[n] - sum(a[k] * x[n-k]).
        // Apply IIR inverse (decoder side): z[n] = y[n] + sum(a[k] * z[n-k]).
        // For a step-up-built LPC the IIR is exactly the inverse of the FIR
        // and z must equal x for any input.
        var parcor = new float[] { 0.3f, -0.2f, 0.1f };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        int order = lpc.Length;

        var x = new float[] { 1f, 2f, 3f, -1f, 0.5f, -0.25f, 4f, 0f, 2f, 1f };
        var y = new float[x.Length];
        for (int n = 0; n < x.Length; n++)
        {
            float s = x[n];
            for (int k = 1; k <= order; k++)
            {
                if (n - k >= 0) s -= lpc[k - 1] * x[n - k];
            }
            y[n] = s;
        }

        var z = new float[x.Length];
        for (int n = 0; n < x.Length; n++)
        {
            float s = y[n];
            for (int k = 1; k <= order; k++)
            {
                if (n - k >= 0) s += lpc[k - 1] * z[n - k];
            }
            z[n] = s;
        }

        for (int n = 0; n < x.Length; n++)
        {
            Assert.Equal(x[n], z[n], precision: 4);
        }
    }

    [Fact]
    public void Compute_LengthMismatch_LpcLonger_Throws()
    {
        var parcor = new float[2];
        var lpc = new float[3];
        Assert.Throws<ArgumentException>(() => AacTnsLpcStepUp.Compute(parcor, lpc));
    }

    [Fact]
    public void MaxOrder_Matches_TnsLongMax()
    {
        Assert.Equal(AacTnsData.MaxOrderLong, AacTnsLpcStepUp.MaxOrder);
    }

    [Fact]
    public void Compute_Span_Overload_OrderZero_NoOp()
    {
        // Spec: order == 0 returns without touching the (zero-length) buffer.
        var lpc = Array.Empty<float>();
        AacTnsLpcStepUp.Compute(ReadOnlySpan<float>.Empty, lpc);
        Assert.Empty(lpc);
    }

    [Fact]
    public void Compute_AllocatingOverload_OrderZero_ReturnsEmpty()
    {
        var lpc = AacTnsLpcStepUp.Compute(ReadOnlySpan<float>.Empty);
        Assert.Empty(lpc);
    }

    [Fact]
    public void Compute_AllocatingOverload_AllocatesIndependentArrayPerCall()
    {
        var parcor = new float[] { 0.1f, 0.2f };
        var a = AacTnsLpcStepUp.Compute(parcor);
        var b = AacTnsLpcStepUp.Compute(parcor);
        Assert.NotSame(a, b);
        Assert.Equal(a, b);
    }

    [Fact]
    public void Compute_Span_And_Allocating_Produce_Same_Output()
    {
        var parcor = new float[] { 0.7f, -0.3f, 0.2f, -0.1f };
        var a = AacTnsLpcStepUp.Compute(parcor);
        var b = new float[parcor.Length];
        AacTnsLpcStepUp.Compute(parcor, b);
        for (int i = 0; i < a.Length; i++)
        {
            Assert.Equal(a[i], b[i], precision: 6);
        }
    }

    [Fact]
    public void Compute_DoesNotMutate_Parcor_Input()
    {
        var parcor = new float[] { 0.5f, 0.25f, 0.125f };
        var copy = (float[])parcor.Clone();
        _ = AacTnsLpcStepUp.Compute(parcor);
        Assert.Equal(copy, parcor);
    }

    [Fact]
    public void Compute_OrderTwo_Negative_Then_Positive_Parcor()
    {
        // k1 = -0.5, k2 = 0.25
        //   After step 1: lpc = [-0.5]
        //   Step 2:
        //     tmp[0] = -0.5 + 0.25 * -0.5 = -0.625
        //   lpc = [-0.625, 0.25]
        var lpc = AacTnsLpcStepUp.Compute(new float[] { -0.5f, 0.25f });
        Assert.Equal(-0.625f, lpc[0], precision: 6);
        Assert.Equal(0.25f, lpc[1], precision: 6);
    }

    [Fact]
    public void MaxOrder_Constant_Is_31()
    {
        Assert.Equal(31, AacTnsLpcStepUp.MaxOrder);
    }

    [Fact]
    public void Compute_OrderFour_HandWorked()
    {
        // k = [0.5, 0.25, 0.125, 0.0625]
        // After order 3 (per existing test): lpc = [0.65625, 0.328125, 0.125]
        // Step 4: k = 0.0625
        //   tmp[0] = 0.65625 + 0.0625 * 0.125    = 0.6640625
        //   tmp[1] = 0.328125 + 0.0625 * 0.328125 = 0.34863281
        //   tmp[2] = 0.125    + 0.0625 * 0.65625 = 0.166015625
        //   lpc = [0.6640625, 0.34863281, 0.166015625, 0.0625]
        var lpc = AacTnsLpcStepUp.Compute(new float[] { 0.5f, 0.25f, 0.125f, 0.0625f });
        Assert.Equal(0.6640625f, lpc[0], precision: 6);
        Assert.Equal(0.34863281f, lpc[1], precision: 6);
        Assert.Equal(0.166015625f, lpc[2], precision: 6);
        Assert.Equal(0.0625f, lpc[3], precision: 6);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(31)]
    public void Compute_LastLpc_Equals_LastParcor_For_Orders(int order)
    {
        var parcor = new float[order];
        for (int i = 0; i < order; i++) parcor[i] = (i * 0.07f) - 0.3f;
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        Assert.Equal(parcor[^1], lpc[^1], precision: 5);
    }

    [Fact]
    public void Compute_Span_Overload_Overwrites_PreFilled_Lpc()
    {
        // Pre-fill with sentinel values; Compute must overwrite them.
        var parcor = new float[] { 0.5f, 0.25f };
        var lpc = new float[] { 999f, -999f };
        AacTnsLpcStepUp.Compute(parcor, lpc);
        Assert.Equal(0.625f, lpc[0], precision: 6);
        Assert.Equal(0.25f, lpc[1], precision: 6);
    }

    [Fact]
    public void Compute_Span_Overload_DoesNotMutate_Parcor()
    {
        var parcor = new float[] { 0.5f, 0.25f, 0.125f };
        var copy = (float[])parcor.Clone();
        var lpc = new float[parcor.Length];
        AacTnsLpcStepUp.Compute(parcor, lpc);
        Assert.Equal(copy, parcor);
    }

    [Theory]
    [InlineData(-0.99f)]
    [InlineData(-0.5f)]
    [InlineData(0f)]
    [InlineData(0.5f)]
    [InlineData(0.99f)]
    public void Compute_OrderOne_MatchesParcor_Across_Range(float k)
    {
        var lpc = AacTnsLpcStepUp.Compute(new float[] { k });
        Assert.Equal(k, lpc[0], precision: 6);
    }
}
