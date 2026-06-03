using Mediar.Codecs.Opus.Encoder.Silk;
using Xunit;

namespace Mediar.Tests;

public class SilkLpcAnalysisTests
{
    [Fact]
    public void BandwidthExpand_ScalesEachCoefficientByChirpToTheIPlusOne()
    {
        var a = new float[] { 1f, 1f, 1f, 1f };
        const float chirp = 0.9f;
        SilkLpcAnalysis.BandwidthExpand(a, 4, chirp);
        Assert.Equal(0.9f,   a[0], 5);
        Assert.Equal(0.81f,  a[1], 5);
        Assert.Equal(0.729f, a[2], 5);
        Assert.Equal(0.6561f,a[3], 5);
    }

    [Fact]
    public void BandwidthExpand_RejectsOutOfRangeChirp()
    {
        var a = new float[2];
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            SilkLpcAnalysis.BandwidthExpand(a, 2, 1.5f));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            SilkLpcAnalysis.BandwidthExpand(a, 2, 0f));
    }

    [Fact]
    public void IsStable_AcceptsInteriorPoleAndRejectsUnitCirclePole()
    {
        // a[0] = -2*cos(theta)*r,  a[1] = r^2  → poles at r·e^{±jθ}.
        // r = 0.5 → safely interior.
        var safe = new float[] { -2f * 0.5f * MathF.Cos(0.7f), 0.5f * 0.5f };
        Assert.True(SilkLpcAnalysis.IsStable(safe, 2));

        // r = 1.0 → poles on the unit circle → unstable.
        var edge = new float[] { -2f * 1.0f * MathF.Cos(0.7f), 1.0f };
        Assert.False(SilkLpcAnalysis.IsStable(edge, 2));
    }

    [Fact]
    public void Analyze_OnSineRecoversStableLpcAndReducesEnergy()
    {
        const int n = 1024;
        var x = new float[n];
        for (int i = 0; i < n; i++) x[i] = MathF.Sin(2f * MathF.PI * 32f * i / n);

        var lpc = new float[10];
        float residual = SilkLpcAnalysis.Analyze(x, lpc, 10);

        double inputE = 0;
        for (int i = 0; i < n; i++) inputE += (double)x[i] * x[i];

        // LPC of order 2 is enough to predict a pure sine; with order 10
        // and a chirp of 0.99 the residual should be a tiny fraction of
        // the input energy.
        Assert.True(residual < inputE * 0.05);
        Assert.True(SilkLpcAnalysis.IsStable(lpc, 10));
    }
}
