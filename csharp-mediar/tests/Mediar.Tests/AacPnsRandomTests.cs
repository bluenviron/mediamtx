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

    [Fact]
    public void DefaultSeed_Constant_IsZero()
    {
        Assert.Equal(0u, AacPnsRandom.DefaultSeed);
    }

    [Fact]
    public void State_Property_DoesNotAdvanceGenerator()
    {
        var prng = new AacPnsRandom(seed: 50u);
        _ = prng.State;
        _ = prng.State;
        Assert.Equal(50u, prng.State);
    }

    [Fact]
    public void Reseed_To_Zero_Restores_DefaultSeed()
    {
        var prng = new AacPnsRandom(seed: 999u);
        prng.Next();
        prng.Reseed(AacPnsRandom.DefaultSeed);
        Assert.Equal(0u, prng.State);
    }

    [Fact]
    public void Reseed_Then_Next_Produces_Reseed_Recurrence()
    {
        var a = new AacPnsRandom(seed: 42u);
        a.Reseed(99u);
        uint expected = unchecked(AacPnsRandom.Multiplier * 99u + AacPnsRandom.Increment);
        Assert.Equal(expected, a.Next());
    }

    [Fact]
    public void NextFloat_Mean_Across_Many_Samples_Is_Near_Zero()
    {
        var prng = new AacPnsRandom(seed: 1u);
        double sum = 0;
        const int n = 10_000;
        for (int i = 0; i < n; i++) sum += prng.NextFloat();
        double mean = sum / n;
        // LCG noise should average near 0 with this many samples.
        Assert.InRange(mean, -0.05, 0.05);
    }

    [Fact]
    public void NextFloat_HasNegativeAndPositiveValues_Over_Many_Samples()
    {
        var prng = new AacPnsRandom(seed: 1u);
        int neg = 0, pos = 0;
        for (int i = 0; i < 500; i++)
        {
            float v = prng.NextFloat();
            if (v < 0) neg++; else if (v > 0) pos++;
        }
        Assert.True(neg > 100);
        Assert.True(pos > 100);
    }

    [Fact]
    public void Fill_LargeBuffer_DoesNotThrow()
    {
        var prng = new AacPnsRandom(seed: 5u);
        var buf = new float[1024];
        prng.Fill(buf);
        Assert.Contains(buf, v => v != 0f);
    }

    [Fact]
    public void Fill_Sequence_Matches_Per_Element_NextFloat()
    {
        var a = new AacPnsRandom(seed: 7u);
        var b = new AacPnsRandom(seed: 7u);
        var ab = new float[32];
        a.Fill(ab);
        for (int i = 0; i < ab.Length; i++)
        {
            Assert.Equal(b.NextFloat(), ab[i]);
        }
        Assert.Equal(a.State, b.State);
    }

    [Fact]
    public void NextSigned_Matches_BitCast_For_Multiple_Calls()
    {
        var a = new AacPnsRandom(seed: 31u);
        var b = new AacPnsRandom(seed: 31u);
        for (int i = 0; i < 32; i++)
        {
            int signed = a.NextSigned();
            uint unsigned = b.Next();
            Assert.Equal(unchecked((int)unsigned), signed);
        }
    }

    [Fact]
    public void Two_Generators_With_Sequential_Seeds_Have_Independent_States()
    {
        var a = new AacPnsRandom(seed: 100u);
        var b = new AacPnsRandom(seed: 101u);
        for (int i = 0; i < 4; i++) a.Next();
        for (int i = 0; i < 4; i++) b.Next();
        Assert.NotEqual(a.State, b.State);
    }
}
