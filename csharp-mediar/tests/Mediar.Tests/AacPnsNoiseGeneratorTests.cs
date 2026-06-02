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

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(4)]
    [InlineData(8)]
    [InlineData(16)]
    [InlineData(64)]
    [InlineData(128)]
    [InlineData(256)]
    public void FillBand_VariousBandSizes_HitTargetEnergy(int len)
    {
        var prng = new AacPnsRandom(seed: 31u);
        float[] band = new float[len];
        AacPnsNoiseGenerator.FillBand(band, 100, prng);
        AssertRelativeEqual(AacPnsNoiseGenerator.TargetBandEnergy(100), EnergyOf(band));
    }

    [Fact]
    public void TargetBandEnergy_Sf6_Equals_8()
    {
        // 2^(6/2) = 2^3 = 8
        Assert.Equal(8.0, AacPnsNoiseGenerator.TargetBandEnergy(6), 12);
    }

    [Fact]
    public void TargetBandEnergy_Sf10_Equals_32()
    {
        // 2^(10/2) = 32
        Assert.Equal(32.0, AacPnsNoiseGenerator.TargetBandEnergy(10), 12);
    }

    [Fact]
    public void FillBand_PrngState_DiffersBetween_NegatedAndNot()
    {
        // Both consume the same number of PRNG draws, so state ends at
        // the same value regardless of negate flag.
        var a = new AacPnsRandom(seed: 1234u);
        var b = new AacPnsRandom(seed: 1234u);
        Span<float> sa = stackalloc float[32];
        Span<float> sb = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(sa, 100, a, negate: false);
        AacPnsNoiseGenerator.FillBand(sb, 100, b, negate: true);
        Assert.Equal(a.State, b.State);
    }

    [Fact]
    public void FillBand_DefaultNegate_IsFalse()
    {
        var prngA = new AacPnsRandom(seed: 50u);
        var prngB = new AacPnsRandom(seed: 50u);
        Span<float> a = stackalloc float[16];
        Span<float> b = stackalloc float[16];
        AacPnsNoiseGenerator.FillBand(a, 100, prngA);
        AacPnsNoiseGenerator.FillBand(b, 100, prngB, negate: false);
        for (int i = 0; i < a.Length; i++) Assert.Equal(a[i], b[i]);
    }

    [Fact]
    public void FillBand_SequentialBands_AdvancePrngContinuously()
    {
        var prng = new AacPnsRandom(seed: 555u);
        Span<float> bandA = stackalloc float[8];
        Span<float> bandB = stackalloc float[8];
        AacPnsNoiseGenerator.FillBand(bandA, 50, prng);
        uint stateAfterA = prng.State;
        AacPnsNoiseGenerator.FillBand(bandB, 50, prng);
        // After second call the state must be different (not reset).
        Assert.NotEqual(stateAfterA, prng.State);
    }

    [Fact]
    public void TargetBandEnergy_EvenSf_IsIntegerPowerOfTwo()
    {
        // sf=14 -> 2^7 = 128
        Assert.Equal(128.0, AacPnsNoiseGenerator.TargetBandEnergy(14), 9);
        // sf=16 -> 2^8 = 256
        Assert.Equal(256.0, AacPnsNoiseGenerator.TargetBandEnergy(16), 9);
        // sf=18 -> 2^9 = 512
        Assert.Equal(512.0, AacPnsNoiseGenerator.TargetBandEnergy(18), 9);
    }

    [Theory]
    [InlineData(-100)]
    [InlineData(-50)]
    [InlineData(-10)]
    [InlineData(0)]
    [InlineData(50)]
    [InlineData(100)]
    [InlineData(200)]
    public void FillBand_ScaleFactorTheory_TargetEnergyHolds(int sf)
    {
        var prng = new AacPnsRandom(seed: 1u);
        Span<float> band = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(band, sf, prng);
        AssertRelativeEqual(AacPnsNoiseGenerator.TargetBandEnergy(sf), EnergyOf(band));
    }

    [Fact]
    public void FillBand_AllSamples_AreFinite_ForLargeScaleFactor()
    {
        var prng = new AacPnsRandom(seed: 42u);
        Span<float> band = stackalloc float[32];
        AacPnsNoiseGenerator.FillBand(band, 250, prng);
        for (int i = 0; i < band.Length; i++)
        {
            Assert.True(float.IsFinite(band[i]), $"sample[{i}]={band[i]}");
        }
    }
}
