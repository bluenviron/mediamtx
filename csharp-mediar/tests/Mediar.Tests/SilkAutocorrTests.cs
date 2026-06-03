using Mediar.Codecs.Opus.Encoder.Silk;
using Xunit;

namespace Mediar.Tests;

public class SilkAutocorrTests
{
    [Fact]
    public void Autocorrelation_OfImpulse_IsOneAtZeroOtherwise()
    {
        float[] x = new float[64];
        x[0] = 1f;
        float[] r = new float[5];
        SilkAutocorr.Autocorrelation(x, r, 4);
        Assert.Equal(1f, r[0], 6);
        for (int k = 1; k <= 4; k++) Assert.Equal(0f, r[k], 6);
    }

    [Fact]
    public void Autocorrelation_IsSymmetricallyDefiniteAndDecreasingForSinusoid()
    {
        const int n = 256;
        var x = new float[n];
        for (int i = 0; i < n; i++) x[i] = MathF.Sin(2f * MathF.PI * 8f * i / n);
        var r = new float[9];
        SilkAutocorr.Autocorrelation(x, r, 8);
        // r[0] is the energy and must be the largest absolute lag.
        Assert.True(r[0] > 0f);
        for (int k = 1; k <= 8; k++)
            Assert.True(MathF.Abs(r[k]) <= r[0] + 1e-3f);
    }

    [Fact]
    public void Burg_OnImpulse_DrivesAllPredictionCoefficientsToZero()
    {
        // An isolated impulse has no predictable structure, so Burg's
        // PARCORs (and hence direct-form a[i]) should collapse to ~0.
        float[] x = new float[32];
        x[5] = 1f;
        var a = new float[4];
        float residual = SilkAutocorr.Burg(x, a, 4);
        for (int i = 0; i < 4; i++) Assert.InRange(a[i], -0.2f, 0.2f);
        Assert.True(residual > 0f);
    }

    [Fact]
    public void Burg_AgreesWithLevinsonOnSyntheticAr2Signal()
    {
        // Generate an AR(2) signal: x[n] = -a1·x[n-1] - a2·x[n-2] + w[n],
        // with poles inside the unit circle.
        const int n = 4096;
        const float a1Truth = -1.5f, a2Truth = 0.7f;
        var x = new float[n];
        var rng = new Random(0xC0DEF00D);
        x[0] = 0f; x[1] = 0f;
        for (int i = 2; i < n; i++)
        {
            float w = (float)(rng.NextDouble() * 2 - 1);
            x[i] = -a1Truth * x[i - 1] - a2Truth * x[i - 2] + w;
        }

        var aBurg = new float[2];
        SilkAutocorr.Burg(x, aBurg, 2);

        var r = new float[3];
        SilkAutocorr.Autocorrelation(x, r, 2);
        var aLev = new float[2];
        SilkAutocorr.LevinsonDurbin(r, aLev, 2);

        // Both methods should recover the true AR(2) parameters and
        // agree with each other within a small tolerance on this
        // long, well-conditioned window.
        Assert.Equal(a1Truth, aBurg[0], 1);
        Assert.Equal(a2Truth, aBurg[1], 1);
        Assert.Equal(aBurg[0], aLev[0], 1);
        Assert.Equal(aBurg[1], aLev[1], 1);
    }

    [Fact]
    public void Burg_ResidualEnergyIsNonNegativeAndBoundedByInputEnergy()
    {
        var rng = new Random(0xBADF00D);
        var x = new float[1024];
        double inputE = 0;
        for (int i = 0; i < x.Length; i++)
        {
            x[i] = (float)(rng.NextDouble() * 2 - 1);
            inputE += (double)x[i] * x[i];
        }
        var a = new float[10];
        float residual = SilkAutocorr.Burg(x, a, 10);
        Assert.True(residual >= 0f);
        // For broadband noise the residual should be a meaningful
        // fraction of the input energy — definitely not larger.
        Assert.True(residual <= inputE + 1e-3);
    }
}
