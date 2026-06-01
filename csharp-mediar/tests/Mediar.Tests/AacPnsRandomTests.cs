using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacPnsRandomTests
{
    [Fact]
    public void DefaultConstructor_StateIsDefaultSeed()
    {
        var prng = new AacPnsRandom();
        Assert.Equal(AacPnsRandom.DefaultSeed, prng.State);
    }

    [Fact]
    public void Seeded_StateMatchesInput()
    {
        var prng = new AacPnsRandom(seed: 42u);
        Assert.Equal(42u, prng.State);
    }

    [Fact]
    public void Next_FromSeedZero_FirstValueIsIncrementConstant()
    {
        // state' = 1664525 * 0 + 1013904223 = 1013904223.
        var prng = new AacPnsRandom(seed: 0u);
        Assert.Equal(AacPnsRandom.Increment, prng.Next());
        Assert.Equal(AacPnsRandom.Increment, prng.State);
    }

    [Fact]
    public void Next_FromSeedOne_FirstValueIsMultiplierPlusIncrement()
    {
        // 1664525 * 1 + 1013904223 = 1015568748.
        var prng = new AacPnsRandom(seed: 1u);
        uint expected = unchecked(AacPnsRandom.Multiplier * 1u + AacPnsRandom.Increment);
        Assert.Equal(expected, prng.Next());
    }

    [Fact]
    public void Next_RecurrenceMatchesHandComputed_FourSteps()
    {
        var prng = new AacPnsRandom(seed: 12345u);
        uint expected = 12345u;
        for (int i = 0; i < 4; i++)
        {
            expected = unchecked(AacPnsRandom.Multiplier * expected + AacPnsRandom.Increment);
            Assert.Equal(expected, prng.Next());
        }
    }

    [Fact]
    public void Next_OverflowWrapsModulo2Pow32()
    {
        // 0xFFFFFFFF * 1664525 + 1013904223 wraps in 32-bit.
        var prng = new AacPnsRandom(seed: 0xFFFFFFFFu);
        uint expected = unchecked(AacPnsRandom.Multiplier * 0xFFFFFFFFu + AacPnsRandom.Increment);
        Assert.Equal(expected, prng.Next());
    }

    [Fact]
    public void Reseed_ResetsState()
    {
        var prng = new AacPnsRandom(seed: 100u);
        prng.Next();
        prng.Next();
        prng.Reseed(99u);
        Assert.Equal(99u, prng.State);
    }

    [Fact]
    public void NextSigned_IsBitCastOfNext()
    {
        var a = new AacPnsRandom(seed: 7u);
        var b = new AacPnsRandom(seed: 7u);
        Assert.Equal(unchecked((int)a.Next()), b.NextSigned());
    }

    [Fact]
    public void NextFloat_IsInClosedOpenIntervalMinusOneToOne()
    {
        var prng = new AacPnsRandom(seed: 1u);
        for (int i = 0; i < 1000; i++)
        {
            float v = prng.NextFloat();
            Assert.InRange(v, -1f, 0.9999999999f);
        }
    }

    [Fact]
    public void Fill_FillsEntireSpan()
    {
        var prng = new AacPnsRandom(seed: 5u);
        Span<float> buf = stackalloc float[16];
        prng.Fill(buf);
        for (int i = 0; i < buf.Length; i++)
        {
            Assert.NotEqual(0f, buf[i]);
        }
    }

    [Fact]
    public void Fill_AdvancesStateOncePerElement()
    {
        var prng1 = new AacPnsRandom(seed: 10u);
        var prng2 = new AacPnsRandom(seed: 10u);

        Span<float> buf = stackalloc float[8];
        prng1.Fill(buf);

        Span<float> manual = stackalloc float[8];
        for (int i = 0; i < manual.Length; i++)
        {
            manual[i] = prng2.NextFloat();
        }

        for (int i = 0; i < buf.Length; i++)
        {
            Assert.Equal(manual[i], buf[i]);
        }
    }

    [Fact]
    public void Fill_EmptySpan_DoesNotAdvanceState()
    {
        var prng = new AacPnsRandom(seed: 1u);
        prng.Fill(Span<float>.Empty);
        Assert.Equal(1u, prng.State);
    }

    [Fact]
    public void SequencesFromSameSeed_AreDeterministic()
    {
        var a = new AacPnsRandom(seed: 100u);
        var b = new AacPnsRandom(seed: 100u);
        for (int i = 0; i < 50; i++)
        {
            Assert.Equal(a.Next(), b.Next());
        }
    }

    [Fact]
    public void SequencesFromDifferentSeeds_DivergeQuickly()
    {
        var a = new AacPnsRandom(seed: 1u);
        var b = new AacPnsRandom(seed: 2u);
        Assert.NotEqual(a.Next(), b.Next());
    }

    [Fact]
    public void NormalisationScale_Is2Pow31Reciprocal()
    {
        Assert.Equal(1f / 2147483648f, AacPnsRandom.NormalisationScale);
    }

    [Fact]
    public void Constants_MatchNumericalRecipesValues()
    {
        Assert.Equal(1664525u, AacPnsRandom.Multiplier);
        Assert.Equal(1013904223u, AacPnsRandom.Increment);
    }
}
