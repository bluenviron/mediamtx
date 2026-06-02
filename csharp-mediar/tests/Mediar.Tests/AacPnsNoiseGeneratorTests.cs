using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacPnsNoiseGeneratorTests
{
    private static double EnergyOf(ReadOnlySpan<float> band)
    {
        double e = 0;
        foreach (var v in band)
        {
            e += (double)v * v;
        }
        return e;
    }

    private static void AssertRelativeEqual(double expected, double actual, double rel = 1e-5)
    {
        double tol = Math.Max(Math.Abs(expected) * rel, 1e-12);
        Assert.InRange(actual, expected - tol, expected + tol);
    }

    [Fact]
    public void FillBand_NullPrng_Throws()
    {
        Span<float> band = stackalloc float[4];
        Assert.Throws<ArgumentNullException>(() =>
        {
            float[] heap = new float[4];
            AacPnsNoiseGenerator.FillBand(heap, 0, null!);
        });
    }

    [Fact]
    public void FillBand_EmptyBand_NoAdvance()
    {
        var prng = new AacPnsRandom(seed: 1u);
        AacPnsNoiseGenerator.FillBand(Span<float>.Empty, 100, prng);
        Assert.Equal(1u, prng.State);
    }

    [Fact]
    public void FillBand_AdvancesPrngOncePerSample()
    {
        var prng1 = new AacPnsRandom(seed: 42u);
        var prng2 = new AacPnsRandom(seed: 42u);
        Span<float> band = stackalloc float[16];

        AacPnsNoiseGenerator.FillBand(band, 100, prng1);

        for (int i = 0; i < band.Length; i++)
        {
            prng2.NextFloat();
        }

        Assert.Equal(prng2.State, prng1.State);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(50)]
    [InlineData(100)]
    [InlineData(150)]
    [InlineData(200)]
    public void FillBand_TotalEnergyMatchesTarget(int sf)
    {
        var prng = new AacPnsRandom(seed: 7u);
        Span<float> band = stackalloc float[64];
        AacPnsNoiseGenerator.FillBand(band, sf, prng);

        double energy = EnergyOf(band);
        double target = AacPnsNoiseGenerator.TargetBandEnergy(sf);

        AssertRelativeEqual(target, energy);
    }

    [Fact]
    public void FillBand_NegativeScaleFactor_StillNormalizes()
    {
        var prng = new AacPnsRandom(seed: 13u);
        Span<float> band = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(band, -40, prng);

        double energy = EnergyOf(band);
        double target = AacPnsNoiseGenerator.TargetBandEnergy(-40);
        AssertRelativeEqual(target, energy);
    }

    [Fact]
    public void FillBand_DeterministicForSameSeed()
    {
        var prngA = new AacPnsRandom(seed: 5u);
        var prngB = new AacPnsRandom(seed: 5u);

        Span<float> a = stackalloc float[32];
        Span<float> b = stackalloc float[32];

        AacPnsNoiseGenerator.FillBand(a, 100, prngA);
        AacPnsNoiseGenerator.FillBand(b, 100, prngB);

        for (int i = 0; i < a.Length; i++)
        {
            Assert.Equal(a[i], b[i]);
        }
    }

    [Fact]
    public void FillBand_DifferentSeedsProduceDifferentSamples()
    {
        var prngA = new AacPnsRandom(seed: 1u);
        var prngB = new AacPnsRandom(seed: 2u);

        Span<float> a = stackalloc float[32];
        Span<float> b = stackalloc float[32];

        AacPnsNoiseGenerator.FillBand(a, 100, prngA);
        AacPnsNoiseGenerator.FillBand(b, 100, prngB);

        bool anyDifferent = false;
        for (int i = 0; i < a.Length; i++)
        {
            if (a[i] != b[i])
            {
                anyDifferent = true;
                break;
            }
        }
        Assert.True(anyDifferent);
    }

    [Fact]
    public void FillBand_NegateFlag_FlipsSign()
    {
        var prngA = new AacPnsRandom(seed: 100u);
        var prngB = new AacPnsRandom(seed: 100u);

        Span<float> a = stackalloc float[16];
        Span<float> b = stackalloc float[16];

        AacPnsNoiseGenerator.FillBand(a, 100, prngA, negate: false);
        AacPnsNoiseGenerator.FillBand(b, 100, prngB, negate: true);

        for (int i = 0; i < a.Length; i++)
        {
            Assert.Equal(a[i], -b[i]);
        }
    }

    [Fact]
    public void FillBand_NegateFlag_PreservesTotalEnergy()
    {
        var prng = new AacPnsRandom(seed: 17u);
        Span<float> band = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(band, 80, prng, negate: true);

        AssertRelativeEqual(AacPnsNoiseGenerator.TargetBandEnergy(80), EnergyOf(band));
    }

    [Fact]
    public void FillBand_SmallBand_OverwritesInitialContents()
    {
        var prng = new AacPnsRandom(seed: 1u);
        Span<float> band = stackalloc float[8];
        for (int i = 0; i < band.Length; i++) band[i] = 99f;

        AacPnsNoiseGenerator.FillBand(band, 100, prng);

        for (int i = 0; i < band.Length; i++)
        {
            Assert.NotEqual(99f, band[i]);
        }
    }

    [Fact]
    public void TargetBandEnergy_KnownValues()
    {
        Assert.Equal(1.0, AacPnsNoiseGenerator.TargetBandEnergy(0), 12);
        Assert.Equal(2.0, AacPnsNoiseGenerator.TargetBandEnergy(2), 12);
        Assert.Equal(4.0, AacPnsNoiseGenerator.TargetBandEnergy(4), 12);
        Assert.Equal(1024.0, AacPnsNoiseGenerator.TargetBandEnergy(20), 9);
    }

    [Fact]
    public void FillBand_LargeBand_HitsTargetEnergyPrecisely()
    {
        var prng = new AacPnsRandom(seed: 31u);
        Span<float> band = stackalloc float[128];
        AacPnsNoiseGenerator.FillBand(band, 120, prng);

        double energy = EnergyOf(band);
        double target = AacPnsNoiseGenerator.TargetBandEnergy(120);
        AssertRelativeEqual(target, energy);
    }

    [Fact]
    public void FillBand_VeryLargeScaleFactor_DoesNotProduceNaN()
    {
        var prng = new AacPnsRandom(seed: 1u);
        Span<float> band = stackalloc float[16];
        AacPnsNoiseGenerator.FillBand(band, 250, prng);

        for (int i = 0; i < band.Length; i++)
        {
            Assert.False(float.IsNaN(band[i]));
        }
    }

    [Fact]
    public void TargetBandEnergy_NegativeSf_LessThanOne()
    {
        // sf=-2 -> 2^(-1) = 0.5; sf=-4 -> 2^(-2) = 0.25
        Assert.Equal(0.5, AacPnsNoiseGenerator.TargetBandEnergy(-2), 12);
        Assert.Equal(0.25, AacPnsNoiseGenerator.TargetBandEnergy(-4), 12);
    }

    [Fact]
    public void TargetBandEnergy_OddSf_Matches_Pow_Formula()
    {
        // sf=1 -> 2^0.5 = sqrt(2); sf=3 -> 2^1.5
        Assert.Equal(Math.Sqrt(2.0), AacPnsNoiseGenerator.TargetBandEnergy(1), 12);
        Assert.Equal(Math.Pow(2.0, 1.5), AacPnsNoiseGenerator.TargetBandEnergy(3), 12);
    }

    [Fact]
    public void FillBand_NullBandAndNullPrng_ReportsPrngFirst()
    {
        // The PRNG null-check happens before band emptiness handling.
        var ex = Assert.Throws<ArgumentNullException>(() =>
        {
            float[] heap = Array.Empty<float>();
            AacPnsNoiseGenerator.FillBand(heap, 0, null!);
        });
        Assert.Equal("prng", ex.ParamName);
    }

    [Fact]
    public void FillBand_Length1Band_HitsTargetEnergy()
    {
        // A 1-sample band's |sample| must end up as sqrt(target).
        var prng = new AacPnsRandom(seed: 9u);
        Span<float> band = stackalloc float[1];
        AacPnsNoiseGenerator.FillBand(band, 100, prng);
        AssertRelativeEqual(AacPnsNoiseGenerator.TargetBandEnergy(100), EnergyOf(band));
    }

    [Fact]
    public void FillBand_EmptyBand_DoesNotThrow_WithExplicitNonNullPrng()
    {
        var prng = new AacPnsRandom(seed: 1u);
        // Empty span + valid prng must be a no-op and not throw.
        AacPnsNoiseGenerator.FillBand(Span<float>.Empty, 0, prng, negate: true);
        Assert.Equal(1u, prng.State);
    }

    [Fact]
    public void FillBand_NegateAndPositive_AreReflectionsAcrossZero()
    {
        // For every pair (a, b) with same seed, a[i] should equal -b[i],
        // and energies must match.
        var prngA = new AacPnsRandom(seed: 99u);
        var prngB = new AacPnsRandom(seed: 99u);
        Span<float> a = stackalloc float[64];
        Span<float> b = stackalloc float[64];
        AacPnsNoiseGenerator.FillBand(a, 50, prngA);
        AacPnsNoiseGenerator.FillBand(b, 50, prngB, negate: true);
        Assert.Equal(EnergyOf(a), EnergyOf(b), 9);
        for (int i = 0; i < a.Length; i++)
        {
            Assert.Equal(a[i], -b[i]);
        }
    }

    [Theory]
    [InlineData(1u)]
    [InlineData(2u)]
    [InlineData(100u)]
    [InlineData(uint.MaxValue)]
    public void FillBand_AnySeed_TargetEnergyHolds(uint seed)
    {
        var prng = new AacPnsRandom(seed);
        Span<float> band = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(band, 100, prng);
        AssertRelativeEqual(AacPnsNoiseGenerator.TargetBandEnergy(100), EnergyOf(band));
    }

    [Fact]
    public void FillBand_NoSampleIsExactlyZero_ForReasonableSeed()
    {
        // PRNG output is essentially full int32 range -> 0 only happens
        // with probability 2^-32 per sample. With seed 7 and 64 samples
        // we expect no exact zeros.
        var prng = new AacPnsRandom(seed: 7u);
        Span<float> band = stackalloc float[64];
        AacPnsNoiseGenerator.FillBand(band, 100, prng);
        for (int i = 0; i < band.Length; i++)
        {
            Assert.NotEqual(0f, band[i]);
        }
    }
}
