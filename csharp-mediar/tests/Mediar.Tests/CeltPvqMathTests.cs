using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the bit-exact PVQ math helpers in
/// <see cref="CeltPvqMath"/> (Opus Phase 2c.3b.1). Reference values come
/// straight from libopus self-tests (<c>test_unit_mathops.c</c>) and
/// <c>celt/rate.h</c> behaviour — these must match byte-for-byte across
/// every conforming CELT decoder.
/// </summary>
public sealed class CeltPvqMathTests
{
    [Theory]
    [InlineData((short)64, (short)32767)]     // libopus self-test vector (smallest valid x)
    [InlineData((short)16320, (short)200)]    // libopus self-test vector
    [InlineData((short)8192, (short)23171)]   // libopus self-test vector
    public void BitexactCos_Matches_LibOpus_Vectors(short input, short expected)
    {
        Assert.Equal(expected, CeltPvqMath.BitexactCos(input));
    }

    [Fact]
    public void BitexactCos_Output_Stays_In_Valid_Range()
    {
        // Valid input range is x ∈ [64, 16320] (libopus asserts the
        // intermediate tmp ≤ 32767 and final x2 ≤ 32766). Inside that
        // range the result must stay in [1, 32767].
        for (int x = 64; x <= 16320; x += 256)
        {
            short c = CeltPvqMath.BitexactCos((short)x);
            Assert.InRange(c, (short)1, (short)32767);
        }
    }

    [Fact]
    public void BitexactCos_Is_Monotonic_Decreasing_Across_Quarter_Turn()
    {
        short prev = CeltPvqMath.BitexactCos(64);
        for (int x = 320; x <= 16320; x += 256)
        {
            short c = CeltPvqMath.BitexactCos((short)x);
            Assert.True(c <= prev, $"cos({x}) = {c} > cos({x - 256}) = {prev}");
            prev = c;
        }
    }

    [Fact]
    public void BitexactLog2Tan_Is_Zero_When_Sin_Equals_Cos()
    {
        // tan = sin/cos = 1 ⇒ log2 = 0 ⇒ result must be 0 for any positive
        // value where the bit-shift normalisation is identical.
        for (int v = 1; v < 100; v++)
        {
            Assert.Equal(0, CeltPvqMath.BitexactLog2Tan(v, v));
        }
    }

    [Fact]
    public void BitexactLog2Tan_Is_Antisymmetric()
    {
        // log2(tan(θ)) = -log2(tan(π/2 - θ)) ⇒ swap args, negate result.
        int[][] pairs =
        {
            new[] { 100, 200 },
            new[] { 500, 1500 },
            new[] { 12345, 6789 },
            new[] { 32000, 31000 },
        };
        foreach (var p in pairs)
        {
            int forward = CeltPvqMath.BitexactLog2Tan(p[0], p[1]);
            int reverse = CeltPvqMath.BitexactLog2Tan(p[1], p[0]);
            Assert.Equal(forward, -reverse);
        }
    }

    [Theory]
    [InlineData(0, 0)]
    [InlineData(1, 1)]
    [InlineData(7, 7)]
    [InlineData(8, 8)]       // (8+0) << 0
    [InlineData(9, 9)]
    [InlineData(15, 15)]     // (8+7) << 0
    [InlineData(16, 16)]     // (8+0) << 1
    [InlineData(17, 18)]
    [InlineData(23, 30)]     // (8+7) << 1
    [InlineData(24, 32)]     // (8+0) << 2
    [InlineData(31, 60)]     // (8+7) << 2
    [InlineData(32, 64)]     // (8+0) << 3
    [InlineData(39, 120)]    // (8+7) << 3
    [InlineData(40, 128)]    // (8+0) << 4 = CeltMaxPulses
    public void GetPulses_Matches_LibOpus_Formula(int i, int expected)
    {
        Assert.Equal(expected, CeltPvqMath.GetPulses(i));
    }

    [Fact]
    public void GetPulses_Tops_Out_At_CeltMaxPulses()
    {
        Assert.Equal(CeltPvqMath.CeltMaxPulses, CeltPvqMath.GetPulses(CeltPvqMath.MaxPseudo));
    }

