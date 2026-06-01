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
        // lpc=[0.5]. State init 0. Input = 1,0,0,0,0:
        //   m=0: sum = 1 + 0 = 1. state[0]=1.
        //   m=1: sum = 0 + 0.5*1 = 0.5. state[0]=0.5.
        //   m=2: sum = 0 + 0.5*0.5 = 0.25. state[0]=0.25.
        //   m=3: 0.125
        //   m=4: 0.0625
        var spec = new float[] { 1f, 0f, 0f, 0f, 0f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f }, reverseDirection: false);
        Assert.Equal(1.0f, spec[0], precision: 5);
        Assert.Equal(0.5f, spec[1], precision: 5);
        Assert.Equal(0.25f, spec[2], precision: 5);
        Assert.Equal(0.125f, spec[3], precision: 5);
        Assert.Equal(0.0625f, spec[4], precision: 5);
    }

    [Fact]
    public void Apply_ReverseDirection_WalksHighToLow()
    {
        // Same kernel as the forward order-one test but walking
        // 4,3,2,1,0. Initial impulse at index 4.
        var spec = new float[] { 0f, 0f, 0f, 0f, 1f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f }, reverseDirection: true);
        Assert.Equal(1.0f, spec[4], precision: 5);
        Assert.Equal(0.5f, spec[3], precision: 5);
        Assert.Equal(0.25f, spec[2], precision: 5);
        Assert.Equal(0.125f, spec[1], precision: 5);
        Assert.Equal(0.0625f, spec[0], precision: 5);
    }

    [Fact]
    public void Apply_OrderTwoForward_MatchesHandComputedSamples()
    {
        // lpc = [0.5, 0.25]. Impulse at 0, length 5.
        //   m=0: sum=1+0+0=1. state=[1,0]
        //   m=1: sum=0+0.5*1+0.25*0=0.5. state=[0.5,1]
        //   m=2: sum=0+0.5*0.5+0.25*1=0.5. state=[0.5,0.5]
        //   m=3: sum=0+0.5*0.5+0.25*0.5=0.375. state=[0.375,0.5]
        //   m=4: sum=0+0.5*0.375+0.25*0.5=0.3125.
        var spec = new float[] { 1f, 0f, 0f, 0f, 0f };
        AacTnsInverseFilter.Apply(spec, new float[] { 0.5f, 0.25f }, reverseDirection: false);
        Assert.Equal(1.0f, spec[0], precision: 5);
        Assert.Equal(0.5f, spec[1], precision: 5);
        Assert.Equal(0.5f, spec[2], precision: 5);
        Assert.Equal(0.375f, spec[3], precision: 5);
        Assert.Equal(0.3125f, spec[4], precision: 5);
    }

    [Fact]
    public void Apply_InverseOfEncoderFir_RecoversInputForward()
    {
        // Build an LPC via the step-up so its inverse is well defined,
        // then verify forward FIR + IIR inverse is identity (forward
        // direction).
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
                if (n - k >= 0) s -= lpc[k - 1] * x[n - k];
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
                if (n + k < x.Length) s -= lpc[k - 1] * x[n + k];
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
                if (n - k >= 0) s -= lpc[k - 1] * x[n - k];
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
        // partial population without overflow.
        var spec = new float[] { 1f, 2f, 3f };
        var lpc = new float[] { 0.1f, 0.1f, 0.1f, 0.1f, 0.1f };
        AacTnsInverseFilter.Apply(spec, lpc, reverseDirection: false);
        // After m=0: sum = 1.
        Assert.Equal(1.0f, spec[0], precision: 5);
        // m=1: sum = 2 + 0.1*1 = 2.1
        Assert.Equal(2.1f, spec[1], precision: 5);
        // m=2: sum = 3 + 0.1*2.1 + 0.1*1 = 3 + 0.21 + 0.1 = 3.31
        Assert.Equal(3.31f, spec[2], precision: 5);
    }
}
