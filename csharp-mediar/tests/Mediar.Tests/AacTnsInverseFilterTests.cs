using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsInverseFilterTests
{
    [Fact]
    public void Apply_OrderZero_NoOp()
    {
        var spec = new float[] { 1f, 2f, 3f };
        var copy = (float[])spec.Clone();
        AacTnsInverseFilter.Apply(spec, ReadOnlySpan<float>.Empty, reverseDirection: false);
        Assert.Equal(copy, spec);
    }

    [Fact]
    public void Apply_EmptySpectrum_NoOp()
    {
        var lpc = new float[] { 0.5f };
        AacTnsInverseFilter.Apply(Span<float>.Empty, lpc, reverseDirection: false);
        // Just shouldn't throw.
    }

    [Fact]
    public void Apply_OrderExceedsMax_Throws()
    {
        var spec = new float[10];
        var lpc = new float[AacTnsInverseFilter.MaxOrder + 1];
        Assert.Throws<ArgumentException>(() =>
            AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: false));
    }

    [Fact]
    public void Apply_AllZeroSpectrum_StaysZero()
    {
        var spec = new float[10];
        var lpc = new float[] { 0.5f, -0.25f };
        AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: false);
        foreach (var s in spec) Assert.Equal(0f, s);
    }

    [Fact]
    public void Apply_ForwardOrderOne_ProducesGeometricRecurrence()
    {
        // lpc=[0.5]. State init 0. Input = 1,0,0,0,0. Per spec §4.6.9.4
        // the inverse recursion is y[m] = x[m] - Σ lpc[k] * past[k]:
        //   m=0: sum = 1 - 0 = 1. state[0]=1.
        //   m=1: sum = 0 - 0.5*1 = -0.5. state[0]=-0.5.
        //   m=2: sum = 0 - 0.5*(-0.5) = 0.25. state[0]=0.25.
        //   m=3: sum = 0 - 0.5*0.25 = -0.125. state[0]=-0.125.
        //   m=4: sum = 0 - 0.5*(-0.125) = 0.0625.
        var spec = new float[] { 1f, 0f, 0f, 0f, 0f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f }, reverseDirection: false);
        Assert.Equal(1.0f, spec[0], precision: 5);
        Assert.Equal(-0.5f, spec[1], precision: 5);
        Assert.Equal(0.25f, spec[2], precision: 5);
        Assert.Equal(-0.125f, spec[3], precision: 5);
        Assert.Equal(0.0625f, spec[4], precision: 5);
    }

    [Fact]
    public void Apply_ReverseDirection_WalksHighToLow()
    {
        // Same kernel as the forward order-one test but walking
        // 4,3,2,1,0. Initial impulse at index 4. Alternating-sign
        // geometric decay per the spec MINUS convention.
        var spec = new float[] { 0f, 0f, 0f, 0f, 1f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f }, reverseDirection: true);
        Assert.Equal(1.0f, spec[4], precision: 5);
        Assert.Equal(-0.5f, spec[3], precision: 5);
        Assert.Equal(0.25f, spec[2], precision: 5);
        Assert.Equal(-0.125f, spec[1], precision: 5);
        Assert.Equal(0.0625f, spec[0], precision: 5);
    }

    [Fact]
    public void Apply_OrderTwoForward_MatchesHandComputedSamples()
    {
        // lpc = [0.5, 0.25]. Impulse at 0, length 5. MINUS convention.
        //   m=0: sum=1-0-0=1.                       state=[1,0]
        //   m=1: sum=0-0.5*1-0.25*0=-0.5.            state=[-0.5,1]
        //   m=2: sum=0-0.5*(-0.5)-0.25*1=0.          state=[0,-0.5]
        //   m=3: sum=0-0.5*0-0.25*(-0.5)=0.125.       state=[0.125,0]
        //   m=4: sum=0-0.5*0.125-0.25*0=-0.0625.
        var spec = new float[] { 1f, 0f, 0f, 0f, 0f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f, 0.25f }, reverseDirection: false);
        Assert.Equal(1.0f, spec[0], precision: 5);
        Assert.Equal(-0.5f, spec[1], precision: 5);
        Assert.Equal(0f, spec[2], precision: 5);
        Assert.Equal(0.125f, spec[3], precision: 5);
        Assert.Equal(-0.0625f, spec[4], precision: 5);
    }

    [Fact]
    public void Apply_InverseOfEncoderFir_RecoversInputForward()
    {
        // Build an LPC via the step-up so its inverse is well defined,
        // then verify forward FIR + IIR inverse is identity (forward
        // direction). With the spec MINUS inverse convention the
        // matching forward FIR is y[n] = x[n] + Σ a[k] x[n-k].
        var parcor = new float[] { 0.4f, -0.3f, 0.2f };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        int order = lpc.Length;
        var x = new float[] { 1f, -2f, 3f, 4f, -1.5f, 0.5f, 2f, 0f, -3f, 1.5f };

        var y = new float[x.Length];
        for (int n = 0; n < x.Length; n++)
        {
            float s = x[n];
            for (int k = 1; k <= order; k++)
            {
                if (n - k >= 0) s += lpc[k - 1] * x[n - k];
            }
            y[n] = s;
        }

        AacTnsInverseFilter.Apply(y, lpc, reverseDirection: false);
        for (int n = 0; n < x.Length; n++)
        {
            Assert.Equal(x[n], y[n], precision: 4);
        }
    }

    [Fact]
    public void Apply_InverseOfEncoderFir_RecoversInputReverse()
    {
        // Same as the forward case but operating in the reverse
        // direction; the FIR / IIR pair must be symmetric in either
        // walk direction.
        var parcor = new float[] { 0.4f, -0.3f, 0.2f };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        int order = lpc.Length;
        var x = new float[] { 1f, -2f, 3f, 4f, -1.5f, 0.5f, 2f, 0f, -3f, 1.5f };

        var y = new float[x.Length];
        for (int n = x.Length - 1; n >= 0; n--)
        {
            float s = x[n];
            for (int k = 1; k <= order; k++)
            {
                if (n + k < x.Length) s += lpc[k - 1] * x[n + k];
            }
            y[n] = s;
        }

        AacTnsInverseFilter.Apply(y, lpc, reverseDirection: true);
        for (int n = 0; n < x.Length; n++)
        {
            Assert.Equal(x[n], y[n], precision: 4);
        }
    }

    [Fact]
    public void Apply_NegativeLpc_InverseStillRecoversInput()
    {
        var parcor = new float[] { -0.5f, 0.25f };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        int order = lpc.Length;
        var x = new float[] { 1f, 2f, 3f, 4f, 5f, 6f };

        var y = new float[x.Length];
        for (int n = 0; n < x.Length; n++)
        {
            float s = x[n];
            for (int k = 1; k <= order; k++)
            {
                if (n - k >= 0) s += lpc[k - 1] * x[n - k];
            }
            y[n] = s;
        }

        AacTnsInverseFilter.Apply(y, lpc, reverseDirection: false);
        for (int n = 0; n < x.Length; n++)
        {
            Assert.Equal(x[n], y[n], precision: 4);
        }
    }

    [Fact]
    public void Apply_SpectrumShorterThanOrder_DoesNotThrow()
    {
        // Order 5 but spectrum length 3; state buffer must accept
        // partial population without overflow. MINUS convention:
        //   m=0: sum = 1.                                state=[1,0,0,0,0]
        //   m=1: sum = 2 - 0.1*1 = 1.9.                  state=[1.9,1,0,0,0]
        //   m=2: sum = 3 - 0.1*1.9 - 0.1*1 = 2.71.
        var spec = new float[] { 1f, 2f, 3f };
        var lpc = new float[] { 0.1f, 0.1f, 0.1f, 0.1f, 0.1f };
        AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: false);
        Assert.Equal(1.0f, spec[0], precision: 5);
        Assert.Equal(1.9f, spec[1], precision: 5);
        Assert.Equal(2.71f, spec[2], precision: 5);
    }

    [Fact]
    public void MaxOrder_Matches_TnsLpcStepUp_MaxOrder()
    {
        Assert.Equal(AacTnsLpcStepUp.MaxOrder, AacTnsInverseFilter.MaxOrder);
    }

    [Fact]
    public void Apply_OrderEqualToMaxOrder_DoesNotThrow()
    {
        var spec = new float[64];
        for (int i = 0; i < spec.Length; i++) spec[i] = 0.001f * (i + 1);
        // All-zero LPC at MaxOrder is well-formed and reduces to identity.
        var lpc = new float[AacTnsInverseFilter.MaxOrder];
        var copy = (float[])spec.Clone();
        AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: false);
        Assert.Equal(copy, spec);
    }

    [Fact]
    public void Apply_LengthOneSpectrum_OrderOne_ProducesInputAsOutput()
    {
        var spec = new float[] { 3.5f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.75f }, reverseDirection: false);
        // First sample's IIR has no past state -> output equals input.
        Assert.Equal(3.5f, spec[0], precision: 5);
    }

    [Fact]
    public void Apply_LengthOneSpectrum_Reverse_AlsoProducesInputAsOutput()
    {
        var spec = new float[] { -1.25f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f, -0.3f }, reverseDirection: true);
        Assert.Equal(-1.25f, spec[0], precision: 5);
    }

    [Fact]
    public void Apply_OrderTwoReverse_MirrorsForwardOnReversedInput()
    {
        // The reverse-direction recursion is the forward recursion
        // applied to the reversed spectrum; mirroring should map back.
        var lpc = new float[] { 0.5f, 0.25f };
        var forward = new float[] { 1f, 0f, 0f, 0f, 0f };
        AacTnsInverseFilter.Apply(forward, lpc, reverseDirection: false);

        var reverse = new float[] { 0f, 0f, 0f, 0f, 1f };
        AacTnsInverseFilter.Apply(reverse, lpc, reverseDirection: true);

        for (int i = 0; i < forward.Length; i++)
        {
            Assert.Equal(forward[i], reverse[forward.Length - 1 - i], precision: 5);
        }
    }

    [Fact]
    public void Apply_IsLinear_ScalingSpectrumScalesOutput()
    {
        var lpc = new float[] { 0.4f, -0.2f };
        var x = new float[] { 1f, 0.5f, -0.25f, 0.75f };
        var y2 = new float[x.Length];
        for (int i = 0; i < x.Length; i++) y2[i] = 2f * x[i];

        AacTnsInverseFilter.Apply(x, lpc, reverseDirection: false);
        AacTnsInverseFilter.Apply(y2, lpc, reverseDirection: false);

        for (int i = 0; i < x.Length; i++)
        {
            Assert.Equal(2f * x[i], y2[i], precision: 4);
        }
    }

    [Fact]
    public void Apply_IsLinear_Additive_For_Identical_State_Reset()
    {
        // y(x1+x2) == y(x1) + y(x2) since state starts at zero each call.
        var lpc = new float[] { 0.3f, 0.1f };
        var x1 = new float[] { 1f, 2f, 3f, 4f };
        var x2 = new float[] { -0.5f, 0.25f, 1.5f, -2f };
        var sum = new float[x1.Length];
        for (int i = 0; i < x1.Length; i++) sum[i] = x1[i] + x2[i];

        AacTnsInverseFilter.Apply(x1, lpc, reverseDirection: false);
        AacTnsInverseFilter.Apply(x2, lpc, reverseDirection: false);
        AacTnsInverseFilter.Apply(sum, lpc, reverseDirection: false);

        for (int i = 0; i < x1.Length; i++)
        {
            Assert.Equal(x1[i] + x2[i], sum[i], precision: 4);
        }
    }

    [Fact]
    public void Apply_State_Is_Not_Propagated_Between_Calls()
    {
        var lpc = new float[] { 0.5f };
        var first = new float[] { 1f };
        var second = new float[] { 1f };
        AacTnsInverseFilter.Apply(first, lpc, reverseDirection: false);
        AacTnsInverseFilter.Apply(second, lpc, reverseDirection: false);
        // Both first samples must equal their inputs (state cleared each call).
        Assert.Equal(1f, first[0]);
        Assert.Equal(1f, second[0]);
    }

    [Fact]
    public void Apply_Order_MaxOrder_Plus_Two_Throws()
    {
        var spec = new float[10];
        var lpc = new float[AacTnsInverseFilter.MaxOrder + 2];
        Assert.Throws<ArgumentException>(() =>
            AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: true));
    }

    [Fact]
    public void Apply_All_Zero_Spectrum_With_NonZero_Lpc_Reverse_StaysZero()
    {
        var spec = new float[8];
        var lpc = new float[] { 0.1f, -0.2f, 0.3f };
        AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: true);
        foreach (var s in spec) Assert.Equal(0f, s);
    }

    [Fact]
    public void Apply_PARCOR_RoundTrip_Length_30_Order_8()
    {
        var parcor = new float[8]
        {
            0.4f, -0.3f, 0.2f, -0.1f, 0.05f, -0.05f, 0.025f, -0.025f,
        };
        var lpc = AacTnsLpcStepUp.Compute(parcor);
        int order = lpc.Length;
        var x = new float[30];
        for (int i = 0; i < x.Length; i++) x[i] = (float)Math.Sin(0.3 * i);
        var y = new float[x.Length];
        for (int n = 0; n < x.Length; n++)
        {
            float s = x[n];
            for (int k = 1; k <= order; k++)
            {
                if (n - k >= 0) s += lpc[k - 1] * x[n - k];
            }
            y[n] = s;
        }

        AacTnsInverseFilter.Apply(y, lpc, reverseDirection: false);
        for (int i = 0; i < x.Length; i++)
        {
            Assert.Equal(x[i], y[i], precision: 3);
        }
    }
}