    [Fact]
    public void Bits2Pulses_Of_Zero_Bits_Returns_Zero_Pulses()
    {
        // bits=0 ⇒ bits-- = -1, all entries cost ≥ 0, so search settles at 0.
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                Assert.Equal(0, CeltPvqMath.Bits2Pulses(band, lm, 0));
            }
        }
    }

    [Fact]
    public void Pulses2Bits_Of_Zero_Pulses_Returns_Zero_Bits()
    {
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                Assert.Equal(0, CeltPvqMath.Pulses2Bits(band, lm, 0));
            }
        }
    }

    [Fact]
    public void Pulses2Bits_Is_Monotonic_Non_Decreasing()
    {
        // Within a single band's cache, costs are non-decreasing with
        // pulse count. Some adjacent entries are equal (e.g. positions
        // 16/17 in the LM=0 band=8 cache both cost 47), so the relation
        // is ≤ not strict.
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                int cacheStart = CeltPvqMath.CacheIndex50[(lm + 1) * CeltConstants.MaxBands + band];
                if (cacheStart < 0) continue;
                int max = CeltPvqMath.CacheBits50[cacheStart];
                int prev = 0;
                for (int p = 1; p <= max; p++)
                {
                    int cost = CeltPvqMath.Pulses2Bits(band, lm, p);
                    Assert.True(cost >= prev, $"decreasing at band={band} lm={lm} p={p}: {cost} < {prev}");
                    prev = cost;
                }
            }
        }
    }

    [Fact]
    public void Pulses2Bits_Then_Bits2Pulses_Recovers_Equivalent_Pulse_Count()
    {
        // The cache occasionally has adjacent entries with identical cost
        // (e.g. positions 15+16 in the LM=0 band=8 cache both cost 47),
        // so Bits2Pulses may legitimately return a different but
        // cost-equivalent pulse index than the original. The right
        // invariant is therefore that Pulses2Bits(recovered) == original cost.
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                int cacheStart = CeltPvqMath.CacheIndex50[(lm + 1) * CeltConstants.MaxBands + band];
                if (cacheStart < 0) continue;
                int maxIdx = CeltPvqMath.CacheBits50[cacheStart];
                for (int p = 1; p <= maxIdx; p++)
                {
                    int cost = CeltPvqMath.Pulses2Bits(band, lm, p);
                    int recovered = CeltPvqMath.Bits2Pulses(band, lm, cost);
                    int recoveredCost = CeltPvqMath.Pulses2Bits(band, lm, recovered);
                    Assert.Equal(cost, recoveredCost);
                }
            }
        }
    }

    [Fact]
    public void Bits2Pulses_Picks_Nearest_Neighbor()
    {
        // For any requested budget, the chosen pseudo-index must be the
        // nearest in absolute cost-difference among {lo, hi} candidates.
        // We assert that no neighboring index would give a strictly
        // smaller diff to the requested bits.
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                int cacheStart = CeltPvqMath.CacheIndex50[(lm + 1) * CeltConstants.MaxBands + band];
                if (cacheStart < 0) continue;
                int maxIdx = CeltPvqMath.CacheBits50[cacheStart];
                int maxBits = CeltPvqMath.CacheBits50[cacheStart + maxIdx] + 1;
                for (int b = 1; b <= maxBits; b += 4)
                {
                    int p = CeltPvqMath.Bits2Pulses(band, lm, b);
                    int chosenDiff = Math.Abs(CeltPvqMath.Pulses2Bits(band, lm, p) - b);
                    if (p > 0)
                    {
                        int prevDiff = Math.Abs(CeltPvqMath.Pulses2Bits(band, lm, p - 1) - b);
                        Assert.True(chosenDiff <= prevDiff,
                            $"band={band} lm={lm} bits={b}: chose p={p} (diff={chosenDiff}) but p-1 (diff={prevDiff}) is closer");
                    }
                    if (p < maxIdx)
                    {
                        int nextDiff = Math.Abs(CeltPvqMath.Pulses2Bits(band, lm, p + 1) - b);
                        Assert.True(chosenDiff <= nextDiff,
                            $"band={band} lm={lm} bits={b}: chose p={p} (diff={chosenDiff}) but p+1 (diff={nextDiff}) is closer");
                    }
                }
            }
        }
    }

    [Fact]
    public void Bits2Pulses_Is_Monotonic_In_Bit_Budget()
    {
        // More bits ⇒ never fewer pulses.
        for (int band = 8; band < CeltConstants.MaxBands; band++)
        {
            for (int lm = 0; lm < 4; lm++)
            {
                int prev = -1;
                for (int b = 0; b < 256; b += 4)
                {
                    int p = CeltPvqMath.Bits2Pulses(band, lm, b);
                    Assert.True(p >= prev, $"non-monotonic at band={band} lm={lm} bits={b}: {p} < {prev}");
                    prev = p;
                }
            }
        }
    }

    [Fact]
    public void CacheIndex50_And_CacheBits50_Have_Expected_Sizes()
    {
        Assert.Equal(105, CeltPvqMath.CacheIndex50.Length);
        Assert.Equal(392, CeltPvqMath.CacheBits50.Length);
    }
}
