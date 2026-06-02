using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacImdctNaiveTests
{
    private static readonly float[] LinearityInputA = new float[] { 0.5f, -1f, 0.25f, 0.75f };
    private static readonly float[] SuperpositionInputA = new float[] { 1f, 0f, 0f, 0f };
    private static readonly float[] SuperpositionInputB = new float[] { 0f, 1f, 0f, 0f };
    private static readonly float[] SuperpositionInputSum = new float[] { 1f, 1f, 0f, 0f };
    private static readonly float[] ImpulseM2DC = new float[] { 1f, 0f };
    private static readonly float[] ImpulseM4DC = new float[] { 1f, 0f, 0f, 0f };

    private static double Cosine(int n, int k, int m)
    {
        int big = 2 * m;
        double n0 = (m + 1) / 2.0;
        return Math.Cos(2.0 * Math.PI / big * (n + n0) * (k + 0.5));
    }

    [Fact]
    public void Inverse_BadOutputLength_Throws()
    {
        Assert.Throws<ArgumentException>(() =>
        {
            float[] coefs = new float[4];
            float[] samples = new float[5];
            AacImdctNaive.Inverse(coefs.AsSpan(), samples.AsSpan());
        });
    }

    [Fact]
    public void Inverse_EmptyInput_NoOp()
    {
        Span<float> samples = stackalloc float[0];
        AacImdctNaive.Inverse(Span<float>.Empty, samples);
    }

    [Theory]
    [InlineData(2)]
    [InlineData(4)]
    [InlineData(8)]
    [InlineData(16)]
    public void Inverse_OutputIsExactlyTwiceInputLength(int m)
    {
        var coefs = new float[m];
        var samples = new float[2 * m];
        AacImdctNaive.Inverse(coefs.AsSpan(), samples.AsSpan());
        Assert.Equal(2 * m, samples.Length);
    }

    [Fact]
    public void Inverse_AllZeroInput_AllZeroOutput()
    {
        var coefs = new float[16];
        var samples = new float[32];
        AacImdctNaive.Inverse(coefs.AsSpan(), samples.AsSpan());
        foreach (var v in samples)
        {
            Assert.Equal(0f, v);
        }
    }

    [Fact]
    public void Inverse_SingleImpulseAtK0_ProducesExpectedCosine()
    {
        // M = 4, N = 8, n0 = 2.5. IMDCT([1,0,0,0])[n] = (2/8) * cos(2pi/8 * (n + 2.5) * 0.5)
        const int m = 4;
        var coefs = new float[m];
        coefs[0] = 1f;
        var samples = AacImdctNaive.Inverse(coefs.AsSpan());

        double scale = 2.0 / (2 * m);
        for (int n = 0; n < 2 * m; n++)
        {
            double expected = scale * Cosine(n, 0, m);
            Assert.Equal(expected, samples[n], 5);
        }
    }

    [Fact]
    public void Inverse_SingleImpulseAtK1_ProducesExpectedCosine()
    {
        const int m = 8;
        var coefs = new float[m];
        coefs[1] = 1f;
        var samples = AacImdctNaive.Inverse(coefs.AsSpan());

        double scale = 2.0 / (2 * m);
        for (int n = 0; n < 2 * m; n++)
        {
            double expected = scale * Cosine(n, 1, m);
            Assert.Equal(expected, samples[n], 5);
        }
    }

    [Fact]
    public void Inverse_Linearity_ScalarTwo()
    {
        // IMDCT(2*x) = 2*IMDCT(x).
        var ref1 = AacImdctNaive.Inverse(LinearityInputA.AsSpan());

        var scaled = new float[LinearityInputA.Length];
        for (int i = 0; i < LinearityInputA.Length; i++) scaled[i] = LinearityInputA[i] * 2f;
        var got = AacImdctNaive.Inverse(scaled.AsSpan());

        for (int i = 0; i < ref1.Length; i++)
        {
            Assert.Equal(2f * ref1[i], got[i], 5);
        }
    }

    [Fact]
    public void Inverse_Linearity_Superposition()
    {
        var ya = AacImdctNaive.Inverse(SuperpositionInputA.AsSpan());
        var yb = AacImdctNaive.Inverse(SuperpositionInputB.AsSpan());
        var ysum = AacImdctNaive.Inverse(SuperpositionInputSum.AsSpan());

        for (int i = 0; i < ysum.Length; i++)
        {
            Assert.Equal(ya[i] + yb[i], ysum[i], 5);
        }
    }

    [Fact]
    public void Inverse_M128Short_FinishesWithoutOverflowOrNaN()
    {
        var coefs = new float[AacImdctNaive.ShortInputLength];
        for (int i = 0; i < coefs.Length; i++) coefs[i] = (float)Math.Sin(i * 0.1);
        var samples = AacImdctNaive.Inverse(coefs.AsSpan());
        foreach (var v in samples)
        {
            Assert.False(float.IsNaN(v));
            Assert.False(float.IsInfinity(v));
        }
    }

    [Fact]
    public void Inverse_TdacRoundTrip_LongPathReconstructsRamp()
    {
        // TDAC: forward MDCT (encoder convention includes 2x scale) then IMDCT with
        // sine window applied on both sides and overlap-add reconstructs the central
        // M samples of the input ramp.
        const int m = 16;
        int n = 2 * m;

        // Build a ramp x[i] = i for i = 0..3M-1.
        var x = new float[3 * m];
        for (int i = 0; i < x.Length; i++) x[i] = i;

        var win = new float[n];
        double scale = Math.PI / n;
        for (int i = 0; i < n; i++)
        {
            win[i] = (float)Math.Sin(scale * (i + 0.5));
        }

        // Frame 1: x[0..2M-1], Frame 2: x[M..3M-1]
        var f1 = new float[n];
        var f2 = new float[n];
        for (int i = 0; i < n; i++) { f1[i] = x[i] * win[i]; f2[i] = x[m + i] * win[i]; }

        var coefs1 = ForwardMdct(f1, m);
        var coefs2 = ForwardMdct(f2, m);

        var y1 = AacImdctNaive.Inverse(coefs1.AsSpan());
        var y2 = AacImdctNaive.Inverse(coefs2.AsSpan());

        // Window both outputs and overlap-add over the middle M samples.
        for (int i = 0; i < n; i++) { y1[i] *= win[i]; y2[i] *= win[i]; }

        // Reconstruction at x[M..2M-1] = y1[M..2M-1] + y2[0..M-1].
        for (int i = 0; i < m; i++)
        {
            float reconstructed = y1[m + i] + y2[i];
            Assert.Equal(x[m + i], reconstructed, 2);
        }
    }

    private static float[] ForwardMdct(ReadOnlySpan<float> timeDomain, int m)
    {
        // AAC encoder convention: forward MDCT includes a 2x scale so that the
        // decoder (2/N)x IMDCT plus windowed OLA gives unity reconstruction.
        int n = timeDomain.Length;
        Assert.Equal(2 * m, n);
        double n0 = (m + 1) / 2.0;
        double omega = 2.0 * Math.PI / n;

        var coefs = new float[m];
        for (int k = 0; k < m; k++)
        {
            double sum = 0.0;
            for (int i = 0; i < n; i++)
            {
                sum += timeDomain[i] * Math.Cos(omega * (i + n0) * (k + 0.5));
            }
            coefs[k] = (float)(2.0 * sum);
        }
        return coefs;
    }

    [Fact]
    public void Constants_MatchAacBlockSizes()
    {
        Assert.Equal(1024, AacImdctNaive.LongInputLength);
        Assert.Equal(128, AacImdctNaive.ShortInputLength);
    }

    [Fact]
    public void Inverse_SamplesOverloadReturnsNewBuffer()
    {
        var got = AacImdctNaive.Inverse(ImpulseM4DC.AsSpan());
        Assert.Equal(8, got.Length);
    }

    [Fact]
    public void Inverse_DcCoefficientOnly_AllSamplesFromCosineTable()
    {
        // M=2, N=4, n0=1.5. spec=[1,0]. Hand-computed values:
        const int m = 2;
        var samples = AacImdctNaive.Inverse(ImpulseM2DC.AsSpan());

        double scale = 2.0 / (2 * m);
        for (int n = 0; n < 2 * m; n++)
        {
            double expected = scale * Cosine(n, 0, m);
            Assert.Equal(expected, samples[n], 5);
        }
    }

    [Fact]
    public void Inverse_AllocatingOverload_EmptyInput_ReturnsEmpty()
    {
        var samples = AacImdctNaive.Inverse(ReadOnlySpan<float>.Empty);
        Assert.Empty(samples);
    }

    [Fact]
    public void Inverse_NegativeImpulse_K0_ProducesNegatedCosine()
    {
        const int m = 4;
        var coefs = new float[m];
        coefs[0] = -1f;
        var samples = AacImdctNaive.Inverse(coefs.AsSpan());

        double scale = 2.0 / (2 * m);
        for (int n = 0; n < 2 * m; n++)
        {
            double expected = -scale * Cosine(n, 0, m);
            Assert.Equal(expected, samples[n], 5);
        }
    }

    [Fact]
    public void Inverse_Linearity_ScalarNegativeOne()
    {
        // IMDCT(-x) = -IMDCT(x).
        var refY = AacImdctNaive.Inverse(LinearityInputA.AsSpan());

        var negated = new float[LinearityInputA.Length];
        for (int i = 0; i < LinearityInputA.Length; i++) negated[i] = -LinearityInputA[i];
        var got = AacImdctNaive.Inverse(negated.AsSpan());

        for (int i = 0; i < refY.Length; i++)
        {
            Assert.Equal(-refY[i], got[i], 5);
        }
    }

    [Theory]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(7)]
    [InlineData(9)]
    public void Inverse_NonDoubleOutput_Length_Throws(int sampleLen)
    {
        Assert.Throws<ArgumentException>(() =>
        {
            float[] coefs = new float[4];
            float[] samples = new float[sampleLen];
            AacImdctNaive.Inverse(coefs.AsSpan(), samples.AsSpan());
        });
    }

    [Fact]
    public void Inverse_AllocatingOverload_AllocatesFreshArray_PerCall()
    {
        var coefs = new float[] { 1f, 0.5f, -0.25f, 0.125f };
        var a = AacImdctNaive.Inverse(coefs.AsSpan());
        var b = AacImdctNaive.Inverse(coefs.AsSpan());
        Assert.NotSame(a, b);
        for (int i = 0; i < a.Length; i++)
        {
            Assert.Equal(a[i], b[i], precision: 5);
        }
    }

    [Fact]
    public void Inverse_DoesNotMutate_Coefficient_Input()
    {
        var coefs = new float[] { 0.5f, -1f, 0.25f, 0.75f };
        var copy = (float[])coefs.Clone();
        var samples = new float[2 * coefs.Length];
        AacImdctNaive.Inverse(coefs.AsSpan(), samples.AsSpan());
        Assert.Equal(copy, coefs);
    }

    [Fact]
    public void Inverse_LongInputLength_Produces_Finite_Samples()
    {
        var coefs = new float[AacImdctNaive.LongInputLength];
        for (int i = 0; i < coefs.Length; i++) coefs[i] = (float)(0.01 * Math.Sin(i * 0.03));
        var samples = AacImdctNaive.Inverse(coefs.AsSpan());
        Assert.Equal(2048, samples.Length);
        foreach (var v in samples)
        {
            Assert.False(float.IsNaN(v));
            Assert.False(float.IsInfinity(v));
        }
    }
}
